package postgres

import (
	"context"
	"fmt"

	"github.com/mkandel/go-checklists/internal/domain"
)

// ChecklistRepo is the Postgres-backed implementation of
// domain.ChecklistRepo. Claim and Save should be called inside
// Store.WithTx when their side-effect writes (the claim_lost notification,
// or the events/notifications from a status change) must be atomic with
// the state change that caused them.
type ChecklistRepo struct {
	db            dbtx
	templates     *TemplateRepo
	events        *EventRepo
	notifications *NotificationRepo
}

var _ domain.ChecklistRepo = (*ChecklistRepo)(nil)

// Create inserts a new checklist, copying items from c.TemplateID's current
// items (c.Items is overwritten).
func (r *ChecklistRepo) Create(ctx context.Context, c *domain.Checklist) error {
	if c.Status == "" {
		c.Status = domain.StatusOpen
	}

	_, templateItems, err := r.templates.Get(ctx, c.TenantID, c.TemplateID)
	if err != nil {
		return fmt.Errorf("postgres: load template for checklist: %w", err)
	}
	c.Items = make([]domain.ChecklistItem, len(templateItems))
	for i, ti := range templateItems {
		c.Items[i] = domain.ChecklistItem{
			Name:          ti.Name,
			Position:      i,
			ValidationRef: ti.ValidationRef,
		}
	}

	row := r.db.QueryRow(ctx,
		`INSERT INTO checklists (tenant_id, template_id, creator_id, assigned_group_id, assigned_user_id, hidden, approver_id, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, created_at`,
		c.TenantID, c.TemplateID, c.CreatorID, c.AssignedGroupID, c.AssignedUserID, c.Hidden, c.ApproverID, string(c.Status),
	)
	if err := row.Scan(&c.ID, &c.CreatedAt); err != nil {
		return fmt.Errorf("postgres: create checklist: %w", err)
	}

	for i := range c.Items {
		c.Items[i].ChecklistID = c.ID
		row := r.db.QueryRow(ctx,
			`INSERT INTO checklist_items (checklist_id, name, position, validation_ref)
			 VALUES ($1, $2, $3, $4)
			 RETURNING id`,
			c.Items[i].ChecklistID, c.Items[i].Name, c.Items[i].Position, c.Items[i].ValidationRef,
		)
		if err := row.Scan(&c.Items[i].ID); err != nil {
			return fmt.Errorf("postgres: create checklist item: %w", err)
		}
	}

	return r.events.Append(ctx, []domain.Event{{
		TenantID:    c.TenantID,
		ChecklistID: c.ID,
		ActorUserID: c.CreatorID,
		Action:      domain.EventCreated,
	}})
}

// Get reads the checklist row FOR UPDATE — a no-op lock outside a
// transaction (released when the implicit single-statement transaction
// ends), but load-bearing when called inside Store.WithTx ahead of a
// domain-method-then-Save sequence: it serializes concurrent status
// transitions on the same checklist against each other. tenantID scopes the
// lookup since id is request-supplied (see domain.ChecklistRepo's doc
// comment for why this is a correctness requirement, not just
// defense-in-depth).
func (r *ChecklistRepo) Get(ctx context.Context, tenantID, id int64) (*domain.Checklist, error) {
	row := r.db.QueryRow(ctx,
		`SELECT id, tenant_id, template_id, creator_id, assigned_group_id, assigned_user_id, hidden, approver_id, status, created_at
		 FROM checklists WHERE id = $1 AND tenant_id = $2 FOR UPDATE`, id, tenantID)

	var c domain.Checklist
	var status string
	if err := row.Scan(&c.ID, &c.TenantID, &c.TemplateID, &c.CreatorID, &c.AssignedGroupID, &c.AssignedUserID,
		&c.Hidden, &c.ApproverID, &status, &c.CreatedAt); err != nil {
		return nil, fmt.Errorf("postgres: get checklist: %w", err)
	}
	c.Status = domain.ChecklistStatus(status)

	rows, err := r.db.Query(ctx,
		`SELECT id, checklist_id, name, position, checked, checked_by, checked_at, validation_ref, assignee_override_user_id
		 FROM checklist_items WHERE checklist_id = $1 AND deleted_at IS NULL ORDER BY position`, id)
	if err != nil {
		return nil, fmt.Errorf("postgres: list checklist items: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var it domain.ChecklistItem
		if err := rows.Scan(&it.ID, &it.ChecklistID, &it.Name, &it.Position, &it.Checked,
			&it.CheckedBy, &it.CheckedAt, &it.ValidationRef, &it.AssigneeOverrideUserID); err != nil {
			return nil, fmt.Errorf("postgres: scan checklist item: %w", err)
		}
		c.Items = append(c.Items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list checklist items: %w", err)
	}
	return &c, nil
}

// Claim assigns the checklist to actingUserID, provided the current
// assigned_user_id matches expectedCurrent (nil means "currently
// unclaimed"). On success it appends a "claimed" event. On failure (the CAS
// lost the race) it writes a claim_lost notification for actingUserID
// naming whoever actually won, and returns false with no error. tenantID
// scopes the lookup since checklistID is request-supplied.
func (r *ChecklistRepo) Claim(ctx context.Context, tenantID, checklistID, actingUserID int64, expectedCurrent *int64) (bool, error) {
	tag, err := r.db.Exec(ctx,
		`UPDATE checklists SET assigned_user_id = $1
		 WHERE id = $2 AND tenant_id = $3 AND assigned_user_id IS NOT DISTINCT FROM $4`,
		actingUserID, checklistID, tenantID, expectedCurrent,
	)
	if err != nil {
		return false, fmt.Errorf("postgres: claim checklist: %w", err)
	}
	if tag.RowsAffected() == 1 {
		if err := r.events.Append(ctx, []domain.Event{{
			TenantID:    tenantID,
			ChecklistID: checklistID,
			ActorUserID: actingUserID,
			Action:      domain.EventClaimed,
		}}); err != nil {
			return false, err
		}
		return true, nil
	}

	var currentAssignee *int64
	err = r.db.QueryRow(ctx,
		`SELECT assigned_user_id FROM checklists WHERE id = $1 AND tenant_id = $2`, checklistID, tenantID,
	).Scan(&currentAssignee)
	if err != nil {
		return false, fmt.Errorf("postgres: read current assignee after lost claim: %w", err)
	}

	message := "someone else claimed this checklist first"
	if currentAssignee != nil {
		message = fmt.Sprintf("user %d claimed this checklist first", *currentAssignee)
	}
	if err := r.notifications.Create(ctx, &domain.Notification{
		TenantID:        tenantID,
		RecipientUserID: actingUserID,
		Type:            domain.EventClaimLost,
		ChecklistID:     &checklistID,
		ActorUserID:     currentAssignee,
		Message:         message,
	}); err != nil {
		return false, fmt.Errorf("postgres: write claim_lost notification: %w", err)
	}
	return false, nil
}

// Save persists the checklist's current items/status, appends events, and
// derives+writes any notifications those events imply (e.g.
// submitted_for_validation notifies the approver). It reconciles c.Items
// against the DB rather than assuming a fixed, pre-existing set of item rows:
// items with ID == 0 are newly added (via domain.Checklist.AddItem) and get
// inserted, existing items are updated in place (this also covers reordering
// and check/uncheck), and any previously-active item id no longer present in
// c.Items (removed via domain.Checklist.RemoveItem) is soft-deleted. c is
// assumed to have been loaded via a tenant-scoped Get earlier in the same
// transaction, so no separate tenantID param is needed here.
func (r *ChecklistRepo) Save(ctx context.Context, c *domain.Checklist, events []domain.Event) error {
	if _, err := r.db.Exec(ctx,
		`UPDATE checklists SET status = $1 WHERE id = $2`, string(c.Status), c.ID,
	); err != nil {
		return fmt.Errorf("postgres: save checklist status: %w", err)
	}

	existingIDs := make(map[int64]bool)
	rows, err := r.db.Query(ctx,
		`SELECT id FROM checklist_items WHERE checklist_id = $1 AND deleted_at IS NULL`, c.ID)
	if err != nil {
		return fmt.Errorf("postgres: list existing checklist items: %w", err)
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("postgres: scan existing checklist item id: %w", err)
		}
		existingIDs[id] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("postgres: list existing checklist items: %w", err)
	}
	rows.Close()

	keptIDs := make(map[int64]bool, len(c.Items))
	for i := range c.Items {
		item := &c.Items[i]
		if item.ID == 0 {
			row := r.db.QueryRow(ctx,
				`INSERT INTO checklist_items (checklist_id, name, position, validation_ref)
				 VALUES ($1, $2, $3, $4)
				 RETURNING id`,
				c.ID, item.Name, item.Position, item.ValidationRef,
			)
			if err := row.Scan(&item.ID); err != nil {
				return fmt.Errorf("postgres: insert checklist item: %w", err)
			}
			item.ChecklistID = c.ID
		}
		keptIDs[item.ID] = true

		if _, err := r.db.Exec(ctx,
			`UPDATE checklist_items
			 SET name = $1, position = $2, checked = $3, checked_by = $4, checked_at = $5, assignee_override_user_id = $6
			 WHERE id = $7`,
			item.Name, item.Position, item.Checked, item.CheckedBy, item.CheckedAt, item.AssigneeOverrideUserID, item.ID,
		); err != nil {
			return fmt.Errorf("postgres: save checklist item %d: %w", item.ID, err)
		}
	}

	for id := range existingIDs {
		if keptIDs[id] {
			continue
		}
		if _, err := r.db.Exec(ctx,
			`UPDATE checklist_items SET deleted_at = now() WHERE id = $1`, id,
		); err != nil {
			return fmt.Errorf("postgres: soft-delete checklist item %d: %w", id, err)
		}
	}

	if len(events) > 0 {
		if err := r.events.Append(ctx, events); err != nil {
			return err
		}
	}

	for _, e := range events {
		notification := notificationForEvent(c, e)
		if notification == nil {
			continue
		}
		if err := r.notifications.Create(ctx, notification); err != nil {
			return fmt.Errorf("postgres: write notification for event %s: %w", e.Action, err)
		}
	}
	return nil
}

// List returns checklists relevant to filter.UserID within filter.TenantID —
// as creator, approver, direct assignee, or (while unclaimed) a member of
// the assigned group, mirroring domain.Checklist.VisibleTo's rule —
// optionally narrowed by filter.Status. Returned checklists have Items ==
// nil; use Get for the full checklist.
func (r *ChecklistRepo) List(ctx context.Context, filter domain.ChecklistFilter) ([]domain.Checklist, error) {
	var status *string
	if filter.Status != nil {
		s := string(*filter.Status)
		status = &s
	}

	rows, err := r.db.Query(ctx,
		`SELECT id, tenant_id, template_id, creator_id, assigned_group_id, assigned_user_id, hidden, approver_id, status, created_at
		 FROM checklists
		 WHERE tenant_id = $1
		 AND (
		     creator_id = $2
		     OR approver_id = $2
		     OR assigned_user_id = $2
		     OR (assigned_user_id IS NULL AND assigned_group_id IN (
		         SELECT group_id FROM user_groups WHERE user_id = $2
		     ))
		 )
		 AND ($3::text IS NULL OR status = $3)
		 ORDER BY created_at DESC`,
		filter.TenantID, filter.UserID, status,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: list checklists: %w", err)
	}
	defer rows.Close()

	var checklists []domain.Checklist
	for rows.Next() {
		var c domain.Checklist
		var st string
		if err := rows.Scan(&c.ID, &c.TenantID, &c.TemplateID, &c.CreatorID, &c.AssignedGroupID, &c.AssignedUserID,
			&c.Hidden, &c.ApproverID, &st, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan checklist: %w", err)
		}
		c.Status = domain.ChecklistStatus(st)
		checklists = append(checklists, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list checklists: %w", err)
	}
	return checklists, nil
}

// ClearUserAssignments clears userID from approver_id and assigned_user_id
// on every checklist in tenantID where it's currently set, appending an
// event for each cleared field. Called when a user is deactivated.
func (r *ChecklistRepo) ClearUserAssignments(ctx context.Context, tenantID, userID int64) error {
	var events []domain.Event

	rows, err := r.db.Query(ctx,
		`UPDATE checklists SET approver_id = NULL WHERE tenant_id = $1 AND approver_id = $2 RETURNING id`,
		tenantID, userID,
	)
	if err != nil {
		return fmt.Errorf("postgres: clear approver assignments: %w", err)
	}
	var approverIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("postgres: scan cleared approver checklist id: %w", err)
		}
		approverIDs = append(approverIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("postgres: clear approver assignments: %w", err)
	}
	rows.Close()
	for _, id := range approverIDs {
		events = append(events, domain.Event{
			TenantID:    tenantID,
			ChecklistID: id,
			ActorUserID: userID,
			Action:      domain.EventApproverChanged,
			Detail:      map[string]any{"reason": "user_deactivated"},
		})
	}

	rows, err = r.db.Query(ctx,
		`UPDATE checklists SET assigned_user_id = NULL WHERE tenant_id = $1 AND assigned_user_id = $2 RETURNING id`,
		tenantID, userID,
	)
	if err != nil {
		return fmt.Errorf("postgres: clear assignee assignments: %w", err)
	}
	var assigneeIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("postgres: scan cleared assignee checklist id: %w", err)
		}
		assigneeIDs = append(assigneeIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("postgres: clear assignee assignments: %w", err)
	}
	rows.Close()
	for _, id := range assigneeIDs {
		events = append(events, domain.Event{
			TenantID:    tenantID,
			ChecklistID: id,
			ActorUserID: userID,
			Action:      domain.EventAssigneeChanged,
			Detail:      map[string]any{"reason": "user_deactivated"},
		})
	}

	if len(events) == 0 {
		return nil
	}
	return r.events.Append(ctx, events)
}

func notificationForEvent(c *domain.Checklist, e domain.Event) *domain.Notification {
	actor := e.ActorUserID
	switch e.Action {
	case domain.EventSubmittedForValidation:
		if c.ApproverID == nil {
			return nil
		}
		return &domain.Notification{
			TenantID:        c.TenantID,
			RecipientUserID: *c.ApproverID,
			Type:            e.Action,
			ChecklistID:     &c.ID,
			ActorUserID:     &actor,
			Message:         "a checklist is awaiting your approval",
		}
	case domain.EventRejected:
		if c.AssignedUserID == nil {
			return nil
		}
		return &domain.Notification{
			TenantID:        c.TenantID,
			RecipientUserID: *c.AssignedUserID,
			Type:            e.Action,
			ChecklistID:     &c.ID,
			ActorUserID:     &actor,
			Message:         "the approver sent this checklist back for changes",
		}
	case domain.EventReopened:
		if c.AssignedUserID == nil || *c.AssignedUserID == actor {
			return nil
		}
		return &domain.Notification{
			TenantID:        c.TenantID,
			RecipientUserID: *c.AssignedUserID,
			Type:            e.Action,
			ChecklistID:     &c.ID,
			ActorUserID:     &actor,
			Message:         "the creator edited this checklist and reopened it",
		}
	default:
		return nil
	}
}

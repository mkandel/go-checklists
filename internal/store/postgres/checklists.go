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

// Create inserts a new checklist. If c.TemplateID is set, items are copied
// from that template's current items (c.Items is overwritten); otherwise
// c.Items is used as-is (ad-hoc checklist).
func (r *ChecklistRepo) Create(ctx context.Context, c *domain.Checklist) error {
	if c.Status == "" {
		c.Status = domain.StatusOpen
	}

	if c.TemplateID != nil {
		_, templateItems, err := r.templates.Get(ctx, *c.TemplateID)
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
	} else {
		for i := range c.Items {
			c.Items[i].Position = i
		}
	}

	row := r.db.QueryRow(ctx,
		`INSERT INTO checklists (template_id, creator_id, assigned_group_id, assigned_user_id, hidden, approver_id, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, created_at`,
		c.TemplateID, c.CreatorID, c.AssignedGroupID, c.AssignedUserID, c.Hidden, c.ApproverID, string(c.Status),
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
		ChecklistID: c.ID,
		ActorUserID: c.CreatorID,
		Action:      domain.EventCreated,
	}})
}

func (r *ChecklistRepo) Get(ctx context.Context, id int64) (*domain.Checklist, error) {
	row := r.db.QueryRow(ctx,
		`SELECT id, template_id, creator_id, assigned_group_id, assigned_user_id, hidden, approver_id, status, created_at
		 FROM checklists WHERE id = $1`, id)

	var c domain.Checklist
	var status string
	if err := row.Scan(&c.ID, &c.TemplateID, &c.CreatorID, &c.AssignedGroupID, &c.AssignedUserID,
		&c.Hidden, &c.ApproverID, &status, &c.CreatedAt); err != nil {
		return nil, fmt.Errorf("postgres: get checklist: %w", err)
	}
	c.Status = domain.ChecklistStatus(status)

	rows, err := r.db.Query(ctx,
		`SELECT id, checklist_id, name, position, checked, checked_by, checked_at, validation_ref, assignee_override_user_id
		 FROM checklist_items WHERE checklist_id = $1 ORDER BY position`, id)
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
// naming whoever actually won, and returns false with no error.
func (r *ChecklistRepo) Claim(ctx context.Context, checklistID, actingUserID int64, expectedCurrent *int64) (bool, error) {
	tag, err := r.db.Exec(ctx,
		`UPDATE checklists SET assigned_user_id = $1
		 WHERE id = $2 AND assigned_user_id IS NOT DISTINCT FROM $3`,
		actingUserID, checklistID, expectedCurrent,
	)
	if err != nil {
		return false, fmt.Errorf("postgres: claim checklist: %w", err)
	}
	if tag.RowsAffected() == 1 {
		if err := r.events.Append(ctx, []domain.Event{{
			ChecklistID: checklistID,
			ActorUserID: actingUserID,
			Action:      domain.EventClaimed,
		}}); err != nil {
			return false, err
		}
		return true, nil
	}

	var currentAssignee *int64
	err = r.db.QueryRow(ctx, `SELECT assigned_user_id FROM checklists WHERE id = $1`, checklistID).Scan(&currentAssignee)
	if err != nil {
		return false, fmt.Errorf("postgres: read current assignee after lost claim: %w", err)
	}

	message := "someone else claimed this checklist first"
	if currentAssignee != nil {
		message = fmt.Sprintf("user %d claimed this checklist first", *currentAssignee)
	}
	if err := r.notifications.Create(ctx, &domain.Notification{
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
// submitted_for_validation notifies the approver).
func (r *ChecklistRepo) Save(ctx context.Context, c *domain.Checklist, events []domain.Event) error {
	if _, err := r.db.Exec(ctx,
		`UPDATE checklists SET status = $1 WHERE id = $2`, string(c.Status), c.ID,
	); err != nil {
		return fmt.Errorf("postgres: save checklist status: %w", err)
	}

	for _, item := range c.Items {
		if _, err := r.db.Exec(ctx,
			`UPDATE checklist_items
			 SET checked = $1, checked_by = $2, checked_at = $3, assignee_override_user_id = $4
			 WHERE id = $5`,
			item.Checked, item.CheckedBy, item.CheckedAt, item.AssigneeOverrideUserID, item.ID,
		); err != nil {
			return fmt.Errorf("postgres: save checklist item %d: %w", item.ID, err)
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

func notificationForEvent(c *domain.Checklist, e domain.Event) *domain.Notification {
	actor := e.ActorUserID
	switch e.Action {
	case domain.EventSubmittedForValidation:
		if c.ApproverID == nil {
			return nil
		}
		return &domain.Notification{
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
			RecipientUserID: *c.AssignedUserID,
			Type:            e.Action,
			ChecklistID:     &c.ID,
			ActorUserID:     &actor,
			Message:         "the approver sent this checklist back for changes",
		}
	default:
		return nil
	}
}

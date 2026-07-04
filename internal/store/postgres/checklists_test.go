package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/mkandel/go-checklists/internal/domain"
)

func TestChecklistRepo_CreateFromTemplateCopiesItems(t *testing.T) {
	ctx := context.Background()
	creator := mustCreateUser(t, "Creator", uniqueName(t, "creator"))

	tmpl := &domain.Template{Name: uniqueName(t, "tmpl")}
	if err := testStore.Templates().CreateVersion(ctx, tmpl, []domain.TemplateItem{
		{Name: "Step 1"}, {Name: "Step 2"},
	}); err != nil {
		t.Fatalf("create template: %v", err)
	}

	c := &domain.Checklist{
		TemplateID:     &tmpl.ID,
		CreatorID:      creator.ID,
		AssignedUserID: &creator.ID,
	}
	if err := testStore.Checklists().Create(ctx, c); err != nil {
		t.Fatalf("create checklist: %v", err)
	}
	if len(c.Items) != 2 {
		t.Fatalf("expected 2 items copied from template, got %d", len(c.Items))
	}
	if c.Items[0].Name != "Step 1" || c.Items[1].Name != "Step 2" {
		t.Fatalf("unexpected item names: %+v", c.Items)
	}

	got, err := testStore.Checklists().Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("get checklist: %v", err)
	}
	if got.Status != domain.StatusOpen {
		t.Fatalf("expected open status, got %s", got.Status)
	}
	if len(got.Items) != 2 {
		t.Fatalf("expected 2 persisted items, got %d", len(got.Items))
	}
}

func TestChecklistRepo_CreateAdHocNoTemplate(t *testing.T) {
	ctx := context.Background()
	creator := mustCreateUser(t, "Creator", uniqueName(t, "creator"))

	c := &domain.Checklist{
		CreatorID:      creator.ID,
		AssignedUserID: &creator.ID,
		Items: []domain.ChecklistItem{
			{Name: "Do the thing"},
		},
	}
	if err := testStore.Checklists().Create(ctx, c); err != nil {
		t.Fatalf("create ad-hoc checklist: %v", err)
	}
	if c.TemplateID != nil {
		t.Fatalf("expected nil template id for ad-hoc checklist")
	}
	if len(c.Items) != 1 || c.Items[0].ID == 0 {
		t.Fatalf("expected 1 persisted item with assigned id, got %+v", c.Items)
	}
}

func TestChecklistRepo_LifecycleWithoutApprover(t *testing.T) {
	ctx := context.Background()
	user := mustCreateUser(t, "Assignee", uniqueName(t, "assignee"))

	c := &domain.Checklist{
		CreatorID:      user.ID,
		AssignedUserID: &user.ID,
		Items: []domain.ChecklistItem{
			{Name: "Item A"},
			{Name: "Item B"},
		},
	}
	if err := testStore.Checklists().Create(ctx, c); err != nil {
		t.Fatalf("create checklist: %v", err)
	}

	now := time.Now()
	for i := range c.Items {
		events, err := c.CheckItem(i, user.ID, now)
		if err != nil {
			t.Fatalf("check item %d: %v", i, err)
		}
		if err := testStore.Checklists().Save(ctx, c, events); err != nil {
			t.Fatalf("save after checking item %d: %v", i, err)
		}
	}

	if c.Status != domain.StatusComplete {
		t.Fatalf("expected complete, got %s", c.Status)
	}

	got, err := testStore.Checklists().Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("get checklist: %v", err)
	}
	if got.Status != domain.StatusComplete {
		t.Fatalf("expected persisted status complete, got %s", got.Status)
	}
	for _, item := range got.Items {
		if !item.Checked {
			t.Fatalf("expected all items checked, got %+v", item)
		}
	}
}

func TestChecklistRepo_LifecycleWithApprover(t *testing.T) {
	ctx := context.Background()
	user := mustCreateUser(t, "Assignee", uniqueName(t, "assignee"))
	approver := mustCreateUser(t, "Approver", uniqueName(t, "approver"))

	c := &domain.Checklist{
		CreatorID:      user.ID,
		AssignedUserID: &user.ID,
		ApproverID:     &approver.ID,
		Items: []domain.ChecklistItem{
			{Name: "Item A"},
		},
	}
	if err := testStore.Checklists().Create(ctx, c); err != nil {
		t.Fatalf("create checklist: %v", err)
	}

	now := time.Now()
	events, err := c.CheckItem(0, user.ID, now)
	if err != nil {
		t.Fatalf("check item: %v", err)
	}
	if err := testStore.Checklists().Save(ctx, c, events); err != nil {
		t.Fatalf("save after check: %v", err)
	}
	if c.Status != domain.StatusValidating {
		t.Fatalf("expected validating, got %s", c.Status)
	}

	notifications, err := testStore.Notifications().ListForUser(ctx, approver.ID)
	if err != nil {
		t.Fatalf("list notifications: %v", err)
	}
	if len(notifications) == 0 || notifications[0].Type != domain.EventSubmittedForValidation {
		t.Fatalf("expected approver to be notified of submission, got %+v", notifications)
	}

	events, err = c.Approve(approver.ID)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := testStore.Checklists().Save(ctx, c, events); err != nil {
		t.Fatalf("save after approve: %v", err)
	}
	if c.Status != domain.StatusComplete {
		t.Fatalf("expected complete after approve, got %s", c.Status)
	}

	got, err := testStore.Checklists().Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("get checklist: %v", err)
	}
	if got.Status != domain.StatusComplete {
		t.Fatalf("expected persisted complete, got %s", got.Status)
	}
}

func TestChecklistRepo_RejectFlow(t *testing.T) {
	ctx := context.Background()
	user := mustCreateUser(t, "Assignee", uniqueName(t, "assignee"))
	approver := mustCreateUser(t, "Approver", uniqueName(t, "approver"))

	c := &domain.Checklist{
		CreatorID:      user.ID,
		AssignedUserID: &user.ID,
		ApproverID:     &approver.ID,
		Items: []domain.ChecklistItem{
			{Name: "Item A"},
			{Name: "Item B"},
		},
	}
	if err := testStore.Checklists().Create(ctx, c); err != nil {
		t.Fatalf("create checklist: %v", err)
	}

	now := time.Now()
	for i := range c.Items {
		events, err := c.CheckItem(i, user.ID, now)
		if err != nil {
			t.Fatalf("check item %d: %v", i, err)
		}
		if err := testStore.Checklists().Save(ctx, c, events); err != nil {
			t.Fatalf("save after checking item %d: %v", i, err)
		}
	}
	if c.Status != domain.StatusValidating {
		t.Fatalf("expected validating, got %s", c.Status)
	}

	rejectEvents, err := c.Reject(approver.ID, []int{0})
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if err := testStore.Checklists().Save(ctx, c, rejectEvents); err != nil {
		t.Fatalf("save after reject: %v", err)
	}
	if c.Status != domain.StatusOpen {
		t.Fatalf("expected open after reject, got %s", c.Status)
	}

	got, err := testStore.Checklists().Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("get checklist: %v", err)
	}
	if got.Status != domain.StatusOpen {
		t.Fatalf("expected persisted open, got %s", got.Status)
	}
	if got.Items[0].Checked {
		t.Fatalf("expected item 0 unchecked after reject")
	}
	if got.Items[0].AssigneeOverrideUserID == nil || *got.Items[0].AssigneeOverrideUserID != user.ID {
		t.Fatalf("expected item 0 override to point at original checker, got %+v", got.Items[0].AssigneeOverrideUserID)
	}
	if !got.Items[1].Checked {
		t.Fatalf("expected item 1 (not rejected) to remain checked")
	}

	notifications, err := testStore.Notifications().ListForUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("list notifications: %v", err)
	}
	found := false
	for _, n := range notifications {
		if n.Type == domain.EventRejected {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected assignee to be notified of rejection, got %+v", notifications)
	}
}

func TestChecklistRepo_ClaimRace(t *testing.T) {
	ctx := context.Background()
	winner := mustCreateUser(t, "Winner", uniqueName(t, "winner"))
	loser := mustCreateUser(t, "Loser", uniqueName(t, "loser"))
	group := mustCreateGroup(t, uniqueName(t, "team"), winner.ID, loser.ID)

	c := &domain.Checklist{
		CreatorID:       winner.ID,
		AssignedGroupID: &group.ID,
		Items: []domain.ChecklistItem{
			{Name: "Item A"},
		},
	}
	if err := testStore.Checklists().Create(ctx, c); err != nil {
		t.Fatalf("create checklist: %v", err)
	}

	// The winner claims first, so the loser's attempt (against the same
	// expectedCurrent=nil) is guaranteed to lose the CAS deterministically.
	ok, err := testStore.Checklists().Claim(ctx, c.ID, winner.ID, nil)
	if err != nil {
		t.Fatalf("winner claim: %v", err)
	}
	if !ok {
		t.Fatalf("expected winner's claim to succeed")
	}

	ok, err = testStore.Checklists().Claim(ctx, c.ID, loser.ID, nil)
	if err != nil {
		t.Fatalf("loser claim: %v", err)
	}
	if ok {
		t.Fatalf("expected loser's claim to fail (already claimed)")
	}

	got, err := testStore.Checklists().Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("get checklist: %v", err)
	}
	if got.AssignedUserID == nil || *got.AssignedUserID != winner.ID {
		t.Fatalf("expected checklist assigned to winner, got %+v", got.AssignedUserID)
	}

	notifications, err := testStore.Notifications().ListForUser(ctx, loser.ID)
	if err != nil {
		t.Fatalf("list notifications: %v", err)
	}
	if len(notifications) == 0 || notifications[0].Type != domain.EventClaimLost {
		t.Fatalf("expected loser to get a claim_lost notification, got %+v", notifications)
	}
	if notifications[0].ActorUserID == nil || *notifications[0].ActorUserID != winner.ID {
		t.Fatalf("expected claim_lost notification to name the winner, got %+v", notifications[0])
	}
}

func TestChecklistRepo_AddItemPersistsWithNewID(t *testing.T) {
	ctx := context.Background()
	creator := mustCreateUser(t, "Creator", uniqueName(t, "creator"))

	c := &domain.Checklist{
		CreatorID:      creator.ID,
		AssignedUserID: &creator.ID,
		Items:          []domain.ChecklistItem{{Name: "Item A"}},
	}
	if err := testStore.Checklists().Create(ctx, c); err != nil {
		t.Fatalf("create checklist: %v", err)
	}

	events, err := c.AddItem(creator.ID, "Item B", "")
	if err != nil {
		t.Fatalf("add item: %v", err)
	}
	if err := testStore.Checklists().Save(ctx, c, events); err != nil {
		t.Fatalf("save after add: %v", err)
	}
	if c.Items[1].ID == 0 {
		t.Fatalf("expected new item to get a real id, got %+v", c.Items[1])
	}

	got, err := testStore.Checklists().Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("get checklist: %v", err)
	}
	if len(got.Items) != 2 || got.Items[1].Name != "Item B" {
		t.Fatalf("expected 2 persisted items with Item B second, got %+v", got.Items)
	}
}

func TestChecklistRepo_RemoveItemSoftDeletes(t *testing.T) {
	ctx := context.Background()
	creator := mustCreateUser(t, "Creator", uniqueName(t, "creator"))

	c := &domain.Checklist{
		CreatorID:      creator.ID,
		AssignedUserID: &creator.ID,
		Items:          []domain.ChecklistItem{{Name: "Item A"}, {Name: "Item B"}},
	}
	if err := testStore.Checklists().Create(ctx, c); err != nil {
		t.Fatalf("create checklist: %v", err)
	}

	now := time.Now()
	checkEvents, err := c.CheckItem(0, creator.ID, now)
	if err != nil {
		t.Fatalf("check item 0: %v", err)
	}
	if err := testStore.Checklists().Save(ctx, c, checkEvents); err != nil {
		t.Fatalf("save after check: %v", err)
	}
	removedID := c.Items[0].ID

	removeEvents, err := c.RemoveItem(creator.ID, 0)
	if err != nil {
		t.Fatalf("remove item: %v", err)
	}
	if err := testStore.Checklists().Save(ctx, c, removeEvents); err != nil {
		t.Fatalf("save after remove: %v", err)
	}

	got, err := testStore.Checklists().Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("get checklist: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].Name != "Item B" {
		t.Fatalf("expected only Item B to remain visible, got %+v", got.Items)
	}

	db := mustOpenRawDB(t)
	var deletedAt sql.NullTime
	if err := db.QueryRowContext(ctx, `SELECT deleted_at FROM checklist_items WHERE id = $1`, removedID).Scan(&deletedAt); err != nil {
		t.Fatalf("query removed item directly: %v", err)
	}
	if !deletedAt.Valid {
		t.Fatalf("expected removed item to have deleted_at set, row should survive as a soft-delete")
	}

	var eventCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM checklist_events WHERE item_id = $1`, removedID).Scan(&eventCount); err != nil {
		t.Fatalf("query events for removed item: %v", err)
	}
	if eventCount == 0 {
		t.Fatalf("expected the removed item's prior events to still reference it via an intact fk")
	}
}

func TestChecklistRepo_ReorderItemsPersistsPositions(t *testing.T) {
	ctx := context.Background()
	creator := mustCreateUser(t, "Creator", uniqueName(t, "creator"))

	c := &domain.Checklist{
		CreatorID:      creator.ID,
		AssignedUserID: &creator.ID,
		Items:          []domain.ChecklistItem{{Name: "First"}, {Name: "Second"}, {Name: "Third"}},
	}
	if err := testStore.Checklists().Create(ctx, c); err != nil {
		t.Fatalf("create checklist: %v", err)
	}

	newOrder := []int64{c.Items[2].ID, c.Items[0].ID, c.Items[1].ID}
	events, err := c.ReorderItems(creator.ID, newOrder)
	if err != nil {
		t.Fatalf("reorder: %v", err)
	}
	if err := testStore.Checklists().Save(ctx, c, events); err != nil {
		t.Fatalf("save after reorder: %v", err)
	}

	got, err := testStore.Checklists().Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("get checklist: %v", err)
	}
	if len(got.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(got.Items))
	}
	if got.Items[0].Name != "Third" || got.Items[1].Name != "First" || got.Items[2].Name != "Second" {
		t.Fatalf("expected persisted order Third,First,Second, got %+v", got.Items)
	}
}

func TestChecklistRepo_SetItemCheckedReopensCompleteChecklist(t *testing.T) {
	ctx := context.Background()
	creator := mustCreateUser(t, "Creator", uniqueName(t, "creator"))
	assignee := mustCreateUser(t, "Assignee", uniqueName(t, "assignee"))

	c := &domain.Checklist{
		CreatorID:      creator.ID,
		AssignedUserID: &assignee.ID,
		Items:          []domain.ChecklistItem{{Name: "Item A"}},
	}
	if err := testStore.Checklists().Create(ctx, c); err != nil {
		t.Fatalf("create checklist: %v", err)
	}

	now := time.Now()
	checkEvents, err := c.CheckItem(0, assignee.ID, now)
	if err != nil {
		t.Fatalf("check item: %v", err)
	}
	if err := testStore.Checklists().Save(ctx, c, checkEvents); err != nil {
		t.Fatalf("save after check: %v", err)
	}
	if c.Status != domain.StatusComplete {
		t.Fatalf("expected complete, got %s", c.Status)
	}

	overrideEvents, err := c.SetItemChecked(creator.ID, 0, false, now)
	if err != nil {
		t.Fatalf("set item checked: %v", err)
	}
	if err := testStore.Checklists().Save(ctx, c, overrideEvents); err != nil {
		t.Fatalf("save after override: %v", err)
	}

	got, err := testStore.Checklists().Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("get checklist: %v", err)
	}
	if got.Status != domain.StatusOpen {
		t.Fatalf("expected persisted status reopened to open, got %s", got.Status)
	}
	if got.Items[0].Checked {
		t.Fatalf("expected item unchecked, got %+v", got.Items[0])
	}

	notifications, err := testStore.Notifications().ListForUser(ctx, assignee.ID)
	if err != nil {
		t.Fatalf("list notifications: %v", err)
	}
	found := false
	for _, n := range notifications {
		if n.Type == domain.EventReopened {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected assignee to be notified of the reopen, got %+v", notifications)
	}
}

func TestChecklistRepo_GroupMembershipTriggerBackstop(t *testing.T) {
	ctx := context.Background()
	outsider := mustCreateUser(t, "Outsider", uniqueName(t, "outsider"))
	member := mustCreateUser(t, "Member", uniqueName(t, "member"))
	group := mustCreateGroup(t, uniqueName(t, "team"), member.ID)

	c := &domain.Checklist{
		CreatorID:       member.ID,
		AssignedGroupID: &group.ID,
		AssignedUserID:  &outsider.ID,
		Items:           []domain.ChecklistItem{{Name: "Item A"}},
	}
	if err := testStore.Checklists().Create(ctx, c); err == nil {
		t.Fatalf("expected DB trigger to reject assigning a non-member user to a group-assigned checklist")
	}
}

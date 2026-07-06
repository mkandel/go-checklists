//go:build integration

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
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Step 1", "Step 2")

	c := &domain.Checklist{
		TenantID:       testTenantID,
		TemplateID:     tmpl.ID,
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
	if c.Name != tmpl.Name {
		t.Fatalf("expected checklist name to default to template name %q, got %q", tmpl.Name, c.Name)
	}

	got, err := testStore.Checklists().Get(ctx, testTenantID, c.ID)
	if err != nil {
		t.Fatalf("get checklist: %v", err)
	}
	if got.Status != domain.StatusOpen {
		t.Fatalf("expected open status, got %s", got.Status)
	}
	if len(got.Items) != 2 {
		t.Fatalf("expected 2 persisted items, got %d", len(got.Items))
	}
	if got.Name != tmpl.Name {
		t.Fatalf("expected persisted checklist name %q, got %q", tmpl.Name, got.Name)
	}
}

func TestChecklistRepo_CreateWithExplicitName(t *testing.T) {
	ctx := context.Background()
	creator := mustCreateUser(t, "Creator", uniqueName(t, "creator"))
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Step 1")

	c := &domain.Checklist{
		TenantID:       testTenantID,
		TemplateID:     tmpl.ID,
		Name:           "My Custom Name",
		CreatorID:      creator.ID,
		AssignedUserID: &creator.ID,
	}
	if err := testStore.Checklists().Create(ctx, c); err != nil {
		t.Fatalf("create checklist: %v", err)
	}
	if c.Name != "My Custom Name" {
		t.Fatalf("expected explicit name to be preserved, got %q", c.Name)
	}

	got, err := testStore.Checklists().Get(ctx, testTenantID, c.ID)
	if err != nil {
		t.Fatalf("get checklist: %v", err)
	}
	if got.Name != "My Custom Name" {
		t.Fatalf("expected persisted checklist name %q, got %q", "My Custom Name", got.Name)
	}
}

func TestChecklistRepo_LifecycleWithoutApprover(t *testing.T) {
	ctx := context.Background()
	user := mustCreateUser(t, "Assignee", uniqueName(t, "assignee"))
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Item A", "Item B")

	c := &domain.Checklist{
		TenantID:       testTenantID,
		TemplateID:     tmpl.ID,
		CreatorID:      user.ID,
		AssignedUserID: &user.ID,
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

	got, err := testStore.Checklists().Get(ctx, testTenantID, c.ID)
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
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Item A")

	c := &domain.Checklist{
		TenantID:       testTenantID,
		TemplateID:     tmpl.ID,
		CreatorID:      user.ID,
		AssignedUserID: &user.ID,
		ApproverID:     &approver.ID,
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

	notifications, err := testStore.Notifications().ListForUser(ctx, testTenantID, approver.ID)
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

	got, err := testStore.Checklists().Get(ctx, testTenantID, c.ID)
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
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Item A", "Item B")

	c := &domain.Checklist{
		TenantID:       testTenantID,
		TemplateID:     tmpl.ID,
		CreatorID:      user.ID,
		AssignedUserID: &user.ID,
		ApproverID:     &approver.ID,
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

	got, err := testStore.Checklists().Get(ctx, testTenantID, c.ID)
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

	notifications, err := testStore.Notifications().ListForUser(ctx, testTenantID, user.ID)
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
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Item A")

	c := &domain.Checklist{
		TenantID:        testTenantID,
		TemplateID:      tmpl.ID,
		CreatorID:       winner.ID,
		AssignedGroupID: &group.ID,
	}
	if err := testStore.Checklists().Create(ctx, c); err != nil {
		t.Fatalf("create checklist: %v", err)
	}

	// The winner claims first, so the loser's attempt (against the same
	// expectedCurrent=nil) is guaranteed to lose the CAS deterministically.
	ok, err := testStore.Checklists().Claim(ctx, testTenantID, c.ID, winner.ID, nil)
	if err != nil {
		t.Fatalf("winner claim: %v", err)
	}
	if !ok {
		t.Fatalf("expected winner's claim to succeed")
	}

	ok, err = testStore.Checklists().Claim(ctx, testTenantID, c.ID, loser.ID, nil)
	if err != nil {
		t.Fatalf("loser claim: %v", err)
	}
	if ok {
		t.Fatalf("expected loser's claim to fail (already claimed)")
	}

	got, err := testStore.Checklists().Get(ctx, testTenantID, c.ID)
	if err != nil {
		t.Fatalf("get checklist: %v", err)
	}
	if got.AssignedUserID == nil || *got.AssignedUserID != winner.ID {
		t.Fatalf("expected checklist assigned to winner, got %+v", got.AssignedUserID)
	}

	notifications, err := testStore.Notifications().ListForUser(ctx, testTenantID, loser.ID)
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
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Item A")

	c := &domain.Checklist{
		TenantID:       testTenantID,
		TemplateID:     tmpl.ID,
		CreatorID:      creator.ID,
		AssignedUserID: &creator.ID,
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

	got, err := testStore.Checklists().Get(ctx, testTenantID, c.ID)
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
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Item A", "Item B")

	c := &domain.Checklist{
		TenantID:       testTenantID,
		TemplateID:     tmpl.ID,
		CreatorID:      creator.ID,
		AssignedUserID: &creator.ID,
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

	got, err := testStore.Checklists().Get(ctx, testTenantID, c.ID)
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
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "First", "Second", "Third")

	c := &domain.Checklist{
		TenantID:       testTenantID,
		TemplateID:     tmpl.ID,
		CreatorID:      creator.ID,
		AssignedUserID: &creator.ID,
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

	got, err := testStore.Checklists().Get(ctx, testTenantID, c.ID)
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
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Item A")

	c := &domain.Checklist{
		TenantID:       testTenantID,
		TemplateID:     tmpl.ID,
		CreatorID:      creator.ID,
		AssignedUserID: &assignee.ID,
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

	got, err := testStore.Checklists().Get(ctx, testTenantID, c.ID)
	if err != nil {
		t.Fatalf("get checklist: %v", err)
	}
	if got.Status != domain.StatusOpen {
		t.Fatalf("expected persisted status reopened to open, got %s", got.Status)
	}
	if got.Items[0].Checked {
		t.Fatalf("expected item unchecked, got %+v", got.Items[0])
	}

	notifications, err := testStore.Notifications().ListForUser(ctx, testTenantID, assignee.ID)
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

func TestChecklistRepo_ListReturnsRelevantOnly(t *testing.T) {
	ctx := context.Background()
	creator := mustCreateUser(t, "Creator", uniqueName(t, "creator"))
	approver := mustCreateUser(t, "Approver", uniqueName(t, "approver"))
	assignee := mustCreateUser(t, "Assignee", uniqueName(t, "assignee"))
	groupMember := mustCreateUser(t, "GroupMember", uniqueName(t, "groupmember"))
	outsider := mustCreateUser(t, "Outsider", uniqueName(t, "outsider"))
	group := mustCreateGroup(t, uniqueName(t, "team"), groupMember.ID)
	tmplDirect := mustCreateTemplate(t, uniqueName(t, "tmpl-direct"), "Item A")
	tmplGroup := mustCreateTemplate(t, uniqueName(t, "tmpl-group"), "Item A")

	direct := &domain.Checklist{
		TenantID:       testTenantID,
		TemplateID:     tmplDirect.ID,
		CreatorID:      creator.ID,
		AssignedUserID: &assignee.ID,
		ApproverID:     &approver.ID,
	}
	if err := testStore.Checklists().Create(ctx, direct); err != nil {
		t.Fatalf("create direct checklist: %v", err)
	}

	unclaimedGroup := &domain.Checklist{
		TenantID:        testTenantID,
		TemplateID:      tmplGroup.ID,
		CreatorID:       creator.ID,
		AssignedGroupID: &group.ID,
	}
	if err := testStore.Checklists().Create(ctx, unclaimedGroup); err != nil {
		t.Fatalf("create group checklist: %v", err)
	}

	for _, tc := range []struct {
		name    string
		userID  int64
		wantIDs []int64
	}{
		{"creator", creator.ID, []int64{direct.ID, unclaimedGroup.ID}},
		{"approver", approver.ID, []int64{direct.ID}},
		{"assignee", assignee.ID, []int64{direct.ID}},
		{"unclaimed group member", groupMember.ID, []int64{unclaimedGroup.ID}},
		{"outsider", outsider.ID, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := testStore.Checklists().List(ctx, domain.ChecklistFilter{TenantID: testTenantID, UserID: tc.userID})
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			gotIDs := make(map[int64]bool, len(got))
			for _, c := range got {
				gotIDs[c.ID] = true
				if c.Items != nil {
					t.Fatalf("expected List to omit items, got %+v", c.Items)
				}
			}
			for _, want := range tc.wantIDs {
				if !gotIDs[want] {
					t.Fatalf("expected checklist %d in results for %s, got ids %v", want, tc.name, gotIDs)
				}
			}
			if len(gotIDs) != len(tc.wantIDs) {
				t.Fatalf("expected exactly %v for %s, got %v", tc.wantIDs, tc.name, gotIDs)
			}
		})
	}
}

func TestChecklistRepo_ListClaimedGroupChecklistNoLongerVisibleToOtherMembers(t *testing.T) {
	ctx := context.Background()
	creator := mustCreateUser(t, "Creator", uniqueName(t, "creator"))
	claimer := mustCreateUser(t, "Claimer", uniqueName(t, "claimer"))
	otherMember := mustCreateUser(t, "OtherMember", uniqueName(t, "othermember"))
	group := mustCreateGroup(t, uniqueName(t, "team"), claimer.ID, otherMember.ID)
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Item A")

	c := &domain.Checklist{
		TenantID:        testTenantID,
		TemplateID:      tmpl.ID,
		CreatorID:       creator.ID,
		AssignedGroupID: &group.ID,
	}
	if err := testStore.Checklists().Create(ctx, c); err != nil {
		t.Fatalf("create checklist: %v", err)
	}

	ok, err := testStore.Checklists().Claim(ctx, testTenantID, c.ID, claimer.ID, nil)
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}

	got, err := testStore.Checklists().List(ctx, domain.ChecklistFilter{TenantID: testTenantID, UserID: otherMember.ID})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, item := range got {
		if item.ID == c.ID {
			t.Fatalf("expected claimed group checklist to no longer be relevant to other group members")
		}
	}
}

func TestChecklistRepo_ListFiltersByStatus(t *testing.T) {
	ctx := context.Background()
	creator := mustCreateUser(t, "Creator", uniqueName(t, "creator"))
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Item A")

	c := &domain.Checklist{
		TenantID:       testTenantID,
		TemplateID:     tmpl.ID,
		CreatorID:      creator.ID,
		AssignedUserID: &creator.ID,
	}
	if err := testStore.Checklists().Create(ctx, c); err != nil {
		t.Fatalf("create checklist: %v", err)
	}

	open := domain.StatusOpen
	got, err := testStore.Checklists().List(ctx, domain.ChecklistFilter{TenantID: testTenantID, UserID: creator.ID, Status: &open})
	if err != nil {
		t.Fatalf("list open: %v", err)
	}
	found := false
	for _, item := range got {
		if item.ID == c.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected open checklist in open-filtered results")
	}

	complete := domain.StatusComplete
	got, err = testStore.Checklists().List(ctx, domain.ChecklistFilter{TenantID: testTenantID, UserID: creator.ID, Status: &complete})
	if err != nil {
		t.Fatalf("list complete: %v", err)
	}
	for _, item := range got {
		if item.ID == c.ID {
			t.Fatalf("expected open checklist to be excluded from complete-filtered results")
		}
	}
}

func TestChecklistRepo_ClearUserAssignments(t *testing.T) {
	ctx := context.Background()
	creator := mustCreateUser(t, "Creator", uniqueName(t, "creator"))
	target := mustCreateUser(t, "Target", uniqueName(t, "target"))
	other := mustCreateUser(t, "Other", uniqueName(t, "other"))
	group := mustCreateGroup(t, uniqueName(t, "team"), target.ID, other.ID)
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Item A")

	// user-only assignment (no group): clearing leaves both columns nil.
	userOnly := &domain.Checklist{
		TenantID:       testTenantID,
		TemplateID:     tmpl.ID,
		CreatorID:      creator.ID,
		AssignedUserID: &target.ID,
		ApproverID:     &target.ID,
	}
	if err := testStore.Checklists().Create(ctx, userOnly); err != nil {
		t.Fatalf("create user-only checklist: %v", err)
	}

	// group + user assignment: clearing the user falls back to unclaimed
	// within the group, group_id untouched.
	groupAndUser := &domain.Checklist{
		TenantID:        testTenantID,
		TemplateID:      tmpl.ID,
		CreatorID:       creator.ID,
		AssignedGroupID: &group.ID,
		AssignedUserID:  &target.ID,
	}
	if err := testStore.Checklists().Create(ctx, groupAndUser); err != nil {
		t.Fatalf("create group+user checklist: %v", err)
	}

	// unrelated checklist assigned to someone else: must be untouched.
	untouched := &domain.Checklist{
		TenantID:       testTenantID,
		TemplateID:     tmpl.ID,
		CreatorID:      creator.ID,
		AssignedUserID: &other.ID,
	}
	if err := testStore.Checklists().Create(ctx, untouched); err != nil {
		t.Fatalf("create untouched checklist: %v", err)
	}

	if err := testStore.Checklists().ClearUserAssignments(ctx, testTenantID, target.ID); err != nil {
		t.Fatalf("clear user assignments: %v", err)
	}

	got, err := testStore.Checklists().Get(ctx, testTenantID, userOnly.ID)
	if err != nil {
		t.Fatalf("get user-only checklist: %v", err)
	}
	if got.AssignedUserID != nil || got.ApproverID != nil {
		t.Fatalf("expected user-only checklist fully unassigned, got %+v", got)
	}

	got, err = testStore.Checklists().Get(ctx, testTenantID, groupAndUser.ID)
	if err != nil {
		t.Fatalf("get group+user checklist: %v", err)
	}
	if got.AssignedUserID != nil {
		t.Fatalf("expected group+user checklist's assignee cleared, got %+v", got.AssignedUserID)
	}
	if got.AssignedGroupID == nil || *got.AssignedGroupID != group.ID {
		t.Fatalf("expected group assignment untouched, got %+v", got.AssignedGroupID)
	}

	got, err = testStore.Checklists().Get(ctx, testTenantID, untouched.ID)
	if err != nil {
		t.Fatalf("get untouched checklist: %v", err)
	}
	if got.AssignedUserID == nil || *got.AssignedUserID != other.ID {
		t.Fatalf("expected unrelated checklist's assignment untouched, got %+v", got.AssignedUserID)
	}

	db := mustOpenRawDB(t)
	var eventCount int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM checklist_events WHERE checklist_id IN ($1, $2) AND action IN ($3, $4) AND actor_user_id = $5`,
		userOnly.ID, groupAndUser.ID, domain.EventApproverChanged, domain.EventAssigneeChanged, target.ID,
	).Scan(&eventCount); err != nil {
		t.Fatalf("query clear events: %v", err)
	}
	if eventCount != 3 {
		t.Fatalf("expected 3 clear events (approver+assignee on user-only, assignee on group+user), got %d", eventCount)
	}
}

func TestChecklistRepo_GroupMembershipTriggerBackstop(t *testing.T) {
	ctx := context.Background()
	outsider := mustCreateUser(t, "Outsider", uniqueName(t, "outsider"))
	member := mustCreateUser(t, "Member", uniqueName(t, "member"))
	group := mustCreateGroup(t, uniqueName(t, "team"), member.ID)
	tmpl := mustCreateTemplate(t, uniqueName(t, "tmpl"), "Item A")

	c := &domain.Checklist{
		TenantID:        testTenantID,
		TemplateID:      tmpl.ID,
		CreatorID:       member.ID,
		AssignedGroupID: &group.ID,
		AssignedUserID:  &outsider.ID,
	}
	if err := testStore.Checklists().Create(ctx, c); err == nil {
		t.Fatalf("expected DB trigger to reject assigning a non-member user to a group-assigned checklist")
	}
}

package postgres_test

import (
	"context"
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

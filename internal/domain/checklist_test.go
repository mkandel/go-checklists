package domain

import (
	"testing"
	"time"
)

func newOpenChecklist(assignedUserID int64, approverID *int64, itemCount int) *Checklist {
	items := make([]ChecklistItem, itemCount)
	for i := range items {
		items[i] = ChecklistItem{ID: int64(i + 1)}
	}
	return &Checklist{
		ID:             1,
		AssignedUserID: &assignedUserID,
		ApproverID:     approverID,
		Status:         StatusOpen,
		Items:          items,
	}
}

func TestCheckItem_CompletesWithoutApprover(t *testing.T) {
	c := newOpenChecklist(42, nil, 2)
	now := time.Now()

	events, err := c.CheckItem(0, 42, now)
	if err != nil {
		t.Fatalf("check item 0: %v", err)
	}
	if c.Status != StatusOpen {
		t.Fatalf("expected still open after partial check, got %s", c.Status)
	}
	if len(events) != 1 || events[0].Action != EventItemChecked {
		t.Fatalf("expected a single item_checked event, got %+v", events)
	}

	events, err = c.CheckItem(1, 42, now)
	if err != nil {
		t.Fatalf("check item 1: %v", err)
	}
	if c.Status != StatusComplete {
		t.Fatalf("expected complete after all items checked with no approver, got %s", c.Status)
	}
	if len(events) != 2 || events[1].Action != EventCompleted {
		t.Fatalf("expected item_checked + completed events, got %+v", events)
	}
}

func TestCheckItem_MovesToValidatingWithApprover(t *testing.T) {
	approver := int64(99)
	c := newOpenChecklist(42, &approver, 1)
	now := time.Now()

	events, err := c.CheckItem(0, 42, now)
	if err != nil {
		t.Fatalf("check item: %v", err)
	}
	if c.Status != StatusValidating {
		t.Fatalf("expected validating, got %s", c.Status)
	}
	if len(events) != 2 || events[1].Action != EventSubmittedForValidation {
		t.Fatalf("expected item_checked + submitted_for_validation events, got %+v", events)
	}
}

func TestCheckItem_WrongUserRejected(t *testing.T) {
	c := newOpenChecklist(42, nil, 1)

	_, err := c.CheckItem(0, 1, time.Now())
	if err != ErrNotAssignee {
		t.Fatalf("expected ErrNotAssignee, got %v", err)
	}
}

func TestCheckItem_UnclaimedRejected(t *testing.T) {
	c := newOpenChecklist(42, nil, 1)
	c.AssignedUserID = nil

	_, err := c.CheckItem(0, 1, time.Now())
	if err != ErrUnclaimed {
		t.Fatalf("expected ErrUnclaimed, got %v", err)
	}
}

func TestRejectFlow_ReassignsToOriginalChecker(t *testing.T) {
	approver := int64(99)
	c := newOpenChecklist(42, &approver, 2)
	now := time.Now()

	if _, err := c.CheckItem(0, 42, now); err != nil {
		t.Fatalf("check item 0: %v", err)
	}
	if _, err := c.CheckItem(1, 42, now); err != nil {
		t.Fatalf("check item 1: %v", err)
	}
	if c.Status != StatusValidating {
		t.Fatalf("expected validating, got %s", c.Status)
	}

	events, err := c.Reject(approver, []int{0})
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if c.Status != StatusOpen {
		t.Fatalf("expected open after reject, got %s", c.Status)
	}
	if len(events) != 2 || events[0].Action != EventItemUnchecked || events[1].Action != EventRejected {
		t.Fatalf("expected item_unchecked + rejected events, got %+v", events)
	}
	if c.Items[0].Checked {
		t.Fatalf("expected item 0 to be unchecked after reject")
	}
	if c.Items[0].AssigneeOverrideUserID == nil || *c.Items[0].AssigneeOverrideUserID != 42 {
		t.Fatalf("expected item 0 override to point at original checker 42, got %+v", c.Items[0].AssigneeOverrideUserID)
	}
	if !c.Items[1].Checked {
		t.Fatalf("expected item 1 (not rejected) to remain checked")
	}

	// The reassigned item can now only be checked by the original checker,
	// even though it's a different user than the checklist's normal assignee.
	if _, err := c.CheckItem(0, 1, now); err != ErrNotAssignee {
		t.Fatalf("expected ErrNotAssignee for non-original-checker, got %v", err)
	}
	if _, err := c.CheckItem(0, 42, now); err != nil {
		t.Fatalf("re-check by original checker: %v", err)
	}
}

func TestApprove_OnlyApproverInValidating(t *testing.T) {
	approver := int64(99)
	c := newOpenChecklist(42, &approver, 1)
	now := time.Now()
	if _, err := c.CheckItem(0, 42, now); err != nil {
		t.Fatalf("check item: %v", err)
	}

	if _, err := c.Approve(42); err != ErrNotApprover {
		t.Fatalf("expected ErrNotApprover, got %v", err)
	}
	events, err := c.Approve(approver)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if c.Status != StatusComplete {
		t.Fatalf("expected complete, got %s", c.Status)
	}
	if len(events) != 2 || events[0].Action != EventApproved || events[1].Action != EventCompleted {
		t.Fatalf("expected approved + completed events, got %+v", events)
	}
}

const testCreatorID = 7

func TestAddItem_RequiresCreator(t *testing.T) {
	c := newOpenChecklist(42, nil, 1)
	c.CreatorID = testCreatorID

	if _, err := c.AddItem(999, "New step", ""); err != ErrNotCreator {
		t.Fatalf("expected ErrNotCreator, got %v", err)
	}
}

func TestAddItem_AppendsAndForcesOpen(t *testing.T) {
	approver := int64(99)
	c := newOpenChecklist(42, &approver, 1)
	c.CreatorID = testCreatorID
	now := time.Now()

	if _, err := c.CheckItem(0, 42, now); err != nil {
		t.Fatalf("check item: %v", err)
	}
	if c.Status != StatusValidating {
		t.Fatalf("expected validating, got %s", c.Status)
	}

	events, err := c.AddItem(testCreatorID, "New step", "some-ref")
	if err != nil {
		t.Fatalf("add item: %v", err)
	}
	if len(c.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(c.Items))
	}
	if c.Items[1].Name != "New step" || c.Items[1].Position != 1 || c.Items[1].Checked {
		t.Fatalf("unexpected new item: %+v", c.Items[1])
	}
	if c.Status != StatusOpen {
		t.Fatalf("expected forced back to open, got %s", c.Status)
	}
	if *c.AssignedUserID != 42 {
		t.Fatalf("expected assignee untouched, got %v", c.AssignedUserID)
	}
	if len(events) != 2 || events[0].Action != EventItemAdded || events[1].Action != EventReopened {
		t.Fatalf("expected item_added + reopened events, got %+v", events)
	}
}

func TestAddItem_NoReopenEventWhenAlreadyOpen(t *testing.T) {
	c := newOpenChecklist(42, nil, 1)
	c.CreatorID = testCreatorID

	events, err := c.AddItem(testCreatorID, "New step", "")
	if err != nil {
		t.Fatalf("add item: %v", err)
	}
	if len(events) != 1 || events[0].Action != EventItemAdded {
		t.Fatalf("expected only an item_added event when already open, got %+v", events)
	}
}

func TestRemoveItem_RequiresCreatorAndRenumbersPositions(t *testing.T) {
	c := newOpenChecklist(42, nil, 3)
	c.CreatorID = testCreatorID
	for i := range c.Items {
		c.Items[i].Position = i
	}

	if _, err := c.RemoveItem(999, 0); err != ErrNotCreator {
		t.Fatalf("expected ErrNotCreator, got %v", err)
	}

	removedID := c.Items[0].ID
	events, err := c.RemoveItem(testCreatorID, 0)
	if err != nil {
		t.Fatalf("remove item: %v", err)
	}
	if len(c.Items) != 2 {
		t.Fatalf("expected 2 items remaining, got %d", len(c.Items))
	}
	if c.Items[0].Position != 0 || c.Items[1].Position != 1 {
		t.Fatalf("expected renumbered positions 0,1, got %d,%d", c.Items[0].Position, c.Items[1].Position)
	}
	if len(events) != 1 || events[0].Action != EventItemRemoved || events[0].ItemID == nil || *events[0].ItemID != removedID {
		t.Fatalf("expected a single item_removed event for the removed item, got %+v", events)
	}
}

func TestRemoveItem_ForcesOpenFromComplete(t *testing.T) {
	c := newOpenChecklist(42, nil, 1)
	c.CreatorID = testCreatorID
	now := time.Now()
	if _, err := c.CheckItem(0, 42, now); err != nil {
		t.Fatalf("check item: %v", err)
	}
	if c.Status != StatusComplete {
		t.Fatalf("expected complete, got %s", c.Status)
	}

	events, err := c.RemoveItem(testCreatorID, 0)
	if err != nil {
		t.Fatalf("remove item: %v", err)
	}
	if c.Status != StatusOpen {
		t.Fatalf("expected forced back to open, got %s", c.Status)
	}
	if len(events) != 2 || events[1].Action != EventReopened {
		t.Fatalf("expected item_removed + reopened events, got %+v", events)
	}
}

func TestReorderItems_RequiresCreatorAndValidPermutation(t *testing.T) {
	c := newOpenChecklist(42, nil, 3)
	c.CreatorID = testCreatorID
	for i := range c.Items {
		c.Items[i].Position = i
	}
	originalIDs := []int64{c.Items[0].ID, c.Items[1].ID, c.Items[2].ID}

	if _, err := c.ReorderItems(999, originalIDs); err != ErrNotCreator {
		t.Fatalf("expected ErrNotCreator, got %v", err)
	}

	if _, err := c.ReorderItems(testCreatorID, originalIDs[:2]); err != ErrInvalidReorder {
		t.Fatalf("expected ErrInvalidReorder for wrong length, got %v", err)
	}
	if _, err := c.ReorderItems(testCreatorID, []int64{originalIDs[0], originalIDs[1], 12345}); err != ErrInvalidReorder {
		t.Fatalf("expected ErrInvalidReorder for unknown id, got %v", err)
	}

	newOrder := []int64{originalIDs[2], originalIDs[0], originalIDs[1]}
	events, err := c.ReorderItems(testCreatorID, newOrder)
	if err != nil {
		t.Fatalf("reorder: %v", err)
	}
	if c.Items[0].ID != originalIDs[2] || c.Items[0].Position != 0 {
		t.Fatalf("expected item %d first at position 0, got %+v", originalIDs[2], c.Items[0])
	}
	if c.Items[1].ID != originalIDs[0] || c.Items[1].Position != 1 {
		t.Fatalf("expected item %d second at position 1, got %+v", originalIDs[0], c.Items[1])
	}
	if c.Items[2].ID != originalIDs[1] || c.Items[2].Position != 2 {
		t.Fatalf("expected item %d third at position 2, got %+v", originalIDs[1], c.Items[2])
	}
	if len(events) != 1 || events[0].Action != EventItemsReordered {
		t.Fatalf("expected a single items_reordered event when already open, got %+v", events)
	}
}

func TestSetItemChecked_RequiresCreatorAndBypassesAssigneeGate(t *testing.T) {
	c := newOpenChecklist(42, nil, 1)
	c.CreatorID = testCreatorID

	if _, err := c.SetItemChecked(999, 0, true, time.Now()); err != ErrNotCreator {
		t.Fatalf("expected ErrNotCreator, got %v", err)
	}

	// Creator can check an item even though they aren't the assignee (42).
	now := time.Now()
	events, err := c.SetItemChecked(testCreatorID, 0, true, now)
	if err != nil {
		t.Fatalf("set item checked: %v", err)
	}
	if !c.Items[0].Checked || c.Items[0].CheckedBy == nil || *c.Items[0].CheckedBy != testCreatorID {
		t.Fatalf("expected item checked by creator, got %+v", c.Items[0])
	}
	// Unlike CheckItem, this never auto-advances the status even though all
	// items are now checked.
	if c.Status != StatusOpen {
		t.Fatalf("expected status to remain open, got %s", c.Status)
	}
	if len(events) != 1 || events[0].Action != EventItemChecked {
		t.Fatalf("expected a single item_checked event, got %+v", events)
	}

	events, err = c.SetItemChecked(testCreatorID, 0, false, now)
	if err != nil {
		t.Fatalf("set item unchecked: %v", err)
	}
	if c.Items[0].Checked || c.Items[0].CheckedBy != nil {
		t.Fatalf("expected item unchecked, got %+v", c.Items[0])
	}
	if len(events) != 1 || events[0].Action != EventItemUnchecked {
		t.Fatalf("expected a single item_unchecked event, got %+v", events)
	}
}

func TestSetItemChecked_ForcesOpenFromValidating(t *testing.T) {
	approver := int64(99)
	c := newOpenChecklist(42, &approver, 1)
	c.CreatorID = testCreatorID
	now := time.Now()
	if _, err := c.CheckItem(0, 42, now); err != nil {
		t.Fatalf("check item: %v", err)
	}
	if c.Status != StatusValidating {
		t.Fatalf("expected validating, got %s", c.Status)
	}

	events, err := c.SetItemChecked(testCreatorID, 0, false, now)
	if err != nil {
		t.Fatalf("set item unchecked: %v", err)
	}
	if c.Status != StatusOpen {
		t.Fatalf("expected forced back to open, got %s", c.Status)
	}
	if len(events) != 2 || events[1].Action != EventReopened {
		t.Fatalf("expected item_unchecked + reopened events, got %+v", events)
	}
}

func TestItemIndex(t *testing.T) {
	c := newOpenChecklist(42, nil, 3)

	if idx, ok := c.ItemIndex(c.Items[1].ID); !ok || idx != 1 {
		t.Fatalf("expected index 1, got (%d, %v)", idx, ok)
	}
	if _, ok := c.ItemIndex(99999); ok {
		t.Fatal("expected ok=false for unknown item id")
	}
}

func TestVisibleTo(t *testing.T) {
	approver := int64(99)
	group := int64(5)

	notHidden := newOpenChecklist(42, nil, 1)
	if !notHidden.VisibleTo(1, false) {
		t.Fatal("expected non-hidden checklist visible to anyone")
	}

	c := newOpenChecklist(42, &approver, 1)
	c.Hidden = true
	c.CreatorID = testCreatorID

	if !c.VisibleTo(testCreatorID, false) {
		t.Fatal("expected creator to see hidden checklist")
	}
	if !c.VisibleTo(approver, false) {
		t.Fatal("expected approver to see hidden checklist")
	}
	if !c.VisibleTo(42, false) {
		t.Fatal("expected claimed assignee to see hidden checklist")
	}
	if c.VisibleTo(1, false) {
		t.Fatal("expected unrelated user not to see hidden checklist")
	}

	c.AssignedUserID = nil
	c.AssignedGroupID = &group
	if !c.VisibleTo(1, true) {
		t.Fatal("expected group member to see hidden, unclaimed group checklist")
	}
	if c.VisibleTo(1, false) {
		t.Fatal("expected non-member not to see hidden, unclaimed group checklist")
	}
}

func TestValidateAssignment(t *testing.T) {
	group := int64(5)
	user := int64(42)

	if err := ValidateAssignment(nil, nil, false); err != ErrAssignmentRequired {
		t.Fatalf("expected ErrAssignmentRequired, got %v", err)
	}
	if err := ValidateAssignment(&group, nil, false); err != nil {
		t.Fatalf("expected group-only assignment to be valid, got %v", err)
	}
	if err := ValidateAssignment(nil, &user, false); err != nil {
		t.Fatalf("expected user-only assignment to be valid, got %v", err)
	}
	if err := ValidateAssignment(&group, &user, false); err != ErrAssigneeNotGroupMember {
		t.Fatalf("expected ErrAssigneeNotGroupMember, got %v", err)
	}
	if err := ValidateAssignment(&group, &user, true); err != nil {
		t.Fatalf("expected valid group+member assignment, got %v", err)
	}
}

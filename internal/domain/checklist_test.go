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

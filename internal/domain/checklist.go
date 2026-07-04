package domain

import (
	"errors"
	"time"
)

var (
	ErrChecklistNotOpen = errors.New("domain: checklist is not open for item edits")
	ErrNotAssignee      = errors.New("domain: user is not the responsible assignee for this item")
	ErrUnclaimed        = errors.New("domain: item has no claimed assignee")
	ErrNotValidating    = errors.New("domain: checklist is not awaiting validation")
	ErrNotApprover      = errors.New("domain: user is not the checklist's approver")
)

// ResponsibleUserFor returns the user responsible for checking off the given
// item: the item's assignee override if it was previously kicked back to its
// original checker by a rejection, otherwise the checklist's normal assignee.
func (c *Checklist) ResponsibleUserFor(item ChecklistItem) *int64 {
	if item.AssigneeOverrideUserID != nil {
		return item.AssigneeOverrideUserID
	}
	return c.AssignedUserID
}

// CheckItem marks the item at itemIndex as checked by actingUserID, and
// evaluates whether the checklist should transition to validating or
// complete as a result. Only valid while the checklist is open. Returns the
// Events caused by this call, for the store layer to append to the audit
// log alongside the state change.
func (c *Checklist) CheckItem(itemIndex int, actingUserID int64, now time.Time) ([]Event, error) {
	if c.Status != StatusOpen {
		return nil, ErrChecklistNotOpen
	}

	item := &c.Items[itemIndex]
	responsible := c.ResponsibleUserFor(*item)
	if responsible == nil {
		return nil, ErrUnclaimed
	}
	if *responsible != actingUserID {
		return nil, ErrNotAssignee
	}

	item.Checked = true
	item.CheckedBy = &actingUserID
	item.CheckedAt = &now

	events := []Event{{
		ChecklistID: c.ID,
		ItemID:      &item.ID,
		ActorUserID: actingUserID,
		Action:      EventItemChecked,
	}}

	if c.allItemsChecked() {
		if c.ApproverID != nil {
			c.Status = StatusValidating
			events = append(events, Event{
				ChecklistID: c.ID,
				ActorUserID: actingUserID,
				Action:      EventSubmittedForValidation,
			})
		} else {
			c.Status = StatusComplete
			events = append(events, Event{
				ChecklistID: c.ID,
				ActorUserID: actingUserID,
				Action:      EventCompleted,
			})
		}
	}
	return events, nil
}

func (c *Checklist) allItemsChecked() bool {
	if len(c.Items) == 0 {
		return false
	}
	for _, item := range c.Items {
		if !item.Checked {
			return false
		}
	}
	return true
}

// Approve completes a checklist that's awaiting validation. Only the
// checklist's approver may call this. Returns the Events caused by this
// call, for the store layer to append to the audit log alongside the state
// change.
func (c *Checklist) Approve(actingUserID int64) ([]Event, error) {
	if c.Status != StatusValidating {
		return nil, ErrNotValidating
	}
	if c.ApproverID == nil || *c.ApproverID != actingUserID {
		return nil, ErrNotApprover
	}
	c.Status = StatusComplete
	return []Event{
		{ChecklistID: c.ID, ActorUserID: actingUserID, Action: EventApproved},
		{ChecklistID: c.ID, ActorUserID: actingUserID, Action: EventCompleted},
	}, nil
}

// Reject unchecks the items at itemIndices, reassigns each one individually
// to whoever originally checked it, and returns the checklist to open. Only
// the checklist's approver may call this, and only while validating.
// Returns the Events caused by this call, for the store layer to append to
// the audit log alongside the state change.
func (c *Checklist) Reject(actingUserID int64, itemIndices []int) ([]Event, error) {
	if c.Status != StatusValidating {
		return nil, ErrNotValidating
	}
	if c.ApproverID == nil || *c.ApproverID != actingUserID {
		return nil, ErrNotApprover
	}

	events := make([]Event, 0, len(itemIndices)+1)
	for _, idx := range itemIndices {
		item := &c.Items[idx]
		if item.CheckedBy != nil {
			override := *item.CheckedBy
			item.AssigneeOverrideUserID = &override
		}
		item.Checked = false
		item.CheckedBy = nil
		item.CheckedAt = nil

		events = append(events, Event{
			ChecklistID: c.ID,
			ItemID:      &item.ID,
			ActorUserID: actingUserID,
			Action:      EventItemUnchecked,
		})
	}

	c.Status = StatusOpen
	events = append(events, Event{ChecklistID: c.ID, ActorUserID: actingUserID, Action: EventRejected})
	return events, nil
}

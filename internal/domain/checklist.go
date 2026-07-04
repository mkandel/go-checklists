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
	ErrNotCreator       = errors.New("domain: user is not this checklist's creator")
	ErrInvalidReorder   = errors.New("domain: reordered item ids do not match the checklist's current items")
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
		TenantID:    c.TenantID,
		ChecklistID: c.ID,
		ItemID:      &item.ID,
		ActorUserID: actingUserID,
		Action:      EventItemChecked,
	}}

	if c.allItemsChecked() {
		if c.ApproverID != nil {
			c.Status = StatusValidating
			events = append(events, Event{
				TenantID: c.TenantID, ChecklistID: c.ID,
				ActorUserID: actingUserID,
				Action:      EventSubmittedForValidation,
			})
		} else {
			c.Status = StatusComplete
			events = append(events, Event{
				TenantID: c.TenantID, ChecklistID: c.ID,
				ActorUserID: actingUserID,
				Action:      EventCompleted,
			})
		}
	}
	return events, nil
}

// ItemIndex resolves a stable item ID to its index in c.Items, for callers
// (e.g. the HTTP layer) that address items by ID rather than position.
func (c *Checklist) ItemIndex(itemID int64) (int, bool) {
	for i, item := range c.Items {
		if item.ID == itemID {
			return i, true
		}
	}
	return 0, false
}

// VisibleTo reports whether userID may see this checklist, given the
// "hidden" visibility rule: non-hidden checklists are visible to everyone;
// hidden ones only to the creator, the approver, the claimed assignee, or
// (while unclaimed) any member of the assigned group. isGroupMember is
// looked up by the caller (e.g. via GroupRepo.IsMember) since domain has no
// DB access, and is only consulted when the checklist is unclaimed.
func (c *Checklist) VisibleTo(userID int64, isGroupMember bool) bool {
	if !c.Hidden {
		return true
	}
	if c.CreatorID == userID {
		return true
	}
	if c.ApproverID != nil && *c.ApproverID == userID {
		return true
	}
	if c.AssignedUserID != nil {
		return *c.AssignedUserID == userID
	}
	return isGroupMember
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
		{TenantID: c.TenantID, ChecklistID: c.ID, ActorUserID: actingUserID, Action: EventApproved},
		{TenantID: c.TenantID, ChecklistID: c.ID, ActorUserID: actingUserID, Action: EventCompleted},
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
			TenantID: c.TenantID, ChecklistID: c.ID,
			ItemID:      &item.ID,
			ActorUserID: actingUserID,
			Action:      EventItemUnchecked,
		})
	}

	c.Status = StatusOpen
	events = append(events, Event{TenantID: c.TenantID, ChecklistID: c.ID, ActorUserID: actingUserID, Action: EventRejected})
	return events, nil
}

// forceReopen puts the checklist back into StatusOpen if it wasn't already
// there, returning a "reopened" event describing the transition. Used by the
// creator-override methods below: any structural or check-state edit they
// make invalidates whatever "all items checked" state got the checklist into
// validating/complete, so they unconditionally return it to open rather than
// trying to re-derive whether it should still be considered done.
func (c *Checklist) forceReopen(actingUserID int64) []Event {
	if c.Status == StatusOpen {
		return nil
	}
	prev := c.Status
	c.Status = StatusOpen
	return []Event{{
		TenantID: c.TenantID, ChecklistID: c.ID,
		ActorUserID: actingUserID,
		Action:      EventReopened,
		Detail:      map[string]any{"previous_status": string(prev)},
	}}
}

// AddItem lets the checklist's creator append a new, unchecked item — at any
// status, not just while open. This is a creator-only override on top of the
// normal assignee/approver-gated flows above, so it always forces the
// checklist back to open (see forceReopen) rather than leaving it in a
// validating/complete state that no longer reflects "all items checked".
func (c *Checklist) AddItem(actingUserID int64, name, validationRef string) ([]Event, error) {
	if actingUserID != c.CreatorID {
		return nil, ErrNotCreator
	}

	c.Items = append(c.Items, ChecklistItem{
		ChecklistID:   c.ID,
		Name:          name,
		Position:      len(c.Items),
		ValidationRef: validationRef,
	})

	events := []Event{{TenantID: c.TenantID, ChecklistID: c.ID, ActorUserID: actingUserID, Action: EventItemAdded, Detail: map[string]any{"name": name}}}
	return append(events, c.forceReopen(actingUserID)...), nil
}

// RemoveItem lets the checklist's creator delete the item at itemIndex,
// regardless of whether it's been checked — its history remains in the event
// log. Remaining items are renumbered to keep Position contiguous. Forces the
// checklist back to open (see forceReopen).
func (c *Checklist) RemoveItem(actingUserID int64, itemIndex int) ([]Event, error) {
	if actingUserID != c.CreatorID {
		return nil, ErrNotCreator
	}

	removed := c.Items[itemIndex]
	c.Items = append(c.Items[:itemIndex], c.Items[itemIndex+1:]...)
	for i := range c.Items {
		c.Items[i].Position = i
	}

	events := []Event{{TenantID: c.TenantID, ChecklistID: c.ID, ItemID: &removed.ID, ActorUserID: actingUserID, Action: EventItemRemoved}}
	return append(events, c.forceReopen(actingUserID)...), nil
}

// ReorderItems lets the checklist's creator rearrange items into newOrder,
// given as the desired sequence of existing item IDs. Forces the checklist
// back to open (see forceReopen) for consistency with the other
// creator-override methods, even though reordering alone doesn't change
// which items are checked.
func (c *Checklist) ReorderItems(actingUserID int64, newOrder []int64) ([]Event, error) {
	if actingUserID != c.CreatorID {
		return nil, ErrNotCreator
	}
	if len(newOrder) != len(c.Items) {
		return nil, ErrInvalidReorder
	}

	byID := make(map[int64]ChecklistItem, len(c.Items))
	for _, item := range c.Items {
		byID[item.ID] = item
	}

	reordered := make([]ChecklistItem, len(newOrder))
	for i, id := range newOrder {
		item, ok := byID[id]
		if !ok {
			return nil, ErrInvalidReorder
		}
		item.Position = i
		reordered[i] = item
	}
	c.Items = reordered

	events := []Event{{TenantID: c.TenantID, ChecklistID: c.ID, ActorUserID: actingUserID, Action: EventItemsReordered}}
	return append(events, c.forceReopen(actingUserID)...), nil
}

// SetItemChecked lets the checklist's creator directly set the checked state
// of the item at itemIndex, bypassing the ResponsibleUserFor/status gating
// that CheckItem enforces. Unlike CheckItem, it never evaluates
// allItemsChecked() to advance the workflow to validating/complete — it's a
// manual fix-up tool, not a way to drive the normal completion flow. Forces
// the checklist back to open (see forceReopen).
func (c *Checklist) SetItemChecked(actingUserID int64, itemIndex int, checked bool, now time.Time) ([]Event, error) {
	if actingUserID != c.CreatorID {
		return nil, ErrNotCreator
	}

	item := &c.Items[itemIndex]
	action := EventItemUnchecked
	if checked {
		action = EventItemChecked
		item.Checked = true
		item.CheckedBy = &actingUserID
		item.CheckedAt = &now
	} else {
		item.Checked = false
		item.CheckedBy = nil
		item.CheckedAt = nil
	}

	events := []Event{{TenantID: c.TenantID, ChecklistID: c.ID, ItemID: &item.ID, ActorUserID: actingUserID, Action: action}}
	return append(events, c.forceReopen(actingUserID)...), nil
}

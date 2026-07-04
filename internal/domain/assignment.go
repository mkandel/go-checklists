package domain

import "errors"

var (
	// ErrAssignmentRequired means neither a group nor a user assignee was set.
	ErrAssignmentRequired = errors.New("domain: checklist must be assigned to a group and/or a user")
	// ErrAssigneeNotGroupMember means both a group and a user were set, but
	// the user does not belong to the group.
	ErrAssigneeNotGroupMember = errors.New("domain: assigned user is not a member of the assigned group")
)

// ValidateAssignment checks the group/user assignment invariant for a
// checklist: at least one of groupID/userID must be set, and if both are
// set, isMember must confirm userID belongs to groupID. The caller looks up
// isMember (e.g. via GroupRepo.IsMember) since domain has no DB access.
func ValidateAssignment(groupID, userID *int64, isMember bool) error {
	if groupID == nil && userID == nil {
		return ErrAssignmentRequired
	}
	if groupID != nil && userID != nil && !isMember {
		return ErrAssigneeNotGroupMember
	}
	return nil
}

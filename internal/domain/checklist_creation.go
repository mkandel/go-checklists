package domain

import "errors"

// ErrChecklistCreationRestricted means tenant restricts checklist creation and
// actor is neither an admin nor a member of the designated creator group.
var ErrChecklistCreationRestricted = errors.New("domain: checklist creation is restricted to admins and members of the designated creator group")

// CanCreateChecklist reports whether actor may create a checklist for tenant.
// isCreatorGroupMember must reflect whether actor already belongs to
// tenant.CreatorGroupID (the caller looks this up, e.g. via
// GroupRepo.IsMember, since domain has no DB access).
func CanCreateChecklist(tenant *Tenant, actor *User, isCreatorGroupMember bool) error {
	if !tenant.RestrictChecklistCreation || actor.IsAdmin {
		return nil
	}
	if tenant.CreatorGroupID != nil && isCreatorGroupMember {
		return nil
	}
	return ErrChecklistCreationRestricted
}

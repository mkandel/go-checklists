package domain

import (
	"context"
	"time"
)

// Event mirrors a checklist_events row. Domain methods that mutate a
// Checklist return the Events they caused so the store layer can append them
// to the audit log in the same transaction as the state change, without the
// domain package needing to know anything about SQL.
type Event struct {
	ChecklistID int64
	ItemID      *int64
	ActorUserID int64
	Action      string
	Detail      map[string]any
}

const (
	EventCreated                = "created"
	EventAssigneeChanged        = "assignee_changed"
	EventApproverChanged        = "approver_changed"
	EventItemChecked            = "item_checked"
	EventItemUnchecked          = "item_unchecked"
	EventSubmittedForValidation = "submitted_for_validation"
	EventRejected               = "rejected"
	EventApproved               = "approved"
	EventCompleted              = "completed"
	EventClaimed                = "claimed"
	EventClaimLost              = "claim_lost"
	EventItemAdded              = "item_added"
	EventItemRemoved            = "item_removed"
	EventItemsReordered         = "items_reordered"
	EventReopened               = "reopened"
)

// UserRepo persists and fetches Users.
type UserRepo interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id int64) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	List(ctx context.Context) ([]User, error)
}

// Session mirrors a sessions row: a server-side session identified by an
// opaque, random token (used directly as its primary key), tied to one user,
// with a fixed expiry (no sliding renewal).
type Session struct {
	Token     string
	UserID    int64
	CreatedAt time.Time
	ExpiresAt time.Time
}

// SessionRepo persists and fetches Sessions.
type SessionRepo interface {
	Create(ctx context.Context, s *Session) error
	Get(ctx context.Context, token string) (*Session, error)
	Delete(ctx context.Context, token string) error

	// Refresh extends token's expiry to newExpiresAt (sliding-renewal support).
	Refresh(ctx context.Context, token string, newExpiresAt time.Time) error

	// DeleteExpired removes every session whose expiry is before now, and
	// returns how many rows were deleted.
	DeleteExpired(ctx context.Context, now time.Time) (int64, error)
}

// GroupRepo persists Groups and their membership.
type GroupRepo interface {
	Create(ctx context.Context, g *Group) error
	AddMember(ctx context.Context, groupID, userID int64) error
	RemoveMember(ctx context.Context, groupID, userID int64) error
	IsMember(ctx context.Context, groupID, userID int64) (bool, error)
	ListMembers(ctx context.Context, groupID int64) ([]User, error)
	List(ctx context.Context) ([]Group, error)
}

// TemplateRepo persists immutable, versioned Templates.
type TemplateRepo interface {
	// CreateVersion inserts a new template version along with its items.
	CreateVersion(ctx context.Context, t *Template, items []TemplateItem) error
	GetLatestByName(ctx context.Context, name string) (*Template, []TemplateItem, error)
	Get(ctx context.Context, id int64) (*Template, []TemplateItem, error)
	List(ctx context.Context) ([]Template, error)
}

// EventRepo appends to the append-only checklist_events audit log.
type EventRepo interface {
	Append(ctx context.Context, events []Event) error
}

// Notification mirrors a notifications row.
type Notification struct {
	ID              int64
	RecipientUserID int64
	Type            string
	ChecklistID     *int64
	ActorUserID     *int64
	Message         string
	ReadAt          *time.Time
}

// NotificationRepo persists and fetches Notifications.
type NotificationRepo interface {
	Create(ctx context.Context, n *Notification) error
	ListForUser(ctx context.Context, userID int64) ([]Notification, error)
	// MarkRead marks notification id read, scoped to userID (its recipient).
	// Returns an error if id doesn't exist or belongs to someone else.
	MarkRead(ctx context.Context, id, userID int64) error
}

// ChecklistFilter narrows ChecklistRepo.List. UserID selects checklists
// relevant to that user (creator, approver, direct assignee, or a member of
// the assigned group while it's unclaimed — mirroring Checklist.VisibleTo).
// A nil Status matches every status.
type ChecklistFilter struct {
	UserID int64
	Status *ChecklistStatus
}

// ChecklistRepo persists Checklists and their items.
type ChecklistRepo interface {
	// Create inserts a new checklist. If c.TemplateID is set, items are
	// copied from that template's current items; otherwise c.Items is used
	// as-is (ad-hoc checklist).
	Create(ctx context.Context, c *Checklist) error
	Get(ctx context.Context, id int64) (*Checklist, error)
	// Claim assigns the checklist to actingUserID, provided the current
	// assigned_user_id matches expectedCurrent (nil means "currently
	// unclaimed"). Returns false if the CAS lost the race.
	Claim(ctx context.Context, checklistID, actingUserID int64, expectedCurrent *int64) (bool, error)
	// Save persists the checklist's current items/status and appends events,
	// all in one transaction.
	Save(ctx context.Context, c *Checklist, events []Event) error
	// List returns checklists matching filter, without their Items (a
	// lighter summary row — full items come from Get).
	List(ctx context.Context, filter ChecklistFilter) ([]Checklist, error)
}

package domain

import (
	"context"
	"errors"
	"time"
)

// ErrUsernameTaken is returned by UserRepo.Create when username is already
// in use within the same tenant (usernames are unique per-tenant, not
// globally — see DESIGN.md's Multi-tenancy section).
var ErrUsernameTaken = errors.New("domain: username is already taken")

// Event mirrors a checklist_events row. Domain methods that mutate a
// Checklist return the Events they caused so the store layer can append them
// to the audit log in the same transaction as the state change, without the
// domain package needing to know anything about SQL. TenantID mirrors the
// owning Checklist's TenantID (denormalized onto checklist_events so future
// tenant-wide activity queries don't need a join back through checklists).
type Event struct {
	TenantID    int64
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

// UserRepo persists and fetches Users. GetByID is deliberately NOT
// tenant-scoped: it's only ever called internally by auth.CurrentUser to
// resolve an already-trusted session (the session token itself is the proof
// of identity, and the tenant isn't known until the user is resolved).
// GetByUsername and List take request-influenced/enumeration paths and are
// tenant-scoped.
type UserRepo interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id int64) (*User, error)
	GetByUsername(ctx context.Context, tenantID int64, username string) (*User, error)
	List(ctx context.Context, tenantID int64) ([]User, error)
	// UpdatePasswordHash replaces userID's stored password hash — used by the
	// password-reset confirm flow.
	UpdatePasswordHash(ctx context.Context, userID int64, hash string) error
	// SetActive suspends (active=false) or reactivates (active=true) userID,
	// scoped to tenantID. There's no hard user delete — suspension is the
	// only removal path, keeping every historical checklist/audit reference
	// to the user intact.
	SetActive(ctx context.Context, tenantID, userID int64, active bool) error
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

	// DeleteByUserID removes every session belonging to userID — used to
	// invalidate a user's other sessions after a password reset.
	DeleteByUserID(ctx context.Context, userID int64) error
}

// PasswordResetTokenRepo persists and fetches PasswordResetTokens. Unlike
// SessionRepo there's no Refresh: a reset token is single-use and short-lived
// (see auth.PasswordResetTokenTTL), not a sliding-renewal credential.
type PasswordResetTokenRepo interface {
	Create(ctx context.Context, t *PasswordResetToken) error
	Get(ctx context.Context, token string) (*PasswordResetToken, error)
	Delete(ctx context.Context, token string) error

	// DeleteExpired removes every token whose expiry is before now, and
	// returns how many rows were deleted.
	DeleteExpired(ctx context.Context, now time.Time) (int64, error)
}

// TenantMailConfig is the full set of per-tenant SMTP settings, as accepted
// by TenantRepo.UpdateMailConfig. Every field is full-replace/required
// except Password: an empty Password means "keep the tenant's existing
// password", so a client never needs to round-trip the real secret back
// just to resubmit the rest of the config unchanged.
type TenantMailConfig struct {
	Host        string
	Port        int
	Username    string
	Password    string
	FromAddress string
}

// TenantRepo persists and fetches Tenants.
type TenantRepo interface {
	Create(ctx context.Context, t *Tenant) error
	GetByID(ctx context.Context, id int64) (*Tenant, error)
	// GetSoleTenant returns the one existing Tenant, and errors if there
	// isn't exactly one. It's a deliberately temporary stand-in for real
	// per-request tenant resolution (subdomain/host/API key), used by
	// single-tenant/on-prem deployments and by handleLogin (which must
	// resolve a tenant before it knows who the user is). Erroring on
	// count != 1 means the day a second tenant is provisioned, any code
	// still depending on this fails loudly instead of silently misfiling
	// data into tenant #1.
	GetSoleTenant(ctx context.Context) (*Tenant, error)
	// UpdateMailConfig replaces tenantID's SMTP config. An empty
	// cfg.Password means "keep the existing password" — see
	// TenantMailConfig.
	UpdateMailConfig(ctx context.Context, tenantID int64, cfg TenantMailConfig) error
	// UpdateChecklistCreationPolicy replaces tenantID's checklist-creation
	// restriction settings — see ChecklistCreationPolicy.
	UpdateChecklistCreationPolicy(ctx context.Context, tenantID int64, policy ChecklistCreationPolicy) error
}

// ChecklistCreationPolicy is the checklist-creation restriction settings for
// a tenant, as accepted by TenantRepo.UpdateChecklistCreationPolicy.
type ChecklistCreationPolicy struct {
	Restrict       bool
	CreatorGroupID *int64
}

// GroupRepo persists Groups and their membership. AddMember, RemoveMember,
// IsMember, and ListMembers take tenantID because groupID/userID are often
// request-influenced (e.g. a client-supplied assignment) — filtering on
// tenantID here prevents cross-tenant membership checks/mutations even if a
// caller passes a foreign tenant's ID.
type GroupRepo interface {
	Create(ctx context.Context, g *Group) error
	AddMember(ctx context.Context, tenantID, groupID, userID int64) error
	RemoveMember(ctx context.Context, tenantID, groupID, userID int64) error
	IsMember(ctx context.Context, tenantID, groupID, userID int64) (bool, error)
	ListMembers(ctx context.Context, tenantID, groupID int64) ([]User, error)
	List(ctx context.Context, tenantID int64) ([]Group, error)
}

// TemplateRepo persists immutable, versioned Templates.
type TemplateRepo interface {
	// CreateVersion inserts a new template version along with its items.
	CreateVersion(ctx context.Context, t *Template, items []TemplateItem) error
	GetLatestByName(ctx context.Context, tenantID int64, name string) (*Template, []TemplateItem, error)
	Get(ctx context.Context, tenantID, id int64) (*Template, []TemplateItem, error)
	List(ctx context.Context, tenantID int64) ([]Template, error)
}

// EventRepo appends to the append-only checklist_events audit log.
type EventRepo interface {
	Append(ctx context.Context, events []Event) error
}

// Notification mirrors a notifications row.
type Notification struct {
	ID              int64
	TenantID        int64
	RecipientUserID int64
	Type            string
	ChecklistID     *int64
	ActorUserID     *int64
	Message         string
	ReadAt          *time.Time

	// Email delivery outbox fields. EmailStatus starts at
	// EmailStatusPending and moves to Sent, Failed (gave up after too many
	// attempts), or Skipped (a permanent-under-current-state condition —
	// no SMTP config / no recipient email / recipient deactivated — as
	// opposed to a transient failure).
	EmailStatus    string
	EmailAttempts  int
	EmailLastError *string
	EmailSentAt    *time.Time
}

const (
	EmailStatusPending = "pending"
	EmailStatusSent    = "sent"
	EmailStatusFailed  = "failed"
	EmailStatusSkipped = "skipped"
)

// NotificationRepo persists and fetches Notifications.
type NotificationRepo interface {
	Create(ctx context.Context, n *Notification) error
	ListForUser(ctx context.Context, tenantID, userID int64) ([]Notification, error)
	// MarkRead marks notification id read, scoped to tenantID and userID
	// (its recipient). Returns an error if id doesn't exist or belongs to
	// someone else / another tenant.
	MarkRead(ctx context.Context, tenantID, id, userID int64) error

	// ListPendingEmail returns up to limit notifications with
	// EmailStatus == EmailStatusPending, oldest first. It is deliberately
	// NOT tenant-scoped: it backs a system-wide delivery sweep run by the
	// background email worker, not a per-request read — the same rationale
	// as UserRepo.GetByID being untenanted.
	ListPendingEmail(ctx context.Context, limit int) ([]Notification, error)
	// MarkEmailSent records a successful delivery.
	MarkEmailSent(ctx context.Context, id int64, sentAt time.Time) error
	// MarkEmailFailed records a delivery attempt failure: increments
	// EmailAttempts and sets EmailLastError, moving EmailStatus to
	// EmailStatusFailed once EmailAttempts reaches maxAttempts, otherwise
	// leaving it at EmailStatusPending for a retry on the next tick.
	MarkEmailFailed(ctx context.Context, id int64, errMsg string, maxAttempts int) error
	// MarkEmailSkipped marks a notification as never going to succeed under
	// current state (no SMTP config, no recipient email, recipient
	// deactivated) — distinct from MarkEmailFailed's transient-failure/retry
	// semantics.
	MarkEmailSkipped(ctx context.Context, id int64) error
}

// ChecklistFilter narrows ChecklistRepo.List. TenantID scopes the query to
// one tenant; UserID selects checklists relevant to that user (creator,
// approver, direct assignee, or a member of the assigned group while it's
// unclaimed — mirroring Checklist.VisibleTo). A nil Status matches every
// status.
type ChecklistFilter struct {
	TenantID int64
	UserID   int64
	Status   *ChecklistStatus
}

// ChecklistRepo persists Checklists and their items. Get and Claim take
// tenantID because id is request-supplied (a URL path parameter) — without
// scoping, Checklist.VisibleTo's "non-hidden checklists are visible to
// everyone" rule would leak cross-tenant data, since VisibleTo has no
// tenant concept of its own and assumes "everyone" means "everyone in this
// one shared install."
type ChecklistRepo interface {
	// Create inserts a new checklist, copying items from c.TemplateID's
	// current items.
	Create(ctx context.Context, c *Checklist) error
	Get(ctx context.Context, tenantID, id int64) (*Checklist, error)
	// Claim assigns the checklist to actingUserID, provided the current
	// assigned_user_id matches expectedCurrent (nil means "currently
	// unclaimed"). Returns false if the CAS lost the race.
	Claim(ctx context.Context, tenantID, checklistID, actingUserID int64, expectedCurrent *int64) (bool, error)
	// Save persists the checklist's current items/status and appends events,
	// all in one transaction.
	Save(ctx context.Context, c *Checklist, events []Event) error
	// List returns checklists matching filter, without their Items (a
	// lighter summary row — full items come from Get).
	List(ctx context.Context, filter ChecklistFilter) ([]Checklist, error)
	// ClearUserAssignments clears userID from approver_id and
	// assigned_user_id on every checklist in tenantID where it's currently
	// set, appending an event for each change. Used when a user is
	// deactivated: they can no longer act (auth.CurrentUser already blocks
	// that), but leaving stale pointers to them makes checklists look
	// actionable when they aren't, so this clears them instead of leaving
	// the UI to detect and flag it.
	ClearUserAssignments(ctx context.Context, tenantID, userID int64) error
}

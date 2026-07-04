package domain

import "time"

// ChecklistStatus is the lifecycle state of a Checklist.
type ChecklistStatus string

const (
	StatusOpen       ChecklistStatus = "open"
	StatusValidating ChecklistStatus = "validating"
	StatusComplete   ChecklistStatus = "complete"
)

// Tenant mirrors a tenants row. On-prem/standalone deployments run with
// exactly one Tenant, auto-provisioned at startup (see
// cmd/checklists-server/main.go); a future SaaS deployment would have many,
// resolved per-request (not yet implemented — see TenantRepo.GetSoleTenant).
type Tenant struct {
	ID   int64
	Name string
	Slug string

	// SMTP config for this tenant's outbound email notifications. Email
	// delivery is enabled for the tenant iff SMTPHost is non-nil. All
	// nullable since a tenant may never configure email at all.
	SMTPHost        *string
	SMTPPort        *int
	SMTPUsername    *string
	SMTPPassword    string `json:"-"`
	SMTPFromAddress *string
}

type User struct {
	ID           int64
	TenantID     int64
	Name         string
	Username     string
	PasswordHash string `json:"-"`
	Email        *string
	IsAdmin      bool
	IsActive     bool
}

type Group struct {
	ID       int64
	TenantID int64
	Name     string
}

type Template struct {
	ID       int64
	TenantID int64
	Name     string
	Version  int
}

type TemplateItem struct {
	ID            int64
	TemplateID    int64
	Name          string
	Position      int
	ValidationRef string
}

type ChecklistItem struct {
	ID          int64
	ChecklistID int64
	Name        string
	Position    int
	Checked     bool
	CheckedBy   *int64
	CheckedAt   *time.Time

	ValidationRef string

	// AssigneeOverrideUserID is set only via the reject flow: when an approver
	// unchecks this item, it points at whoever originally checked it. Nil means
	// responsibility defers to the checklist's normal assignee.
	AssigneeOverrideUserID *int64
}

type Checklist struct {
	ID              int64
	TenantID        int64
	TemplateID      *int64
	CreatorID       int64
	AssignedGroupID *int64
	AssignedUserID  *int64
	Hidden          bool
	ApproverID      *int64
	Status          ChecklistStatus
	CreatedAt       time.Time
	Items           []ChecklistItem
}

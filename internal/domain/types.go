package domain

import "time"

// ChecklistStatus is the lifecycle state of a Checklist.
type ChecklistStatus string

const (
	StatusOpen       ChecklistStatus = "open"
	StatusValidating ChecklistStatus = "validating"
	StatusComplete   ChecklistStatus = "complete"
)

type User struct {
	ID           int64
	Name         string
	Username     string
	PasswordHash string `json:"-"`
	IsAdmin      bool
	IsActive     bool
}

type Group struct {
	ID   int64
	Name string
}

type Template struct {
	ID      int64
	Name    string
	Version int
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

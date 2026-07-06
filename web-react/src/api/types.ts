// Wire types mirror internal/domain/types.go exactly, including its casing.
// Most domain structs have no json tags and serialize PascalCase; only
// request bodies and the two wrapper responses noted below use lowerCamel.

export type ChecklistStatus = 'open' | 'validating' | 'complete'

export interface User {
  ID: number
  TenantID: number
  Name: string
  Username: string
  Email: string | null
  IsAdmin: boolean
  IsActive: boolean
}

// GET /api/me wraps domain.User with an extra lowerCamel field.
export interface Me extends User {
  notificationsEnabled: boolean
}

export interface ChecklistItem {
  ID: number
  ChecklistID: number
  Name: string
  Position: number
  Checked: boolean
  CheckedBy: number | null
  CheckedAt: string | null
  ValidationRef: string
  AssigneeOverrideUserID: number | null
}

export interface Checklist {
  ID: number
  TenantID: number
  TemplateID: number
  Name: string
  CreatorID: number
  AssignedGroupID: number | null
  AssignedUserID: number | null
  Hidden: boolean
  ApproverID: number | null
  Status: ChecklistStatus
  CreatedAt: string
  Items: ChecklistItem[]
}

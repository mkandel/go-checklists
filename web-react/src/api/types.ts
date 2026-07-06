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

export interface CreateChecklistRequest {
  template_id: number
  name?: string
  assigned_group_id?: number | null
  assigned_user_id?: number | null
  hidden: boolean
  approver_id?: number | null
}

export interface Group {
  ID: number
  TenantID: number
  Name: string
}

export interface Template {
  ID: number
  TenantID: number
  Name: string
  Version: number
}

export interface TemplateItem {
  ID: number
  TemplateID: number
  Name: string
  Position: number
  ValidationRef: string
}

// GET/POST template-detail responses embed domain.Template (its fields
// promoted flat into the JSON object, not nested), plus a lowerCamel "items".
export interface TemplateDetail extends Template {
  items: TemplateItem[]
}

export interface CreateTemplateItem {
  name: string
  validation_ref?: string
}

export interface CreateTemplateRequest {
  name: string
  items: CreateTemplateItem[]
}

export interface Notification {
  ID: number
  TenantID: number
  RecipientUserID: number
  Type: string
  ChecklistID: number | null
  ActorUserID: number | null
  Message: string
  ReadAt: string | null
}

export interface BulkCreateUserResult {
  row: number
  username?: string
  status: string
  error?: string
  user?: User
}

export interface TenantMailConfig {
  host: string
  port: number
  username: string
  from_address: string
  configured: boolean
}

export interface ChecklistPolicy {
  restrict: boolean
  creator_group_id: number | null
}

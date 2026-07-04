# Checklists — Design

A web-based, database-driven, shareable checklist system, written in Go. This is a
rewrite of a stalled 2011-2012 Perl project (`ChecklistsPerl`) — the original never
got past module stubs, so this document reflects fresh design decisions made for the
Go version, informed by (but not bound to) the original's intent.

## Goals

- Create reusable **templates**, and use them to instantiate discrete **checklists**.
- Checklists can be shared, assigned to a person or a team, and reassigned.
- Optional approval step: a checklist can require a designated approver to sign off
  after all items are checked, before it's considered complete.
- Ad-hoc checklists (no template) are supported.
- Full audit trail: who did what, and when.

## Data model

### `users`
`id, name, username, is_admin, is_active`

Deactivation is soft-delete (`is_active = false`). Deactivated users can't receive new
assignments; existing assignments and all historical events referencing them are left
intact. If a deactivated user is currently assigned to something, there's no automatic
reassignment — the UI displays a "user inactive" indicator, and it's on the creator to
notice and reassign manually.

### `groups`
`id, name`

### `user_groups`
`user_id, group_id` — many-to-many. First-class: N users per group, N groups per user.

### `templates`
`id, name, version`

Templates are versioned **immutably** — editing a template creates a new version row
rather than mutating in place. This keeps "what did this checklist's template actually
say at the time" answerable, and means existing checklists are never retroactively
affected by later template edits (this also falls out naturally from the fact that
checklist items are copied into their own rows at instantiation, not referenced live).

### `template_items`
`id, template_id, name, validation_ref`

`validation_ref` names a validation routine to run against the item (see
[Open items](#open-items-deferred) — the exact Go-side dispatch mechanism for this is
not yet designed).

### `checklists`
`id, template_id (nullable), creator_id, assigned_group_id (nullable),
assigned_user_id (nullable), hidden, approver_id (nullable), status, created_at`

- `template_id` is nullable — **ad-hoc checklists** (no template) are fully supported.
  "Create new from this checklist" is a universal clone operation: works on any
  checklist regardless of status or template origin, copies just the item list into a
  brand new checklist (fresh `creator_id` = whoever cloned it, no assignee/approver/
  checked-state carried over).
- `creator_id` is fixed at creation and never changes — it's provenance, not a
  transferable "owner" role. The original design's separate creator/owner split is
  replaced with a single fixed creator plus a mutable assignee.
- **Assignment** (`assigned_group_id` / `assigned_user_id`): at least one must be set;
  any combination is valid:
  - **group only** — assigned to a team, unclaimed. No item edits are allowed until a
    specific person claims it.
  - **user only** — direct individual assignment, no team context.
  - **both** — assigned to a team, claimed by a specific member. This is the only
    state in which item edits happen. When both are set, `assigned_user_id` **must**
    be a member of `assigned_group_id` — enforced in the domain layer *and* via a DB
    trigger as a backstop against writes that bypass application logic.
  - **Claiming**: any member of `assigned_group_id`, or the creator, can set or change
    `assigned_user_id` (self-claim or hand off to a teammate). Claim attempts are
    race-safe via a conditional update (`UPDATE ... WHERE assigned_user_id IS NULL`,
    or matching the expected current value for a reassignment) — the losing claimant
    is notified in-UI ("X assigned it to Y"), written in the same transaction as the
    winning claim.
- **Approver** (`approver_id`): can be set or changed by the creator or the current
  approver, any time before `status = complete`. Can be *any* user in the system —
  not constrained by membership in the assignee's group. No restriction on
  self-approval (e.g. approver == assignee, or approver == whoever checked the last
  item) — that's a legitimate process choice left to the creator.
- **`hidden`**: when true, the checklist is visible only to the creator, the
  assignee (the claimed user, or all members of the assigned group if unclaimed), and
  the approver. Everyone else can't see it at all. (Non-hidden checklists are visible
  to everyone, read-only for non-assignees.)
- Item list is **fixed at creation** — items can't be added or removed afterward, for
  either template-based or ad-hoc checklists. This is what makes "all items checked"
  a well-defined trigger for the status transition.

### `checklist_items`
`id, checklist_id, name, checked, checked_by, checked_at, validation_ref,
assignee_override_user_id (nullable)`

`assignee_override_user_id` is used only by the reject flow (see below) — when null,
responsibility for the item defers to the checklist's normal assignee.

### `checklist_events`
`id, checklist_id, item_id (nullable), actor_user_id, action, detail, created_at`

Append-only audit log — the single source of truth for history. Current-state fields
elsewhere (status, checked, assignee, etc.) are a fast-path cache of the latest event,
not an independent source of truth. Example `action` values: `created`,
`assignee_changed`, `approver_changed`, `item_checked`, `item_unchecked`,
`submitted_for_validation`, `rejected`, `approved`, `completed`, `claimed`,
`claim_lost`.

### `notifications`
`id, recipient_user_id, type, checklist_id, actor_user_id, message,
read_at (nullable), created_at`

Channel-agnostic at the data layer — a row just records "this happened, this person
should know." Delivery is a separate concern layered on top. In-UI only for now (no
email), but the schema doesn't preclude adding email or other channels later.
Delivered via **SSE** (server-sent events) rather than polling, for immediacy without
websocket complexity.

## Lifecycle / state machine

```
        all items checked, no approver set
  open ─────────────────────────────────────► complete
   │
   │ all items checked, approver is set
   ▼
validating ──────approver approves──────────► complete (terminal)
   │
   │ approver rejects (unchecks specific items)
   ▼
  open   (implicitly — no longer all-checked; rejected items get
          assignee_override_user_id = whoever originally checked them)
```

- While `status = validating`, **only the approver** can mutate item-checked state —
  the assignee is locked out of item edits until the checklist is back in `open`.
  This is what makes "reject" a real, singular action (uncheck specific items) rather
  than something that could race against ordinary assignee edits.
- `complete` is **terminal** — no resurrection. The only way forward from a completed
  checklist is cloning it into a brand new one.

## Concurrency

Given the assignment model, only one person can ever be the legitimate editor of a
checklist's items at a time (the claimed `assigned_user_id`), which removes most of
the classic "two people editing at once" race. What's left:

- **Claiming**: a conditional update (compare-and-swap on `assigned_user_id`)
  prevents two simultaneous claims from both succeeding.
- **Status transitions** (`open→validating`, `validating→complete`,
  `validating→open`): guarded by a row lock (`SELECT ... FOR UPDATE`) on the
  checklist at the transition point, inside the same transaction as the item update
  and event-log write.

## Architecture

```
cmd/checklists-server/     main.go — wiring, config, http server startup
internal/
  domain/                  pure business logic, no DB/HTTP imports
    checklist.go            state machine, reject flow, per-item reassignment
    assignment.go           claim/reassign logic, group-membership checks
    template.go              template → checklist instantiation
  store/
    postgres/                repositories: UserStore, ChecklistStore, TemplateStore,
                              EventStore, NotificationStore (pgx)
    migrations/              SQL migrations
  api/
    handlers.go              HTTP endpoints
    middleware.go            auth/session, logging
  auth/                     session-based login
web/                        Go templates + static assets
```

- `domain` depends on interfaces (`ChecklistRepo`, `EventRecorder`, etc.), not
  concrete Postgres code — testable without a real DB.
- Every state transition and the claim operation write their event-log and/or
  notification rows in the **same transaction** as the state change itself, so the
  audit trail can never drift from actual state.

### Storage & auth
- **Postgres** (not SQLite/MySQL) — chosen for proper concurrent multi-user access
  (row locking, enum-like status types), given the inherently multi-user nature of
  assignment/sharing.
- Simple username/password + server-side sessions (no OAuth/SSO planned yet).

### Frontend
**Go templates + htmx + Alpine.js + SortableJS**, plain CSS/Tailwind for styling —
deliberately not a separate SPA (React/Vue). Rationale: this app is fundamentally
forms and lists (checklists, items, assignment, approval), not a highly animated
client app, and the author's background/preference favors a hypermedia-driven stack
over adopting a client-side framework's whole worldview.

- **htmx**: partial-page swaps for everything server-driven (check an item, claim an
  assignment, submit approval) plus the SSE extension for live notifications.
- **Alpine.js**: small local UI state only (collapsing the notes field, dropdowns,
  confirm dialogs) — not a client-side state management layer.
- **SortableJS**: drag-and-drop reordering, drag-and-drop checklist-from-template
  creation, and the visual template builder (arranging/reordering item blocks while
  authoring a template) — all the same underlying pattern, wired to htmx requests on
  drop.
- **Notes field**: plain `<textarea>`, no rich-text editor. Bare URLs are
  auto-linkified at render time (e.g. via `goldmark` in autolink mode) rather than
  supporting authored `[text](url)` links or a WYSIWYG toolbar — deliberately the
  simplest option. **Flagged for review during testing** — there's a real chance this
  won't feel right in practice and gets swapped for Markdown-lite link syntax
  instead. (Tracked as an open task, not just this note.)

## Open items (deferred)

- **Validation dispatch mechanism**: the original Perl design used dynamic dispatch
  (`Some::Validation->routine_name`) for per-item validation routines. Go doesn't
  support that kind of dispatch as naturally — a named-function registry is the
  likely replacement, but this hasn't been designed yet.
- **"Save ad-hoc checklist as a new template"**: discussed as a natural extension of
  the clone operation (thin wrapper reusing the same item-list-copy logic), not yet
  committed to as a v1 feature.
- Email notifications: schema supports adding a channel later; not being built now.

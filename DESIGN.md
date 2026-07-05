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
- Full audit trail: who did what, and when.

## Data model

### `tenants`
`id, name, slug, smtp_host (nullable), smtp_port (nullable), smtp_username
(nullable), smtp_password, smtp_from_address (nullable),
restrict_checklist_creation, checklist_creator_group_id (nullable)`

See [Multi-tenancy](#multi-tenancy) for how this table is used, and
[Notifications](#notifications) for the `smtp_*` columns.

`restrict_checklist_creation` (default `false`) and `checklist_creator_group_id`
gate an opt-in policy: when enabled, only admins and members of the designated
group may create a checklist (`domain.CanCreateChecklist`, enforced
identically by both `internal/api` and `internal/web`). Off by default —
every active user can create checklists until a tenant admin turns this on via
`PUT /api/admin/tenant/checklist-policy` or the `/admin/checklist-policy` UI
page. This reuses the existing group concept rather than adding a new
role/permission tier.

### `users`
`id, tenant_id, name, username, password_hash, is_admin, is_active`

Deactivation is soft-delete (`is_active = false`) — there is no hard user delete, since
checklist/audit history references users indefinitely. A tenant admin toggles this via
`POST /admin/users/{id}/active` (suspend/reactivate buttons on the admin users page,
`UserRepo.SetActive`, tenant-scoped); an admin can't suspend their own account. Deactivated
users can't receive new assignments; existing assignments and all historical events
referencing them are left intact. If a deactivated user is currently assigned to
something, there's no automatic reassignment — the UI displays a "user inactive"
indicator, and it's on the creator to notice and reassign manually. A deactivated user
also can't log in — `Login` checks `is_active` after verifying the password — and
suspension takes effect immediately for an already-logged-in user too: `auth.CurrentUser`
re-checks `is_active` on every request, not just at login, so an existing session is
revoked right away rather than remaining valid until it expires.

### `sessions`
`token (primary key), user_id, created_at, expires_at`

A server-side session, keyed directly by its own opaque token (32 random bytes,
base64 URL-encoded) rather than a separate surrogate id. Fixed 7-day expiry, no
sliding renewal — logging in always creates a new session rather than extending an
existing one. See [Storage & auth](#storage--auth) for the full login/logout mechanics.

### `groups`
`id, tenant_id, name`

### `user_groups`
`user_id, group_id` — many-to-many. First-class: N users per group, N groups per user.
No `tenant_id` of its own — both `user_id` and `group_id` already belong to the same
tenant (enforced by the composite FKs described in [Multi-tenancy](#multi-tenancy)),
so scope is inherited transitively.

### `templates`
`id, tenant_id, name, version`

Templates are versioned **immutably** — editing a template creates a new version row
rather than mutating in place. This keeps "what did this checklist's template actually
say at the time" answerable, and means existing checklists are never retroactively
affected by later template edits (this also falls out naturally from the fact that
checklist items are copied into their own rows at instantiation, not referenced live).

### `template_items`
`id, template_id, name, validation_ref`

`validation_ref` names a validation routine to run against the item. Stored today;
there's no dispatch mechanism wired up yet to actually run it.

### `checklists`
`id, tenant_id, template_id, creator_id, assigned_group_id (nullable),
assigned_user_id (nullable), hidden, approver_id (nullable), status, created_at`

- `template_id` is required — every checklist is instantiated from a template, which
  supplies its initial item list.
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
- **Item list edits**: normally the item list is fixed at creation — this is what
  makes "all items checked" a well-defined trigger for the status transition. The
  **creator**, however, can
  always add, remove, or reorder items, and can directly check/uncheck any item,
  regardless of the checklist's current status (including `complete`) and
  regardless of who the current assignee/approver is. This is an override layered
  on top of the normal assignee-only/approver-only flows below, not a replacement
  for them — see [Creator overrides](#creator-overrides).

### `checklist_items`
`id, checklist_id, name, checked, checked_by, checked_at, validation_ref,
assignee_override_user_id (nullable), deleted_at (nullable)`

`assignee_override_user_id` is used only by the reject flow (see below) — when null,
responsibility for the item defers to the checklist's normal assignee.

`deleted_at` is a soft-delete marker set when the creator removes an item (see
[Creator overrides](#creator-overrides)) — same pattern as `users.is_active`. Rows
are never hard-deleted, since `checklist_events` references them and a real
`DELETE` would either orphan or be blocked by history.

### `checklist_events`
`id, tenant_id, checklist_id, item_id (nullable), actor_user_id, action, detail,
created_at`

Append-only audit log — the single source of truth for history. Current-state fields
elsewhere (status, checked, assignee, etc.) are a fast-path cache of the latest event,
not an independent source of truth. Example `action` values: `created`,
`assignee_changed`, `approver_changed`, `item_checked`, `item_unchecked`,
`submitted_for_validation`, `rejected`, `approved`, `completed`, `claimed`,
`claim_lost`, `item_added`, `item_removed`, `items_reordered`, `reopened`.

### `notifications`
`id, tenant_id, recipient_user_id, type, checklist_id, actor_user_id, message,
read_at (nullable), email_status, email_attempts, email_last_error (nullable),
email_sent_at (nullable), created_at`

Channel-agnostic at the data layer — a row just records "this happened, this person
should know." Delivery is a separate concern layered on top. In-app delivery is
poll-based (`GET /notifications`, `GET /notifications/badge`) plus a live push on
top: `GET /notifications/stream` is a Server-Sent Events endpoint backed by an
in-process broker (`internal/notify.Hub`) that wakes any connected client the
moment a notification is created for them, so the badge updates immediately
instead of waiting for its next poll. The poll stays as a fallback (browsers
without `EventSource`, or a proxy that drops long-lived connections). The
broker is in-process only, so it works for a single running `checklists-server`
instance — a multi-instance v2 deployment would need a shared broker (e.g.
Postgres `LISTEN/NOTIFY`) instead.

**Email delivery** (per-tenant SMTP) is now implemented as an outbox pattern rather
than sending inline from the HTTP handler that creates the notification: sending
synchronously would tie checklist-action request latency to an external SMTP
round-trip and give no retry path for a transient failure. Instead:

- `email_status` (`pending` / `sent` / `failed` / `skipped`), `email_attempts`,
  `email_last_error` (last error only, overwritten each attempt — not a history
  log), and `email_sent_at` track delivery independently of the row's `read_at`
  (in-app read state and email delivery state are unrelated).
- A background worker (`runEmailDelivery` in `cmd/checklists-server`, same
  ticker/shutdown pattern as the existing session-cleanup goroutine) polls
  `NotificationRepo.ListPendingEmail` every 60s, in batches of 50.
- Each tenant configures its own outbound SMTP (`tenants.smtp_host/port/username/
  password/from_address`); email delivery is enabled for a tenant iff `smtp_host`
  is set. Reference provider: Brevo (`smtp-relay.brevo.com:587`), reachable via the
  standard library's `net/smtp` with no third-party dependency.
- A notification is marked `skipped` (permanent under current state, not retried)
  when the tenant has no SMTP configured, the recipient has no email address, or
  the recipient is deactivated. It's marked `failed`, with `email_attempts`
  incremented, on a transient send error — retried on the next tick until
  `email_attempts` reaches a max (10), at which point it stays `failed` for good.
- Admin-only `GET`/`PUT /admin/tenant/mail-config` manage a tenant's SMTP settings.
  The password is never returned in the `GET` response; on `PUT`, an empty
  `password` means "keep the existing one" rather than requiring the admin to
  resubmit the secret unchanged.
- Out of scope for this pass: per-user opt-out of email notifications, and a
  "send test email" admin action (the worker naturally exercises real delivery
  the first time any notification fires after mail config is set).

## Multi-tenancy

The app is built as **shared-schema multi-tenant SaaS from the start**, with on-prem/
standalone installs modeled as "SaaS with exactly one hardcoded, invisible tenant" —
not as a separate fork or mode. This is cheaper to build in from day one than to
retrofit onto a single-tenant schema later, and this project has no deployed data yet,
so this was the moment to do it.

- **`tenants`** (`id, name, slug`) is the root table. `tenant_id` is added to every
  "root" entity table — `users`, `groups`, `templates`, `checklists`,
  `checklist_events`, `notifications` — but not to child tables reached only via an
  already-tenant-scoped parent (`user_groups`, `template_items`, `checklist_items`)
  or to `sessions` (an opaque, non-enumerable token is already a global primary key,
  never looked up by tenant).
- **Composite uniqueness and composite foreign keys**, not Postgres RLS, are the
  enforcement mechanism for v1. `users.username`, `groups.name`, and
  `templates(name, version)` are unique per-tenant (`UNIQUE(tenant_id, ...)`) rather
  than globally. Every FK from a child row to a tenant-scoped parent (e.g.
  `checklists.assigned_group_id → groups.id`) is a composite FK against
  `(tenant_id, id)` on the parent, so the database itself rejects a row that
  references another tenant's data — not just best-effort application-layer
  checking.
- **Repo-layer signatures**: a method that takes a domain struct pointer (which
  already carries `TenantID`) doesn't need a separate parameter. A method that takes
  a bare ID or business key takes an explicit `tenantID int64` right after `ctx`, and
  filters on it. This is a correctness requirement, not just future-proofing: without
  it, `ChecklistRepo.Get` fetching by a raw path-supplied ID would let
  `Checklist.VisibleTo`'s "everyone can see non-hidden checklists" rule leak
  cross-tenant data the moment a second tenant existed.
- **`domain.TenantRepo.GetSoleTenant`** is the deliberately temporary stand-in for
  real per-request tenant resolution: it errors if the tenant count isn't exactly 1,
  rather than silently taking the first one, so provisioning a second tenant makes
  anything still calling it fail loudly instead of misfiling data. `handleLogin` is
  the one pre-auth caller (it must resolve *a* tenant before it knows who the user
  is); every other handler reads `actor.TenantID` off the already-authenticated user.
- **On-prem provisioning**: `main.go` auto-creates a single `"Default"` tenant on
  first startup if none exists, so standalone installs need zero manual setup.

### v2 SaaS scope (not built yet)

- **Per-request tenant resolution** (subdomain, host header, or API key) for login
  and every other endpoint — v1 hardcodes the sole tenant via `GetSoleTenant`.
- **Self-service tenant signup/provisioning UI.**
- **Per-tenant billing/plan tiers.**
- **Postgres Row-Level Security** — the composite FKs added now are the correctness
  backstop; RLS would be an additional defense-in-depth layer on top, not required
  for v1 correctness.

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
- `complete` is terminal **for the normal assignee/approver-driven flow** — the only
  way *they* can move it forward is cloning it into a brand new checklist. The
  creator, however, can always reopen it via a [creator override](#creator-overrides).

## Creator overrides

Independent of the state machine above, the checklist's **creator** can always:
add an item, remove an item (even if already checked — its history stays in the
event log via `checklist_items.deleted_at`, see above), reorder items, or directly
check/uncheck any item — at any status, including `complete`. This is additive:
it doesn't replace or change the assignee-only `CheckItem` flow or the
approver-only `Reject`/`Approve` flow, it's a separate escape hatch for the one
person who's supposed to be able to fix anything about their own checklist.

Any one of these actions unconditionally forces `status` back to `open` (emitting
a `reopened` event, and notifying the current assignee if they aren't the one who
made the edit), regardless of what status the checklist was in or whether the
edit happens to leave every item checked. This is deliberately simple rather than
trying to re-derive whether the checklist should immediately re-complete: advancing
to `validating`/`complete` only ever happens through the normal `CheckItem`/
`Approve` path, driven by the assignee/approver, not as a side effect of a
creator edit. `assigned_user_id` itself is never touched by these actions, so
"reopened" means "back to open, same assignee as before."

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

See [docs/architecture.md](docs/architecture.md) for a diagram of the
component structure, an authenticated-request sequence diagram, and the
multi-tenancy ER diagram.

```
cmd/checklists-server/     main.go — wiring, config, http server startup
internal/
  domain/                  pure business logic, no DB/HTTP imports
    checklist.go            state machine, reject flow, per-item reassignment
    assignment.go           claim/reassign logic, group-membership checks
    template.go              template → checklist instantiation
  store/
    postgres/                repositories: TenantStore, UserStore, ChecklistStore,
                              TemplateStore, EventStore, NotificationStore (pgx)
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
- **Password hashing**: bcrypt (`golang.org/x/crypto/bcrypt`), default cost.
- **Session tokens**: 32 random bytes (`crypto/rand`), base64 URL-encoded, used
  directly as the session's primary key — no separate lookup id. 7-day TTL
  (`internal/auth.SessionTTL`), with sliding renewal: `auth.CurrentUser` extends a
  session to a fresh `now + SessionTTL` once less than half its TTL remains
  (`renewThreshold`), rather than on every request.
- **Cookie**: `checklists_session`, `HttpOnly`, `SameSite=Lax`, `Secure` whenever
  the request came in over TLS directly, or — when the operator sets
  `config.TrustProxy`/`TRUST_PROXY=true` — whenever a trusted reverse proxy in
  front (e.g. Caddy) reports `X-Forwarded-Proto: https` (`isSecureRequest`,
  `internal/api/handlers.go`). `TrustProxy` must stay `false` unless a reverse
  proxy the operator controls actually sits in front and sets that header
  itself — otherwise a client could spoof it. A second, non-`HttpOnly`
  `checklists_csrf` cookie is set alongside it (stateless double-submit
  pattern — see CSRF entry below).
- **Login rate limiting**: `auth.LoginLimiter`, in-memory and IP-keyed
  (`net.SplitHostPort(r.RemoteAddr)`, or — when `TrustProxy` is enabled — the
  left-most address in `X-Forwarded-For`, since `RemoteAddr` would otherwise
  always be the reverse proxy's own address), 5 failures / 15-minute fixed
  window. A successful login clears the window. Deliberately not
  username-keyed, to avoid a targeted-lockout DoS against a specific user;
  doesn't survive a restart and doesn't coordinate across instances — both
  accepted for now, matching the project's no-premature-abstraction
  convention.
- **Expired-session cleanup**: `SessionRepo.DeleteExpired` runs off an hourly
  `time.Ticker` goroutine started in `main.go`, sharing the process's shutdown
  context and a `sync.WaitGroup` so it exits cleanly alongside the HTTP server.
- **Password reset**: `POST /password-reset/request` (username, not email —
  looked up via the same `UserRepo.GetByUsername` login uses) always responds
  `204` regardless of whether the username matched, is active, or has an email
  on file (enumeration-safe, matching the login-error philosophy above); a
  second `auth.LoginLimiter` instance IP-rate-limits it independently of login.
  A match generates an opaque, single-use token (`password_reset_tokens`,
  same shape as `sessions`) with a 1-hour TTL (`auth.PasswordResetTokenTTL`,
  deliberately much shorter than `SessionTTL` — a leaked reset link is a
  bigger risk than a leaked session cookie) and emails a link built from the
  request's own host/scheme, sent synchronously via `mail.Send` (unlike the
  async notification outbox, a user waiting on a reset link needs it
  immediately). `POST /password-reset/confirm` (token + new password) hashes
  the password, updates it, deletes the token (single-use), deletes every
  other session belonging to that user (`SessionRepo.DeleteByUserID` — a
  password change should invalidate any leaked/compromised old session), and
  logs the caller into a fresh session, mirroring `handleLogin`/
  `handleRegister`'s cookie setup. Swept by the same hourly-ticker pattern as
  session cleanup (`runPasswordResetTokenCleanup` in `main.go`).
- `internal/auth` is framework-agnostic (no `net/http`) — it only knows about
  `domain.UserRepo`/`domain.SessionRepo`. `internal/api/middleware.go` owns the
  cookie itself: reading it into the request context on every request, and
  `RequireAuth` rejects requests with no resolved user. Login/logout errors are
  deliberately undifferentiated (`ErrInvalidCredentials` covers both "no such
  user" and "wrong password") to avoid username enumeration.

### User provisioning
Three ways a user row gets created, all going through the same `UserRepo.Create` and
its `(tenant_id, username)` uniqueness check (a collision returns `domain.ErrUsernameTaken`,
mapped to `409 Conflict`):

- **`POST /register`** — self-service, no auth required. Joins the caller to
  `GetSoleTenant`'s tenant (see [Multi-tenancy](#multi-tenancy) — same v1 boundary as
  login) as an ordinary, active, non-admin user, and logs them in immediately
  (reuses `auth.Login` right after `Create`, so a successful registration sets the
  same session/CSRF cookies a manual login would).
- **`POST /admin/users`** — admin-only, provisions a single user directly into
  `actor.TenantID` (the calling admin's own tenant — an admin can never create a user
  in a different tenant), active immediately, `is_admin` settable in the request.
- **`POST /admin/users/bulk`** — admin-only, same tenant-scoping as above, but reads
  a `text/csv` body: one row per user, no header, columns
  `username,password,name[,is_admin]`. Rows are processed independently — a bad row
  (duplicate username, missing field) is captured as a per-row error and does **not**
  abort the batch; only a CSV syntax error aborts the whole request with `400`. The
  response is always `200` with a per-row `{row, username, status, error, user}`
  breakdown, so the caller can see exactly which rows succeeded without a partial,
  silent failure.

### Frontend

**Implemented.** Product name is **ChecklistHQ**, centralized as
`internal/web.AppName` and exposed to every template via the `{{appName}}`
template func (`layout.html` and all page titles reference it — no literal
"Checklists" string anywhere in the UI).

Server-rendered with **Go templates + htmx + Alpine.js + SortableJS**, plain
CSS for styling — deliberately not a separate SPA (React/Vue). Rationale: this
app is fundamentally forms and lists (checklists, items, assignment,
approval), not a highly animated client app, and the author's
background/preference favors a hypermedia-driven stack over adopting a
client-side framework's whole worldview.

- **htmx**: partial-page swaps for everything server-driven (check an item, claim an
  assignment, submit approval). Live notifications reuse this: a small script opens
  an SSE connection (`/notifications/stream`) and, on each push, dispatches the same
  `notificationsRead` DOM event the badge's own polling and mark-read fragment
  already trigger — no new htmx wiring needed on the badge itself.
- **Alpine.js**: small local UI state only (collapsing the notes field, dropdowns,
  confirm dialogs) — not a client-side state management layer.
- **SortableJS**: drag-and-drop reordering, drag-and-drop checklist-from-template
  creation, and the visual template builder (arranging/reordering item blocks while
  authoring a template) — all the same underlying pattern, wired to htmx requests on
  drop.
- **Theming**: light/dark/system, via CSS custom properties in `app.css`
  (`:root` for light tokens, `[data-theme="dark"]` for overrides) and a
  `<select>` in the nav. The choice is stored client-side only
  (`localStorage`, no account/DB sync — this is a per-browser display
  preference, not data other users or devices need to see), and applied by
  an inline bootstrap script in `layout.html`'s `<head>` (before the
  stylesheet loads) so there's no flash of the wrong theme on load;
  `static/js/theme.js` handles the switcher itself and live-updates an open
  tab if "System" is selected and the OS theme changes.
- **Notes field**: plain `<textarea>`, no rich-text editor. Bare URLs are
  auto-linkified at render time (e.g. via `goldmark` in autolink mode) rather than
  supporting authored `[text](url)` links or a WYSIWYG toolbar — deliberately the
  simplest option. **Flagged for review during testing** — there's a real chance this
  won't feel right in practice and gets swapped for Markdown-lite link syntax
  instead. (Tracked as an open task, not just this note.)

#### Routes: `internal/api` vs. `internal/web`

The JSON API lives entirely under `/api/*` (`internal/api.RegisterRoutes`); the
browser UI is served from `/` (`internal/web.RegisterRoutes`). Each runs on its
own `*http.ServeMux` and `http.Server`, listening on independently configurable
ports (`api_port`/`web_port`, see README's Configuration section) so the two
surfaces can be exposed or firewalled separately. `cmd/checklists-server/main.go`
wraps each mux in its own `api.WithSession` call, so session/CSRF handling is
identical on both, and the shared auth endpoints (`/login`, `/register`,
`/logout`) are registered on both muxes via the exported
`api.RegisterAuthRoutes` — the one piece `main.go` (the composition root)
bridges between the two packages, since `internal/web` itself must not import
`internal/api`. Sessions remain valid across both ports: validation is a
database lookup, not tied to which server issued the cookie, and browser
cookies aren't port-scoped. `internal/web` depends on the same
`internal/domain` and store layer as `internal/api`, but does
**not** import or call into `internal/api` — each package independently
implements its own handler logic against the shared domain layer, even where
that means near-duplicate code (e.g. `internal/api/checklists.go`'s
`handleCreateChecklist` and `internal/web/checklists.go`'s
`handleCreateChecklistUI` run the same assignment-validation and
checklist-creation-policy checks against the same domain functions, coded
twice). This is a deliberate choice over extracting a shared handler layer:
the two surfaces render different things (JSON vs. HTML fragments) and are
expected to diverge further, so a shared abstraction would need to fork
almost immediately anyway.

Pages/fragments implemented in `internal/web`: login/register, checklist list
+ create + detail (state-machine-gated actions: claim, check/uncheck,
creator override, approve/reject, add/remove item, reorder), template list +
detail + new-version builder (drag-drop item ordering), group management,
admin user management (single-create + bulk CSV), tenant mail config,
checklist-creation policy (see below), and notifications with an
unread-count badge (polling, plus a live SSE push on top — see
[notifications](#notifications)).

#### Checklist-creation restriction

Per-tenant, **opt-in, default-off** (`tenants.restrict_checklist_creation`).
When off, any active user can create a checklist — unchanged from the
original behavior. When a tenant admin turns it on (via `PUT
/api/admin/tenant/checklist-policy` or the `/admin/checklist-policy` UI
page) and designates a `checklist_creator_group_id`, checklist creation is
limited to admins plus members of that one group. This reuses the existing
group concept rather than adding a new role/permission tier;
`domain.CanCreateChecklist` is the single pure function both `internal/api`
and `internal/web` call to enforce it identically, and both surfaces gate
creation at the same two points: the create action itself, and the
create-page/"new checklist" link (so a disallowed user can't reach the form
by guessing the URL).

## Open items (deferred)

- **v2 scope**: per-request tenant resolution, self-service tenant signup,
  per-tenant billing, and Postgres RLS (see [Multi-tenancy](#multi-tenancy)).

CSRF protection, login rate limiting, sliding session renewal, and expired-session
cleanup — all previously listed here — are implemented; see "Storage & auth" above.
Graceful shutdown (`signal.NotifyContext` + `http.Server.Shutdown`) was added in the
same pass, in `cmd/checklists-server/main.go`.

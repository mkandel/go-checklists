# Notes for the future Qwik pass

Written while building full feature parity into `web-react/` in one long
unsupervised session (2026-07-05/06). `web-qwik/` doesn't exist yet — this
captures decisions and gaps discovered while building the React SPA so the
Qwik pass doesn't have to rediscover them from scratch. Qwik is a separate
codebase; none of this is shared code, just shared knowledge.

## 1. SSE notifications are unreachable as currently wired — use polling

`GET /notifications/stream` (`internal/web/notifications.go`, backed by
`notify.Hub`) exists **only on the `:8081` web server**, not under `/api/`.
`web-react/vite.config.ts`'s dev proxy only forwards `/api`, `/login`,
`/register`, `/logout`, `/password-reset` to `:8080`, and in production the
built SPA is served by `internal/webreact` on the API server's own origin.
There is no route from either dev or prod SPA serving to the `:8081`
process, so the SSE endpoint can't be reached without proxy/deployment
changes.

React's `NotificationBadge.tsx` and `NotificationsList.tsx` use 20s polling
against `GET /api/notifications` instead — this mirrors the old UI's own
framing of its poll as an "additive fallback" to push, so polling-only isn't
a regression in spirit, just in latency.

**If Qwik wants real push**, the actual fix is adding an
`/api/notifications/stream` endpoint to `internal/api` (reusing
`notify.Hub`) rather than routing around it again — worth doing once, shared
by both SPAs. Until then, do the same polling workaround.

## 2. Two backend API gaps were found and fixed — already resolved, don't redo

While building the admin-users vertical, `internal/api/users.go` had no
sortable/filterable admin list and no suspend/reactivate endpoint (those
existed only in `internal/web/admin_users.go`). Fixed by adding, exactly
mirroring the web layer's logic (self-suspend 403 guard,
`ClearUserAssignments` on suspend, same sort-column allowlist):

- `GET /api/admin/users?sort=&dir=&show_inactive=1` — `handleAdminListUsers`
- `POST /api/admin/users/{id}/active` `{"active": bool}` — `handleAdminSetUserActive`

These now exist in `internal/api/users.go` alongside the pre-existing
`GET /api/users` (unsorted, no inactive-visibility control, used for
dropdowns — kept separate on purpose), `POST /api/admin/users`,
`POST /api/admin/users/bulk`, `GET /api/admin/users/export.csv`. Qwik can
just consume these; no further backend work needed for admin users.

## 3. Drag-and-drop reordering: native HTML5, no library dependency

Both `ChecklistDetail.tsx` (checklist items, creator-only) and
`TemplateCreate.tsx` (template item builder) use plain
`draggable`/`onDragStart`/`onDragOver`/`onDrop` — no SortableJS or other
drag library. This was the first drag-and-drop UI in the React app; worked
fine for a flat single-list reorder. The pattern:

```tsx
<li
  draggable
  onDragStart={() => setDragId(item.ID)}
  onDragOver={(e) => e.preventDefault()}
  onDrop={() => handleDrop(item.ID)}
>
```

On drop, splice the dragged id out of an id array and reinsert it at the
target index, then POST the full reordered id list. Qwik doesn't have to
copy this verbatim — Qwik's resumability model and event-handling story
differ enough from React's that it's worth re-deriving the idiomatic Qwik
way to do drag-and-drop rather than transliterating JSX — but the *domain*
logic (splice id out, reinsert, POST whole order) is the part worth
keeping.

## 4. JSON shape gotchas worth getting right the first time

- `GET/POST` template-detail responses (`/api/templates/{id}`,
  `POST /api/templates`) **embed** `domain.Template` — its fields
  (`ID`, `TenantID`, `Name`, `Version`) serialize flat at the top level of
  the response object, alongside a separate lowerCamel `items` key. It is
  **not** nested under a `"Template"` key. See
  `internal/api/templates.go`'s `templateResponse struct { domain.Template;
  Items []domain.TemplateItem \`json:"items"\` }`.
- Domain structs with no `json` tags (e.g. `domain.User`, `domain.Group`,
  `domain.Checklist`) serialize with **PascalCase** field names exactly as
  Go names them (`ID`, `Name`, `TenantID`, `IsActive`, ...). Only
  hand-written request/response wrapper types (the ones with explicit
  `json:"..."` tags, like `tenantMailConfigResponse` or
  `bulkCreateUserResult`) use lowerCamel/snake_case. Check the Go struct
  before guessing a field's casing — there's no single convention across
  the whole API surface.
- `POST /password-reset/request` takes a **form-urlencoded** body
  (`r.ParseForm()` / `r.FormValue("username")`), not JSON — unlike almost
  every other mutation in the API. `POST /password-reset/confirm` and
  `POST /register`, by contrast, are JSON.
- Suspending a user clears their checklist assignments/approver role
  server-side (`ClearUserAssignments`) as part of the same transaction —
  no separate client-side cleanup needed after calling
  `POST /api/admin/users/{id}/active` with `{"active": false}`.

## 5. Business logic mirrored client-side (from `internal/domain/checklist.go`)

To gate action buttons without a round-trip, React computes these locally
from a fetched `Checklist` + the current user (`Me`):

```
responsibleUserFor(item) = item.AssigneeOverrideUserID ?? checklist.AssignedUserID
canCheck   = status === 'open' && !item.Checked && responsibleUserFor(item) === me.ID
canClaim   = checklist.AssignedUserID == null
isCreator  = me.ID === checklist.CreatorID
isApproverValidating = status === 'validating' && checklist.ApproverID === me.ID
```

These are just read-only UI gates — the server re-validates every mutation
independently, so getting one of these subtly wrong client-side is a UX bug,
not a security bug. Still worth getting right the first time rather than
re-deriving from scratch by reading `internal/web/checklist_detail.go`'s
`buildChecklistPanelData` again.

## 6. Tooling gotchas hit while building this (environment-specific, not code)

Not Qwik-specific, but will bite again: WSL has no native `node`/`npm` on
this machine — the JS toolchain (`npm install`, `npm run build`) must run
from native PowerShell, not `wsl bash -lc`. Referencing `$PATH` in any
`wsl bash -lc` invocation silently swallows all output of that command
(and looks like a hang/no-op, not an error) — see
`feedback_wsl_powershell_quoting.md` in the Claude memory store for the
full list of these gotchas and their fixes.

# Notes from building web-qwik (follow-up to NOTES-QWIK.md)

Written after finishing full feature parity into `web-qwik/` (2026-07-06).
`NOTES-QWIK.md` captured lessons *before* this build started, from the React
pass; this file captures what was actually learned *while* building Qwik
itself, for a hypothetical future frontend or maintenance pass.

## 1. `?id=` query-string routing for detail pages, not dynamic segments

`web-qwik/src/routes/(app)/checklists/view/index.tsx` and the equivalent
template detail route use a static route with the record id in a query
string (`/checklists/view?id=123`) instead of a Qwik City dynamic path
segment (`/checklists/[id]`). Two reasons, both load-bearing:

- The SSG adapter only pre-renders routes with no dynamic segments —
  per-tenant checklist/template ids aren't known at build time, so
  `[id]` isn't buildable at all under SSG.
- A `?id=` route is itself fully static (one prerendered page, same as
  `/checklists` or `/login`), which sidesteps DESIGN.md's flagged
  historical Qwik SSG bug around client-side URL updates on dynamic routes
  entirely — rather than leaning on `internal/webqwik`'s SPA fallback to
  serve the right static shell for a hard-loaded `/checklists/<id>` and
  hoping client-side routing then behaves. That fallback path was never
  exercised in a real browser (see item 4 below), so avoiding it rather than
  depending on it was the safer call.

The id is read via `useLocation().url.searchParams.get('id')`, same pattern
in both the checklist and template detail routes.

## 2. Qwik `<option>` JSX gotcha

`<option>` requires a **single string child** — unlike React, Qwik doesn't
accept multiple mixed text/expression child nodes (e.g.
`<option>{t.Name} (v{t.Version})</option>`) and either errors or renders
wrong. Fix: combine into one template literal so there's exactly one child:

```tsx
<option value={String(t.ID)}>{`${t.Name} (v${t.Version})`}</option>
```

Relatedly, `value` props on `<select>`/`<option>` need `String(id)` — Qwik
is stricter about coercing numbers to the string form the DOM actually
wants than React's JSX was in the equivalent spot.

## 3. NOTES-QWIK.md's items carried over with no new backend work

Confirmed directly, no surprises:

- **#1 (SSE unreachable)** — same story as React: `/notifications/stream`
  only exists on the web port, unreachable from the Qwik build's
  origin/proxy setup either. Qwik polls `GET /api/notifications` every 20s,
  same as React.
- **#2 (admin API gaps)** — already fixed in the backend during the React
  pass (`GET /api/admin/users`, `POST /api/admin/users/{id}/active`); Qwik's
  admin vertical (`web-qwik/src/routes/(app)/admin/users/index.tsx`)
  consumes them directly, no further backend changes needed.
- **#4 (JSON shape gotchas)** — template response flattening, PascalCase
  domain structs, form-urlencoded password-reset-request body, and
  suspend-clears-assignments-server-side all held exactly as documented.
  Nothing new found.
- **#5 (business logic mirrored client-side)** — `responsibleUserFor`,
  `canCheck`, `canClaim`, `isCreator`, `isApproverValidating` all
  re-derived client-side in Qwik's checklist detail route, same rules as
  React's, same "UX bug not security bug" reasoning (server re-validates
  every mutation regardless).

Item #3 (native HTML5 drag-and-drop, no library) also carried over as-is —
Qwik's `draggable`/`onDragStart$`/`onDragOver$`/`onDrop$` handlers work the
same way, just with Qwik's `$`-suffixed event/lazy-loading convention.

## 4. Outstanding gap: no browser smoke test

This entire build was done without access to a real browser. The known
historical Qwik SSG bug DESIGN.md flags — client-side route URL updates
misbehaving with JS enabled in a production build — could **not** be
manually verified against this app. The `?id=` routing decision (item 1
above) was specifically chosen to sidestep that bug rather than test
around it, but that's a design mitigation, not a substitute for actually
loading the built site in a browser and clicking through it.

Before treating `web-qwik` as production-ready: build it
(`scripts/build-frontends.ps1` or `cd web-qwik && npm ci && npm run build`),
run it via `WEB_FRONTEND=qwik`, and manually click through at least
login → checklist list → checklist detail (hard-loaded URL and
client-side navigation to it) → admin users, watching for the flagged
routing bug and any resumability/hydration surprises. This is the same
category of outstanding gap `web-react` has for its own untested paths
(see `NOTES-QWIK.md`) — neither SPA build has had a real-browser pass yet.

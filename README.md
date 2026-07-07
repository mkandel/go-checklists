# ChecklistHQ (go-checklists)

A web-based, database-driven, shareable checklist system: create reusable
templates, instantiate them as checklists, assign/reassign to a person or a
team, and optionally require a designated approver to sign off before a
checklist is considered complete. Full audit trail of who did what, and when.

Design rationale and data model are in [DESIGN.md](DESIGN.md); a diagram of the
component structure and request lifecycle is in
[docs/architecture.md](docs/architecture.md).

Both an HTTP/JSON API (under `/api/*`) and a browser UI are backed by the
same Postgres-backed domain layer. The browser UI is available in three
interchangeable builds, selected at runtime via `WEB_FRONTEND` (see
[Frontend builds](#frontend-builds)): the original server-rendered htmx +
Alpine.js + SortableJS UI (default), a React SPA, and a Qwik SPA. This
project doubles as a learning/exploration vehicle for comparing frontend
stacks, so all three are maintained side by side rather than picking one
permanently — they are required to behave identically for every feature,
but not required to look identical. See DESIGN.md's
[Frontend](DESIGN.md#frontend) section for the rationale.

## Status

Early and under active development. The API surface and browser UI (auth,
checklists, templates, groups, users, notifications, self-service
registration, admin user management including suspend/reactivate, per-tenant
SMTP email delivery for notifications, an opt-in creator-vs-user
checklist-creation restriction, forgot-password/reset-password flow) are
functional and tested.

## Requirements

- Go 1.26+
- Postgres 16+ (a `docker-compose.yml` is included for local development)

## Quickstart

```sh
# 1. Start Postgres and the Caddy reverse proxy
docker compose up -d

# 2. Configure
cp config.example.json config.json
# edit config.json if you changed the docker-compose credentials/port

# 3. Run (migrations run automatically on startup)
go run ./cmd/checklists-server -c config.json
```

Browse to `http://localhost/` — that's Caddy on port 80, forwarding to the
app's web port per the checked-in [`Caddyfile`](Caddyfile).

The JSON API and the browser UI listen on two independent ports —
`LISTEN_HOST:API_PORT` (default `:8080`) and `LISTEN_HOST:WEB_PORT` (default
`:8081`) — each backed by its own `*http.ServeMux` and `http.Server`, so
either can be exposed or firewalled independently. The browser UI is served
from `/` on the web port (start at `/login` or `/register`); the JSON API
lives under `/api/*` on the API port. Login/register/logout are registered
on both ports so each is self-contained for authentication; sessions are
valid on either port since validation is a database lookup, not tied to
which port issued the cookie.

### Reverse proxy

`WEB_PORT` defaults to an unprivileged port (`8081`) precisely so the app
never needs elevated privileges; a reverse proxy is expected to own port
`80`/`443` in front of it. `docker-compose.yml` includes a `caddy` service
for this, configured by the root [`Caddyfile`](Caddyfile), which forwards
`:80` to the app on the host at `:8081`. Set `TRUST_PROXY=true` (already the
`config.example.json` default) so the app honors the proxy's `X-Forwarded-Proto`
and `X-Forwarded-For` headers — this is what lets the session/CSRF cookies'
`Secure` flag and per-client login rate limiting work correctly through a
proxy; only enable it when a trusted reverse proxy is actually in front; a
directly internet-facing app must leave it `false`.

Once a real domain points at the server, edit the `Caddyfile`'s `:80` to the
domain name instead — Caddy will then automatically obtain and renew a
Let's Encrypt certificate with no other configuration changes.

## Configuration

Configuration is layered, in increasing order of precedence: a JSON config
file, environment variables, then command-line flags.

| Setting          | Env var             | Flag             | Config file key |
|------------------|---------------------|------------------|------------------|
| Config file path | `CHECKLISTS_CONFIG` | `-c` / `-config` | —                |
| Listen host      | `LISTEN_HOST`       | `-host`          | `host`           |
| API listen port  | `API_PORT`          | `-api-port`      | `api_port`       |
| Web listen port  | `WEB_PORT`          | `-web-port`      | `web_port`       |
| Database URL     | `DATABASE_URL`      | `-database-url`  | `database_url`   |
| Trust proxy      | `TRUST_PROXY`       | `-trust-proxy`   | `trust_proxy`    |
| Web frontend     | `WEB_FRONTEND`      | `-web-frontend`  | `web_frontend`   |
| Notifications enabled | `NOTIFICATIONS_ENABLED` | `-notifications-enabled` | `notifications_enabled` |

`DATABASE_URL` is required (from any source). `WEB_FRONTEND` accepts
`server` (default), `react`, or `qwik` — see
[Frontend builds](#frontend-builds). See
[`config.example.json`](config.example.json) for a sample config file:

```sh
cp config.example.json config.json
go run ./cmd/checklists-server -c config.json
```

### Versioning

The running build's version is baked in at compile time and shown in the
web UI's footer and `GET /api/healthz`. Plain `go build`/`go run` leaves it
as `dev`; a release build sets it via `-ldflags`, pinned to a git tag:

```sh
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" -o checklists-server ./cmd/checklists-server
```

## Development

```sh
go build ./...
go vet ./...
go test ./...              # unit tests only (internal/domain, internal/auth, internal/config, internal/mail)
go test -tags=integration ./...  # + internal/store/postgres, internal/api, internal/web
gofmt -l .
```

`internal/store/postgres`, `internal/api`, and `internal/web` are gated behind
the `integration` build tag — all spin up a real Postgres via
[testcontainers](https://golang.testcontainers.org/), so Docker must be running
to build/run them.

Sample data for manual testing: `go run ./cmd/seed` (idempotent — see
[Quickstart](#quickstart) for starting Postgres first). An end-to-end check
against a real database is `go run ./cmd/smoketest`.

A Postman collection for manually exercising the JSON API by hand (auth,
checklists, notifications, users, templates, groups, tenant admin) lives under
[postman/](postman/) — see its README for import/usage instructions. An
[OpenAPI 3.0 spec](docs/openapi.yaml) covering the same API is also available,
for generating clients or browsing in a tool like Swagger UI/Redoc.

`go run ./cmd/stresstest -n 100` fires `N` concurrent goroutines at a single
checklist's claim endpoint against a real database, to confirm the `FOR
UPDATE` row lock in `ChecklistRepo.Claim` actually serializes concurrent
claims under real contention (exactly one winner, the rest get `409`) — a
guarantee the existing sequential claim tests can't exercise, since they
call `Claim` one goroutine at a time.

Coverage reports should scope `-coverpkg` to exclude `cmd/...` (e.g.
`-coverpkg=$(go list ./... | grep -v /cmd/)`); the `cmd/*` packages are
`main` entrypoints exercised by running the binary, not by `go test`, so
including them just reports a misleading 0% for each and drags down the
overall number.

### Frontend builds

`WEB_FRONTEND` picks which browser UI `cmd/checklists-server` serves on the
web port, at runtime — no rebuild of the Go binary needed to switch:

| Value (default in **bold**) | Source                | Served from                       |
|------------------------------|-----------------------|------------------------------------|
| **`server`**                  | `internal/web`        | Go templates, rendered per-request |
| `react`                       | `web-react/` (Vite)   | `internal/webreact` (embedded static build) |
| `qwik`                        | `web-qwik/` (Qwik SSG)| `internal/webqwik` (embedded static build)  |

`react` and `qwik` are static builds embedded into the Go binary via
`//go:embed`, the same way `internal/web`'s templates/assets are — there's no
separate Node process to run or deploy. Build them with
`scripts/build-frontends.ps1` (or `cd web-react && npm ci && npm run build`,
respectively for `web-qwik`) before starting the server with
`WEB_FRONTEND=react`/`qwik`; each `internal/webreact`/`internal/webqwik`
package's `dist/` only ships a placeholder `.gitkeep` in git (build output is
gitignored), so `go build ./...` always succeeds on a fresh checkout even
before either frontend has been built — the server just serves a friendly
503 explaining the frontend needs to be built first, rather than a blank
page. All three frontends talk to the same unchanged `/api/*` JSON API; only
`internal/web` (`server` mode) is server-rendered, so switching frontends
never changes API behavior. See DESIGN.md's [Frontend](DESIGN.md#frontend)
section for the CORS/proxy setup that lets `react`/`qwik` call the API
cross-origin during local development.

`web-react/` (Vite + React + TypeScript + `react-router-dom`) and
`web-qwik/` (Qwik SSG + TypeScript + Qwik City) both have full feature parity
with the server-rendered UI: login/registration/password reset, a
session-aware route guard (redirects to `/login` on a 401 from
`GET /api/me`), checklist list/create/detail with all mutations
(claim/check/approve/reject/add/remove/reorder), groups, templates, a
polling-based notifications list and badge (both poll
`GET /api/notifications` rather than using SSE — see `NOTES-QWIK.md` for
why), and admin pages (users, mail config, checklist creation policy).
`web-qwik/`'s checklist/template detail pages use a `?id=` query-string
route rather than a dynamic path segment, since Qwik's SSG adapter can't
pre-render per-tenant dynamic ids at build time — see `NOTES-QWIK-FOLLOWUP.md`
at the repo root for that decision and other Qwik-specific gotchas/gaps
discovered while building it, alongside `NOTES-QWIK.md`'s original
React-pass notes. `scripts/build-frontends.ps1` skips any frontend whose
source directory isn't present.

### GoLand run configurations

Shared, checked-in run configs live under `.run/` and show up automatically
when the project is opened in GoLand:

| Config                       | What it does                                                   |
|-------------------------------|------------------------------------------------------------------|
| **Run Server (sample DB)**    | Runs `cmd/checklists-server` against `DATABASE_URL`, `WEB_FRONTEND=server`; before launch, runs **Seed Sample Database**. Requires `docker compose up -d` already running. |
| **Run Server (sample DB w/reset)** | Same, but runs **Reset Sample Database** then **Seed Sample Database** before launch, for a clean slate every time. |
| **Run Server (sample DB) (with proxy)** | Same as **Run Server (sample DB)**, with `TRUST_PROXY=true`, run via the WSL - Ubuntu target. |
| **Run Server (React UI)**     | Same, with `WEB_FRONTEND=react`. See [Frontend builds](#frontend-builds). |
| **Run Server (Qwik UI)**      | Same, with `WEB_FRONTEND=qwik`. See [Frontend builds](#frontend-builds). |
| **Seed Sample Database**      | Runs `go run ./cmd/seed` — idempotent, safe to re-run. |
| **Reset Sample Database**     | Runs `go run ./cmd/resetdb` — wipes and recreates the sample database. |
| **Unit Tests**                | `go test` over the whole module, no build tag — the non-DB packages. |
| **Integration Tests**         | Same, with `-tags=integration` — brings in `internal/store/postgres`, `internal/api`, and `internal/web`. Docker must be running (testcontainers). |
| **Smoke Test**                | Runs `go run ./cmd/smoketest` — full login → create → check → complete flow against a real Postgres. |

## Backup & restore

`scripts/backup.sh [out-dir]` dumps the `checklists` database (via `docker
compose exec postgres pg_dump -Fc`) to a timestamped custom-format file
under `out-dir` (default `./backups`, gitignored). `scripts/restore.sh
<dump-file>` restores from such a file — it **drops and recreates** the
`checklists` database first, so stop the app and confirm you mean it; it
prompts before doing so. Both require `docker compose up -d postgres`
already running.

```sh
./scripts/backup.sh
./scripts/restore.sh backups/checklists-20260704T120000Z.dump
```

There's no automated backup schedule yet — these are manual/cron-friendly
building blocks, not a managed backup service.

## Security

- Sessions are server-side, cookie-based, with sliding renewal and a
  background sweep of expired sessions.
- CSRF protection via a stateless double-submit cookie.
- Login attempts are rate-limited per IP.
- The server shuts down gracefully (`SIGINT`/`SIGTERM`), draining in-flight
  requests before exiting.

See DESIGN.md's [Storage & auth](DESIGN.md#storage--auth) section for details.

## Access logging

Every request to either server (API or web) is logged via the standard `log`
package once handled: client IP (proxy-aware, per `TRUST_PROXY` — see
[Reverse proxy](#reverse-proxy)), method, path, status code, and duration.
There's no structured/JSON log format or external shipping yet — this is
plain stdout/stderr text, suitable for `docker logs`/`journalctl` or piping
to a log aggregator's own tailing agent.

## Error pages

The web UI renders a branded HTML error page (matching the normal layout —
nav, footer, `app.css`) for an unmatched route (404) and for an unrecovered
panic (500) — both servers wrap their outermost handler in a recover
middleware (`api.WithRecover` / `web.WithRecover`) so a panic gets a real
response instead of net/http silently closing the connection. htmx fragment
endpoints (used for in-page partial swaps) keep their existing plain-text
error responses, since a full `<html>` page isn't valid to swap into a
fragment target.

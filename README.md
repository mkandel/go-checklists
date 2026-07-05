# ChecklistHQ (go-checklists)

A web-based, database-driven, shareable checklist system: create reusable
templates, instantiate them as checklists, assign/reassign to a person or a
team, and optionally require a designated approver to sign off before a
checklist is considered complete. Full audit trail of who did what, and when.

Design rationale and data model are in [DESIGN.md](DESIGN.md); a diagram of the
component structure and request lifecycle is in
[docs/architecture.md](docs/architecture.md).

Both an HTTP/JSON API (under `/api/*`) and a server-rendered browser UI
(htmx + Alpine.js + SortableJS, see DESIGN.md's Frontend section) are backed
by the same Postgres-backed domain layer.

## Status

Early and under active development. The API surface and browser UI (auth,
checklists, templates, groups, users, notifications, self-service
registration, admin user management, per-tenant SMTP email delivery for
notifications, an opt-in creator-vs-user checklist-creation restriction) are
functional and tested; password reset is not built yet.

## Requirements

- Go 1.26+
- Postgres 16+ (a `docker-compose.yml` is included for local development)

## Quickstart

```sh
# 1. Start Postgres
docker compose up -d

# 2. Configure
cp .env.example .env
# edit .env if you changed the docker-compose credentials/port

# 3. Run (migrations run automatically on startup)
go run ./cmd/checklists-server
```

The JSON API and the browser UI listen on two independent ports —
`LISTEN_HOST:API_PORT` (default `:8080`) and `LISTEN_HOST:WEB_PORT` (default
`:80`) — each backed by its own `*http.ServeMux` and `http.Server`, so either
can be exposed or firewalled independently. The browser UI is served from `/`
on the web port (start at `/login` or `/register`); the JSON API lives under
`/api/*` on the API port. Login/register/logout are registered on both ports
so each is self-contained for authentication; sessions are valid on either
port since validation is a database lookup, not tied to which port issued the
cookie. Binding the web port to `80` typically requires elevated privileges
(or a reverse proxy) outside of local development — set `WEB_PORT` to an
unprivileged port if that's not available.

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

`DATABASE_URL` is required (from any source). Example config file:

```json
{
  "host": "0.0.0.0",
  "api_port": 8080,
  "web_port": 80,
  "database_url": "postgres://checklists:checklists@localhost:5432/checklists?sslmode=disable"
}
```

```sh
go run ./cmd/checklists-server -c /etc/checklists/config.json
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

### GoLand run configurations

Shared, checked-in run configs live under `.run/` and show up automatically
when the project is opened in GoLand:

| Config                       | What it does                                                   |
|-------------------------------|------------------------------------------------------------------|
| **Run Server (sample DB)**    | Runs `cmd/checklists-server` against `DATABASE_URL`; before launch, runs **Seed Sample Database**. Requires `docker compose up -d` already running. |
| **Seed Sample Database**      | Runs `go run ./cmd/seed` — idempotent, safe to re-run. |
| **Unit Tests**                | `go test` over the whole module, no build tag — the non-DB packages. |
| **Integration Tests**         | Same, with `-tags=integration` — brings in `internal/store/postgres`, `internal/api`, and `internal/web`. Docker must be running (testcontainers). |
| **Smoke Test**                | Runs `go run ./cmd/smoketest` — full login → create → check → complete flow against a real Postgres. |

## Security

- Sessions are server-side, cookie-based, with sliding renewal and a
  background sweep of expired sessions.
- CSRF protection via a stateless double-submit cookie.
- Login attempts are rate-limited per IP.
- The server shuts down gracefully (`SIGINT`/`SIGTERM`), draining in-flight
  requests before exiting.

See DESIGN.md's [Storage & auth](DESIGN.md#storage--auth) section for details.

# go-checklists

A web-based, database-driven, shareable checklist system: create reusable
templates, instantiate them as checklists, assign/reassign to a person or a
team, and optionally require a designated approver to sign off before a
checklist is considered complete. Full audit trail of who did what, and when.

Design rationale and data model are in [DESIGN.md](DESIGN.md); a diagram of the
component structure and request lifecycle is in
[docs/architecture.md](docs/architecture.md).

Currently an HTTP/JSON API backed by Postgres — no web UI yet (see DESIGN.md's
Frontend section for the planned htmx/Alpine.js direction).

## Status

Early and under active development. The API surface (auth, checklists,
templates, groups, users, notifications, self-service registration, admin
user management) is functional and tested; things like password reset and a
browser UI are not built yet.

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

The server listens on `LISTEN_HOST:LISTEN_PORT` (default `:8080`).

## Configuration

Configuration is layered, in increasing order of precedence: a JSON config
file, environment variables, then command-line flags.

| Setting        | Env var           | Flag             | Config file key |
|----------------|-------------------|------------------|------------------|
| Config file path | `CHECKLISTS_CONFIG` | `-c` / `-config` | —              |
| Listen host    | `LISTEN_HOST`     | `-host`          | `host`           |
| Listen port    | `LISTEN_PORT`     | `-port`          | `port`           |
| Database URL   | `DATABASE_URL`    | `-database-url`  | `database_url`   |

`DATABASE_URL` is required (from any source). Example config file:

```json
{
  "host": "0.0.0.0",
  "port": 8080,
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
go test ./...              # unit tests only (internal/domain, internal/auth, internal/config)
go test -tags=integration ./...  # + internal/store/postgres and internal/api
gofmt -l .
```

`internal/store/postgres` and `internal/api` are gated behind the `integration`
build tag — both spin up a real Postgres via
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
| **Integration Tests**         | Same, with `-tags=integration` — brings in `internal/store/postgres` and `internal/api`. Docker must be running (testcontainers). |
| **Smoke Test**                | Runs `go run ./cmd/smoketest` — full login → create → check → complete flow against a real Postgres. |

## Security

- Sessions are server-side, cookie-based, with sliding renewal and a
  background sweep of expired sessions.
- CSRF protection via a stateless double-submit cookie.
- Login attempts are rate-limited per IP.
- The server shuts down gracefully (`SIGINT`/`SIGTERM`), draining in-flight
  requests before exiting.

See DESIGN.md's [Storage & auth](DESIGN.md#storage--auth) section for details.

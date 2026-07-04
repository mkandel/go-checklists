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
templates, groups, users, notifications) is functional and tested; things
like password reset, self-service registration, and a browser UI are not
built yet.

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
go test ./...
gofmt -l .
```

Postgres-layer tests spin up a real Postgres via
[testcontainers](https://golang.testcontainers.org/) — Docker must be running.

## Security

- Sessions are server-side, cookie-based, with sliding renewal and a
  background sweep of expired sessions.
- CSRF protection via a stateless double-submit cookie.
- Login attempts are rate-limited per IP.
- The server shuts down gracefully (`SIGINT`/`SIGTERM`), draining in-flight
  requests before exiting.

See DESIGN.md's [Storage & auth](DESIGN.md#storage--auth) section for details.

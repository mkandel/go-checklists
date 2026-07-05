# Postman collection

Manual test collection for the ChecklistHQ JSON API.

## Import

1. Postman → Import → select both `ChecklistHQ.postman_collection.json` and
   `ChecklistHQ.Local.postman_environment.json`.
2. Select the "ChecklistHQ Local" environment (top-right environment picker).
   Change `base_url` there if testing against something other than
   `http://localhost:8080` (e.g. a docker-compose or Caddy-fronted instance).

## Usage

Requests authenticate automatically — no need to run Login by hand. A
collection-level pre-request script logs in as `{{admin_username}}` /
`{{admin_password}}` (collection variables, defaulting to `admin` /
`password123` — the `cmd/seed` sample admin) the first time you run any
request, then stores the resulting `checklists_csrf` cookie into the
collection variable `csrf_token` and attaches it as `X-CSRF-Token` on every
subsequent non-GET request. The session cookie itself is handled by Postman's
own cookie jar — nothing to configure. This only works against a database
seeded via `go run ./cmd/seed` (e.g. the "Run Server (sample DB)" GoLand run
config); against a fresh/unseeded DB, either seed it first or run
**Auth > Register** to create a user (auto-login is skipped for the Auth
folder's own requests so it doesn't interfere).

To act as a different user, either change `admin_username`/`admin_password`
before the first request, or run **Auth > Login** by hand at any point with a
different body — its Tests script overwrites `csrf_token`, so later requests
pick up the new session automatically.

Endpoints under **Users**, **Templates** (create), **Groups** (create/mutate),
and **Tenant Admin** require the logged-in user to have `is_admin = true`.
There's no self-service admin promotion — flip the flag via the seed script
or a direct DB update if you need an admin account for testing.

IDs referenced by path (`checklist_id`, `item_id`, `user_id`, `group_id`,
`template_id`, `notification_id`) are plain collection variables — copy them
in from a prior response as you create/list resources; the collection doesn't
auto-chain them.

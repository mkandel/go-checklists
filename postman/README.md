# Postman collection

Manual test collection for the ChecklistHQ JSON API.

## Import

1. Postman → Import → select both `ChecklistHQ.postman_collection.json` and
   `ChecklistHQ.Local.postman_environment.json`.
2. Select the "ChecklistHQ Local" environment (top-right environment picker).
   Change `base_url` there if testing against something other than
   `http://localhost:8080` (e.g. a docker-compose or Caddy-fronted instance).

## Usage

Run **Auth > Register** (or **Auth > Login** if the user already exists)
first. The request's Tests script reads the `checklists_csrf` cookie Postman
just received and stores it in the collection variable `csrf_token`; a
collection-level pre-request script then attaches it as `X-CSRF-Token` on
every subsequent non-GET request automatically. The session cookie itself is
handled by Postman's own cookie jar — nothing to configure.

Endpoints under **Users**, **Templates** (create), **Groups** (create/mutate),
and **Tenant Admin** require the logged-in user to have `is_admin = true`.
There's no self-service admin promotion — flip the flag via the seed script
or a direct DB update if you need an admin account for testing.

IDs referenced by path (`checklist_id`, `item_id`, `user_id`, `group_id`,
`template_id`, `notification_id`) are plain collection variables — copy them
in from a prior response as you create/list resources; the collection doesn't
auto-chain them.

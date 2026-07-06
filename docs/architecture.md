# Architecture

Graphical companion to [DESIGN.md](../DESIGN.md#architecture) — kept here as a
standalone reference. These are [Mermaid](https://mermaid.js.org/) diagrams;
GitHub renders them inline, and most editors (GoLand, VS Code) do too with a
Mermaid plugin.

## Component / package structure

```mermaid
flowchart TB
    subgraph client["Client"]
        browser["Browser\nWEB_FRONTEND selects one:\nhtmx/Alpine.js UI (server-rendered)\nReact SPA / Qwik SPA (fetch() only)"]
    end

    subgraph cmd["cmd/checklists-server"]
        main["main.go\nconfig load, migrate,\ndefault-tenant provisioning,\nWEB_FRONTEND switch, two-mux composition,\nemail worker, dual server start/shutdown"]
    end

    subgraph mux["two *http.ServeMux (composed in main.go, one per port)"]
        withsession["api.WithSession\n(wraps each mux independently:\nauth, CSRF, logging)"]
        withcors["api.WithCORS\n(api mux only; fallback for\nreact/qwik dev servers not behind Caddy)"]
    end

    subgraph api["internal/api  (/api/*, JSON)"]
        apihandlers["handlers.go / checklists.go / templates.go /\ngroups.go / users.go / notifications.go /\ntenant_mail.go / tenant_checklist_policy.go"]
    end

    subgraph web["WEB_FRONTEND=server (default): internal/web  (/, server-rendered UI)"]
        webhandlers["checklists.go / templates_ui.go / groups.go /\nadmin_users.go / admin_mail.go /\nadmin_checklist_policy.go / notifications.go"]
        tmpl["templates.go\nGo html/template layout + funcMap\n({{appName}} = ChecklistHQ)"]
    end

    subgraph spas["WEB_FRONTEND=react / qwik: static SPA builds"]
        webreact["internal/webreact\nembeds web-react/dist\n(Vite + React, client fetch() only)"]
        webqwik["internal/webqwik\nembeds web-qwik/dist\n(Qwik SSG, client fetch() only)"]
    end

    subgraph authpkg["internal/auth"]
        session["session-based login\nbcrypt, sliding-renewal sessions"]
        limiter["LoginLimiter\nIP-keyed rate limiting"]
    end

    subgraph mailpkg["internal/mail"]
        smtp["net/smtp wrapper\nper-tenant SMTP config"]
    end

    subgraph domain["internal/domain (pure logic, no DB/HTTP imports)"]
        checklist["checklist.go\nstate machine, VisibleTo"]
        assignment["assignment.go\nclaim/reassign, group checks"]
        template["template.go\ntemplate -> checklist instantiation"]
        checklistcreation["checklist_creation.go\nCanCreateChecklist policy check"]
        ports["ports.go\nrepo interfaces (ChecklistRepo, UserRepo, ...)"]
    end

    subgraph store["internal/store/postgres"]
        repos["TenantStore, UserStore, GroupStore,\nTemplateStore, ChecklistStore,\nEventStore, NotificationStore, SessionStore"]
        migrations["migrations/\n00001_init.sql"]
    end

    subgraph db["Postgres"]
        tables[("tenants, users, groups, templates,\nchecklists, checklist_items,\nchecklist_events, notifications, sessions,\npassword_reset_tokens")]
    end

    browser -->|HTTP/JSON, cookies| withsession
    withsession --> withcors
    withcors --> apihandlers
    withsession --> webhandlers
    withsession --> webreact
    withsession --> webqwik
    webhandlers --> tmpl
    apihandlers --> session
    webhandlers --> session
    apihandlers --> limiter
    apihandlers -->|actor.TenantID +\ndomain structs| ports
    webhandlers -->|actor.TenantID +\ndomain structs| ports
    webreact -.fetch /api/*.-> apihandlers
    webqwik -.fetch /api/*.-> apihandlers
    ports -.implemented by.-> repos
    checklist --- ports
    assignment --- ports
    template --- ports
    checklistcreation --- ports
    repos --> tables
    migrations -.applied to.-> tables
    main --> withsession
    main --> repos
    main --> smtp
    repos -.email delivery.-> smtp
```

`internal/web` does not import `internal/api` (or vice versa) — each
registers its own routes onto its own mux (one per port) and implements its
own handler logic against the same `internal/domain`/`internal/store` layers,
even where that means near-duplicate code between the two packages.
`internal/webreact`/`internal/webqwik` are different in kind, not just
degree: they don't touch `internal/domain`/`internal/store` at all — they
serve a static build and talk to `internal/api` exclusively over HTTP, the
same as any other API client. The one exception to the "no cross-imports"
rule is auth: `main.go` calls the exported `api.RegisterAuthRoutes` on every
web-port mux so `/login`, `/register`, `/logout` work regardless of which
`WEB_FRONTEND` is active, without `internal/web`/`internal/webreact`/
`internal/webqwik` importing `internal/api` themselves. See
[DESIGN.md — Frontend](../DESIGN.md#frontend) for why.

## Request lifecycle (authenticated write)

Example: `POST /checklists/{id}/check` — an authenticated, CSRF-protected,
tenant-scoped state transition.

```mermaid
sequenceDiagram
    participant C as Client
    participant MW as Middleware<br/>(auth, CSRF, logging)
    participant H as Handler<br/>(checklists.go)
    participant Dom as domain<br/>(checklist.go)
    participant Repo as ChecklistStore<br/>(postgres)
    participant DB as Postgres

    C->>MW: POST /checklists/{id}/check<br/>Cookie: checklists_session<br/>X-CSRF-Token header
    MW->>MW: resolve session -> actor (*domain.User)<br/>validate X-CSRF-Token == checklists_csrf cookie
    MW->>H: r.Context() carries actor (incl. TenantID)
    H->>Repo: Get(ctx, actor.TenantID, id)  [FOR UPDATE]
    Repo->>DB: SELECT ... WHERE tenant_id = $1 AND id = $2 FOR UPDATE
    DB-->>Repo: checklist row
    Repo-->>H: *domain.Checklist
    H->>Dom: checklist.VisibleTo(actor), state transition rules
    Dom-->>H: ok / error
    H->>Repo: transaction: update checklist + insert checklist_event
    Repo->>DB: transaction: UPDATE checklists, INSERT checklist_events, COMMIT
    DB-->>Repo: ok
    Repo-->>H: ok
    H-->>C: 200 OK, updated checklist JSON
```

## Multi-tenancy data model

Composite `UNIQUE(tenant_id, id)` on each root table plus composite FKs from
every child table pin every row to a single tenant at the database level, not
just in application code. See [DESIGN.md — Multi-tenancy](../DESIGN.md#multi-tenancy)
for the full rationale.

```mermaid
erDiagram
    tenants ||--o{ users : "tenant_id"
    tenants ||--o{ groups : "tenant_id"
    tenants ||--o{ templates : "tenant_id"
    tenants ||--o{ checklists : "tenant_id"
    tenants ||--o{ notifications : "tenant_id"
    tenants ||--o{ checklist_events : "tenant_id"
    users ||--o{ user_groups : "member of"
    groups ||--o{ user_groups : "has member"
    templates ||--o{ template_items : "has item"
    templates ||--o{ checklists : "instantiated as"
    checklists ||--o{ checklist_items : "has item"
    checklists ||--o{ checklist_events : "audit log"
    users ||--o{ sessions : "authenticates"
    users ||--o{ password_reset_tokens : "requests reset"

    tenants {
        bigint id PK
        text name
        text slug UK
        text smtp_host
        int smtp_port
        text smtp_username
        text smtp_password
        text smtp_from_address
        boolean restrict_checklist_creation
        bigint checklist_creator_group_id FK
    }
    users {
        bigint id PK
        bigint tenant_id FK
        text username UK
        text email
        boolean is_admin
        boolean is_active
    }
    checklists {
        bigint id PK
        bigint tenant_id FK
        bigint template_id FK
        bigint creator_id FK
        bigint assigned_user_id FK
        bigint assigned_group_id FK
        bigint approver_id FK
        text status
        boolean hidden
    }
    notifications {
        bigint id PK
        bigint tenant_id FK
        bigint user_id FK
        text email_status
        int email_attempts
        text email_last_error
        timestamptz email_sent_at
    }
```

# Architecture

Graphical companion to [DESIGN.md](../DESIGN.md#architecture) — kept here as a
standalone reference. These are [Mermaid](https://mermaid.js.org/) diagrams;
GitHub renders them inline, and most editors (GoLand, VS Code) do too with a
Mermaid plugin.

## Component / package structure

```mermaid
flowchart TB
    subgraph client["Client"]
        browser["Browser (htmx/Alpine.js UI)\n/ JSON HTTP client"]
    end

    subgraph cmd["cmd/checklists-server"]
        main["main.go\nconfig load, migrate,\ndefault-tenant provisioning,\ntwo-mux composition, email worker,\ndual server start/shutdown"]
    end

    subgraph mux["two *http.ServeMux (composed in main.go, one per port)"]
        withsession["api.WithSession\n(wraps each mux independently:\nauth, CSRF, logging)"]
    end

    subgraph api["internal/api  (/api/*, JSON)"]
        apihandlers["handlers.go / checklists.go / templates.go /\ngroups.go / users.go / notifications.go /\ntenant_mail.go / tenant_checklist_policy.go"]
    end

    subgraph web["internal/web  (/, server-rendered UI)"]
        webhandlers["checklists.go / templates_ui.go / groups.go /\nadmin_users.go / admin_mail.go /\nadmin_checklist_policy.go / notifications.go"]
        tmpl["templates.go\nGo html/template layout + funcMap\n({{appName}} = ChecklistHQ)"]
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
        tables[("tenants, users, groups, templates,\nchecklists, checklist_items,\nchecklist_events, notifications, sessions")]
    end

    browser -->|HTTP/JSON, cookies| withsession
    withsession --> apihandlers
    withsession --> webhandlers
    webhandlers --> tmpl
    apihandlers --> session
    webhandlers --> session
    apihandlers --> limiter
    apihandlers -->|actor.TenantID +\ndomain structs| ports
    webhandlers -->|actor.TenantID +\ndomain structs| ports
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
even where that means near-duplicate code between the two packages. The one
exception is auth: `main.go` calls the exported `api.RegisterAuthRoutes` on
both muxes so `/login`, `/register`, `/logout` work on either port without
`internal/web` importing `internal/api`. See
[DESIGN.md — Frontend](../DESIGN.md#frontend) for why.

## Request lifecycle (authenticated write)

Example: `POST /checklists/{id}/check` — an authenticated, CSRF-protected,
tenant-scoped state transition.

```mermaid
sequenceDiagram
    participant C as Client
    participant MW as Middleware\n(auth, CSRF, logging)
    participant H as Handler\n(checklists.go)
    participant Dom as domain\n(checklist.go)
    participant Repo as ChecklistStore\n(postgres)
    participant DB as Postgres

    C->>MW: POST /checklists/{id}/check\nCookie: checklists_session\nX-CSRF-Token header
    MW->>MW: resolve session -> actor (*domain.User)\nvalidate X-CSRF-Token == checklists_csrf cookie
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

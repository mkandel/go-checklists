# Architecture

Graphical companion to [DESIGN.md](../DESIGN.md#architecture) — kept here as a
standalone reference. These are [Mermaid](https://mermaid.js.org/) diagrams;
GitHub renders them inline, and most editors (GoLand, VS Code) do too with a
Mermaid plugin.

## Component / package structure

```mermaid
flowchart TB
    subgraph client["Client"]
        browser["Browser / HTTP client"]
    end

    subgraph cmd["cmd/checklists-server"]
        main["main.go\nconfig load, migrate,\ndefault-tenant provisioning,\nserver start/shutdown"]
    end

    subgraph api["internal/api"]
        mux["NewMux\nroutes + middleware chain"]
        middleware["middleware.go\nauth, CSRF, logging"]
        handlers["handlers.go / checklists.go / templates.go /\ngroups.go / users.go / notifications.go"]
    end

    subgraph authpkg["internal/auth"]
        session["session-based login\nbcrypt, sliding-renewal sessions"]
        limiter["LoginLimiter\nIP-keyed rate limiting"]
    end

    subgraph domain["internal/domain (pure logic, no DB/HTTP imports)"]
        checklist["checklist.go\nstate machine, VisibleTo"]
        assignment["assignment.go\nclaim/reassign, group checks"]
        template["template.go\ntemplate -> checklist instantiation"]
        ports["ports.go\nrepo interfaces (ChecklistRepo, UserRepo, ...)"]
    end

    subgraph store["internal/store/postgres"]
        repos["TenantStore, UserStore, GroupStore,\nTemplateStore, ChecklistStore,\nEventStore, NotificationStore, SessionStore"]
        migrations["migrations/\n00001_init.sql"]
    end

    subgraph db["Postgres"]
        tables[("tenants, users, groups, templates,\nchecklists, checklist_items,\nchecklist_events, notifications, sessions")]
    end

    browser -->|HTTP/JSON, cookies| mux
    mux --> middleware --> handlers
    handlers --> session
    handlers --> limiter
    handlers -->|actor.TenantID +\ndomain structs| ports
    ports -.implemented by.-> repos
    checklist --- ports
    assignment --- ports
    template --- ports
    repos --> tables
    migrations -.applied to.-> tables
    main --> mux
    main --> repos
```

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
    Repo->>DB: BEGIN; UPDATE checklists ...; INSERT INTO checklist_events ...; COMMIT
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
```

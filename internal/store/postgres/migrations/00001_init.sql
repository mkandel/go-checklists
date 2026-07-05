-- +goose Up

CREATE TABLE tenants (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    -- SMTP config for this tenant's outbound email notifications. Email
    -- delivery is enabled for the tenant iff smtp_host IS NOT NULL. All
    -- nullable since a tenant may never configure email at all.
    smtp_host TEXT,
    smtp_port INT,
    smtp_username TEXT,
    smtp_password TEXT,
    smtp_from_address TEXT,
    restrict_checklist_creation BOOLEAN NOT NULL DEFAULT FALSE,
    checklist_creator_group_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    username TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    email TEXT,
    is_admin BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, username)
);

CREATE TABLE sessions (
    token TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE groups (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, name)
);

ALTER TABLE tenants
    ADD CONSTRAINT tenants_creator_group_fk
    FOREIGN KEY (id, checklist_creator_group_id) REFERENCES groups (tenant_id, id);

CREATE TABLE user_groups (
    user_id BIGINT NOT NULL REFERENCES users(id),
    group_id BIGINT NOT NULL REFERENCES groups(id),
    PRIMARY KEY (user_id, group_id)
);

CREATE TABLE templates (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, name, version)
);

CREATE TABLE template_items (
    id BIGSERIAL PRIMARY KEY,
    template_id BIGINT NOT NULL REFERENCES templates(id),
    name TEXT NOT NULL,
    position INT NOT NULL,
    validation_ref TEXT
);

CREATE TABLE checklists (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL REFERENCES tenants(id),
    template_id BIGINT,
    creator_id BIGINT NOT NULL,
    assigned_group_id BIGINT,
    assigned_user_id BIGINT,
    hidden BOOLEAN NOT NULL DEFAULT FALSE,
    approver_id BIGINT,
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'validating', 'complete')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT assignee_required CHECK (assigned_group_id IS NOT NULL OR assigned_user_id IS NOT NULL),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, template_id) REFERENCES templates(tenant_id, id),
    FOREIGN KEY (tenant_id, creator_id) REFERENCES users(tenant_id, id),
    FOREIGN KEY (tenant_id, assigned_group_id) REFERENCES groups(tenant_id, id),
    FOREIGN KEY (tenant_id, assigned_user_id) REFERENCES users(tenant_id, id),
    FOREIGN KEY (tenant_id, approver_id) REFERENCES users(tenant_id, id)
);

CREATE TABLE checklist_items (
    id BIGSERIAL PRIMARY KEY,
    checklist_id BIGINT NOT NULL REFERENCES checklists(id),
    name TEXT NOT NULL,
    position INT NOT NULL,
    checked BOOLEAN NOT NULL DEFAULT FALSE,
    checked_by BIGINT REFERENCES users(id),
    checked_at TIMESTAMPTZ,
    validation_ref TEXT,
    assignee_override_user_id BIGINT REFERENCES users(id),
    deleted_at TIMESTAMPTZ
);

CREATE TABLE checklist_events (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL REFERENCES tenants(id),
    checklist_id BIGINT NOT NULL,
    item_id BIGINT REFERENCES checklist_items(id),
    actor_user_id BIGINT NOT NULL,
    action TEXT NOT NULL,
    detail JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id, checklist_id) REFERENCES checklists(tenant_id, id),
    FOREIGN KEY (tenant_id, actor_user_id) REFERENCES users(tenant_id, id)
);

CREATE TABLE notifications (
    id BIGSERIAL PRIMARY KEY,
    tenant_id BIGINT NOT NULL REFERENCES tenants(id),
    recipient_user_id BIGINT NOT NULL,
    type TEXT NOT NULL,
    checklist_id BIGINT,
    actor_user_id BIGINT,
    message TEXT NOT NULL,
    read_at TIMESTAMPTZ,
    -- Email delivery outbox. email_status: pending (not yet attempted or
    -- still retrying), sent, failed (gave up after too many attempts), or
    -- skipped (no SMTP config / no recipient email / recipient deactivated
    -- -- states that will never succeed, distinct from a transient failure).
    email_status TEXT NOT NULL DEFAULT 'pending' CHECK (email_status IN ('pending', 'sent', 'failed', 'skipped')),
    email_attempts INT NOT NULL DEFAULT 0,
    email_last_error TEXT,
    email_sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id, recipient_user_id) REFERENCES users(tenant_id, id),
    FOREIGN KEY (tenant_id, checklist_id) REFERENCES checklists(tenant_id, id),
    FOREIGN KEY (tenant_id, actor_user_id) REFERENCES users(tenant_id, id)
);

-- Enforces that when a checklist is assigned to both a group and a specific
-- user, that user must actually belong to the group. Backstops the same
-- check performed in the domain layer.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION check_assignee_group_membership() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.assigned_group_id IS NOT NULL AND NEW.assigned_user_id IS NOT NULL THEN
        IF NOT EXISTS (
            SELECT 1 FROM user_groups
            WHERE user_id = NEW.assigned_user_id AND group_id = NEW.assigned_group_id
        ) THEN
            RAISE EXCEPTION 'assigned_user_id % is not a member of assigned_group_id %',
                NEW.assigned_user_id, NEW.assigned_group_id;
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_check_assignee_group_membership
    BEFORE INSERT OR UPDATE ON checklists
    FOR EACH ROW EXECUTE FUNCTION check_assignee_group_membership();

-- +goose Down

DROP TRIGGER IF EXISTS trg_check_assignee_group_membership ON checklists;
DROP FUNCTION IF EXISTS check_assignee_group_membership();
DROP TABLE notifications;
DROP TABLE checklist_events;
DROP TABLE checklist_items;
DROP TABLE checklists;
DROP TABLE template_items;
DROP TABLE templates;
DROP TABLE user_groups;
ALTER TABLE tenants DROP CONSTRAINT IF EXISTS tenants_creator_group_fk;
DROP TABLE groups;
DROP TABLE sessions;
DROP TABLE users;
DROP TABLE tenants;

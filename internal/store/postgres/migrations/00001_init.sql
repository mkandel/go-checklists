-- +goose Up

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    username TEXT NOT NULL UNIQUE,
    is_admin BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE groups (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_groups (
    user_id BIGINT NOT NULL REFERENCES users(id),
    group_id BIGINT NOT NULL REFERENCES groups(id),
    PRIMARY KEY (user_id, group_id)
);

CREATE TABLE templates (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (name, version)
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
    template_id BIGINT REFERENCES templates(id),
    creator_id BIGINT NOT NULL REFERENCES users(id),
    assigned_group_id BIGINT REFERENCES groups(id),
    assigned_user_id BIGINT REFERENCES users(id),
    hidden BOOLEAN NOT NULL DEFAULT FALSE,
    approver_id BIGINT REFERENCES users(id),
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'validating', 'complete')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT assignee_required CHECK (assigned_group_id IS NOT NULL OR assigned_user_id IS NOT NULL)
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
    assignee_override_user_id BIGINT REFERENCES users(id)
);

CREATE TABLE checklist_events (
    id BIGSERIAL PRIMARY KEY,
    checklist_id BIGINT NOT NULL REFERENCES checklists(id),
    item_id BIGINT REFERENCES checklist_items(id),
    actor_user_id BIGINT NOT NULL REFERENCES users(id),
    action TEXT NOT NULL,
    detail JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE notifications (
    id BIGSERIAL PRIMARY KEY,
    recipient_user_id BIGINT NOT NULL REFERENCES users(id),
    type TEXT NOT NULL,
    checklist_id BIGINT REFERENCES checklists(id),
    actor_user_id BIGINT REFERENCES users(id),
    message TEXT NOT NULL,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
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
DROP TABLE groups;
DROP TABLE users;

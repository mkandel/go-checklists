-- +goose Up

ALTER TABLE checklist_items ADD COLUMN deleted_at TIMESTAMPTZ;

-- +goose Down

ALTER TABLE checklist_items DROP COLUMN deleted_at;

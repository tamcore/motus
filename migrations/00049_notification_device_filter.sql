-- +goose Up
-- Optional device filter for notification rules: when non-empty, only
-- events of these devices trigger the rule. Empty means all devices.
ALTER TABLE notification_rules
    ADD COLUMN device_ids BIGINT[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE notification_rules DROP COLUMN IF EXISTS device_ids;

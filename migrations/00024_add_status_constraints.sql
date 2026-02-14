-- +goose Up
-- +goose StatementBegin
-- Add CHECK constraints for status columns to enforce valid values at database level

-- Notification log status constraint
ALTER TABLE notification_log ADD CONSTRAINT valid_notification_status
    CHECK (status IN ('pending', 'sent', 'failed'));

-- Command status constraint
ALTER TABLE commands ADD CONSTRAINT valid_command_status
    CHECK (status IN ('pending', 'sent', 'executed', 'failed'));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE notification_log DROP CONSTRAINT IF EXISTS valid_notification_status;
ALTER TABLE commands DROP CONSTRAINT IF EXISTS valid_command_status;
-- +goose StatementEnd

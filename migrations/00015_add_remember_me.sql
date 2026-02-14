-- +goose Up
-- Add remember_me flag to sessions for long-lived session support.
ALTER TABLE sessions ADD COLUMN remember_me BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE sessions DROP COLUMN IF EXISTS remember_me;

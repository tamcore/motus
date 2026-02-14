-- +goose Up
-- Add API token support for bearer authentication.
-- Tokens are used by Home Assistant and other integrations.

ALTER TABLE users ADD COLUMN token VARCHAR(255) UNIQUE;
CREATE INDEX idx_users_token ON users(token);

-- +goose Down
DROP INDEX IF EXISTS idx_users_token;
ALTER TABLE users DROP COLUMN IF EXISTS token;

-- +goose Up
-- +goose StatementBegin

-- Add optional expiration date to API keys.
-- NULL means the key never expires (backward compatible with existing keys).
ALTER TABLE api_keys ADD COLUMN expires_at TIMESTAMPTZ;

-- Index for efficient cleanup/filtering of expired keys.
CREATE INDEX idx_api_keys_expires_at ON api_keys(expires_at) WHERE expires_at IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_api_keys_expires_at;
ALTER TABLE api_keys DROP COLUMN IF EXISTS expires_at;
-- +goose StatementEnd

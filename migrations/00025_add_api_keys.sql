-- +goose Up
-- +goose StatementBegin

-- API keys table: supports multiple named keys per user with permission levels.
-- Replaces the single token column on the users table.
CREATE TABLE api_keys (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token       TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    permissions TEXT NOT NULL DEFAULT 'full' CHECK (permissions IN ('full', 'readonly')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ
);

CREATE INDEX idx_api_keys_token ON api_keys(token);
CREATE INDEX idx_api_keys_user_id ON api_keys(user_id);

-- Migrate existing user tokens into the api_keys table.
-- These become "full" permission keys named "Legacy Token".
INSERT INTO api_keys (user_id, token, name, permissions)
SELECT id, token, 'Legacy Token', 'full'
FROM users
WHERE token IS NOT NULL AND token != '';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS api_keys;
-- +goose StatementEnd

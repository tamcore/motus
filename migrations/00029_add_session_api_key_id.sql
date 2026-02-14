-- +goose Up
-- Track which API key (if any) was used to create a session. This allows the
-- auth middleware to enforce the API key's permission level (e.g. readonly)
-- on subsequent cookie-authenticated requests, closing a privilege escalation
-- where a readonly API key token could create an unrestricted session.
ALTER TABLE sessions ADD COLUMN api_key_id BIGINT REFERENCES api_keys(id) ON DELETE SET NULL;
CREATE INDEX idx_sessions_api_key_id ON sessions(api_key_id);

-- +goose Down
DROP INDEX IF EXISTS idx_sessions_api_key_id;
ALTER TABLE sessions DROP COLUMN IF EXISTS api_key_id;

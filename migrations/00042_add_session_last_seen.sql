-- +goose Up
ALTER TABLE sessions
  ADD COLUMN last_seen_at TIMESTAMP,
  ADD COLUMN last_seen_ip VARCHAR(45),
  ADD COLUMN last_seen_user_agent TEXT;

-- +goose Down
ALTER TABLE sessions
  DROP COLUMN IF EXISTS last_seen_at,
  DROP COLUMN IF EXISTS last_seen_ip,
  DROP COLUMN IF EXISTS last_seen_user_agent;

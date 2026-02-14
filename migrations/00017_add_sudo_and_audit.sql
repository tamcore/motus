-- +goose Up

-- Sudo session support: store original admin user ID when impersonating.
ALTER TABLE sessions ADD COLUMN original_user_id BIGINT REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE sessions ADD COLUMN is_sudo BOOLEAN NOT NULL DEFAULT FALSE;

-- Audit log for tracking admin actions and significant system events.
CREATE TABLE audit_log (
    id BIGSERIAL PRIMARY KEY,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    action VARCHAR(50) NOT NULL,
    resource_type VARCHAR(50),
    resource_id BIGINT,
    details JSONB,
    ip_address INET,
    user_agent TEXT
);

CREATE INDEX idx_audit_log_timestamp ON audit_log(timestamp);
CREATE INDEX idx_audit_log_user_id ON audit_log(user_id);
CREATE INDEX idx_audit_log_action ON audit_log(action);

-- +goose Down

DROP TABLE IF EXISTS audit_log;
ALTER TABLE sessions DROP COLUMN IF EXISTS is_sudo;
ALTER TABLE sessions DROP COLUMN IF EXISTS original_user_id;

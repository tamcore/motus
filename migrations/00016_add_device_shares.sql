-- +goose Up
-- Shareable device tracking links with token-based public access.
CREATE TABLE device_shares (
    id BIGSERIAL PRIMARY KEY,
    device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    token VARCHAR(64) NOT NULL UNIQUE,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_device_shares_token ON device_shares(token);
CREATE INDEX idx_device_shares_device_id ON device_shares(device_id);

-- +goose Down
DROP TABLE IF EXISTS device_shares;

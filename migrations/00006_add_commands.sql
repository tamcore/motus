-- +goose Up
-- Commands table for queuing and tracking device commands.
-- Supports both queued (pending) and immediate (sent) execution.

CREATE TABLE commands (
    id BIGSERIAL PRIMARY KEY,
    device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL,
    attributes JSONB,
    status VARCHAR(20) DEFAULT 'pending',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    executed_at TIMESTAMP
);

CREATE INDEX idx_commands_device_id ON commands(device_id);
CREATE INDEX idx_commands_status ON commands(status);

-- +goose Down
DROP TABLE IF EXISTS commands;

-- +goose Up
ALTER TABLE devices ADD COLUMN last_seen TIMESTAMP;

-- Set initial last_seen to updated_at for existing devices.
UPDATE devices SET last_seen = updated_at WHERE last_seen IS NULL;

-- Index for timeout queries (find online devices past cutoff).
CREATE INDEX idx_devices_last_seen ON devices(last_seen);

-- +goose Down
DROP INDEX IF EXISTS idx_devices_last_seen;
ALTER TABLE devices DROP COLUMN last_seen;

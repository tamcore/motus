-- +goose Up
-- Add 'moving' to the valid device status values.
-- This enables accurate motion reporting to Home Assistant and Traccar clients.
ALTER TABLE devices DROP CONSTRAINT IF EXISTS valid_device_status;
ALTER TABLE devices ADD CONSTRAINT valid_device_status
CHECK (status IN ('online', 'offline', 'unknown', 'moving'));

-- +goose Down
-- Revert devices with 'moving' status back to 'online' before restoring the old constraint.
UPDATE devices SET status = 'online' WHERE status = 'moving';
ALTER TABLE devices DROP CONSTRAINT IF EXISTS valid_device_status;
ALTER TABLE devices ADD CONSTRAINT valid_device_status
CHECK (status IN ('online', 'offline', 'unknown'));

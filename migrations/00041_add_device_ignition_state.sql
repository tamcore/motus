-- +goose Up
ALTER TABLE devices ADD COLUMN ignition_on BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE devices ADD COLUMN last_ignition_time TIMESTAMPTZ;

-- +goose Down
ALTER TABLE devices DROP COLUMN last_ignition_time;
ALTER TABLE devices DROP COLUMN ignition_on;

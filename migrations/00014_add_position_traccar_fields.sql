-- +goose Up

-- Add new Traccar-compatible position fields.
ALTER TABLE positions ADD COLUMN protocol VARCHAR(50);
ALTER TABLE positions ADD COLUMN server_time TIMESTAMP NOT NULL DEFAULT NOW();
ALTER TABLE positions ADD COLUMN device_time TIMESTAMP NOT NULL DEFAULT NOW();
ALTER TABLE positions ADD COLUMN valid BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE positions ADD COLUMN address TEXT;
ALTER TABLE positions ADD COLUMN accuracy NUMERIC(8,2);
ALTER TABLE positions ADD COLUMN network JSONB;
ALTER TABLE positions ADD COLUMN geofence_ids BIGINT[];
ALTER TABLE positions ADD COLUMN outdated BOOLEAN NOT NULL DEFAULT false;

-- Backfill server_time and device_time from existing timestamp column.
UPDATE positions SET server_time = timestamp, device_time = timestamp WHERE server_time = NOW();

-- +goose Down
ALTER TABLE positions DROP COLUMN IF EXISTS outdated;
ALTER TABLE positions DROP COLUMN IF EXISTS geofence_ids;
ALTER TABLE positions DROP COLUMN IF EXISTS network;
ALTER TABLE positions DROP COLUMN IF EXISTS accuracy;
ALTER TABLE positions DROP COLUMN IF EXISTS address;
ALTER TABLE positions DROP COLUMN IF EXISTS valid;
ALTER TABLE positions DROP COLUMN IF EXISTS device_time;
ALTER TABLE positions DROP COLUMN IF EXISTS server_time;
ALTER TABLE positions DROP COLUMN IF EXISTS protocol;

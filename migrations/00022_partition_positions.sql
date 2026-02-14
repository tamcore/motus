-- +goose Up
-- +goose NO TRANSACTION
--
-- Convert the positions table from a regular heap table to a
-- range-partitioned table partitioned by the "timestamp" column (monthly).
--
-- Key design decisions:
--   1. Foreign keys TO positions (devices.position_id, events.position_id)
--      must be dropped because PostgreSQL does not support foreign keys
--      referencing a partitioned table unless the FK includes the partition
--      key. Application-level referential integrity is maintained instead.
--   2. The foreign key FROM positions (device_id -> devices) is preserved
--      on each partition automatically.
--   3. The primary key changes from (id) to (id, timestamp) because the
--      partition key must be part of any unique/primary constraint.
--   4. Existing data is migrated into the partitioned table by creating
--      monthly partitions covering all existing data, then copying rows.
--   5. We create partitions for: all months with existing data + current
--      month + next 3 months. The Go partition manager handles future
--      partition creation automatically.
--
-- This migration uses NO TRANSACTION because:
--   - CREATE INDEX CONCURRENTLY requires it
--   - Individual DDL statements are each transactional

-- Step 1: Drop foreign keys that reference positions(id).
-- devices.position_id -> positions(id)
ALTER TABLE devices DROP CONSTRAINT IF EXISTS devices_position_id_fkey;
-- events.position_id -> positions(id)
ALTER TABLE events DROP CONSTRAINT IF EXISTS events_position_id_fkey;

-- Step 2: Rename the existing positions table to preserve data.
ALTER TABLE positions RENAME TO positions_old;

-- Step 3: Rename existing indexes so they don't conflict.
ALTER INDEX IF EXISTS idx_positions_device_id RENAME TO idx_positions_old_device_id;
ALTER INDEX IF EXISTS idx_positions_timestamp RENAME TO idx_positions_old_timestamp;
ALTER INDEX IF EXISTS idx_positions_device_timestamp RENAME TO idx_positions_old_device_timestamp;

-- Step 4: Create the new partitioned table with the same schema.
-- The primary key must include the partition key (timestamp).
CREATE TABLE positions (
    id BIGINT NOT NULL DEFAULT nextval('positions_id_seq'),
    device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    protocol VARCHAR(50),
    server_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    device_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    timestamp TIMESTAMPTZ NOT NULL,
    valid BOOLEAN NOT NULL DEFAULT true,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    altitude DOUBLE PRECISION,
    speed DOUBLE PRECISION,
    course DOUBLE PRECISION,
    address TEXT,
    accuracy NUMERIC(8,2),
    network JSONB,
    geofence_ids BIGINT[],
    outdated BOOLEAN NOT NULL DEFAULT false,
    attributes JSONB,
    PRIMARY KEY (id, timestamp)
) PARTITION BY RANGE (timestamp);

-- Step 5: Reassign the sequence to the new partitioned table.
ALTER SEQUENCE positions_id_seq OWNED BY positions.id;

-- Step 6: Create a default partition to catch any rows that don't match
-- a specific partition range (safety net).
CREATE TABLE positions_default PARTITION OF positions DEFAULT;

-- Step 7: Use a DO block to dynamically create monthly partitions covering
-- all existing data, plus the current month and 3 months into the future.
-- +goose StatementBegin
DO $$
DECLARE
    min_ts TIMESTAMPTZ;
    max_ts TIMESTAMPTZ;
    partition_start DATE;
    partition_end DATE;
    partition_name TEXT;
    future_limit DATE;
BEGIN
    -- Find the date range of existing data.
    SELECT MIN(timestamp), MAX(timestamp) INTO min_ts, max_ts FROM positions_old;

    -- If there is no existing data, start from the current month.
    IF min_ts IS NULL THEN
        min_ts := date_trunc('month', NOW());
    END IF;

    -- Create partitions from the earliest data month through 3 months in the future.
    future_limit := (date_trunc('month', NOW()) + INTERVAL '4 months')::DATE;
    partition_start := date_trunc('month', min_ts)::DATE;

    WHILE partition_start < future_limit LOOP
        partition_end := (partition_start + INTERVAL '1 month')::DATE;
        partition_name := 'positions_y' || to_char(partition_start, 'YYYY') || 'm' || to_char(partition_start, 'MM');

        -- Detach default partition temporarily so we can create the new range partition.
        ALTER TABLE positions DETACH PARTITION positions_default;

        EXECUTE format(
            'CREATE TABLE %I PARTITION OF positions FOR VALUES FROM (%L) TO (%L)',
            partition_name, partition_start, partition_end
        );

        -- Re-attach default partition.
        ALTER TABLE positions ATTACH PARTITION positions_default DEFAULT;

        partition_start := partition_end;
    END LOOP;
END $$;
-- +goose StatementEnd

-- Step 8: Copy existing data into the partitioned table.
-- PostgreSQL routes rows to the correct partition automatically.
INSERT INTO positions (
    id, device_id, protocol, server_time, device_time, timestamp, valid,
    latitude, longitude, altitude, speed, course, address, accuracy,
    network, geofence_ids, outdated, attributes
)
SELECT
    id, device_id, protocol, server_time, device_time, timestamp, valid,
    latitude, longitude, altitude, speed, course, address, accuracy,
    network, geofence_ids, outdated, attributes
FROM positions_old;

-- Step 9: Drop the old table (data has been migrated).
DROP TABLE positions_old;

-- Step 10: Create indexes on the partitioned table.
-- These will automatically propagate to all current and future partitions.
CREATE INDEX idx_positions_device_id ON positions(device_id);
CREATE INDEX idx_positions_timestamp ON positions(timestamp);
CREATE INDEX idx_positions_device_timestamp ON positions(device_id, timestamp DESC);

-- +goose Down
-- +goose NO TRANSACTION
--
-- Revert: convert back to a regular (non-partitioned) table and restore FKs.

-- Step 1: Rename partitioned table.
ALTER TABLE positions RENAME TO positions_partitioned;

-- Step 2: Rename indexes.
ALTER INDEX IF EXISTS idx_positions_device_id RENAME TO idx_positions_partitioned_device_id;
ALTER INDEX IF EXISTS idx_positions_timestamp RENAME TO idx_positions_partitioned_timestamp;
ALTER INDEX IF EXISTS idx_positions_device_timestamp RENAME TO idx_positions_partitioned_device_timestamp;

-- Step 3: Create regular table with original schema (id as sole PK).
CREATE TABLE positions (
    id BIGSERIAL PRIMARY KEY,
    device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    protocol VARCHAR(50),
    server_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    device_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    timestamp TIMESTAMPTZ NOT NULL,
    valid BOOLEAN NOT NULL DEFAULT true,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    altitude DOUBLE PRECISION,
    speed DOUBLE PRECISION,
    course DOUBLE PRECISION,
    address TEXT,
    accuracy NUMERIC(8,2),
    network JSONB,
    geofence_ids BIGINT[],
    outdated BOOLEAN NOT NULL DEFAULT false,
    attributes JSONB
);

-- Step 4: Copy data back.
INSERT INTO positions (
    id, device_id, protocol, server_time, device_time, timestamp, valid,
    latitude, longitude, altitude, speed, course, address, accuracy,
    network, geofence_ids, outdated, attributes
)
SELECT
    id, device_id, protocol, server_time, device_time, timestamp, valid,
    latitude, longitude, altitude, speed, course, address, accuracy,
    network, geofence_ids, outdated, attributes
FROM positions_partitioned;

-- Step 5: Sync the sequence.
SELECT setval('positions_id_seq', COALESCE((SELECT MAX(id) FROM positions), 1));

-- Step 6: Drop partitioned table (and all partitions).
DROP TABLE positions_partitioned CASCADE;

-- Step 7: Restore indexes.
CREATE INDEX idx_positions_device_id ON positions(device_id);
CREATE INDEX idx_positions_timestamp ON positions(timestamp);
CREATE INDEX idx_positions_device_timestamp ON positions(device_id, timestamp DESC);

-- Step 8: Restore foreign keys to positions(id).
ALTER TABLE devices
    ADD CONSTRAINT devices_position_id_fkey
    FOREIGN KEY (position_id) REFERENCES positions(id) ON DELETE SET NULL;

ALTER TABLE events
    ADD CONSTRAINT events_position_id_fkey
    FOREIGN KEY (position_id) REFERENCES positions(id) ON DELETE SET NULL;

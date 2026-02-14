-- +goose Up
-- Convert all TIMESTAMP columns to TIMESTAMPTZ for timezone safety.
-- Existing values are treated as UTC via the AT TIME ZONE 'UTC' clause.
-- audit_log.timestamp is already TIMESTAMPTZ (created in migration 00017).

-- users
ALTER TABLE users
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC';

-- devices
ALTER TABLE devices
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'UTC',
    ALTER COLUMN last_update TYPE TIMESTAMPTZ USING last_update AT TIME ZONE 'UTC';

-- positions
ALTER TABLE positions
    ALTER COLUMN timestamp TYPE TIMESTAMPTZ USING timestamp AT TIME ZONE 'UTC',
    ALTER COLUMN server_time TYPE TIMESTAMPTZ USING server_time AT TIME ZONE 'UTC',
    ALTER COLUMN device_time TYPE TIMESTAMPTZ USING device_time AT TIME ZONE 'UTC';

-- sessions
ALTER TABLE sessions
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN expires_at TYPE TIMESTAMPTZ USING expires_at AT TIME ZONE 'UTC';

-- events
ALTER TABLE events
    ALTER COLUMN timestamp TYPE TIMESTAMPTZ USING timestamp AT TIME ZONE 'UTC';

-- geofences
ALTER TABLE geofences
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'UTC';

-- commands
ALTER TABLE commands
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN executed_at TYPE TIMESTAMPTZ USING executed_at AT TIME ZONE 'UTC';

-- notification_rules
ALTER TABLE notification_rules
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'UTC';

-- notification_log
ALTER TABLE notification_log
    ALTER COLUMN sent_at TYPE TIMESTAMPTZ USING sent_at AT TIME ZONE 'UTC',
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC';

-- device_shares
ALTER TABLE device_shares
    ALTER COLUMN expires_at TYPE TIMESTAMPTZ USING expires_at AT TIME ZONE 'UTC',
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC';

-- +goose Down
-- Revert TIMESTAMPTZ back to TIMESTAMP (drops timezone info).

ALTER TABLE device_shares
    ALTER COLUMN created_at TYPE TIMESTAMP,
    ALTER COLUMN expires_at TYPE TIMESTAMP;

ALTER TABLE notification_log
    ALTER COLUMN created_at TYPE TIMESTAMP,
    ALTER COLUMN sent_at TYPE TIMESTAMP;

ALTER TABLE notification_rules
    ALTER COLUMN updated_at TYPE TIMESTAMP,
    ALTER COLUMN created_at TYPE TIMESTAMP;

ALTER TABLE commands
    ALTER COLUMN executed_at TYPE TIMESTAMP,
    ALTER COLUMN created_at TYPE TIMESTAMP;

ALTER TABLE geofences
    ALTER COLUMN updated_at TYPE TIMESTAMP,
    ALTER COLUMN created_at TYPE TIMESTAMP;

ALTER TABLE events
    ALTER COLUMN timestamp TYPE TIMESTAMP;

ALTER TABLE sessions
    ALTER COLUMN expires_at TYPE TIMESTAMP,
    ALTER COLUMN created_at TYPE TIMESTAMP;

ALTER TABLE positions
    ALTER COLUMN device_time TYPE TIMESTAMP,
    ALTER COLUMN server_time TYPE TIMESTAMP,
    ALTER COLUMN timestamp TYPE TIMESTAMP;

ALTER TABLE devices
    ALTER COLUMN last_update TYPE TIMESTAMP,
    ALTER COLUMN updated_at TYPE TIMESTAMP,
    ALTER COLUMN created_at TYPE TIMESTAMP;

ALTER TABLE users
    ALTER COLUMN created_at TYPE TIMESTAMP;

-- +goose Up

-- Rename last_seen to last_update for Traccar compatibility.
ALTER TABLE devices RENAME COLUMN last_seen TO last_update;

-- Add new Traccar-compatible device fields.
ALTER TABLE devices ADD COLUMN position_id BIGINT REFERENCES positions(id) ON DELETE SET NULL;
ALTER TABLE devices ADD COLUMN group_id BIGINT;
ALTER TABLE devices ADD COLUMN phone VARCHAR(255);
ALTER TABLE devices ADD COLUMN model VARCHAR(255);
ALTER TABLE devices ADD COLUMN contact VARCHAR(255);
ALTER TABLE devices ADD COLUMN category VARCHAR(255);
ALTER TABLE devices ADD COLUMN disabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE devices ADD COLUMN attributes JSONB;

-- +goose Down
ALTER TABLE devices DROP COLUMN IF EXISTS attributes;
ALTER TABLE devices DROP COLUMN IF EXISTS disabled;
ALTER TABLE devices DROP COLUMN IF EXISTS category;
ALTER TABLE devices DROP COLUMN IF EXISTS contact;
ALTER TABLE devices DROP COLUMN IF EXISTS model;
ALTER TABLE devices DROP COLUMN IF EXISTS phone;
ALTER TABLE devices DROP COLUMN IF EXISTS group_id;
ALTER TABLE devices DROP COLUMN IF EXISTS position_id;
ALTER TABLE devices RENAME COLUMN last_update TO last_seen;

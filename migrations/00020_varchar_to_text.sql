-- +goose Up
-- Replace arbitrary VARCHAR(255) limits with TEXT for flexibility.
-- Bounded domain values (status, type, role) and fixed-length tokens
-- retain their VARCHAR constraints. password_hash is tightened to
-- VARCHAR(60) to match the bcrypt output length exactly.

-- users: email, name, token to TEXT; password_hash to VARCHAR(60)
ALTER TABLE users
    ALTER COLUMN email TYPE TEXT,
    ALTER COLUMN name TYPE TEXT,
    ALTER COLUMN token TYPE TEXT,
    ALTER COLUMN password_hash TYPE VARCHAR(60);

-- devices: name, phone, model, contact, category to TEXT
-- unique_id stays VARCHAR(255) (protocol-specific identifier)
ALTER TABLE devices
    ALTER COLUMN name TYPE TEXT,
    ALTER COLUMN phone TYPE TEXT,
    ALTER COLUMN model TYPE TEXT,
    ALTER COLUMN contact TYPE TEXT,
    ALTER COLUMN category TYPE TEXT;

-- geofences: name to TEXT
ALTER TABLE geofences
    ALTER COLUMN name TYPE TEXT;

-- notification_rules: name to TEXT
ALTER TABLE notification_rules
    ALTER COLUMN name TYPE TEXT;

-- +goose Down
-- Revert TEXT columns back to VARCHAR(255) and password_hash to VARCHAR(255).

ALTER TABLE notification_rules
    ALTER COLUMN name TYPE VARCHAR(255);

ALTER TABLE geofences
    ALTER COLUMN name TYPE VARCHAR(255);

ALTER TABLE devices
    ALTER COLUMN category TYPE VARCHAR(255),
    ALTER COLUMN contact TYPE VARCHAR(255),
    ALTER COLUMN model TYPE VARCHAR(255),
    ALTER COLUMN phone TYPE VARCHAR(255),
    ALTER COLUMN name TYPE VARCHAR(255);

ALTER TABLE users
    ALTER COLUMN password_hash TYPE VARCHAR(255),
    ALTER COLUMN token TYPE VARCHAR(255),
    ALTER COLUMN name TYPE VARCHAR(255),
    ALTER COLUMN email TYPE VARCHAR(255);

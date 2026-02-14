-- +goose Up
ALTER TABLE commands ADD COLUMN result TEXT;

-- Extend command type allowlist to include 'custom' for raw device commands.
ALTER TABLE commands DROP CONSTRAINT IF EXISTS valid_command_type;
ALTER TABLE commands ADD CONSTRAINT valid_command_type
    CHECK (type IN ('rebootDevice', 'positionPeriodic', 'positionSingle', 'sosNumber', 'custom'));

-- +goose Down
ALTER TABLE commands DROP CONSTRAINT IF EXISTS valid_command_type;
ALTER TABLE commands ADD CONSTRAINT valid_command_type
    CHECK (type IN ('rebootDevice', 'positionPeriodic', 'positionSingle', 'sosNumber'));
ALTER TABLE commands DROP COLUMN result;

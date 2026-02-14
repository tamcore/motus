-- +goose Up
-- Convert notification_rules.event_type (single string) to event_types (text array).
-- Existing rows are migrated by wrapping the scalar value in a one-element array.

-- Drop old B-tree index on the scalar column.
DROP INDEX IF EXISTS idx_notification_rules_event_type;

-- Convert column: VARCHAR → TEXT[] using ARRAY[old_value].
ALTER TABLE notification_rules
    ALTER COLUMN event_type TYPE TEXT[] USING ARRAY[event_type];

-- Rename column to reflect plural semantics.
ALTER TABLE notification_rules
    RENAME COLUMN event_type TO event_types;

-- GIN index for efficient ANY() lookups.
CREATE INDEX idx_notification_rules_event_types ON notification_rules USING GIN (event_types);

-- +goose Down
-- Revert: take the first array element back to a scalar VARCHAR column.
DROP INDEX IF EXISTS idx_notification_rules_event_types;

ALTER TABLE notification_rules
    RENAME COLUMN event_types TO event_type;

ALTER TABLE notification_rules
    ALTER COLUMN event_type TYPE VARCHAR(50) USING event_type[1];

CREATE INDEX idx_notification_rules_event_type ON notification_rules (event_type);

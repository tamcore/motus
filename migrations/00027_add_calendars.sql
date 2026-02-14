-- +goose Up

-- Calendars table for time-based geofence triggers.
-- Stores iCalendar (RFC 5545) data that defines when geofences are active.
CREATE TABLE calendars (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    data TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_calendars_user_id ON calendars(user_id);

-- Add calendar_id foreign key to geofences table.
-- A geofence with a calendar_id only triggers events when the current time
-- matches the calendar schedule. NULL means the geofence is always active.
ALTER TABLE geofences ADD COLUMN calendar_id BIGINT REFERENCES calendars(id) ON DELETE SET NULL;

-- Add calendar_id and expiration_time to devices (Traccar compatibility).
ALTER TABLE devices ADD COLUMN calendar_id BIGINT REFERENCES calendars(id) ON DELETE SET NULL;
ALTER TABLE devices ADD COLUMN expiration_time TIMESTAMPTZ;

-- User-calendar association for sharing calendars across users.
CREATE TABLE user_calendars (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    calendar_id BIGINT NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, calendar_id)
);

CREATE INDEX idx_user_calendars_user_id ON user_calendars(user_id);
CREATE INDEX idx_user_calendars_calendar_id ON user_calendars(calendar_id);

-- +goose Down
DROP TABLE IF EXISTS user_calendars;
ALTER TABLE devices DROP COLUMN IF EXISTS expiration_time;
ALTER TABLE devices DROP COLUMN IF EXISTS calendar_id;
ALTER TABLE geofences DROP COLUMN IF EXISTS calendar_id;
DROP TABLE IF EXISTS calendars;

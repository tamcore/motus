-- +goose Up

-- Add speed_limit column to devices for overspeed detection.
ALTER TABLE devices ADD COLUMN speed_limit NUMERIC(5,2);

-- Update event type constraint to include deviceIdle.
ALTER TABLE events DROP CONSTRAINT IF EXISTS valid_event_type;
ALTER TABLE events ADD CONSTRAINT valid_event_type
CHECK (type IN ('geofenceEnter', 'geofenceExit', 'deviceOnline', 'deviceOffline', 'overspeed', 'motion', 'deviceIdle'));

-- +goose Down
ALTER TABLE events DROP CONSTRAINT IF EXISTS valid_event_type;
ALTER TABLE events ADD CONSTRAINT valid_event_type
CHECK (type IN ('geofenceEnter', 'geofenceExit', 'deviceOnline', 'deviceOffline', 'overspeed', 'motion'));

ALTER TABLE devices DROP COLUMN IF EXISTS speed_limit;

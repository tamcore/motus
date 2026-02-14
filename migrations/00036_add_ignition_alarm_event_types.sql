-- +goose Up

-- Add ignitionOn, ignitionOff, and alarm to the valid event types.
-- These were added by the IgnitionService and AlarmService but the
-- CHECK constraint was never updated (last changed in migration 00012).
ALTER TABLE events DROP CONSTRAINT IF EXISTS valid_event_type;
ALTER TABLE events ADD CONSTRAINT valid_event_type
CHECK (type IN ('geofenceEnter', 'geofenceExit', 'deviceOnline', 'deviceOffline', 'overspeed', 'motion', 'deviceIdle', 'ignitionOn', 'ignitionOff', 'alarm'));

-- +goose Down
ALTER TABLE events DROP CONSTRAINT IF EXISTS valid_event_type;
ALTER TABLE events ADD CONSTRAINT valid_event_type
CHECK (type IN ('geofenceEnter', 'geofenceExit', 'deviceOnline', 'deviceOffline', 'overspeed', 'motion', 'deviceIdle'));

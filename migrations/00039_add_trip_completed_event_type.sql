-- +goose Up

-- Add tripCompleted to the valid event types. The MileageService creates
-- this event when a trip ends (committed pending mileage), but migration
-- 00038 added the device.mileage / device.pending_mileage columns without
-- updating the events check constraint, so every trip commit logs
-- SQLSTATE 23514 and the event is dropped.
ALTER TABLE events DROP CONSTRAINT IF EXISTS valid_event_type;
ALTER TABLE events ADD CONSTRAINT valid_event_type
CHECK (type IN ('geofenceEnter', 'geofenceExit', 'deviceOnline', 'deviceOffline', 'overspeed', 'motion', 'deviceIdle', 'ignitionOn', 'ignitionOff', 'alarm', 'tripCompleted'));

-- +goose Down
ALTER TABLE events DROP CONSTRAINT IF EXISTS valid_event_type;
ALTER TABLE events ADD CONSTRAINT valid_event_type
CHECK (type IN ('geofenceEnter', 'geofenceExit', 'deviceOnline', 'deviceOffline', 'overspeed', 'motion', 'deviceIdle', 'ignitionOn', 'ignitionOff', 'alarm'));

-- +goose Up

-- Command type allowlist constraint.
ALTER TABLE commands ADD CONSTRAINT valid_command_type
CHECK (type IN ('rebootDevice', 'positionPeriodic', 'positionSingle', 'sosNumber'));

-- Device status constraint.
ALTER TABLE devices ADD CONSTRAINT valid_device_status
CHECK (status IN ('online', 'offline', 'unknown'));

-- Event type constraint.
ALTER TABLE events ADD CONSTRAINT valid_event_type
CHECK (type IN ('geofenceEnter', 'geofenceExit', 'deviceOnline', 'deviceOffline', 'overspeed', 'motion'));

-- Coordinate bounds validation.
ALTER TABLE positions ADD CONSTRAINT valid_latitude CHECK (latitude >= -90 AND latitude <= 90);
ALTER TABLE positions ADD CONSTRAINT valid_longitude CHECK (longitude >= -180 AND longitude <= 180);

-- +goose Down
ALTER TABLE commands DROP CONSTRAINT IF EXISTS valid_command_type;
ALTER TABLE devices DROP CONSTRAINT IF EXISTS valid_device_status;
ALTER TABLE events DROP CONSTRAINT IF EXISTS valid_event_type;
ALTER TABLE positions DROP CONSTRAINT IF EXISTS valid_latitude;
ALTER TABLE positions DROP CONSTRAINT IF EXISTS valid_longitude;

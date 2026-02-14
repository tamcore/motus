-- +goose Up

-- Geofences with PostGIS geometry for spatial queries.
CREATE TABLE geofences (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT DEFAULT '',
    geometry GEOMETRY(GEOMETRY, 4326) NOT NULL,
    attributes JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Spatial index for fast containment queries (ST_Contains, ST_Intersects).
CREATE INDEX idx_geofences_geometry ON geofences USING GIST (geometry);

-- User-geofence association (same pattern as user_devices).
CREATE TABLE user_geofences (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    geofence_id BIGINT NOT NULL REFERENCES geofences(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, geofence_id)
);

CREATE INDEX idx_user_geofences_user_id ON user_geofences(user_id);
CREATE INDEX idx_user_geofences_geofence_id ON user_geofences(geofence_id);

-- Events table for tracking geofence enter/exit and other device events.
CREATE TABLE events (
    id BIGSERIAL PRIMARY KEY,
    device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    geofence_id BIGINT REFERENCES geofences(id) ON DELETE SET NULL,
    type VARCHAR(50) NOT NULL,
    position_id BIGINT REFERENCES positions(id) ON DELETE SET NULL,
    timestamp TIMESTAMP NOT NULL DEFAULT NOW(),
    attributes JSONB
);

CREATE INDEX idx_events_device_id ON events(device_id);
CREATE INDEX idx_events_geofence_id ON events(geofence_id);
CREATE INDEX idx_events_type ON events(type);
CREATE INDEX idx_events_timestamp ON events(timestamp);

-- +goose Down
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS user_geofences;
DROP TABLE IF EXISTS geofences;

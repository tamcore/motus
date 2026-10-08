-- +goose Up
-- Geofences attached to devices (many-to-many). Opt-in filter: a device with
-- at least one attached geofence is only checked against those geofences; a
-- device without attachments is checked against all geofences of its users.
CREATE TABLE device_geofences (
    device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    geofence_id BIGINT NOT NULL REFERENCES geofences(id) ON DELETE CASCADE,
    PRIMARY KEY (device_id, geofence_id)
);

CREATE INDEX idx_device_geofences_geofence_id ON device_geofences(geofence_id);

-- +goose Down
DROP TABLE IF EXISTS device_geofences;

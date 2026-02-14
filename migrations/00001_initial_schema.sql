-- +goose Up
-- Core schema for Motus GPS tracking system.
-- Uses double precision for lat/lon (PostGIS can be added later if needed).

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE devices (
    id BIGSERIAL PRIMARY KEY,
    unique_id VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL DEFAULT '',
    protocol VARCHAR(50) NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'unknown',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE positions (
    id BIGSERIAL PRIMARY KEY,
    device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    altitude DOUBLE PRECISION,
    speed DOUBLE PRECISION,
    course DOUBLE PRECISION,
    timestamp TIMESTAMP NOT NULL,
    attributes JSONB
);

CREATE INDEX idx_positions_device_id ON positions(device_id);
CREATE INDEX idx_positions_timestamp ON positions(timestamp);
CREATE INDEX idx_positions_device_timestamp ON positions(device_id, timestamp DESC);

CREATE TABLE sessions (
    id VARCHAR(64) PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP NOT NULL
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);

CREATE TABLE user_devices (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, device_id)
);

-- +goose Down
DROP TABLE IF EXISTS user_devices;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS positions;
DROP TABLE IF EXISTS devices;
DROP TABLE IF EXISTS users;

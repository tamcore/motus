-- +goose Up
-- +goose NO TRANSACTION

-- Foreign key indexes (CRITICAL for JOIN performance).
-- events.position_id is used in event-position joins but has no index.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_events_position_id
    ON events(position_id);

-- devices.position_id is used for latest-position lookups but has no index.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_devices_position_id
    ON devices(position_id);

-- Query performance indexes.
-- notification_log.created_at for time-range queries on notification history.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notification_log_created_at
    ON notification_log(created_at);

-- audit_log cursor pagination: (timestamp DESC, id DESC) for keyset paging.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audit_log_cursor
    ON audit_log(timestamp DESC, id DESC);

-- devices(status) with INCLUDE(id) for statistics GROUP BY status queries.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_devices_status_include_id
    ON devices(status) INCLUDE (id);

-- user_devices(device_id, user_id) for reverse lookups joining on device_id.
-- The PK (user_id, device_id) only helps user_id-first queries.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_user_devices_device_user
    ON user_devices(device_id, user_id);

-- +goose Down
-- +goose NO TRANSACTION

DROP INDEX CONCURRENTLY IF EXISTS idx_user_devices_device_user;
DROP INDEX CONCURRENTLY IF EXISTS idx_devices_status_include_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_audit_log_cursor;
DROP INDEX CONCURRENTLY IF EXISTS idx_notification_log_created_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_devices_position_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_events_position_id;

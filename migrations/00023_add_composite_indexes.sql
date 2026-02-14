-- +goose Up
-- +goose NO TRANSACTION
-- Add composite indexes for query performance optimization
-- These indexes are created CONCURRENTLY to avoid locking tables in production

-- Index for event filtering by device + type + time
-- Covers: GetByDevice, GetRecentByDeviceAndType, GetByFilters queries
-- Replaces need for separate device_id and type indexes in combination
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_events_device_type_timestamp
    ON events (device_id, type, timestamp DESC);

-- Partial index for notification rule dispatch by user and event type
-- Only indexes enabled rules (smaller, faster)
-- Covers: GetByEventType query for notification dispatch
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notification_rules_user_event_enabled
    ON notification_rules (user_id, event_type) WHERE enabled = true;

-- Partial index for pending command lookup by device
-- Only indexes pending commands (most commands eventually transition out)
-- Covers: GetPendingByDevice query
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_commands_device_pending
    ON commands (device_id) WHERE status = 'pending';

-- +goose Down
-- +goose NO TRANSACTION
DROP INDEX CONCURRENTLY IF EXISTS idx_events_device_type_timestamp;
DROP INDEX CONCURRENTLY IF EXISTS idx_notification_rules_user_event_enabled;
DROP INDEX CONCURRENTLY IF EXISTS idx_commands_device_pending;

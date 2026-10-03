-- +goose Up
-- Optional geofence filter for geofenceEnter/geofenceExit rules. An empty
-- array means the rule applies to all geofences. Deleted geofences are not
-- removed from the array on purpose: a rule filtered to a deleted geofence
-- must stop firing rather than silently broaden to all geofences.
ALTER TABLE notification_rules
    ADD COLUMN geofence_ids BIGINT[] NOT NULL DEFAULT '{}';

-- New "command" channel: send a device command to the triggering device.
ALTER TABLE notification_rules
    DROP CONSTRAINT IF EXISTS chk_notification_rules_channel;
ALTER TABLE notification_rules
    ADD CONSTRAINT chk_notification_rules_channel CHECK (channel IN ('webhook', 'command'));

-- Command deliveries for offline devices are logged as "queued" (the command
-- waits in the pending queue until the device reconnects).
ALTER TABLE notification_log
    DROP CONSTRAINT IF EXISTS valid_notification_status;
ALTER TABLE notification_log
    ADD CONSTRAINT valid_notification_status
    CHECK (status IN ('pending', 'sent', 'failed', 'queued'));

-- +goose Down
-- "queued" is not allowed by the previous constraint; the closest previous
-- status is "pending". (Command rules and their logs are deleted below, but
-- map the status first so the constraint can be restored in any case.)
UPDATE notification_log SET status = 'pending' WHERE status = 'queued';

ALTER TABLE notification_log
    DROP CONSTRAINT IF EXISTS valid_notification_status;
ALTER TABLE notification_log
    ADD CONSTRAINT valid_notification_status
    CHECK (status IN ('pending', 'sent', 'failed'));

-- notification_log rows of command rules are removed by ON DELETE CASCADE.
DELETE FROM notification_rules WHERE channel = 'command';

ALTER TABLE notification_rules
    DROP CONSTRAINT IF EXISTS chk_notification_rules_channel;
ALTER TABLE notification_rules
    ADD CONSTRAINT chk_notification_rules_channel CHECK (channel IN ('webhook'));

ALTER TABLE notification_rules
    DROP COLUMN IF EXISTS geofence_ids;

-- +goose Up
-- Convert any existing ntfy notification rules to webhook.
-- The ntfy topic+server are combined into a webhook URL, and any ntfy-specific
-- config fields (title, priority, tags) are moved into webhook headers.
UPDATE notification_rules
SET
    channel = 'webhook',
    config = jsonb_build_object(
        'webhookUrl',
        COALESCE(config->>'server', 'https://ntfy.sh') || '/' || COALESCE(config->>'topic', ''),
        'headers',
        jsonb_strip_nulls(jsonb_build_object(
            'Title', config->>'title',
            'Priority', config->>'priority',
            'Tags', config->>'tags'
        ))
    )
WHERE channel = 'ntfy';

-- Add a CHECK constraint so ntfy can never be used again.
ALTER TABLE notification_rules
    ADD CONSTRAINT chk_notification_rules_channel CHECK (channel IN ('webhook'));

-- +goose Down
ALTER TABLE notification_rules
    DROP CONSTRAINT IF EXISTS chk_notification_rules_channel;

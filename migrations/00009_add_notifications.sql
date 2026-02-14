-- +goose Up
CREATE TABLE notification_rules (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    event_type VARCHAR(50) NOT NULL,
    channel VARCHAR(20) NOT NULL,
    config JSONB NOT NULL,
    template TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notification_rules_user_id ON notification_rules(user_id);
CREATE INDEX idx_notification_rules_event_type ON notification_rules(event_type);
CREATE INDEX idx_notification_rules_enabled ON notification_rules(enabled);

CREATE TABLE notification_log (
    id BIGSERIAL PRIMARY KEY,
    rule_id BIGINT NOT NULL REFERENCES notification_rules(id) ON DELETE CASCADE,
    event_id BIGINT REFERENCES events(id) ON DELETE SET NULL,
    status VARCHAR(20) NOT NULL,
    sent_at TIMESTAMP,
    error TEXT,
    response_code INT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notification_log_rule_id ON notification_log(rule_id);
CREATE INDEX idx_notification_log_event_id ON notification_log(event_id);
CREATE INDEX idx_notification_log_status ON notification_log(status);

-- +goose Down
DROP TABLE IF EXISTS notification_log;
DROP TABLE IF EXISTS notification_rules;

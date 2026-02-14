package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamcore/motus/internal/model"
)

// NotificationRepository handles notification rule and log persistence.
type NotificationRepository struct {
	pool *pgxpool.Pool
}

// NewNotificationRepository creates a new notification repository.
func NewNotificationRepository(pool *pgxpool.Pool) *NotificationRepository {
	return &NotificationRepository{pool: pool}
}

// Create inserts a new notification rule.
func (r *NotificationRepository) Create(ctx context.Context, rule *model.NotificationRule) error {
	configJSON, err := json.Marshal(rule.Config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	err = r.pool.QueryRow(ctx, `
		INSERT INTO notification_rules (user_id, name, event_types, channel, config, template, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		RETURNING id, created_at, updated_at
	`, rule.UserID, rule.Name, rule.EventTypes, rule.Channel, configJSON, rule.Template, rule.Enabled).
		Scan(&rule.ID, &rule.CreatedAt, &rule.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create notification rule: %w", err)
	}
	return nil
}

// GetByID retrieves a single notification rule by its ID.
func (r *NotificationRepository) GetByID(ctx context.Context, id int64) (*model.NotificationRule, error) {
	var rule model.NotificationRule
	var configJSON []byte

	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, name, event_types, channel, config, template, enabled, created_at, updated_at
		FROM notification_rules
		WHERE id = $1
	`, id).Scan(
		&rule.ID, &rule.UserID, &rule.Name, &rule.EventTypes, &rule.Channel,
		&configJSON, &rule.Template, &rule.Enabled, &rule.CreatedAt, &rule.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get notification rule by id: %w", err)
	}
	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &rule.Config); err != nil {
			return nil, fmt.Errorf("unmarshal config: %w", err)
		}
	}
	return &rule, nil
}

// GetByUser retrieves all notification rules for a user.
func (r *NotificationRepository) GetByUser(ctx context.Context, userID int64) ([]*model.NotificationRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, name, event_types, channel, config, template, enabled, created_at, updated_at
		FROM notification_rules
		WHERE user_id = $1
		ORDER BY name
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get notification rules by user: %w", err)
	}
	defer rows.Close()

	rules := make([]*model.NotificationRule, 0, 16)
	for rows.Next() {
		var rule model.NotificationRule
		var configJSON []byte
		if err := rows.Scan(
			&rule.ID, &rule.UserID, &rule.Name, &rule.EventTypes, &rule.Channel,
			&configJSON, &rule.Template, &rule.Enabled, &rule.CreatedAt, &rule.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan notification rule: %w", err)
		}
		if len(configJSON) > 0 {
			if err := json.Unmarshal(configJSON, &rule.Config); err != nil {
				slog.Warn("failed to unmarshal notification config",
					slog.Int64("ruleID", rule.ID),
					slog.Any("error", err))
				rule.Config = make(map[string]interface{})
			}
		}
		rules = append(rules, &rule)
	}
	return rules, rows.Err()
}

// GetAll retrieves all notification rules with owner names.
func (r *NotificationRepository) GetAll(ctx context.Context) ([]*model.NotificationRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT nr.id, nr.user_id, nr.name, nr.event_types, nr.channel, nr.config, nr.template, nr.enabled, nr.created_at, nr.updated_at,
			COALESCE(u.name, '') AS owner_name
		FROM notification_rules nr
		LEFT JOIN users u ON u.id = nr.user_id
		ORDER BY nr.name
	`)
	if err != nil {
		return nil, fmt.Errorf("get all notification rules: %w", err)
	}
	defer rows.Close()

	rules := make([]*model.NotificationRule, 0, 16)
	for rows.Next() {
		var rule model.NotificationRule
		var configJSON []byte
		if err := rows.Scan(
			&rule.ID, &rule.UserID, &rule.Name, &rule.EventTypes, &rule.Channel,
			&configJSON, &rule.Template, &rule.Enabled, &rule.CreatedAt, &rule.UpdatedAt,
			&rule.OwnerName,
		); err != nil {
			return nil, fmt.Errorf("scan notification rule: %w", err)
		}
		if len(configJSON) > 0 {
			if err := json.Unmarshal(configJSON, &rule.Config); err != nil {
				slog.Warn("failed to unmarshal notification config",
					slog.Int64("ruleID", rule.ID),
					slog.Any("error", err))
				rule.Config = make(map[string]interface{})
			}
		}
		rules = append(rules, &rule)
	}
	return rules, rows.Err()
}

// GetByEventType retrieves enabled notification rules for a user matching a given event type.
func (r *NotificationRepository) GetByEventType(ctx context.Context, userID int64, eventType string) ([]*model.NotificationRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, name, event_types, channel, config, template, enabled, created_at, updated_at
		FROM notification_rules
		WHERE user_id = $1 AND $2 = ANY(event_types) AND enabled = true
	`, userID, eventType)
	if err != nil {
		return nil, fmt.Errorf("get notification rules by event type: %w", err)
	}
	defer rows.Close()

	rules := make([]*model.NotificationRule, 0, 16)
	for rows.Next() {
		var rule model.NotificationRule
		var configJSON []byte
		if err := rows.Scan(
			&rule.ID, &rule.UserID, &rule.Name, &rule.EventTypes, &rule.Channel,
			&configJSON, &rule.Template, &rule.Enabled, &rule.CreatedAt, &rule.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan notification rule: %w", err)
		}
		if len(configJSON) > 0 {
			if err := json.Unmarshal(configJSON, &rule.Config); err != nil {
				slog.Warn("failed to unmarshal notification config",
					slog.Int64("ruleID", rule.ID),
					slog.Any("error", err))
				rule.Config = make(map[string]interface{})
			}
		}
		rules = append(rules, &rule)
	}
	return rules, rows.Err()
}

// Update modifies an existing notification rule.
func (r *NotificationRepository) Update(ctx context.Context, rule *model.NotificationRule) error {
	configJSON, err := json.Marshal(rule.Config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	_, err = r.pool.Exec(ctx, `
		UPDATE notification_rules
		SET name = $1, event_types = $2, channel = $3, config = $4, template = $5, enabled = $6, updated_at = NOW()
		WHERE id = $7 AND user_id = $8
	`, rule.Name, rule.EventTypes, rule.Channel, configJSON, rule.Template, rule.Enabled, rule.ID, rule.UserID)
	if err != nil {
		return fmt.Errorf("update notification rule: %w", err)
	}
	return nil
}

// Delete removes a notification rule by ID.
func (r *NotificationRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM notification_rules WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete notification rule: %w", err)
	}
	return nil
}

// LogDelivery records a notification delivery attempt.
func (r *NotificationRepository) LogDelivery(ctx context.Context, entry *model.NotificationLog) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO notification_log (rule_id, event_id, status, sent_at, error, response_code, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		RETURNING id, created_at
	`, entry.RuleID, entry.EventID, entry.Status, entry.SentAt, entry.Error, entry.ResponseCode).
		Scan(&entry.ID, &entry.CreatedAt)
	if err != nil {
		return fmt.Errorf("log notification delivery: %w", err)
	}
	return nil
}

// GetLogsByRule retrieves recent delivery logs for a notification rule.
func (r *NotificationRepository) GetLogsByRule(ctx context.Context, ruleID int64, limit int) ([]*model.NotificationLog, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, rule_id, event_id, status, sent_at, error, response_code, created_at
		FROM notification_log
		WHERE rule_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, ruleID, limit)
	if err != nil {
		return nil, fmt.Errorf("get notification logs by rule: %w", err)
	}
	defer rows.Close()

	var logs []*model.NotificationLog
	for rows.Next() {
		var l model.NotificationLog
		if err := rows.Scan(&l.ID, &l.RuleID, &l.EventID, &l.Status, &l.SentAt, &l.Error, &l.ResponseCode, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan notification log: %w", err)
		}
		logs = append(logs, &l)
	}
	return logs, rows.Err()
}

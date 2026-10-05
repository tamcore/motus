package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
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
	err := r.pool.QueryRow(ctx, `
		INSERT INTO notification_rules (user_id, name, event_types, channel, config, template, enabled, geofence_ids, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		RETURNING id, created_at, updated_at
	`, rule.UserID, rule.Name, rule.EventTypes, rule.Channel, configParam(rule.Config), rule.Template, rule.Enabled, geofenceIDsParam(rule.GeofenceIDs)).
		Scan(&rule.ID, &rule.CreatedAt, &rule.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create notification rule: %w", err)
	}
	return nil
}

// GetByID retrieves a single notification rule by its ID.
func (r *NotificationRepository) GetByID(ctx context.Context, id int64) (*model.NotificationRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, name, event_types, channel, config, template, enabled, geofence_ids, created_at, updated_at
		FROM notification_rules
		WHERE id = $1
	`, id)
	if err != nil {
		return nil, fmt.Errorf("get notification rule by id: %w", err)
	}
	rule, err := pgx.CollectOneRow(rows, rowToNotificationRule(false))
	if err != nil {
		return nil, fmt.Errorf("get notification rule by id: %w", err)
	}
	return rule, nil
}

// GetByUser retrieves all notification rules for a user.
func (r *NotificationRepository) GetByUser(ctx context.Context, userID int64) ([]*model.NotificationRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, name, event_types, channel, config, template, enabled, geofence_ids, created_at, updated_at
		FROM notification_rules
		WHERE user_id = $1
		ORDER BY name
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get notification rules by user: %w", err)
	}
	return pgx.CollectRows(rows, rowToNotificationRule(false))
}

// GetAll retrieves all notification rules with owner names.
func (r *NotificationRepository) GetAll(ctx context.Context) ([]*model.NotificationRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT nr.id, nr.user_id, nr.name, nr.event_types, nr.channel, nr.config, nr.template, nr.enabled, nr.geofence_ids, nr.created_at, nr.updated_at,
			COALESCE(u.name, '') AS owner_name
		FROM notification_rules nr
		LEFT JOIN users u ON u.id = nr.user_id
		ORDER BY nr.name
	`)
	if err != nil {
		return nil, fmt.Errorf("get all notification rules: %w", err)
	}
	return pgx.CollectRows(rows, rowToNotificationRule(true))
}

// GetByEventType retrieves enabled notification rules for a user matching a given event type.
func (r *NotificationRepository) GetByEventType(ctx context.Context, userID int64, eventType string) ([]*model.NotificationRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, name, event_types, channel, config, template, enabled, geofence_ids, created_at, updated_at
		FROM notification_rules
		WHERE user_id = $1 AND $2 = ANY(event_types) AND enabled = true
	`, userID, eventType)
	if err != nil {
		return nil, fmt.Errorf("get notification rules by event type: %w", err)
	}
	return pgx.CollectRows(rows, rowToNotificationRule(false))
}

// Update modifies an existing notification rule.
func (r *NotificationRepository) Update(ctx context.Context, rule *model.NotificationRule) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE notification_rules
		SET name = $1, event_types = $2, channel = $3, config = $4, template = $5, enabled = $6, geofence_ids = $9, updated_at = NOW()
		WHERE id = $7 AND user_id = $8
	`, rule.Name, rule.EventTypes, rule.Channel, configParam(rule.Config), rule.Template, rule.Enabled, rule.ID, rule.UserID, geofenceIDsParam(rule.GeofenceIDs))
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
	return pgx.AppendRows([]*model.NotificationLog(nil), rows, func(row pgx.CollectableRow) (*model.NotificationLog, error) {
		var l model.NotificationLog
		if err := row.Scan(&l.ID, &l.RuleID, &l.EventID, &l.Status, &l.SentAt, &l.Error, &l.ResponseCode, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan notification log: %w", err)
		}
		return &l, nil
	})
}

// configParam maps a nil config to JSON null, as the NOT NULL config column
// rejects the SQL NULL pgx encodes for a nil map.
func configParam(config map[string]any) any {
	if config == nil {
		return "null"
	}
	return config
}

// geofenceIDsParam maps a nil geofence filter to an empty array: pgx encodes
// a nil slice as NULL, which the NOT NULL geofence_ids column rejects.
func geofenceIDsParam(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}

func rowToNotificationRule(withOwner bool) pgx.RowToFunc[*model.NotificationRule] {
	return func(row pgx.CollectableRow) (*model.NotificationRule, error) {
		var rule model.NotificationRule
		dest := []any{
			&rule.ID, &rule.UserID, &rule.Name, &rule.EventTypes, &rule.Channel,
			&rule.Config, &rule.Template, &rule.Enabled, &rule.GeofenceIDs, &rule.CreatedAt, &rule.UpdatedAt,
		}
		if withOwner {
			dest = append(dest, &rule.OwnerName)
		}
		if err := row.Scan(dest...); err != nil {
			return nil, fmt.Errorf("scan notification rule: %w", err)
		}
		return &rule, nil
	}
}

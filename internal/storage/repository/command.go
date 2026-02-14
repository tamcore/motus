package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamcore/motus/internal/model"
)

// CommandRepository handles command persistence.
type CommandRepository struct {
	pool *pgxpool.Pool
}

// NewCommandRepository creates a new command repository.
func NewCommandRepository(pool *pgxpool.Pool) *CommandRepository {
	return &CommandRepository{pool: pool}
}

// Create inserts a new command into the database.
func (r *CommandRepository) Create(ctx context.Context, cmd *model.Command) error {
	attrs, err := json.Marshal(cmd.Attributes)
	if err != nil {
		return fmt.Errorf("marshal command attributes: %w", err)
	}

	err = r.pool.QueryRow(ctx,
		`INSERT INTO commands (device_id, type, attributes, status)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, created_at`,
		cmd.DeviceID, cmd.Type, attrs, cmd.Status,
	).Scan(&cmd.ID, &cmd.CreatedAt)
	if err != nil {
		return fmt.Errorf("create command: %w", err)
	}
	return nil
}

// GetPendingByDevice returns all pending commands for a device.
func (r *CommandRepository) GetPendingByDevice(ctx context.Context, deviceID int64) ([]*model.Command, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, device_id, type, attributes, status, result, created_at, executed_at
		 FROM commands
		 WHERE device_id = $1 AND status = 'pending'
		 ORDER BY created_at ASC`, deviceID,
	)
	if err != nil {
		return nil, fmt.Errorf("get pending commands: %w", err)
	}
	defer rows.Close()

	var commands []*model.Command
	for rows.Next() {
		cmd := &model.Command{}
		var attrs []byte
		if err := rows.Scan(&cmd.ID, &cmd.DeviceID, &cmd.Type, &attrs, &cmd.Status, &cmd.Result, &cmd.CreatedAt, &cmd.ExecutedAt); err != nil {
			return nil, fmt.Errorf("scan command: %w", err)
		}
		if len(attrs) > 0 {
			_ = json.Unmarshal(attrs, &cmd.Attributes)
		}
		commands = append(commands, cmd)
	}
	return commands, rows.Err()
}

// UpdateStatus updates the status of a command and optionally sets executed_at.
func (r *CommandRepository) UpdateStatus(ctx context.Context, id int64, status string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE commands SET status = $1, executed_at = CASE WHEN $2 = 'executed' THEN NOW() ELSE executed_at END
		 WHERE id = $3`,
		status, status, id,
	)
	if err != nil {
		return fmt.Errorf("update command status: %w", err)
	}
	return nil
}

// ListByDevice returns the most recent limit commands for a device, ordered newest first.
func (r *CommandRepository) ListByDevice(ctx context.Context, deviceID int64, limit int) ([]*model.Command, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, device_id, type, attributes, status, result, created_at, executed_at
		 FROM commands
		 WHERE device_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2`,
		deviceID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list commands by device: %w", err)
	}
	defer rows.Close()

	var commands []*model.Command
	for rows.Next() {
		cmd := &model.Command{}
		var attrs []byte
		if err := rows.Scan(&cmd.ID, &cmd.DeviceID, &cmd.Type, &attrs, &cmd.Status, &cmd.Result, &cmd.CreatedAt, &cmd.ExecutedAt); err != nil {
			return nil, fmt.Errorf("scan command: %w", err)
		}
		if len(attrs) > 0 {
			_ = json.Unmarshal(attrs, &cmd.Attributes)
		}
		commands = append(commands, cmd)
	}
	return commands, rows.Err()
}

// AppendResult appends a result chunk to a command's result column (newline-separated).
// On the first append (when result is NULL) it also sets status="executed" and executed_at=NOW().
func (r *CommandRepository) AppendResult(ctx context.Context, id int64, chunk string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE commands
		 SET result = CASE WHEN result IS NULL THEN $1 ELSE result || E'\n' || $1 END,
		     status = CASE WHEN result IS NULL THEN 'executed' ELSE status END,
		     executed_at = CASE WHEN result IS NULL THEN NOW() ELSE executed_at END
		 WHERE id = $2`,
		chunk, id,
	)
	if err != nil {
		return fmt.Errorf("append command result: %w", err)
	}
	return nil
}

// GetLatestSentByDevice returns the most recent command with status "sent" or "executed"
// for a device. Used to associate incoming SMS response chunks with the command that
// triggered them.
func (r *CommandRepository) GetLatestSentByDevice(ctx context.Context, deviceID int64) (*model.Command, error) {
	cmd := &model.Command{}
	var attrs []byte
	err := r.pool.QueryRow(ctx,
		`SELECT id, device_id, type, attributes, status, result, created_at, executed_at
		 FROM commands
		 WHERE device_id = $1 AND status IN ('sent', 'executed')
		 ORDER BY created_at DESC
		 LIMIT 1`,
		deviceID,
	).Scan(&cmd.ID, &cmd.DeviceID, &cmd.Type, &attrs, &cmd.Status, &cmd.Result, &cmd.CreatedAt, &cmd.ExecutedAt)
	if err != nil {
		return nil, fmt.Errorf("get latest sent command: %w", err)
	}
	if len(attrs) > 0 {
		_ = json.Unmarshal(attrs, &cmd.Attributes)
	}
	return cmd, nil
}

package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
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
	err := r.pool.QueryRow(ctx,
		`INSERT INTO commands (device_id, type, attributes, status)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, created_at`,
		cmd.DeviceID, cmd.Type, cmd.Attributes, cmd.Status,
	).Scan(&cmd.ID, &cmd.CreatedAt)
	if err != nil {
		return fmt.Errorf("create command: %w", err)
	}
	return nil
}

// PendingCommand is a pending command with the device fields needed to send it.
type PendingCommand struct {
	Command  *model.Command
	UniqueID string
	Protocol string
}

// GetPendingByUniqueIDs returns the pending commands of the given devices in one
// query, oldest first.
func (r *CommandRepository) GetPendingByUniqueIDs(ctx context.Context, uniqueIDs []string) ([]PendingCommand, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+commandColumns+`, d.unique_id, d.protocol
		 FROM commands c
		 JOIN devices d ON d.id = c.device_id
		 WHERE c.status = 'pending' AND d.unique_id = ANY($1)
		 ORDER BY c.created_at ASC, c.id ASC`, uniqueIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("get pending commands: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (PendingCommand, error) {
		pc := PendingCommand{Command: &model.Command{}}
		if err := scanCommand(row, pc.Command, &pc.UniqueID, &pc.Protocol); err != nil {
			return pc, fmt.Errorf("scan pending command: %w", err)
		}
		return pc, nil
	})
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
		`SELECT `+commandColumns+`
		 FROM commands c
		 WHERE device_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2`,
		deviceID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list commands by device: %w", err)
	}
	return pgx.AppendRows([]*model.Command(nil), rows, rowToCommand)
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
	rows, err := r.pool.Query(ctx,
		`SELECT `+commandColumns+`
		 FROM commands c
		 WHERE device_id = $1 AND status IN ('sent', 'executed')
		 ORDER BY created_at DESC
		 LIMIT 1`,
		deviceID,
	)
	if err != nil {
		return nil, fmt.Errorf("get latest sent command: %w", err)
	}
	cmd, err := pgx.CollectOneRow(rows, rowToCommand)
	if err != nil {
		return nil, fmt.Errorf("get latest sent command: %w", err)
	}
	return cmd, nil
}

const commandColumns = `c.id, c.device_id, c.type, c.attributes, c.status, c.result, c.created_at, c.executed_at`

// scanCommand scans a command row, followed by extra, into cmd.
func scanCommand(row pgx.Row, cmd *model.Command, extra ...any) error {
	return row.Scan(append([]any{&cmd.ID, &cmd.DeviceID, &cmd.Type, &cmd.Attributes, &cmd.Status, &cmd.Result, &cmd.CreatedAt, &cmd.ExecutedAt}, extra...)...)
}

func rowToCommand(row pgx.CollectableRow) (*model.Command, error) {
	cmd := &model.Command{}
	if err := scanCommand(row, cmd); err != nil {
		return nil, fmt.Errorf("scan command: %w", err)
	}
	return cmd, nil
}

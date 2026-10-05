// Package partition manages PostgreSQL range partitions for the positions table.
//
// The positions table is partitioned by RANGE on the "timestamp" column with
// monthly partitions. This package handles:
//   - Creating future partitions proactively (before they are needed)
//   - Dropping old partitions based on a configurable retention policy
//   - Running as a background service with periodic checks
//
// Partition naming convention: positions_y{YYYY}m{MM}
// Example: positions_y2026m02 covers 2026-02-01 to 2026-03-01
package partition

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamcore/motus/internal/ticker"
)

// lookaheadMonths is how many months ahead partitions are created.
const lookaheadMonths = 3

// Manager handles automatic partition creation and optional retention for the
// positions table. It runs as a background goroutine, periodically checking
// whether new partitions need to be created or old ones dropped.
type Manager struct {
	pool          *pgxpool.Pool
	retentionDays int
	checkInterval time.Duration
	logger        *slog.Logger
}

// NewManager creates a partition manager.
//
// Parameters:
//   - pool: database connection pool
//   - retentionDays: drop partitions older than this many days (0 = disabled)
//   - checkInterval: how often to run maintenance checks
//   - logger: structured logger (nil = slog.Default())
func NewManager(pool *pgxpool.Pool, retentionDays int, checkInterval time.Duration, logger *slog.Logger) *Manager {
	return &Manager{
		pool:          pool,
		retentionDays: retentionDays,
		checkInterval: checkInterval,
		logger:        cmp.Or(logger, slog.Default()),
	}
}

// Start runs the partition manager in the foreground, blocking until the
// context is cancelled. It performs an immediate maintenance run, then checks
// periodically based on the configured interval.
func (m *Manager) Start(ctx context.Context) {
	m.logger.Info("partition manager started",
		slog.Int("retentionDays", m.retentionDays),
		slog.String("interval", m.checkInterval.String()),
		slog.Int("lookaheadMonths", lookaheadMonths),
	)

	run := func() {
		if err := m.RunOnce(ctx); err != nil {
			m.logger.Error("partition maintenance error", slog.Any("error", err))
		}
	}
	run()
	ticker.Every(ctx, m.checkInterval, run)
	m.logger.Info("partition manager stopped")
}

// RunOnce performs a single maintenance cycle.
func (m *Manager) RunOnce(ctx context.Context) error {
	if err := m.ensureFuturePartitions(ctx); err != nil {
		return fmt.Errorf("ensure future partitions: %w", err)
	}

	if m.retentionDays > 0 {
		if err := m.dropExpiredPartitions(ctx); err != nil {
			return fmt.Errorf("drop expired partitions: %w", err)
		}
	}

	return nil
}

// ensureFuturePartitions creates monthly partitions from the current month
// through the configured lookahead period.
func (m *Manager) ensureFuturePartitions(ctx context.Context) error {
	now := time.Now().UTC()
	// Start from the current month.
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	for i := range lookaheadMonths + 1 {
		partStart := start.AddDate(0, i, 0)
		partEnd := partStart.AddDate(0, 1, 0)
		name := PartitionName(partStart)

		created, err := m.createPartitionIfNotExists(ctx, name, partStart, partEnd)
		if err != nil {
			return fmt.Errorf("create partition %s: %w", name, err)
		}
		if created {
			m.logger.Info("created partition",
				slog.String("name", name),
				slog.String("from", partStart.Format("2006-01-02")),
				slog.String("to", partEnd.Format("2006-01-02")),
			)
		}
	}

	return nil
}

// createPartitionIfNotExists creates a partition with the given name and range
// if it does not already exist. Returns true if a new partition was created.
func (m *Manager) createPartitionIfNotExists(ctx context.Context, name string, start, end time.Time) (bool, error) {
	// Check if the partition already exists by querying pg_class.
	var exists bool
	err := m.pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE c.relname = $1 AND n.nspname = 'public'
		)`, name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check partition exists: %w", err)
	}
	if exists {
		return false, nil
	}

	// Detach default partition, create the new partition, re-attach default.
	// This is done in a transaction to ensure atomicity.
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Move any rows from the default partition that belong in the new range.
	// We detach, create the new partition, then move data and re-attach.
	if _, err := tx.Exec(ctx, `ALTER TABLE positions DETACH PARTITION positions_default`); err != nil {
		return false, fmt.Errorf("detach default: %w", err)
	}

	// name and dates come from time.Format and are safe to embed.
	createSQL := fmt.Sprintf(
		`CREATE TABLE %s PARTITION OF positions FOR VALUES FROM ('%s') TO ('%s')`,
		name,
		start.Format("2006-01-02"),
		end.Format("2006-01-02"),
	)
	// A failed statement aborts the transaction; the deferred rollback restores
	// the attached default partition.
	if _, err := tx.Exec(ctx, createSQL); err != nil {
		return false, fmt.Errorf("create partition: %w", err)
	}

	// Move any rows from default that now belong in the new partition.
	moveSQL := fmt.Sprintf(
		`WITH moved AS (
			DELETE FROM positions_default
			WHERE timestamp >= $1 AND timestamp < $2
			RETURNING *
		)
		INSERT INTO %s SELECT * FROM moved`, name)
	if _, err := tx.Exec(ctx, moveSQL, start, end); err != nil {
		return false, fmt.Errorf("move rows from default partition: %w", err)
	}

	if _, err := tx.Exec(ctx, `ALTER TABLE positions ATTACH PARTITION positions_default DEFAULT`); err != nil {
		return false, fmt.Errorf("re-attach default: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}

	return true, nil
}

// dropExpiredPartitions drops partitions whose entire date range is older than
// the retention period. Only drops named partitions (positions_yYYYYmMM), never
// the default partition.
func (m *Manager) dropExpiredPartitions(ctx context.Context) error {
	cutoff := time.Now().UTC().AddDate(0, 0, -m.retentionDays)
	// Round down to the start of the month containing the cutoff.
	// We only drop partitions whose END date is before or equal to the cutoff month start.
	cutoffMonth := time.Date(cutoff.Year(), cutoff.Month(), 1, 0, 0, 0, 0, time.UTC)

	partitions, err := m.ListPartitions(ctx)
	if err != nil {
		return fmt.Errorf("list partitions: %w", err)
	}

	for _, p := range partitions {
		if p.RangeEnd.Before(cutoffMonth) || p.RangeEnd.Equal(cutoffMonth) {
			m.logger.Info("dropping expired partition",
				slog.String("name", p.Name),
				slog.String("rangeEnd", p.RangeEnd.Format("2006-01-02")),
				slog.String("cutoff", cutoffMonth.Format("2006-01-02")),
			)

			if _, err := m.pool.Exec(ctx, `DROP TABLE IF EXISTS `+pgx.Identifier{p.Name}.Sanitize()); err != nil {
				return fmt.Errorf("drop partition %s: %w", p.Name, err)
			}
		}
	}

	return nil
}

// PartitionInfo holds metadata about an existing partition.
type PartitionInfo struct {
	Name     string
	RangeEnd time.Time
}

// ListPartitions returns information about all existing monthly positions
// partitions. The range end is derived from the canonical partition name.
func (m *Manager) ListPartitions(ctx context.Context) ([]PartitionInfo, error) {
	rows, err := m.pool.Query(ctx, `
		SELECT c.relname
		FROM pg_class c
		JOIN pg_inherits i ON c.oid = i.inhrelid
		JOIN pg_class parent ON parent.oid = i.inhparent
		WHERE parent.relname = 'positions'
		  AND c.relname != 'positions_default'
		ORDER BY c.relname
	`)
	if err != nil {
		return nil, fmt.Errorf("query partitions: %w", err)
	}
	defer rows.Close()

	var partitions []PartitionInfo
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan partition: %w", err)
		}
		start, err := time.Parse(partitionNameLayout, name)
		if err != nil {
			m.logger.Debug("skipping non-canonical partition",
				slog.String("partition", name),
				slog.Any("error", err),
			)
			continue
		}
		partitions = append(partitions, PartitionInfo{Name: name, RangeEnd: start.AddDate(0, 1, 0)})
	}

	return partitions, rows.Err()
}

// PartitionName returns the canonical partition name for a given month.
// Format: positions_y{YYYY}m{MM}
func PartitionName(t time.Time) string {
	return t.Format(partitionNameLayout)
}

const partitionNameLayout = "positions_y2006m01"

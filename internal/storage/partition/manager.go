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
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// validPartitionNameRE matches the canonical partition name format: positions_yYYYYmMM.
// Only names that match this pattern are safe to interpolate into DDL statements.
var validPartitionNameRE = regexp.MustCompile(`^positions_y\d{4}m\d{2}$`)

// validatePartitionName returns an error if name does not match the expected
// positions_yYYYYmMM format, preventing identifier injection in DDL.
func validatePartitionName(name string) error {
	if !validPartitionNameRE.MatchString(name) {
		return fmt.Errorf("invalid partition name %q: must match positions_yYYYYmMM", name)
	}
	return nil
}

// Manager handles automatic partition creation and optional retention for the
// positions table. It runs as a background goroutine, periodically checking
// whether new partitions need to be created or old ones dropped.
type Manager struct {
	pool          *pgxpool.Pool
	retentionDays int
	checkInterval time.Duration
	lookahead     int // months ahead to create partitions
	logger        *slog.Logger
}

// NewManager creates a partition manager.
//
// Parameters:
//   - pool: database connection pool
//   - retentionDays: drop partitions older than this many days (0 = disabled)
//   - checkInterval: how often to run maintenance checks
func NewManager(pool *pgxpool.Pool, retentionDays int, checkInterval time.Duration) *Manager {
	return &Manager{
		pool:          pool,
		retentionDays: retentionDays,
		checkInterval: checkInterval,
		lookahead:     3, // create partitions 3 months ahead
		logger:        slog.Default(),
	}
}

// SetLogger configures the structured logger for this manager.
func (m *Manager) SetLogger(l *slog.Logger) {
	if l != nil {
		m.logger = l
	}
}

// Start runs the partition manager in the foreground, blocking until the
// context is cancelled. It performs an immediate maintenance run, then checks
// periodically based on the configured interval.
func (m *Manager) Start(ctx context.Context) {
	m.logger.Info("partition manager started",
		slog.Int("retentionDays", m.retentionDays),
		slog.String("interval", m.checkInterval.String()),
		slog.Int("lookaheadMonths", m.lookahead),
	)

	// Run immediately on startup.
	m.runMaintenance(ctx)

	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("partition manager stopped")
			return
		case <-ticker.C:
			m.runMaintenance(ctx)
		}
	}
}

// RunOnce performs a single maintenance cycle. This is useful for testing
// or manual invocation without the background loop.
func (m *Manager) RunOnce(ctx context.Context) error {
	return m.runMaintenanceWithError(ctx)
}

// runMaintenance performs a single maintenance cycle, logging any errors.
func (m *Manager) runMaintenance(ctx context.Context) {
	if err := m.runMaintenanceWithError(ctx); err != nil {
		m.logger.Error("partition maintenance error", slog.Any("error", err))
	}
}

// runMaintenanceWithError performs a single maintenance cycle, returning errors.
func (m *Manager) runMaintenanceWithError(ctx context.Context) error {
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

	for i := 0; i <= m.lookahead; i++ {
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
	if err := validatePartitionName(name); err != nil {
		return false, err
	}

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

	// name is validated above; dates come from time.Time and are safe to embed.
	createSQL := fmt.Sprintf(
		`CREATE TABLE %s PARTITION OF positions FOR VALUES FROM ('%s') TO ('%s')`,
		name,
		start.Format("2006-01-02"),
		end.Format("2006-01-02"),
	)
	if _, err := tx.Exec(ctx, createSQL); err != nil {
		// Re-attach default before returning error.
		_, _ = tx.Exec(ctx, `ALTER TABLE positions ATTACH PARTITION positions_default DEFAULT`)
		return false, fmt.Errorf("create partition: %w", err)
	}

	// Move any rows from default that now belong in the new partition.
	// name is validated above; use parameters for the timestamp bounds.
	moveSQL := fmt.Sprintf(
		`WITH moved AS (
			DELETE FROM positions_default
			WHERE timestamp >= $1 AND timestamp < $2
			RETURNING *
		)
		INSERT INTO %s SELECT * FROM moved`, name)
	if _, err := tx.Exec(ctx, moveSQL, start, end); err != nil {
		// This can fail if there are no rows, which is fine.
		// Log but don't fail.
		m.logger.Debug("note: moving rows from default partition",
			slog.String("partition", name),
			slog.Any("error", err),
		)
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

	partitions, err := m.listPartitions(ctx)
	if err != nil {
		return fmt.Errorf("list partitions: %w", err)
	}

	for _, p := range partitions {
		if p.Name == "positions_default" {
			continue
		}

		// Parse the partition end date from its range.
		if p.RangeEnd.Before(cutoffMonth) || p.RangeEnd.Equal(cutoffMonth) {
			m.logger.Info("dropping expired partition",
				slog.String("name", p.Name),
				slog.String("rangeEnd", p.RangeEnd.Format("2006-01-02")),
				slog.String("cutoff", cutoffMonth.Format("2006-01-02")),
			)

			if err := validatePartitionName(p.Name); err != nil {
				m.logger.Error("skipping drop: partition name failed validation",
					slog.String("name", p.Name),
					slog.Any("error", err),
				)
				continue
			}
			dropSQL := fmt.Sprintf(`DROP TABLE IF EXISTS %s`, p.Name)
			if _, err := m.pool.Exec(ctx, dropSQL); err != nil {
				return fmt.Errorf("drop partition %s: %w", p.Name, err)
			}
		}
	}

	return nil
}

// PartitionInfo holds metadata about an existing partition.
type PartitionInfo struct {
	Name       string
	RangeStart time.Time
	RangeEnd   time.Time
}

// listPartitions returns information about all existing positions partitions.
func (m *Manager) listPartitions(ctx context.Context) ([]PartitionInfo, error) {
	rows, err := m.pool.Query(ctx, `
		SELECT c.relname,
			   pg_get_expr(c.relpartbound, c.oid) as partition_expr
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
		var name, expr string
		if err := rows.Scan(&name, &expr); err != nil {
			return nil, fmt.Errorf("scan partition: %w", err)
		}

		start, end, err := parsePartitionBounds(expr)
		if err != nil {
			m.logger.Warn("cannot parse partition bounds",
				slog.String("partition", name),
				slog.Any("error", err),
			)
			continue
		}

		partitions = append(partitions, PartitionInfo{
			Name:       name,
			RangeStart: start,
			RangeEnd:   end,
		})
	}

	return partitions, rows.Err()
}

// ListPartitions returns information about all existing positions partitions.
// This is the exported version for use by handlers or monitoring.
func (m *Manager) ListPartitions(ctx context.Context) ([]PartitionInfo, error) {
	return m.listPartitions(ctx)
}

// PartitionName returns the canonical partition name for a given month.
// Format: positions_y{YYYY}m{MM}
func PartitionName(t time.Time) string {
	return fmt.Sprintf("positions_y%04dm%02d", t.Year(), t.Month())
}

// parsePartitionBounds extracts start and end dates from a PostgreSQL
// partition bound expression like:
//
//	FOR VALUES FROM ('2026-01-01 00:00:00+00') TO ('2026-02-01 00:00:00+00')
func parsePartitionBounds(expr string) (time.Time, time.Time, error) {
	// Find all quoted strings in the expression.
	dates := extractQuotedStrings(expr)
	if len(dates) < 2 {
		return time.Time{}, time.Time{}, fmt.Errorf("expected 2 dates in %q, found %d", expr, len(dates))
	}

	start, err := parsePartitionDate(dates[0])
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse start date %q: %w", dates[0], err)
	}

	end, err := parsePartitionDate(dates[1])
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse end date %q: %w", dates[1], err)
	}

	return start, end, nil
}

// extractQuotedStrings extracts all single-quoted strings from the input.
func extractQuotedStrings(s string) []string {
	var result []string
	inQuote := false
	var current []byte

	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			if inQuote {
				result = append(result, string(current))
				current = current[:0]
				inQuote = false
			} else {
				inQuote = true
			}
		} else if inQuote {
			current = append(current, s[i])
		}
	}

	return result
}

// parsePartitionDate parses a date string from PostgreSQL partition bounds.
// Handles formats like: "2026-01-01 00:00:00+00", "2026-01-01", etc.
func parsePartitionDate(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02 15:04:05-07",
		"2006-01-02 15:04:05+00",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse date %q", s)
}

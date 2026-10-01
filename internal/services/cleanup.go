package services

import (
	"cmp"
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const expiredRetention = 7 * 24 * time.Hour

// CleanupService manages deletion of expired sessions and device shares.
// Runs periodically to prevent unbounded table growth.
type CleanupService struct {
	pool     *pgxpool.Pool
	interval time.Duration
	logger   *slog.Logger
}

// NewCleanupService creates a new cleanup service.
// interval specifies how often to run cleanup (e.g., 24 hours).
func NewCleanupService(pool *pgxpool.Pool, interval time.Duration, logger *slog.Logger) *CleanupService {
	return &CleanupService{
		pool:     pool,
		interval: interval,
		logger:   cmp.Or(logger, slog.Default()),
	}
}

// Start runs the cleanup service in a blocking loop until ctx is cancelled.
// Performs cleanup immediately on start, then periodically at the configured interval.
func (s *CleanupService) Start(ctx context.Context) {
	s.logger.Info("starting expired data cleanup service")
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Run immediately on startup
	if err := s.RunOnce(ctx); err != nil {
		s.logger.Error("cleanup error on startup", slog.Any("error", err))
	}

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("stopping expired data cleanup service")
			return
		case <-ticker.C:
			if err := s.RunOnce(ctx); err != nil {
				s.logger.Error("cleanup error", slog.Any("error", err))
			}
		}
	}
}

// RunOnce performs a single cleanup cycle.
// Deletes sessions and device shares that expired more than 7 days ago.
// Shares with NULL expires_at never expire.
func (s *CleanupService) RunOnce(ctx context.Context) error {
	cutoff := time.Now().Add(-expiredRetention)
	for _, c := range []struct{ msg, query string }{
		{"cleaned expired sessions", `DELETE FROM sessions WHERE expires_at < $1`},
		{"cleaned expired device shares", `DELETE FROM device_shares WHERE expires_at IS NOT NULL AND expires_at < $1`},
	} {
		tag, err := s.pool.Exec(ctx, c.query, cutoff)
		if err != nil {
			return err
		}
		if n := tag.RowsAffected(); n > 0 {
			s.logger.Info(c.msg, slog.Int64("count", n))
		}
	}
	return nil
}

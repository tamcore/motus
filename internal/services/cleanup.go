package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CleanupService manages deletion of expired sessions and device shares.
// Runs periodically to prevent unbounded table growth.
type CleanupService struct {
	pool     *pgxpool.Pool
	interval time.Duration
	logger   *slog.Logger
}

// NewCleanupService creates a new cleanup service.
// interval specifies how often to run cleanup (e.g., 24 hours).
func NewCleanupService(pool *pgxpool.Pool, interval time.Duration) *CleanupService {
	return &CleanupService{
		pool:     pool,
		interval: interval,
		logger:   slog.Default(),
	}
}

// SetLogger configures the structured logger for this service.
func (s *CleanupService) SetLogger(l *slog.Logger) {
	if l != nil {
		s.logger = l
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
func (s *CleanupService) RunOnce(ctx context.Context) error {
	// Clean up expired sessions (keep 7 days past expiration for audit purposes)
	sessionsDeleted, err := s.cleanExpiredSessions(ctx)
	if err != nil {
		return err
	}
	if sessionsDeleted > 0 {
		s.logger.Info("cleaned expired sessions",
			slog.Int64("count", sessionsDeleted),
		)
	}

	// Clean up expired device shares (keep 7 days past expiration)
	sharesDeleted, err := s.cleanExpiredShares(ctx)
	if err != nil {
		return err
	}
	if sharesDeleted > 0 {
		s.logger.Info("cleaned expired device shares",
			slog.Int64("count", sharesDeleted),
		)
	}

	return nil
}

// cleanExpiredSessions deletes sessions that expired more than 7 days ago.
func (s *CleanupService) cleanExpiredSessions(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-7 * 24 * time.Hour)

	tag, err := s.pool.Exec(ctx,
		`DELETE FROM sessions WHERE expires_at < $1`,
		cutoff,
	)
	if err != nil {
		return 0, err
	}

	return tag.RowsAffected(), nil
}

// cleanExpiredShares deletes device shares that expired more than 7 days ago.
// Shares with NULL expires_at (never expire) are not deleted.
func (s *CleanupService) cleanExpiredShares(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-7 * 24 * time.Hour)

	tag, err := s.pool.Exec(ctx,
		`DELETE FROM device_shares
		 WHERE expires_at IS NOT NULL AND expires_at < $1`,
		cutoff,
	)
	if err != nil {
		return 0, err
	}

	return tag.RowsAffected(), nil
}

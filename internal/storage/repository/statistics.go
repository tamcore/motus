package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PlatformStats holds platform-wide aggregate statistics.
type PlatformStats struct {
	TotalUsers        int64
	TotalDevices      int64
	TotalPositions    int64
	TotalEvents       int64
	NotificationsSent int64
	DevicesByStatus   map[string]int64
	PositionsToday    int64
	ActiveUsers       int64
}

// UserStats holds statistics for a specific user.
type UserStats struct {
	UserID          int64
	DevicesOwned    int64
	TotalPositions  int64
	LastLogin       *time.Time
	EventsTriggered int64
	GeofencesOwned  int64
}

// StatisticsRepository provides aggregate statistics queries.
type StatisticsRepository struct {
	pool *pgxpool.Pool
}

// NewStatisticsRepository creates a new statistics repository.
func NewStatisticsRepository(pool *pgxpool.Pool) *StatisticsRepository {
	return &StatisticsRepository{pool: pool}
}

// GetPlatformStats returns platform-wide aggregate statistics.
func (r *StatisticsRepository) GetPlatformStats(ctx context.Context) (*PlatformStats, error) {
	stats := &PlatformStats{
		DevicesByStatus: make(map[string]int64),
	}

	todayStart := time.Now().UTC().Truncate(24 * time.Hour)
	err := r.pool.QueryRow(ctx,
		`SELECT
			(SELECT COUNT(*) FROM users),
			(SELECT COUNT(*) FROM devices),
			(SELECT COUNT(*) FROM positions),
			(SELECT COUNT(*) FROM events),
			(SELECT COUNT(*) FROM notification_log),
			(SELECT COUNT(*) FROM positions WHERE timestamp >= $1),
			(SELECT COUNT(DISTINCT user_id) FROM sessions
			 WHERE expires_at > NOW() AND created_at >= NOW() - INTERVAL '24 hours')`,
		todayStart,
	).Scan(&stats.TotalUsers, &stats.TotalDevices, &stats.TotalPositions, &stats.TotalEvents,
		&stats.NotificationsSent, &stats.PositionsToday, &stats.ActiveUsers)
	if err != nil {
		return nil, fmt.Errorf("fetch statistics: %w", err)
	}

	// Device status distribution (separate query for GROUP BY).
	rows, err := r.pool.Query(ctx, `SELECT COALESCE(status, 'unknown'), COUNT(*) FROM devices GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("device status distribution: %w", err)
	}
	var status string
	var count int64
	if _, err := pgx.ForEachRow(rows, []any{&status, &count}, func() error {
		stats.DevicesByStatus[status] = count
		return nil
	}); err != nil {
		return nil, fmt.Errorf("device status rows: %w", err)
	}

	return stats, nil
}

// GetUserStats returns statistics for a specific user.
func (r *StatisticsRepository) GetUserStats(ctx context.Context, userID int64) (*UserStats, error) {
	stats := &UserStats{UserID: userID}

	// Count devices owned.
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_devices WHERE user_id = $1`, userID,
	).Scan(&stats.DevicesOwned)
	if err != nil {
		return nil, fmt.Errorf("count user devices: %w", err)
	}

	// Count total positions for user's devices.
	err = r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM positions p
		 JOIN user_devices ud ON ud.device_id = p.device_id
		 WHERE ud.user_id = $1`, userID,
	).Scan(&stats.TotalPositions)
	if err != nil {
		return nil, fmt.Errorf("count user positions: %w", err)
	}

	// Last login (most recent session creation).
	var lastLogin *time.Time
	err = r.pool.QueryRow(ctx,
		`SELECT MAX(created_at) FROM sessions WHERE user_id = $1`, userID,
	).Scan(&lastLogin)
	if err != nil {
		return nil, fmt.Errorf("get last login: %w", err)
	}
	stats.LastLogin = lastLogin

	// Events triggered for user's devices.
	err = r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM events e
		 JOIN user_devices ud ON ud.device_id = e.device_id
		 WHERE ud.user_id = $1`, userID,
	).Scan(&stats.EventsTriggered)
	if err != nil {
		return nil, fmt.Errorf("count user events: %w", err)
	}

	// Count geofences owned.
	err = r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_geofences WHERE user_id = $1`, userID,
	).Scan(&stats.GeofencesOwned)
	if err != nil {
		return nil, fmt.Errorf("count user geofences: %w", err)
	}

	return stats, nil
}

package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PlatformStats holds platform-wide aggregate statistics.
type PlatformStats struct {
	TotalUsers        int64            `json:"totalUsers"`
	TotalDevices      int64            `json:"totalDevices"`
	TotalPositions    int64            `json:"totalPositions"`
	TotalEvents       int64            `json:"totalEvents"`
	NotificationsSent int64            `json:"notificationsSent"`
	DevicesByStatus   map[string]int64 `json:"devicesByStatus"`
	PositionsToday    int64            `json:"positionsToday"`
	ActiveUsers       int64            `json:"activeUsers"`
}

// UserStats holds statistics for a specific user.
type UserStats struct {
	UserID          int64      `json:"userId"`
	DevicesOwned    int64      `json:"devicesOwned"`
	TotalPositions  int64      `json:"totalPositions"`
	LastLogin       *time.Time `json:"lastLogin"`
	EventsTriggered int64      `json:"eventsTriggered"`
	GeofencesOwned  int64      `json:"geofencesOwned"`
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

	// Count users.
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&stats.TotalUsers)
	if err != nil {
		return nil, fmt.Errorf("count users: %w", err)
	}

	// Count devices.
	err = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM devices`).Scan(&stats.TotalDevices)
	if err != nil {
		return nil, fmt.Errorf("count devices: %w", err)
	}

	// Optimize: Combine all counts into a single query with CTEs to reduce round trips.
	todayStart := time.Now().UTC().Truncate(24 * time.Hour)
	err = r.pool.QueryRow(ctx,
		`WITH counts AS (
			SELECT
				(SELECT COUNT(*) FROM positions) AS total_positions,
				(SELECT COUNT(*) FROM events) AS total_events,
				(SELECT COUNT(*) FROM notification_log) AS notifications_sent,
				(SELECT COUNT(*) FROM positions WHERE timestamp >= $1) AS positions_today,
				(SELECT COUNT(DISTINCT user_id) FROM sessions
				 WHERE expires_at > NOW() AND created_at >= NOW() - INTERVAL '24 hours') AS active_users
		)
		SELECT total_positions, total_events, notifications_sent, positions_today, active_users
		FROM counts`,
		todayStart,
	).Scan(&stats.TotalPositions, &stats.TotalEvents, &stats.NotificationsSent, &stats.PositionsToday, &stats.ActiveUsers)
	if err != nil {
		return nil, fmt.Errorf("fetch statistics: %w", err)
	}

	// Device status distribution (separate query for GROUP BY).
	rows, err := r.pool.Query(ctx, `SELECT COALESCE(status, 'unknown'), COUNT(*) FROM devices GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("device status distribution: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("scan device status: %w", err)
		}
		stats.DevicesByStatus[status] = count
	}
	if err := rows.Err(); err != nil {
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

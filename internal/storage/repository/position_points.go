package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/tamcore/motus/internal/model"
)

// PointsByDeviceAndTimeRange returns the points of a device in the time range,
// ordered by timestamp and sampled like StreamByDeviceAndTimeRange.
func (r *PositionRepository) PointsByDeviceAndTimeRange(ctx context.Context, deviceID int64, from, to time.Time, limit int) ([]model.PositionPoint, error) {
	points, err := r.points(ctx, limit, `FROM positions p
		 WHERE p.device_id = $1 AND p.timestamp >= $2 AND p.timestamp <= $3`, deviceID, from, to)
	if err != nil {
		return nil, fmt.Errorf("points by device and time range: %w", err)
	}
	return points, nil
}

// PointsByUserAndTimeRange returns the points of all devices of a user in the
// time range, ordered by timestamp and sampled like StreamByUserAndTimeRange.
func (r *PositionRepository) PointsByUserAndTimeRange(ctx context.Context, userID int64, from, to time.Time, limit int) ([]model.PositionPoint, error) {
	points, err := r.points(ctx, limit, `FROM positions p
		 JOIN user_devices ud ON ud.device_id = p.device_id
		 WHERE ud.user_id = $1 AND p.timestamp >= $2 AND p.timestamp <= $3`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("points by user and time range: %w", err)
	}
	return points, nil
}

// points keeps every ceil(total/limit)-th row of fromWhere (limit<=0 keeps
// all). Counting first sizes the result exactly; fromWhere must alias
// positions as p and use only $1..$3.
func (r *PositionRepository) points(ctx context.Context, limit int, fromWhere string, args ...any) ([]model.PositionPoint, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT count(*) `+fromWhere, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count: %w", err)
	}
	if total == 0 {
		return []model.PositionPoint{}, nil
	}
	if limit <= 0 || total < int64(limit) {
		limit = int(total)
	}
	stride := (total + int64(limit) - 1) / int64(limit)
	size := (total + stride - 1) / stride

	// LIMIT keeps the bound when rows arrive between the count and the scan.
	rows, err := r.pool.Query(ctx, `SELECT timestamp, latitude, longitude, COALESCE(speed, 0) FROM (
			SELECT p.timestamp, p.latitude, p.longitude, p.speed,
				row_number() OVER (ORDER BY p.timestamp) - 1 AS rn
			`+fromWhere+`
		 ) w
		 WHERE rn % $4 = 0
		 ORDER BY timestamp ASC
		 LIMIT $5`, append(args, stride, size)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	points := make([]model.PositionPoint, 0, size)
	for rows.Next() {
		var p model.PositionPoint
		if err := rows.Scan(&p.FixTime, &p.Lat, &p.Lon, &p.Speed); err != nil {
			return nil, fmt.Errorf("scan point: %w", err)
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

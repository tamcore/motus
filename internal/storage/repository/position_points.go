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

// points keeps every ceil(total/limit)-th row of fromWhere (limit<=0 keeps
// all). Counting first sizes the result; fromWhere must alias positions as p
// and use only $1..$3.
func (r *PositionRepository) points(ctx context.Context, limit int, fromWhere string, args ...any) ([]model.PositionPoint, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT count(*) `+fromWhere, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count: %w", err)
	}
	if total == 0 {
		return []model.PositionPoint{}, nil
	}
	if limit <= 0 {
		limit = int(total)
	}
	stride := max(1, (total+int64(limit)-1)/int64(limit))
	size := min((total+stride-1)/stride, int64(limit))

	// LIMIT keeps the bound when rows arrive between the count and the scan.
	rows, err := r.pool.Query(ctx, `SELECT timestamp, latitude, longitude, COALESCE(speed, 0) FROM (
			SELECT p.timestamp, p.latitude, p.longitude, p.speed,
				row_number() OVER (ORDER BY p.timestamp) - 1 AS rn
			`+fromWhere+`
		 ) w
		 WHERE rn % $4 = 0
		 ORDER BY timestamp ASC
		 LIMIT $5`, append(args, stride, limit)...)
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

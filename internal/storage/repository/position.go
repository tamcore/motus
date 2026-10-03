package repository

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamcore/motus/internal/model"
)

// PositionRepository handles position persistence.
type PositionRepository struct {
	pool *pgxpool.Pool
}

// NewPositionRepository creates a new position repository.
func NewPositionRepository(pool *pgxpool.Pool) *PositionRepository {
	return &PositionRepository{pool: pool}
}

// positionColumns is the list of columns selected for position queries.
const positionColumns = `id, device_id, protocol, server_time, device_time,
	timestamp, valid, latitude, longitude, altitude, speed, course,
	address, accuracy, network, geofence_ids, outdated, attributes`

const qualifiedPositionColumns = `p.id, p.device_id, p.protocol, p.server_time, p.device_time,
	p.timestamp, p.valid, p.latitude, p.longitude, p.altitude, p.speed, p.course,
	p.address, p.accuracy, p.network, p.geofence_ids, p.outdated, p.attributes`

// Create inserts a new position record.
func (r *PositionRepository) Create(ctx context.Context, p *model.Position) error {
	attrs, err := json.Marshal(p.Attributes)
	if err != nil {
		return fmt.Errorf("marshal attributes: %w", err)
	}

	var network []byte
	if p.Network != nil {
		network, _ = json.Marshal(p.Network)
	}

	// Default server_time to now if not set.
	if p.ServerTime == nil || p.ServerTime.IsZero() {
		now := time.Now().UTC()
		p.ServerTime = &now
	}
	// Default device_time to fix_time if not set.
	if p.DeviceTime == nil || p.DeviceTime.IsZero() {
		p.DeviceTime = &p.Timestamp
	}

	err = r.pool.QueryRow(ctx,
		`INSERT INTO positions (device_id, protocol, server_time, device_time, timestamp, valid,
			latitude, longitude, altitude, speed, course, address, accuracy, network, geofence_ids, outdated, attributes)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		 RETURNING id`,
		p.DeviceID, p.Protocol, p.ServerTime, p.DeviceTime, p.Timestamp, p.Valid,
		p.Latitude, p.Longitude, p.Altitude, p.Speed, p.Course,
		p.Address, p.Accuracy, network, p.GeofenceIDs, p.Outdated, attrs,
	).Scan(&p.ID)
	if err != nil {
		return fmt.Errorf("create position: %w", err)
	}
	return nil
}

// GetLatestByDevice returns the most recent position for a device.
func (r *PositionRepository) GetLatestByDevice(ctx context.Context, deviceID int64) (*model.Position, error) {
	p := &model.Position{}
	err := scanPosition(r.pool.QueryRow(ctx,
		`SELECT `+positionColumns+` FROM positions WHERE device_id = $1 ORDER BY timestamp DESC LIMIT 1`, deviceID,
	), p)
	if err != nil {
		return nil, fmt.Errorf("get latest position: %w", err)
	}
	return p, nil
}

// GetLatestByUser returns the latest position for each device the user has access to.
// The per-device LATERAL lookup uses the (device_id, timestamp DESC) index instead
// of sorting every position of the user's devices.
func (r *PositionRepository) GetLatestByUser(ctx context.Context, userID int64) ([]*model.Position, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT p.* FROM user_devices ud
		 CROSS JOIN LATERAL (
			SELECT `+positionColumns+` FROM positions
			WHERE device_id = ud.device_id ORDER BY timestamp DESC LIMIT 1
		 ) p
		 WHERE ud.user_id = $1
		 ORDER BY p.device_id`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get latest positions by user: %w", err)
	}
	defer rows.Close()

	return pgx.CollectRows(rows, rowToPosition)
}

// GetLatestAll returns the latest position for every device in the system (admin use).
func (r *PositionRepository) GetLatestAll(ctx context.Context) ([]*model.Position, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT p.* FROM devices d
		 CROSS JOIN LATERAL (
			SELECT `+positionColumns+` FROM positions
			WHERE device_id = d.id ORDER BY timestamp DESC LIMIT 1
		 ) p
		 ORDER BY p.device_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("get latest positions (all): %w", err)
	}
	defer rows.Close()

	return pgx.CollectRows(rows, rowToPosition)
}

// GetPreviousByDevice returns the position immediately before the given timestamp
// for a device. Returns nil, nil if no previous position exists.
func (r *PositionRepository) GetPreviousByDevice(ctx context.Context, deviceID int64, beforeTime time.Time) (*model.Position, error) {
	p := &model.Position{}
	err := scanPosition(r.pool.QueryRow(ctx,
		`SELECT `+positionColumns+`
		 FROM positions
		 WHERE device_id = $1 AND timestamp < $2
		 ORDER BY timestamp DESC
		 LIMIT 1`, deviceID, beforeTime,
	), p)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get previous position: %w", err)
	}
	return p, nil
}

// GetByID retrieves a single position by its ID.
func (r *PositionRepository) GetByID(ctx context.Context, id int64) (*model.Position, error) {
	p := &model.Position{}
	err := scanPosition(r.pool.QueryRow(ctx,
		`SELECT `+positionColumns+` FROM positions WHERE id = $1`, id,
	), p)
	if err != nil {
		return nil, fmt.Errorf("get position by id: %w", err)
	}
	return p, nil
}

// UpdateGeofenceIDs updates the geofence_ids column of a stored position.
// Called after geofence containment is computed so the stored row reflects
// the correct geofences for REST API reads (e.g. Home Assistant polling).
func (r *PositionRepository) UpdateGeofenceIDs(ctx context.Context, positionID int64, ids []int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE positions SET geofence_ids = $1 WHERE id = $2`,
		ids, positionID,
	)
	if err != nil {
		return fmt.Errorf("update position geofence ids: %w", err)
	}
	return nil
}

// UpdateAddress sets the address field on a stored position. This is used
// by the geocoding integration to persist addresses for idle/stopped positions.
func (r *PositionRepository) UpdateAddress(ctx context.Context, positionID int64, address string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE positions SET address = $1 WHERE id = $2`,
		address, positionID,
	)
	if err != nil {
		return fmt.Errorf("update position address: %w", err)
	}
	return nil
}

// GetLastMovingPosition returns the most recent position for a device where
// speed meets or exceeds the given threshold. Used by mileage tracking to
// determine when a trip ended (time since last moving position > stop duration).
func (r *PositionRepository) GetLastMovingPosition(ctx context.Context, deviceID int64, speedThreshold float64) (*model.Position, error) {
	p := &model.Position{}
	err := scanPosition(r.pool.QueryRow(ctx,
		`SELECT `+positionColumns+`
		 FROM positions
		 WHERE device_id = $1 AND speed >= $2
		 ORDER BY timestamp DESC
		 LIMIT 1`, deviceID, speedThreshold,
	), p)
	if err != nil {
		return nil, fmt.Errorf("get last moving position: %w", err)
	}
	return p, nil
}

// StreamByDeviceAndTimeRange calls fn for each position in the time range,
// ordered by timestamp ascending. limit<=0 streams all matching rows; a
// positive limit streams at most that many, evenly spaced over the range.
// Errors from fn abort the stream.
func (r *PositionRepository) StreamByDeviceAndTimeRange(
	ctx context.Context, deviceID int64, from, to time.Time, limit int,
	fn func(*model.Position) error,
) error {
	err := r.streamSampled(ctx, fn, limit, `SELECT `+positionColumns+`
		 FROM positions
		 WHERE device_id = $1 AND timestamp >= $2 AND timestamp <= $3`, deviceID, from, to)
	if err != nil {
		return fmt.Errorf("stream positions by device and time range: %w", err)
	}
	return nil
}

// StreamTrackByDeviceAndTimeRange calls fn for every position in the time
// range, ordered by timestamp ascending, unsampled. Only Timestamp, Latitude,
// Longitude, Speed and Address are set. fn receives the same reused value on
// every call and must not retain p or its pointers.
func (r *PositionRepository) StreamTrackByDeviceAndTimeRange(
	ctx context.Context, deviceID int64, from, to time.Time,
	fn func(*model.Position) error,
) error {
	rows, err := r.pool.Query(ctx,
		`SELECT timestamp, latitude, longitude, speed, address
		 FROM positions
		 WHERE device_id = $1 AND timestamp >= $2 AND timestamp <= $3
		 ORDER BY timestamp ASC`, deviceID, from, to)
	if err != nil {
		return fmt.Errorf("stream track by device and time range: %w", err)
	}
	defer rows.Close()

	var (
		p     model.Position
		speed pgtype.Float8
		addr  pgtype.Text
	)
	dest := []any{&p.Timestamp, &p.Latitude, &p.Longitude, &speed, &addr}
	for rows.Next() {
		if err := rows.Scan(dest...); err != nil {
			return fmt.Errorf("scan track position: %w", err)
		}
		p.Speed, p.Address = nil, nil
		if speed.Valid {
			p.Speed = &speed.Float64
		}
		if addr.Valid {
			p.Address = &addr.String
		}
		if err := fn(&p); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("stream track by device and time range: %w", err)
	}
	return nil
}

// CountByUserAndTimeRange counts positions of the user's devices with a
// timestamp in [from, to].
func (r *PositionRepository) CountByUserAndTimeRange(ctx context.Context, userID int64, from, to time.Time) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM positions p
		 JOIN user_devices ud ON ud.device_id = p.device_id
		 WHERE ud.user_id = $1 AND p.timestamp >= $2 AND p.timestamp <= $3`, userID, from, to,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count positions by user and time range: %w", err)
	}
	return n, nil
}

// CountAllByTimeRange counts positions of all devices with a timestamp in [from, to].
func (r *PositionRepository) CountAllByTimeRange(ctx context.Context, from, to time.Time) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM positions WHERE timestamp >= $1 AND timestamp <= $2`, from, to,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count positions by time range: %w", err)
	}
	return n, nil
}

const userRangeFilter = `FROM positions p
		 JOIN user_devices ud ON ud.device_id = p.device_id
		 WHERE ud.user_id = $1 AND p.timestamp >= $2 AND p.timestamp <= $3`

// StreamByUserAndTimeRange calls fn for each position belonging to any device
// owned by userID within the time range, ordered by timestamp ascending. With
// limit > 0 at most limit rows are kept, sampled per device so that busy
// devices do not crowd out quiet ones (see deviceStrides).
func (r *PositionRepository) StreamByUserAndTimeRange(
	ctx context.Context, userID int64, from, to time.Time, limit int,
	fn func(*model.Position) error,
) error {
	if err := r.streamUserRange(ctx, userID, from, to, limit, fn); err != nil {
		return fmt.Errorf("stream positions by user and time range: %w", err)
	}
	return nil
}

func (r *PositionRepository) streamUserRange(ctx context.Context, userID int64, from, to time.Time, limit int, fn func(*model.Position) error) error {
	plain := `SELECT ` + qualifiedPositionColumns + ` ` + userRangeFilter + ` ORDER BY p.timestamp ASC`
	if limit <= 0 {
		return r.stream(ctx, fn, plain, userID, from, to)
	}
	counts, err := r.countByDevice(ctx, userID, from, to)
	if err != nil {
		return err
	}
	strides, sampled := deviceStrides(counts, limit)
	// LIMIT keeps the bound when rows arrive between the count and the scan.
	if !sampled {
		return r.stream(ctx, fn, plain+` LIMIT $4`, userID, from, to, limit)
	}
	ids := make([]int64, 0, len(strides))
	steps := make([]int64, 0, len(strides))
	for id, stride := range strides {
		ids = append(ids, id)
		steps = append(steps, stride)
	}
	return r.stream(ctx, fn, `SELECT `+positionColumns+` FROM (
			SELECT `+qualifiedPositionColumns+`, row_number() OVER (PARTITION BY p.device_id ORDER BY p.timestamp DESC) - 1 AS rn
			`+userRangeFilter+` AND p.device_id = ANY($4)
		 ) w
		 JOIN unnest($4::bigint[], $5::bigint[]) AS s(device_id, stride) USING (device_id)
		 WHERE w.rn % s.stride = 0
		 ORDER BY timestamp ASC
		 LIMIT $6`, userID, from, to, ids, steps, limit)
}

func (r *PositionRepository) countByDevice(ctx context.Context, userID int64, from, to time.Time) (map[int64]int64, error) {
	rows, err := r.pool.Query(ctx, `SELECT p.device_id, count(*) `+userRangeFilter+` GROUP BY p.device_id`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("count by device: %w", err)
	}
	counts := make(map[int64]int64)
	var id, n int64
	if _, err := pgx.ForEachRow(rows, []any{&id, &n}, func() error {
		counts[id] = n
		return nil
	}); err != nil {
		return nil, fmt.Errorf("count by device: %w", err)
	}
	return counts, nil
}

// deviceStrides splits limit fairly across devices: devices with fewer rows
// than their share keep all of them, the rest is shared among busier devices;
// every device gets at least one row while budget remains. It returns each
// device's sampling stride (keep rows where rn%stride == 0) and whether any row
// is dropped. Devices left without budget are omitted.
func deviceStrides(counts map[int64]int64, limit int) (map[int64]int64, bool) {
	ids := slices.SortedFunc(maps.Keys(counts), func(a, b int64) int {
		return cmp.Or(cmp.Compare(counts[a], counts[b]), cmp.Compare(a, b))
	})
	strides := make(map[int64]int64, len(ids))
	remaining := int64(limit)
	sampled := false
	for i, id := range ids {
		count := counts[id]
		if count <= 0 {
			continue
		}
		take := min(count, max(remaining/int64(len(ids)-i), min(1, remaining)))
		if take <= 0 {
			sampled = true
			continue
		}
		stride := (count + take - 1) / take
		strides[id] = stride
		remaining -= (count + stride - 1) / stride
		sampled = sampled || stride > 1
	}
	return strides, sampled
}

// streamSampled streams the rows of query ordered by timestamp. With limit > 0
// it keeps every ceil(total/limit)-th row, so at most limit rows spread over the
// whole result instead of only its start. query's own parameters must be $1..$3.
func (r *PositionRepository) streamSampled(ctx context.Context, fn func(*model.Position) error, limit int, query string, args ...any) error {
	if limit <= 0 {
		return r.stream(ctx, fn, query+` ORDER BY timestamp ASC`, args...)
	}
	// Counting separately keeps the window streaming: count(*) OVER () would
	// buffer every row of the range before emitting the first one.
	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM (`+query+`) s`, args...).Scan(&total); err != nil {
		return fmt.Errorf("count: %w", err)
	}
	// LIMIT keeps the bound when rows arrive between the count and the scan.
	if total <= int64(limit) {
		return r.stream(ctx, fn, query+` ORDER BY timestamp ASC LIMIT $4`, append(args, limit)...)
	}
	stride := (total + int64(limit) - 1) / int64(limit)
	return r.stream(ctx, fn, `SELECT `+positionColumns+` FROM (
			SELECT s.*, row_number() OVER (ORDER BY s.timestamp) - 1 AS rn
			FROM (`+query+`) s
		 ) w
		 WHERE rn % $4 = 0
		 ORDER BY timestamp ASC
		 LIMIT $5`, append(args, stride, limit)...)
}

func (r *PositionRepository) stream(ctx context.Context, fn func(*model.Position) error, query string, args ...any) error {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		p, err := rowToPosition(rows)
		if err != nil {
			return err
		}
		if err := fn(p); err != nil {
			return err
		}
	}
	return rows.Err()
}

// scanPosition scans a single row into a Position.
func scanPosition(scanner pgx.Row, p *model.Position) error {
	var attrs, network []byte
	// protocol column is nullable (added in migration 00014 without NOT NULL),
	// so we scan into *string to handle NULL values from pre-existing rows.
	var protocol *string
	// accuracy is non-nullable in the model (for Home Assistant compatibility),
	// but may be NULL in old rows before migration 00026. Scan into pointer and default to 0.0.
	var accuracy *float64
	err := scanner.Scan(
		&p.ID, &p.DeviceID, &protocol, &p.ServerTime, &p.DeviceTime,
		&p.Timestamp, &p.Valid, &p.Latitude, &p.Longitude, &p.Altitude, &p.Speed, &p.Course,
		&p.Address, &accuracy, &network, &p.GeofenceIDs, &p.Outdated, &attrs,
	)
	if err != nil {
		return err
	}
	if protocol != nil {
		p.Protocol = *protocol
	}
	if accuracy != nil {
		p.Accuracy = *accuracy
	} else {
		p.Accuracy = 0.0 // Default for Home Assistant compatibility
	}
	if len(attrs) > 0 {
		if err := json.Unmarshal(attrs, &p.Attributes); err != nil {
			slog.Warn("failed to unmarshal position attributes",
				slog.Int64("positionID", p.ID),
				slog.Any("error", err))
			p.Attributes = make(map[string]any)
		}
	}
	// Always ensure attributes is a non-nil map for Home Assistant
	// compatibility. HA expects {} (empty object), never null. The JSONB
	// value "null" round-trips through json.Unmarshal as a nil map, so we
	// must handle that case as well as SQL NULL (empty bytes).
	if p.Attributes == nil {
		p.Attributes = make(map[string]any)
	}
	if len(network) > 0 {
		if err := json.Unmarshal(network, &p.Network); err != nil {
			slog.Warn("failed to unmarshal position network",
				slog.Int64("positionID", p.ID),
				slog.Any("error", err))
			p.Network = make(map[string]any)
		}
	}
	// Same treatment for network: must be {} not null for Home Assistant.
	if p.Network == nil {
		p.Network = make(map[string]any)
	}
	return nil
}

func rowToPosition(row pgx.CollectableRow) (*model.Position, error) {
	p := &model.Position{}
	if err := scanPosition(row, p); err != nil {
		return nil, fmt.Errorf("scan position: %w", err)
	}
	return p, nil
}

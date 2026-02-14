package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
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
func (r *PositionRepository) GetLatestByUser(ctx context.Context, userID int64) ([]*model.Position, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT ON (p.device_id)
			p.id, p.device_id, p.protocol, p.server_time, p.device_time,
			p.timestamp, p.valid, p.latitude, p.longitude, p.altitude, p.speed, p.course,
			p.address, p.accuracy, p.network, p.geofence_ids, p.outdated, p.attributes
		 FROM positions p
		 JOIN user_devices ud ON ud.device_id = p.device_id
		 WHERE ud.user_id = $1
		 ORDER BY p.device_id, p.timestamp DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get latest positions by user: %w", err)
	}
	defer rows.Close()

	return scanPositions(rows)
}

// GetLatestAll returns the latest position for every device in the system (admin use).
func (r *PositionRepository) GetLatestAll(ctx context.Context) ([]*model.Position, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT ON (p.device_id)
			p.id, p.device_id, p.protocol, p.server_time, p.device_time,
			p.timestamp, p.valid, p.latitude, p.longitude, p.altitude, p.speed, p.course,
			p.address, p.accuracy, p.network, p.geofence_ids, p.outdated, p.attributes
		 FROM positions p
		 ORDER BY p.device_id, p.timestamp DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("get latest positions (all): %w", err)
	}
	defer rows.Close()

	return scanPositions(rows)
}

// maxPositionsPerQuery is the safety cap on the number of positions returned
// by a single time-range query. This prevents unbounded memory usage while
// still being large enough for 24h trails at high update frequencies (e.g.
// a simulator sending one position per second would produce ~86 400 points
// per day, but real devices typically send every 5-60s).
const maxPositionsPerQuery = 10000

// GetByDeviceAndTimeRange returns positions for a device within a time range.
// A limit of 0 or negative means "use maxPositionsPerQuery". Any positive
// value is capped at maxPositionsPerQuery.
func (r *PositionRepository) GetByDeviceAndTimeRange(
	ctx context.Context, deviceID int64, from, to time.Time, limit int,
) ([]*model.Position, error) {
	if limit <= 0 || limit > maxPositionsPerQuery {
		limit = maxPositionsPerQuery
	}

	rows, err := r.pool.Query(ctx,
		`SELECT `+positionColumns+`
		 FROM positions
		 WHERE device_id = $1 AND timestamp >= $2 AND timestamp <= $3
		 ORDER BY timestamp ASC
		 LIMIT $4`,
		deviceID, from, to, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get positions by time range: %w", err)
	}
	defer rows.Close()

	return scanPositions(rows)
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

// GetByIDs retrieves multiple positions by their IDs.
// Returns only the positions that exist; missing IDs are silently skipped.
func (r *PositionRepository) GetByIDs(ctx context.Context, ids []int64) ([]*model.Position, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	rows, err := r.pool.Query(ctx,
		`SELECT `+positionColumns+` FROM positions WHERE id = ANY($1)`, ids,
	)
	if err != nil {
		return nil, fmt.Errorf("get positions by ids: %w", err)
	}
	defer rows.Close()

	return scanPositions(rows)
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

// scanPosition scans a single row into a Position.
func scanPosition(scanner interface {
	Scan(dest ...interface{}) error
}, p *model.Position) error {
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
			p.Attributes = make(map[string]interface{})
		}
	}
	// Always ensure attributes is a non-nil map for Home Assistant
	// compatibility. HA expects {} (empty object), never null. The JSONB
	// value "null" round-trips through json.Unmarshal as a nil map, so we
	// must handle that case as well as SQL NULL (empty bytes).
	if p.Attributes == nil {
		p.Attributes = make(map[string]interface{})
	}
	if len(network) > 0 {
		if err := json.Unmarshal(network, &p.Network); err != nil {
			slog.Warn("failed to unmarshal position network",
				slog.Int64("positionID", p.ID),
				slog.Any("error", err))
			p.Network = make(map[string]interface{})
		}
	}
	// Same treatment for network: must be {} not null for Home Assistant.
	if p.Network == nil {
		p.Network = make(map[string]interface{})
	}
	return nil
}

func scanPositions(rows pgx.Rows) ([]*model.Position, error) {
	positions := make([]*model.Position, 0, 32)
	for rows.Next() {
		p := &model.Position{}
		if err := scanPosition(rows, p); err != nil {
			return nil, fmt.Errorf("scan position: %w", err)
		}
		positions = append(positions, p)
	}
	return positions, rows.Err()
}

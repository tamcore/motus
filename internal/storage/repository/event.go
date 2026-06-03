package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamcore/motus/internal/model"
)

// EventRepository handles event persistence.
type EventRepository struct {
	pool *pgxpool.Pool
}

// NewEventRepository creates a new event repository.
func NewEventRepository(pool *pgxpool.Pool) *EventRepository {
	return &EventRepository{pool: pool}
}

// Create inserts a new event.
func (r *EventRepository) Create(ctx context.Context, e *model.Event) error {
	attrs, err := json.Marshal(e.Attributes)
	if err != nil {
		return fmt.Errorf("marshal attributes: %w", err)
	}

	err = r.pool.QueryRow(ctx, `
		INSERT INTO events (device_id, geofence_id, type, position_id, timestamp, attributes)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, e.DeviceID, e.GeofenceID, e.Type, e.PositionID, e.Timestamp, attrs).
		Scan(&e.ID)
	if err != nil {
		return fmt.Errorf("create event: %w", err)
	}
	return nil
}

// GetByDevice retrieves the most recent events for a device.
func (r *EventRepository) GetByDevice(ctx context.Context, deviceID int64, limit int) ([]*model.Event, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, device_id, geofence_id, type, position_id, timestamp, attributes
		FROM events
		WHERE device_id = $1
		ORDER BY timestamp DESC
		LIMIT $2
	`, deviceID, limit)
	if err != nil {
		return nil, fmt.Errorf("get events by device: %w", err)
	}
	defer rows.Close()

	events := make([]*model.Event, 0, 32)
	for rows.Next() {
		var e model.Event
		var attrs []byte
		if err := rows.Scan(&e.ID, &e.DeviceID, &e.GeofenceID, &e.Type, &e.PositionID, &e.Timestamp, &attrs); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		if len(attrs) > 0 {
			if err := json.Unmarshal(attrs, &e.Attributes); err != nil {
				slog.Warn("failed to unmarshal event attributes",
					slog.Int64("eventID", e.ID),
					slog.Any("error", err))
				e.Attributes = make(map[string]interface{})
			}
		}
		events = append(events, &e)
	}
	return events, rows.Err()
}

// GetRecentByDeviceAndType retrieves the most recent events for a device
// filtered by event type, limited to the specified count.
func (r *EventRepository) GetRecentByDeviceAndType(ctx context.Context, deviceID int64, eventType string, limit int) ([]*model.Event, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, device_id, geofence_id, type, position_id, timestamp, attributes
		FROM events
		WHERE device_id = $1 AND type = $2
		ORDER BY timestamp DESC
		LIMIT $3
	`, deviceID, eventType, limit)
	if err != nil {
		return nil, fmt.Errorf("get recent events by device and type: %w", err)
	}
	defer rows.Close()

	events := make([]*model.Event, 0, 32)
	for rows.Next() {
		var e model.Event
		var attrs []byte
		if err := rows.Scan(&e.ID, &e.DeviceID, &e.GeofenceID, &e.Type, &e.PositionID, &e.Timestamp, &attrs); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		if len(attrs) > 0 {
			if err := json.Unmarshal(attrs, &e.Attributes); err != nil {
				slog.Warn("failed to unmarshal event attributes",
					slog.Int64("eventID", e.ID),
					slog.Any("error", err))
				e.Attributes = make(map[string]interface{})
			}
		}
		events = append(events, &e)
	}
	return events, rows.Err()
}

// GetByUser retrieves the most recent events for all devices a user has access to.
func (r *EventRepository) GetByUser(ctx context.Context, userID int64, limit int) ([]*model.Event, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	rows, err := r.pool.Query(ctx, `
		SELECT e.id, e.device_id, e.geofence_id, e.type, e.position_id, e.timestamp, e.attributes
		FROM events e
		JOIN user_devices ud ON ud.device_id = e.device_id
		WHERE ud.user_id = $1
		ORDER BY e.timestamp DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("get events by user: %w", err)
	}
	defer rows.Close()

	events := make([]*model.Event, 0, 32)
	for rows.Next() {
		var e model.Event
		var attrs []byte
		if err := rows.Scan(&e.ID, &e.DeviceID, &e.GeofenceID, &e.Type, &e.PositionID, &e.Timestamp, &attrs); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		if len(attrs) > 0 {
			if err := json.Unmarshal(attrs, &e.Attributes); err != nil {
				slog.Warn("failed to unmarshal event attributes",
					slog.Int64("eventID", e.ID),
					slog.Any("error", err))
				e.Attributes = make(map[string]interface{})
			}
		}
		events = append(events, &e)
	}
	return events, rows.Err()
}

// DeviceTripTotal holds the aggregate trip distance for a single device.
type DeviceTripTotal struct {
	DeviceID   int64
	DistanceKm float64
	TripCount  int
}

// SumTripDistance aggregates tripCompleted event distances per device for the
// given device IDs and time window. Returns per-device totals and the grand
// total. No row cap — uses SQL SUM instead of fetching rows.
func (r *EventRepository) SumTripDistance(ctx context.Context, deviceIDs []int64, from, to time.Time) ([]DeviceTripTotal, float64, error) {
	if len(deviceIDs) == 0 {
		return nil, 0, nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT device_id,
		       COALESCE(SUM((attributes->>'distance')::float8), 0) AS km,
		       COUNT(*) AS trips
		FROM events
		WHERE type = 'tripCompleted'
		  AND device_id = ANY($1)
		  AND timestamp >= $2
		  AND timestamp < $3
		GROUP BY device_id
	`, deviceIDs, from, to)
	if err != nil {
		return nil, 0, fmt.Errorf("sum trip distance: %w", err)
	}
	defer rows.Close()

	var totals []DeviceTripTotal
	var grandTotal float64
	for rows.Next() {
		var t DeviceTripTotal
		if err := rows.Scan(&t.DeviceID, &t.DistanceKm, &t.TripCount); err != nil {
			return nil, 0, fmt.Errorf("scan trip total: %w", err)
		}
		totals = append(totals, t)
		grandTotal += t.DistanceKm
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return totals, grandTotal, nil
}

// GetByFilters retrieves events filtered by user access, device IDs, event types, and time range.
func (r *EventRepository) GetByFilters(ctx context.Context, userID int64, deviceIDs []int64, eventTypes []string, from, to time.Time) ([]*model.Event, error) {
	query := `
		SELECT DISTINCT e.id, e.device_id, e.geofence_id, e.type, e.position_id, e.timestamp, e.attributes
		FROM events e
		INNER JOIN user_devices ud ON e.device_id = ud.device_id
		WHERE ud.user_id = $1
		  AND e.timestamp >= $2
		  AND e.timestamp <= $3
	`

	args := []interface{}{userID, from, to}
	argIdx := 4

	if len(deviceIDs) > 0 {
		placeholders := make([]string, len(deviceIDs))
		for i, id := range deviceIDs {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, id)
			argIdx++
		}
		query += " AND e.device_id IN (" + strings.Join(placeholders, ",") + ")"
	}

	if len(eventTypes) > 0 {
		placeholders := make([]string, len(eventTypes))
		for i, t := range eventTypes {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, t)
			argIdx++
		}
		query += " AND e.type IN (" + strings.Join(placeholders, ",") + ")"
	}

	query += " ORDER BY e.timestamp DESC LIMIT 1000"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get events by filters: %w", err)
	}
	defer rows.Close()

	events := make([]*model.Event, 0, 32)
	for rows.Next() {
		var e model.Event
		var attrs []byte
		if err := rows.Scan(&e.ID, &e.DeviceID, &e.GeofenceID, &e.Type, &e.PositionID, &e.Timestamp, &attrs); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		if len(attrs) > 0 {
			if err := json.Unmarshal(attrs, &e.Attributes); err != nil {
				slog.Warn("failed to unmarshal event attributes",
					slog.Int64("eventID", e.ID),
					slog.Any("error", err))
				e.Attributes = make(map[string]interface{})
			}
		}
		events = append(events, &e)
	}
	return events, rows.Err()
}

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamcore/motus/internal/model"
)

// DeviceRepository handles device persistence.
type DeviceRepository struct {
	pool *pgxpool.Pool
}

// NewDeviceRepository creates a new device repository.
func NewDeviceRepository(pool *pgxpool.Pool) *DeviceRepository {
	return &DeviceRepository{pool: pool}
}

// deviceColumns is the list of columns selected for device queries.
const deviceColumns = `id, unique_id, name, protocol, status, speed_limit, last_update,
	position_id, group_id, phone, model, contact, category, disabled, mileage, pending_mileage,
	ignition_on, last_ignition_time, attributes, battery_level,
	created_at, updated_at`

// scanDevice scans deviceColumns, followed by extra, into d.
func scanDevice(row pgx.Row, d *model.Device, extra ...any) error {
	var attrs []byte
	dest := append([]any{
		&d.ID, &d.UniqueID, &d.Name, &d.Protocol, &d.Status, &d.SpeedLimit, &d.LastUpdate,
		&d.PositionID, &d.GroupID, &d.Phone, &d.Model, &d.Contact, &d.Category, &d.Disabled,
		&d.Mileage, &d.PendingMileage,
		&d.IgnitionOn, &d.LastIgnitionTime, &attrs, &d.BatteryLevel,
		&d.CreatedAt, &d.UpdatedAt,
	}, extra...)
	if err := row.Scan(dest...); err != nil {
		return err
	}
	if len(attrs) > 0 {
		if err := json.Unmarshal(attrs, &d.Attributes); err != nil {
			slog.Warn("failed to unmarshal device attributes",
				slog.Int64("deviceID", d.ID),
				slog.Any("error", err))
			d.Attributes = make(map[string]any)
		}
	}
	// Always ensure attributes is a non-nil map for Home Assistant
	// compatibility. HA expects {} (empty object), never null. The JSONB
	// value "null" round-trips through json.Unmarshal as a nil map, so we
	// must handle that case as well as SQL NULL (empty bytes).
	if d.Attributes == nil {
		d.Attributes = make(map[string]any)
	}
	return nil
}

// UserHasAccess checks if a user has access to a device.
func (r *DeviceRepository) UserHasAccess(ctx context.Context, user *model.User, deviceID int64) bool {
	if user.IsAdmin() {
		return true
	}
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM user_devices WHERE user_id = $1 AND device_id = $2)`,
		user.ID, deviceID,
	).Scan(&exists)
	return err == nil && exists
}

// GetByID retrieves a device by its ID.
func (r *DeviceRepository) GetByID(ctx context.Context, id int64) (*model.Device, error) {
	d := &model.Device{}
	err := scanDevice(r.pool.QueryRow(ctx,
		`SELECT `+deviceColumns+` FROM devices WHERE id = $1`, id,
	), d)
	if err != nil {
		return nil, fmt.Errorf("get device by id: %w", err)
	}
	return d, nil
}

// GetByUniqueID retrieves a device by its unique identifier.
func (r *DeviceRepository) GetByUniqueID(ctx context.Context, uniqueID string) (*model.Device, error) {
	d := &model.Device{}
	err := scanDevice(r.pool.QueryRow(ctx,
		`SELECT `+deviceColumns+` FROM devices WHERE unique_id = $1`, uniqueID,
	), d)
	if err != nil {
		return nil, fmt.Errorf("get device by unique_id: %w", err)
	}
	return d, nil
}

// GetByUser retrieves all devices a user has access to.
func (r *DeviceRepository) GetByUser(ctx context.Context, userID int64) ([]*model.Device, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+deviceColumns+`
		 FROM devices d
		 JOIN user_devices ud ON ud.device_id = d.id
		 WHERE ud.user_id = $1
		 ORDER BY d.name`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get devices by user: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (*model.Device, error) {
		d, err := rowToDevice(row)
		return &d, err
	})
}

// GetAll retrieves all devices, ordered by name.
func (r *DeviceRepository) GetAll(ctx context.Context) ([]model.Device, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+deviceColumns+` FROM devices ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("get all devices: %w", err)
	}
	return pgx.CollectRows(rows, rowToDevice)
}

// GetAllWithOwners returns all devices with owner name from user_devices join.
func (r *DeviceRepository) GetAllWithOwners(ctx context.Context) ([]*model.Device, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+deviceColumns+`, COALESCE(
			(SELECT u.name FROM user_devices ud JOIN users u ON u.id = ud.user_id WHERE ud.device_id = d.id LIMIT 1),
			''
		) AS owner_name
		FROM devices d
		ORDER BY d.name`,
	)
	if err != nil {
		return nil, fmt.Errorf("get all devices with owners: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (*model.Device, error) {
		var d model.Device
		if err := scanDevice(row, &d, &d.OwnerName); err != nil {
			return nil, fmt.Errorf("scan device with owner: %w", err)
		}
		return &d, nil
	})
}

// GetTimedOut returns devices with status 'online' or 'moving' whose
// last_update is before the given cutoff time (or NULL). This pushes
// the filtering to SQL so we don't need to load every device into memory.
func (r *DeviceRepository) GetTimedOut(ctx context.Context, cutoff time.Time) ([]model.Device, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+deviceColumns+` FROM devices
		 WHERE status IN ('online', 'moving')
		   AND (last_update IS NULL OR last_update < $1)`, cutoff,
	)
	if err != nil {
		return nil, fmt.Errorf("get timed out devices: %w", err)
	}
	return pgx.CollectRows(rows, rowToDevice)
}

// GetUserIDs returns the user IDs associated with a device.
func (r *DeviceRepository) GetUserIDs(ctx context.Context, deviceID int64) ([]int64, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT user_id FROM user_devices WHERE device_id = $1`, deviceID,
	)
	if err != nil {
		return nil, fmt.Errorf("get user ids for device: %w", err)
	}
	return pgx.AppendRows([]int64(nil), rows, pgx.RowTo[int64])
}

// Create inserts a new device and associates it with a user.
func (r *DeviceRepository) Create(ctx context.Context, d *model.Device, userID int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	attrs, _ := json.Marshal(d.Attributes)

	err = tx.QueryRow(ctx,
		`INSERT INTO devices (unique_id, name, protocol, status, speed_limit, phone, model, contact, category, disabled, mileage, attributes)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 RETURNING id, created_at, updated_at`,
		d.UniqueID, d.Name, d.Protocol, d.Status, d.SpeedLimit,
		d.Phone, d.Model, d.Contact, d.Category, d.Disabled, d.Mileage, attrs,
	).Scan(&d.ID, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert device: %w", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO user_devices (user_id, device_id) VALUES ($1, $2)`,
		userID, d.ID,
	)
	if err != nil {
		return fmt.Errorf("associate device with user: %w", err)
	}

	return tx.Commit(ctx)
}

// Update modifies an existing device.
func (r *DeviceRepository) Update(ctx context.Context, d *model.Device) error {
	attrs, _ := json.Marshal(d.Attributes)

	_, err := r.pool.Exec(ctx,
		`UPDATE devices SET unique_id = $1, name = $2, protocol = $3, status = $4, speed_limit = $5, last_update = $6,
			position_id = $7, phone = $8, model = $9, contact = $10, category = $11, disabled = $12,
			mileage = $13, pending_mileage = $14, attributes = $15,
			ignition_on = $16, last_ignition_time = $17,
			updated_at = NOW()
		 WHERE id = $18`,
		d.UniqueID, d.Name, d.Protocol, d.Status, d.SpeedLimit, d.LastUpdate,
		d.PositionID, d.Phone, d.Model, d.Contact, d.Category, d.Disabled,
		d.Mileage, d.PendingMileage, attrs,
		d.IgnitionOn, d.LastIgnitionTime,
		d.ID,
	)
	if err != nil {
		return fmt.Errorf("update device: %w", err)
	}
	return nil
}

// SetIgnitionState records the ignition state of a position at ts and reports
// whether it changed. Positions older than last_ignition_time are ignored. An
// unchanged "on" state refreshes last_ignition_time; an unchanged "off" state
// writes nothing. The row lock makes concurrent positions see each other's
// writes, so one change reports changed exactly once.
func (r *DeviceRepository) SetIgnitionState(ctx context.Context, id int64, on bool, ts time.Time) (bool, error) {
	var prev bool
	err := r.pool.QueryRow(ctx,
		`WITH prev AS (SELECT ignition_on FROM devices WHERE id = $1 FOR UPDATE)
		 UPDATE devices d SET ignition_on = $2, last_ignition_time = $3
		 FROM prev
		 WHERE d.id = $1
		   AND (d.last_ignition_time IS NULL OR d.last_ignition_time <= $3)
		   AND (prev.ignition_on <> $2 OR $2)
		 RETURNING prev.ignition_on`,
		id, on, ts,
	).Scan(&prev)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("set ignition state: %w", err)
	}
	return prev != on, nil
}

// MarkOnline records a new position on a device: status online, last_update,
// position_id, battery_level (when batteryLevel is non-nil; nil keeps the
// last known level), and clears disabled. Only these columns are written, so
// concurrent edits to other fields are kept. Returns the updated device.
func (r *DeviceRepository) MarkOnline(ctx context.Context, id, positionID int64, at time.Time, batteryLevel *float64) (*model.Device, error) {
	d := &model.Device{}
	err := scanDevice(r.pool.QueryRow(ctx,
		`UPDATE devices SET status = 'online', last_update = $2, position_id = $3, disabled = false,
			battery_level = COALESCE($4, battery_level), updated_at = NOW()
		 WHERE id = $1
		 RETURNING `+deviceColumns, id, at, positionID, batteryLevel,
	), d)
	if err != nil {
		return nil, fmt.Errorf("mark device online: %w", err)
	}
	return d, nil
}

// UpdateProtocol sets the protocol field on a device without touching other
// columns. Used by the protocol server to resync the protocol when a known
// device starts sending packets via a different protocol than what is stored.
func (r *DeviceRepository) UpdateProtocol(ctx context.Context, id int64, protocol string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE devices SET protocol = $1, updated_at = NOW() WHERE id = $2`,
		protocol, id,
	)
	if err != nil {
		return fmt.Errorf("update device protocol: %w", err)
	}
	return nil
}

// Delete removes a device by ID. Cascades to positions and user_devices.
func (r *DeviceRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM devices WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete device: %w", err)
	}
	return nil
}

func rowToDevice(row pgx.CollectableRow) (model.Device, error) {
	var d model.Device
	if err := scanDevice(row, &d); err != nil {
		return model.Device{}, fmt.Errorf("scan device: %w", err)
	}
	return d, nil
}

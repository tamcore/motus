package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

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
	position_id, group_id, phone, model, contact, category, disabled, mileage, pending_mileage, attributes,
	created_at, updated_at`

// scanDevice scans a device row into a model.Device.
func scanDevice(scanner interface {
	Scan(dest ...interface{}) error
}, d *model.Device) error {
	var attrs []byte
	err := scanner.Scan(
		&d.ID, &d.UniqueID, &d.Name, &d.Protocol, &d.Status, &d.SpeedLimit, &d.LastUpdate,
		&d.PositionID, &d.GroupID, &d.Phone, &d.Model, &d.Contact, &d.Category, &d.Disabled,
		&d.Mileage, &d.PendingMileage, &attrs,
		&d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return err
	}
	if len(attrs) > 0 {
		if err := json.Unmarshal(attrs, &d.Attributes); err != nil {
			slog.Warn("failed to unmarshal device attributes",
				slog.Int64("deviceID", d.ID),
				slog.Any("error", err))
			d.Attributes = make(map[string]interface{})
		}
	}
	// Always ensure attributes is a non-nil map for Home Assistant
	// compatibility. HA expects {} (empty object), never null. The JSONB
	// value "null" round-trips through json.Unmarshal as a nil map, so we
	// must handle that case as well as SQL NULL (empty bytes).
	if d.Attributes == nil {
		d.Attributes = make(map[string]interface{})
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
		`SELECT d.id, d.unique_id, d.name, d.protocol, d.status, d.speed_limit, d.last_update,
			d.position_id, d.group_id, d.phone, d.model, d.contact, d.category, d.disabled,
			d.mileage, d.pending_mileage, d.attributes,
			d.created_at, d.updated_at
		 FROM devices d
		 JOIN user_devices ud ON ud.device_id = d.id
		 WHERE ud.user_id = $1
		 ORDER BY d.name`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get devices by user: %w", err)
	}
	defer rows.Close()

	devices := make([]*model.Device, 0, 32)
	for rows.Next() {
		d := &model.Device{}
		if err := scanDevice(rows, d); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

// GetAll retrieves all devices, ordered by name.
func (r *DeviceRepository) GetAll(ctx context.Context) ([]model.Device, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+deviceColumns+` FROM devices ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("get all devices: %w", err)
	}
	defer rows.Close()

	devices := make([]model.Device, 0, 32)
	for rows.Next() {
		var d model.Device
		if err := scanDevice(rows, &d); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

// GetAllWithOwners returns all devices with owner name from user_devices join.
func (r *DeviceRepository) GetAllWithOwners(ctx context.Context) ([]model.Device, error) {
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
	defer rows.Close()

	devices := make([]model.Device, 0, 32)
	for rows.Next() {
		var d model.Device
		var attrs []byte
		err := rows.Scan(
			&d.ID, &d.UniqueID, &d.Name, &d.Protocol, &d.Status, &d.SpeedLimit, &d.LastUpdate,
			&d.PositionID, &d.GroupID, &d.Phone, &d.Model, &d.Contact, &d.Category, &d.Disabled,
			&d.Mileage, &d.PendingMileage, &attrs,
			&d.CreatedAt, &d.UpdatedAt,
			&d.OwnerName,
		)
		if err != nil {
			return nil, fmt.Errorf("scan device with owner: %w", err)
		}
		if len(attrs) > 0 {
			if err := json.Unmarshal(attrs, &d.Attributes); err != nil {
				slog.Warn("failed to unmarshal device attributes",
					slog.Int64("deviceID", d.ID),
					slog.Any("error", err))
				d.Attributes = make(map[string]interface{})
			}
		}
		if d.Attributes == nil {
			d.Attributes = make(map[string]interface{})
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
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
	defer rows.Close()

	devices := make([]model.Device, 0, 16)
	for rows.Next() {
		var d model.Device
		if err := scanDevice(rows, &d); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

// GetUserIDs returns the user IDs associated with a device.
func (r *DeviceRepository) GetUserIDs(ctx context.Context, deviceID int64) ([]int64, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT user_id FROM user_devices WHERE device_id = $1`, deviceID,
	)
	if err != nil {
		return nil, fmt.Errorf("get user ids for device: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan user id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
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
		`UPDATE devices SET name = $1, protocol = $2, status = $3, speed_limit = $4, last_update = $5,
			position_id = $6, phone = $7, model = $8, contact = $9, category = $10, disabled = $11,
			mileage = $12, pending_mileage = $13, attributes = $14,
			updated_at = NOW()
		 WHERE id = $15`,
		d.Name, d.Protocol, d.Status, d.SpeedLimit, d.LastUpdate,
		d.PositionID, d.Phone, d.Model, d.Contact, d.Category, d.Disabled,
		d.Mileage, d.PendingMileage, attrs,
		d.ID,
	)
	if err != nil {
		return fmt.Errorf("update device: %w", err)
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

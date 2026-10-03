package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamcore/motus/internal/model"
)

// ErrTrailBookmarkNotFound is returned (wrapped) by GetByID when no bookmark
// has the given ID.
var ErrTrailBookmarkNotFound = errors.New("trail bookmark not found")

// TrailBookmarkRepository handles trail bookmark persistence.
type TrailBookmarkRepository struct {
	pool *pgxpool.Pool
}

// NewTrailBookmarkRepository creates a new trail bookmark repository.
func NewTrailBookmarkRepository(pool *pgxpool.Pool) *TrailBookmarkRepository {
	return &TrailBookmarkRepository{pool: pool}
}

const trailBookmarkColumns = `b.id, b.user_id, b.device_id, COALESCE(d.name, ''), b.name, b.description,
	b.from_time, b.to_time, b.created_at, b.updated_at`

func scanTrailBookmark(row pgx.Row, b *model.TrailBookmark) error {
	return row.Scan(&b.ID, &b.UserID, &b.DeviceID, &b.DeviceName, &b.Name, &b.Description,
		&b.From, &b.To, &b.CreatedAt, &b.UpdatedAt)
}

// Create inserts a new bookmark and sets its ID and timestamps.
func (r *TrailBookmarkRepository) Create(ctx context.Context, b *model.TrailBookmark) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO trail_bookmarks (user_id, device_id, name, description, from_time, to_time)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`, b.UserID, b.DeviceID, b.Name, b.Description, b.From, b.To).Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create trail bookmark: %w", err)
	}
	return nil
}

// GetByID retrieves a bookmark by its ID, including the device name.
func (r *TrailBookmarkRepository) GetByID(ctx context.Context, id int64) (*model.TrailBookmark, error) {
	var b model.TrailBookmark
	err := scanTrailBookmark(r.pool.QueryRow(ctx, `
		SELECT `+trailBookmarkColumns+`
		FROM trail_bookmarks b
		LEFT JOIN devices d ON d.id = b.device_id
		WHERE b.id = $1
	`, id), &b)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("get trail bookmark by id: %w", ErrTrailBookmarkNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get trail bookmark by id: %w", err)
	}
	return &b, nil
}

// ListForUser returns the user's own bookmarks, newest range first. For
// non-admins, bookmarks of devices the user is no longer assigned to are
// omitted. A non-nil deviceID restricts the result to that device.
func (r *TrailBookmarkRepository) ListForUser(ctx context.Context, user *model.User, deviceID *int64) ([]*model.TrailBookmark, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+trailBookmarkColumns+`
		FROM trail_bookmarks b
		JOIN devices d ON d.id = b.device_id
		WHERE b.user_id = $1
		  AND ($2::bigint IS NULL OR b.device_id = $2)
		  AND ($3 OR EXISTS (
			SELECT 1 FROM user_devices ud
			WHERE ud.user_id = b.user_id AND ud.device_id = b.device_id
		  ))
		ORDER BY b.from_time DESC, b.id DESC
	`, user.ID, deviceID, user.IsAdmin())
	if err != nil {
		return nil, fmt.Errorf("list trail bookmarks: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (*model.TrailBookmark, error) {
		var b model.TrailBookmark
		if err := scanTrailBookmark(row, &b); err != nil {
			return nil, fmt.Errorf("scan trail bookmark: %w", err)
		}
		return &b, nil
	})
}

// Update replaces the bookmark's device, name, description and range.
func (r *TrailBookmarkRepository) Update(ctx context.Context, b *model.TrailBookmark) error {
	err := r.pool.QueryRow(ctx, `
		UPDATE trail_bookmarks
		SET device_id = $1, name = $2, description = $3, from_time = $4, to_time = $5, updated_at = NOW()
		WHERE id = $6
		RETURNING updated_at
	`, b.DeviceID, b.Name, b.Description, b.From, b.To, b.ID).Scan(&b.UpdatedAt)
	if err != nil {
		return fmt.Errorf("update trail bookmark: %w", err)
	}
	return nil
}

// Delete removes a bookmark by ID.
func (r *TrailBookmarkRepository) Delete(ctx context.Context, id int64) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM trail_bookmarks WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete trail bookmark: %w", err)
	}
	return nil
}

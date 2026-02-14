package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamcore/motus/internal/model"
)

// CalendarRepository handles calendar persistence.
type CalendarRepository struct {
	pool *pgxpool.Pool
}

// NewCalendarRepository creates a new calendar repository.
func NewCalendarRepository(pool *pgxpool.Pool) *CalendarRepository {
	return &CalendarRepository{pool: pool}
}

// Create inserts a new calendar and associates it with the user.
func (r *CalendarRepository) Create(ctx context.Context, c *model.Calendar) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO calendars (user_id, name, data, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		RETURNING id, created_at, updated_at
	`, c.UserID, c.Name, c.Data).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create calendar: %w", err)
	}

	// Associate calendar with the creating user.
	_, err = r.pool.Exec(ctx, `
		INSERT INTO user_calendars (user_id, calendar_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, c.UserID, c.ID)
	if err != nil {
		return fmt.Errorf("associate calendar with user: %w", err)
	}

	return nil
}

// GetByID retrieves a calendar by its ID.
func (r *CalendarRepository) GetByID(ctx context.Context, id int64) (*model.Calendar, error) {
	var c model.Calendar
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, name, data, created_at, updated_at
		FROM calendars
		WHERE id = $1
	`, id).Scan(&c.ID, &c.UserID, &c.Name, &c.Data, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get calendar by id: %w", err)
	}
	return &c, nil
}

// GetByUser retrieves all calendars associated with a user.
func (r *CalendarRepository) GetByUser(ctx context.Context, userID int64) ([]*model.Calendar, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.user_id, c.name, c.data, c.created_at, c.updated_at
		FROM calendars c
		JOIN user_calendars uc ON c.id = uc.calendar_id
		WHERE uc.user_id = $1
		ORDER BY c.name
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get calendars by user: %w", err)
	}
	defer rows.Close()

	calendars := make([]*model.Calendar, 0, 8)
	for rows.Next() {
		var c model.Calendar
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Data, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan calendar: %w", err)
		}
		calendars = append(calendars, &c)
	}
	return calendars, rows.Err()
}

// GetAll retrieves all calendars with owner names.
func (r *CalendarRepository) GetAll(ctx context.Context) ([]*model.Calendar, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.user_id, c.name, c.data, c.created_at, c.updated_at,
			COALESCE(u.name, '') AS owner_name
		FROM calendars c
		LEFT JOIN users u ON u.id = c.user_id
		ORDER BY c.name
	`)
	if err != nil {
		return nil, fmt.Errorf("get all calendars: %w", err)
	}
	defer rows.Close()

	calendars := make([]*model.Calendar, 0, 8)
	for rows.Next() {
		var c model.Calendar
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Data, &c.CreatedAt, &c.UpdatedAt, &c.OwnerName); err != nil {
			return nil, fmt.Errorf("scan calendar: %w", err)
		}
		calendars = append(calendars, &c)
	}
	return calendars, rows.Err()
}

// Update modifies an existing calendar.
func (r *CalendarRepository) Update(ctx context.Context, c *model.Calendar) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE calendars
		SET name = $1, data = $2, updated_at = NOW()
		WHERE id = $3
	`, c.Name, c.Data, c.ID)
	if err != nil {
		return fmt.Errorf("update calendar: %w", err)
	}
	return nil
}

// Delete removes a calendar by ID. Geofences referencing this calendar
// will have their calendar_id set to NULL (via ON DELETE SET NULL).
func (r *CalendarRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM calendars WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete calendar: %w", err)
	}
	return nil
}

// UserHasAccess checks if a user has access to a calendar.
func (r *CalendarRepository) UserHasAccess(ctx context.Context, user *model.User, calendarID int64) bool {
	if user.IsAdmin() {
		return true
	}
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM user_calendars WHERE user_id = $1 AND calendar_id = $2)`,
		user.ID, calendarID,
	).Scan(&exists)
	return err == nil && exists
}

// AssociateUser links a calendar to a user. No-op if the association already exists.
func (r *CalendarRepository) AssociateUser(ctx context.Context, userID, calendarID int64) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_calendars (user_id, calendar_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, userID, calendarID)
	if err != nil {
		return fmt.Errorf("associate user with calendar: %w", err)
	}
	return nil
}

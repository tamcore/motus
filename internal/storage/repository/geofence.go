package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamcore/motus/internal/model"
)

// GeofenceRepository handles geofence persistence with PostGIS spatial queries.
type GeofenceRepository struct {
	pool *pgxpool.Pool
}

// NewGeofenceRepository creates a new geofence repository.
func NewGeofenceRepository(pool *pgxpool.Pool) *GeofenceRepository {
	return &GeofenceRepository{pool: pool}
}

// isWKT returns true if the input looks like WKT (starts with a geometry type keyword).
func isWKT(s string) bool {
	upper := strings.ToUpper(strings.TrimSpace(s))
	return strings.HasPrefix(upper, "POLYGON") ||
		strings.HasPrefix(upper, "CIRCLE") ||
		strings.HasPrefix(upper, "LINESTRING") ||
		strings.HasPrefix(upper, "POINT") ||
		strings.HasPrefix(upper, "MULTIPOLYGON")
}

// Create inserts a new geofence. The geometry field accepts GeoJSON or WKT.
func (r *GeofenceRepository) Create(ctx context.Context, g *model.Geofence) error {
	attrs, err := json.Marshal(g.Attributes)
	if err != nil {
		return fmt.Errorf("marshal attributes: %w", err)
	}

	// Determine which input we have: the Area (WKT) or Geometry (GeoJSON).
	geomInput := g.Geometry
	if geomInput == "" {
		geomInput = g.Area
	}

	var geomExpr string
	if isWKT(geomInput) {
		geomExpr = "ST_GeomFromText($3, 4326)"
	} else {
		geomExpr = "ST_GeomFromGeoJSON($3)"
	}

	query := fmt.Sprintf(`
		INSERT INTO geofences (name, description, geometry, attributes, calendar_id, created_at, updated_at)
		VALUES ($1, $2, %s, $4, $5, NOW(), NOW())
		RETURNING id, ST_AsText(geometry), ST_AsGeoJSON(geometry), calendar_id, created_at, updated_at
	`, geomExpr)

	err = r.pool.QueryRow(ctx, query,
		g.Name, g.Description, geomInput, attrs, g.CalendarID,
	).Scan(&g.ID, &g.Area, &g.Geometry, &g.CalendarID, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create geofence: %w", err)
	}
	return nil
}

// GetByID retrieves a geofence by its ID, returning area as WKT and geometry as GeoJSON.
func (r *GeofenceRepository) GetByID(ctx context.Context, id int64) (*model.Geofence, error) {
	var g model.Geofence
	var attrs []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, description, ST_AsText(geometry), ST_AsGeoJSON(geometry), attributes, calendar_id, created_at, updated_at
		FROM geofences
		WHERE id = $1
	`, id).Scan(&g.ID, &g.Name, &g.Description, &g.Area, &g.Geometry, &attrs, &g.CalendarID, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get geofence by id: %w", err)
	}
	if len(attrs) > 0 {
		if err := json.Unmarshal(attrs, &g.Attributes); err != nil {
			slog.Warn("failed to unmarshal geofence attributes",
				slog.Int64("geofenceID", g.ID),
				slog.Any("error", err))
			g.Attributes = make(map[string]interface{})
		}
	}
	return &g, nil
}

// GetByUser retrieves all geofences associated with a user.
func (r *GeofenceRepository) GetByUser(ctx context.Context, userID int64) ([]*model.Geofence, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT g.id, g.name, g.description, ST_AsText(g.geometry), ST_AsGeoJSON(g.geometry), g.attributes, g.calendar_id, g.created_at, g.updated_at
		FROM geofences g
		JOIN user_geofences ug ON g.id = ug.geofence_id
		WHERE ug.user_id = $1
		ORDER BY g.name
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get geofences by user: %w", err)
	}
	defer rows.Close()

	geofences := make([]*model.Geofence, 0, 16)
	for rows.Next() {
		var g model.Geofence
		var attrs []byte
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.Area, &g.Geometry, &attrs, &g.CalendarID, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan geofence: %w", err)
		}
		if len(attrs) > 0 {
			if err := json.Unmarshal(attrs, &g.Attributes); err != nil {
				slog.Warn("failed to unmarshal geofence attributes",
					slog.Int64("geofenceID", g.ID),
					slog.Any("error", err))
				g.Attributes = make(map[string]interface{})
			}
		}
		geofences = append(geofences, &g)
	}
	return geofences, rows.Err()
}

// GetAll retrieves all geofences, ordered by name.
func (r *GeofenceRepository) GetAll(ctx context.Context) ([]*model.Geofence, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, description, ST_AsText(geometry), ST_AsGeoJSON(geometry), attributes, calendar_id, created_at, updated_at
		FROM geofences
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("get all geofences: %w", err)
	}
	defer rows.Close()

	geofences := make([]*model.Geofence, 0, 16)
	for rows.Next() {
		var g model.Geofence
		var attrs []byte
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.Area, &g.Geometry, &attrs, &g.CalendarID, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan geofence: %w", err)
		}
		if len(attrs) > 0 {
			if err := json.Unmarshal(attrs, &g.Attributes); err != nil {
				slog.Warn("failed to unmarshal geofence attributes",
					slog.Int64("geofenceID", g.ID),
					slog.Any("error", err))
				g.Attributes = make(map[string]interface{})
			}
		}
		geofences = append(geofences, &g)
	}
	return geofences, rows.Err()
}

// GetAllWithOwners retrieves all geofences with owner names from user_geofences join.
func (r *GeofenceRepository) GetAllWithOwners(ctx context.Context) ([]*model.Geofence, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT g.id, g.name, g.description, ST_AsText(g.geometry), ST_AsGeoJSON(g.geometry),
			g.attributes, g.calendar_id, g.created_at, g.updated_at,
			COALESCE(
				(SELECT u.name FROM user_geofences ug JOIN users u ON u.id = ug.user_id WHERE ug.geofence_id = g.id LIMIT 1),
				''
			) AS owner_name
		FROM geofences g
		ORDER BY g.name
	`)
	if err != nil {
		return nil, fmt.Errorf("get all geofences with owners: %w", err)
	}
	defer rows.Close()

	geofences := make([]*model.Geofence, 0, 16)
	for rows.Next() {
		var g model.Geofence
		var attrs []byte
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.Area, &g.Geometry, &attrs, &g.CalendarID, &g.CreatedAt, &g.UpdatedAt, &g.OwnerName); err != nil {
			return nil, fmt.Errorf("scan geofence with owner: %w", err)
		}
		if len(attrs) > 0 {
			if err := json.Unmarshal(attrs, &g.Attributes); err != nil {
				slog.Warn("failed to unmarshal geofence attributes",
					slog.Int64("geofenceID", g.ID),
					slog.Any("error", err))
				g.Attributes = make(map[string]interface{})
			}
		}
		geofences = append(geofences, &g)
	}
	return geofences, rows.Err()
}

// Update modifies an existing geofence. Accepts GeoJSON or WKT for geometry.
func (r *GeofenceRepository) Update(ctx context.Context, g *model.Geofence) error {
	attrs, err := json.Marshal(g.Attributes)
	if err != nil {
		return fmt.Errorf("marshal attributes: %w", err)
	}

	// Use Area (WKT) or Geometry (GeoJSON) as geometry input.
	geomInput := g.Geometry
	if geomInput == "" {
		geomInput = g.Area
	}

	var geomExpr string
	if isWKT(geomInput) {
		geomExpr = "ST_GeomFromText($3, 4326)"
	} else {
		geomExpr = "ST_GeomFromGeoJSON($3)"
	}

	query := fmt.Sprintf(`
		UPDATE geofences
		SET name = $1, description = $2, geometry = %s, attributes = $4, calendar_id = $5, updated_at = NOW()
		WHERE id = $6
	`, geomExpr)

	_, err = r.pool.Exec(ctx, query,
		g.Name, g.Description, geomInput, attrs, g.CalendarID, g.ID,
	)
	if err != nil {
		return fmt.Errorf("update geofence: %w", err)
	}
	return nil
}

// Delete removes a geofence by ID. Cascades to user_geofences; events set geofence_id to NULL.
func (r *GeofenceRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM geofences WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete geofence: %w", err)
	}
	return nil
}

// AssociateUser links a geofence to a user. No-op if the association already exists.
func (r *GeofenceRepository) AssociateUser(ctx context.Context, userID, geofenceID int64) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_geofences (user_id, geofence_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, userID, geofenceID)
	if err != nil {
		return fmt.Errorf("associate user with geofence: %w", err)
	}
	return nil
}

// UserHasAccess checks if a user has access to a geofence.
func (r *GeofenceRepository) UserHasAccess(ctx context.Context, user *model.User, geofenceID int64) bool {
	if user.IsAdmin() {
		return true
	}
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM user_geofences WHERE user_id = $1 AND geofence_id = $2)`,
		user.ID, geofenceID,
	).Scan(&exists)
	return err == nil && exists
}

// CheckContainment returns the IDs of geofences (associated with the given user)
// that contain the specified point. PostGIS uses longitude/latitude (X/Y) order.
func (r *GeofenceRepository) CheckContainment(ctx context.Context, userID int64, lat, lon float64) ([]int64, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT g.id
		FROM geofences g
		JOIN user_geofences ug ON g.id = ug.geofence_id
		WHERE ug.user_id = $1
		  AND ST_Contains(g.geometry, ST_SetSRID(ST_MakePoint($2, $3), 4326))
	`, userID, lon, lat) // PostGIS: ST_MakePoint(lon, lat)
	if err != nil {
		return nil, fmt.Errorf("check geofence containment: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan geofence id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

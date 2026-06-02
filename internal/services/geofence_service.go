package services

import (
	"context"
	"fmt"

	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// GeofenceService bundles geofence creation with validation and audit logging
// so the OAS handler and MCP tool share identical behaviour.
type GeofenceService struct {
	repo        repository.GeofenceRepo
	auditLogger *audit.Logger
}

// NewGeofenceService returns a GeofenceService backed by the given repo.
// auditLogger may be nil (audit entries are silently skipped).
func NewGeofenceService(repo repository.GeofenceRepo, auditLogger *audit.Logger) *GeofenceService {
	return &GeofenceService{repo: repo, auditLogger: auditLogger}
}

// CreateGeofenceInput holds the validated inputs for creating a geofence.
type CreateGeofenceInput struct {
	Name        string
	Description string
	Geometry    string // GeoJSON — used when non-empty (ST_GeomFromGeoJSON path)
	Area        string // WKT — used when Geometry is empty (ST_GeomFromText path)
	CalendarID  *int64
	Attributes  map[string]interface{}
}

// CreateForUser validates, persists, and audits a new geofence for user.
func (s *GeofenceService) CreateForUser(ctx context.Context, user *model.User, in CreateGeofenceInput) (*model.Geofence, error) {
	if in.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if err := validateDisplayName(in.Name); err != nil {
		return nil, err
	}
	if err := validateDescription(in.Description); err != nil {
		return nil, err
	}
	if in.Geometry == "" && in.Area == "" {
		return nil, fmt.Errorf("geometry or area is required")
	}

	g := &model.Geofence{
		Name:        in.Name,
		Description: in.Description,
		Geometry:    in.Geometry,
		Area:        in.Area,
		CalendarID:  in.CalendarID,
		Attributes:  in.Attributes,
	}

	if err := s.repo.Create(ctx, g); err != nil {
		return nil, fmt.Errorf("create geofence: %w", err)
	}
	if err := s.repo.AssociateUser(ctx, user.ID, g.ID); err != nil {
		return nil, fmt.Errorf("associate user: %w", err)
	}

	if s.auditLogger != nil {
		s.auditLogger.Log(ctx, &user.ID,
			audit.ActionGeofenceCreate, audit.ResourceGeofence, &g.ID,
			map[string]interface{}{"name": g.Name}, "", "")
	}
	return g, nil
}

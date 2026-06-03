package services

import (
	"context"
	"fmt"

	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// UpdateGeofenceInput holds the fields that may be changed. nil pointer fields
// mean "no change"; CalendarID uses CalendarIDSet as a tri-state flag.
type UpdateGeofenceInput struct {
	Name          *string                // nil = keep existing
	Description   *string                // nil = keep existing
	Area          *string                // WKT; nil = keep existing
	Geometry      *string                // GeoJSON; nil = keep existing
	CalendarID    *int64                 // new calendar ID (ignored unless CalendarIDSet)
	CalendarIDSet bool                   // true = apply CalendarID change (clear if nil)
	Attributes    map[string]interface{} // nil = keep existing
	AttributesSet bool                   // true = apply Attributes change
}

// UpdateForUser applies a partial update to a geofence owned by user and
// emits an audit entry on success.
func (s *GeofenceService) UpdateForUser(ctx context.Context, user *model.User, geofenceID int64, in UpdateGeofenceInput) (*model.Geofence, error) {
	if !s.repo.UserHasAccess(ctx, user, geofenceID) {
		return nil, fmt.Errorf("access denied")
	}
	existing, err := s.repo.GetByID(ctx, geofenceID)
	if err != nil || existing == nil {
		return nil, fmt.Errorf("geofence not found")
	}

	updated := *existing
	if in.Name != nil && *in.Name != "" {
		if err := validateDisplayName(*in.Name); err != nil {
			return nil, err
		}
		updated.Name = *in.Name
	}
	if in.Description != nil {
		if err := validateDescription(*in.Description); err != nil {
			return nil, err
		}
		updated.Description = *in.Description
	}
	if in.Geometry != nil && *in.Geometry != "" {
		updated.Geometry = *in.Geometry
		updated.Area = ""
	}
	if in.Area != nil && *in.Area != "" {
		updated.Area = *in.Area
		updated.Geometry = ""
	}
	if in.CalendarIDSet {
		updated.CalendarID = in.CalendarID
	}
	if in.AttributesSet {
		updated.Attributes = in.Attributes
	}

	if err := s.repo.Update(ctx, &updated); err != nil {
		return nil, fmt.Errorf("update geofence: %w", err)
	}
	if s.auditLogger != nil {
		s.auditLogger.Log(ctx, &user.ID,
			audit.ActionGeofenceUpdate, audit.ResourceGeofence, &updated.ID,
			map[string]interface{}{"name": updated.Name}, "", "")
	}
	return &updated, nil
}

// DeleteForUser deletes a geofence owned by user and emits an audit entry.
func (s *GeofenceService) DeleteForUser(ctx context.Context, user *model.User, geofenceID int64) error {
	if !s.repo.UserHasAccess(ctx, user, geofenceID) {
		return fmt.Errorf("access denied")
	}
	if err := s.repo.Delete(ctx, geofenceID); err != nil {
		return fmt.Errorf("delete geofence: %w", err)
	}
	if s.auditLogger != nil {
		id := geofenceID
		s.auditLogger.Log(ctx, &user.ID,
			audit.ActionGeofenceDelete, audit.ResourceGeofence, &id,
			nil, "", "")
	}
	return nil
}

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

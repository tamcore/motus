package handlers

import (
	"context"

	"github.com/go-faster/jx"
	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/services"
)

// --- ogen Handler methods ---

// ListGeofences returns all geofences for the authenticated user.
func (h *Handler) ListGeofences(ctx context.Context) (oas.ListGeofencesRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.Error{Error: "unauthorized"}, nil
	}
	geofences, err := h.cfg.Geofences.GetByUser(ctx, user.ID)
	if err != nil {
		return &oas.Error{Error: "failed to list geofences"}, nil
	}
	if geofences == nil {
		geofences = []*model.Geofence{}
	}
	result := make(oas.ListGeofencesOKApplicationJSON, len(geofences))
	for i, g := range geofences {
		result[i] = geofenceToOAS(g)
	}
	return &result, nil
}

// GetGeofence returns a single geofence by ID.
func (h *Handler) GetGeofence(ctx context.Context, params oas.GetGeofenceParams) (oas.GetGeofenceRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.GetGeofenceUnauthorized{Error: "unauthorized"}, nil
	}
	if !h.cfg.Geofences.UserHasAccess(ctx, user, params.ID) {
		return &oas.GetGeofenceForbidden{Error: "access denied"}, nil
	}
	geofence, err := h.cfg.Geofences.GetByID(ctx, params.ID)
	if err != nil {
		return &oas.GetGeofenceNotFound{Error: "geofence not found"}, nil
	}
	out := geofenceToOAS(geofence)
	return &out, nil
}

// CreateGeofence adds a new geofence and associates it with the authenticated user.
func (h *Handler) CreateGeofence(ctx context.Context, req *oas.GeofenceInput) (oas.CreateGeofenceRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.CreateGeofenceUnauthorized{Error: "unauthorized"}, nil
	}

	desc, _ := req.Description.Get()
	geom, _ := req.Geometry.Get()

	var calID *int64
	if v, ok := req.CalendarId.Get(); ok {
		calID = &v
	}
	var attrs map[string]any
	if req.Attributes.Set {
		attrs = rawToAttrs(map[string]jx.Raw(req.Attributes.Value))
	}

	geofence, err := h.cfg.GeofenceService.CreateForUser(ctx, user, services.CreateGeofenceInput{
		Name:        req.Name,
		Description: desc,
		Geometry:    geom,
		Area:        req.Area,
		CalendarID:  calID,
		Attributes:  attrs,
	})
	if err != nil {
		return &oas.CreateGeofenceBadRequest{Error: err.Error()}, nil
	}
	out := geofenceToOAS(geofence)
	return &out, nil
}

// UpdateGeofence modifies an existing geofence.
func (h *Handler) UpdateGeofence(ctx context.Context, req *oas.GeofenceUpdateInput, params oas.UpdateGeofenceParams) (oas.UpdateGeofenceRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.UpdateGeofenceUnauthorized{Error: "unauthorized"}, nil
	}

	in := services.UpdateGeofenceInput{}
	if n, ok := req.Name.Get(); ok {
		in.Name = &n
	}
	if req.Description.Set {
		desc, _ := req.Description.Get()
		in.Description = &desc
	}
	if geom, ok := req.Geometry.Get(); ok && geom != "" {
		in.Geometry = &geom
	}
	if a, ok := req.Area.Get(); ok && a != "" {
		in.Area = &a
	}
	if req.CalendarId.Set {
		in.CalendarIDSet = true
		if v, ok := req.CalendarId.Get(); ok {
			in.CalendarID = &v
		}
	}
	if req.Attributes.Set {
		attrs := rawToAttrs(map[string]jx.Raw(req.Attributes.Value))
		in.Attributes = attrs
		in.AttributesSet = true
	}

	updated, err := h.cfg.GeofenceService.UpdateForUser(ctx, user, params.ID, in)
	if err != nil {
		switch err.Error() {
		case "access denied":
			return &oas.UpdateGeofenceForbidden{Error: "access denied"}, nil
		case "geofence not found":
			return &oas.UpdateGeofenceNotFound{Error: "geofence not found"}, nil
		default:
			return &oas.UpdateGeofenceBadRequest{Error: err.Error()}, nil
		}
	}

	out := geofenceToOAS(updated)
	return &out, nil
}

// DeleteGeofence removes a geofence by ID.
func (h *Handler) DeleteGeofence(ctx context.Context, params oas.DeleteGeofenceParams) (oas.DeleteGeofenceRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.DeleteGeofenceUnauthorized{Error: "unauthorized"}, nil
	}
	if !h.cfg.Geofences.UserHasAccess(ctx, user, params.ID) {
		return &oas.DeleteGeofenceForbidden{Error: "access denied"}, nil
	}
	if err := h.cfg.Geofences.Delete(ctx, params.ID); err != nil {
		return &oas.DeleteGeofenceForbidden{Error: "failed to delete geofence"}, nil
	}
	if h.cfg.AuditLogger != nil {
		id := params.ID
		h.cfg.AuditLogger.Log(ctx, &user.ID,
			audit.ActionGeofenceDelete, audit.ResourceGeofence, &id,
			nil, "", "")
	}
	return &oas.DeleteGeofenceNoContent{}, nil
}

// AdminListGeofences returns all geofences in the system (admin only).
func (h *Handler) AdminListGeofences(ctx context.Context) (oas.AdminListGeofencesRes, error) {
	if _, err := requireAdminCtx(ctx); err != nil {
		return &oas.AdminListGeofencesForbidden{Error: err.Error()}, nil
	}
	geofences, err := h.cfg.Geofences.GetAllWithOwners(ctx)
	if err != nil {
		return &oas.AdminListGeofencesForbidden{Error: "failed to list geofences"}, nil
	}
	if geofences == nil {
		geofences = []*model.Geofence{}
	}
	result := make(oas.AdminListGeofencesOKApplicationJSON, len(geofences))
	for i, g := range geofences {
		result[i] = geofenceToOAS(g)
	}
	return &result, nil
}

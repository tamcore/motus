package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-faster/jx"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// GeofenceHandler handles geofence CRUD endpoints.
type GeofenceHandler struct {
	geofences repository.GeofenceRepo
	audit     *audit.Logger
}

// NewGeofenceHandler creates a new geofence handler.
func NewGeofenceHandler(geofences repository.GeofenceRepo) *GeofenceHandler {
	return &GeofenceHandler{geofences: geofences}
}

// SetAuditLogger configures audit logging for geofence events.
func (h *GeofenceHandler) SetAuditLogger(logger *audit.Logger) {
	h.audit = logger
}

type geofenceRequest struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Area        string                 `json:"area"`     // WKT format (Traccar compatibility)
	Geometry    string                 `json:"geometry"` // GeoJSON string
	CalendarID  *int64                 `json:"calendarId,omitempty"`
	Attributes  map[string]interface{} `json:"attributes"`
}

// List returns all geofences for the authenticated user.
// GET /api/geofences
func (h *GeofenceHandler) List(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	geofences, err := h.geofences.GetByUser(r.Context(), user.ID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to list geofences")
		return
	}
	if geofences == nil {
		geofences = []*model.Geofence{}
	}
	api.RespondJSON(w, http.StatusOK, geofences)
}

// AdminListAll returns all geofences in the system with owner info (admin only).
// GET /api/admin/geofences
func (h *GeofenceHandler) AdminListAll(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAdmin(w, r); !ok {
		return
	}
	geofences, err := h.geofences.GetAllWithOwners(r.Context())
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to list geofences")
		return
	}
	if geofences == nil {
		geofences = []*model.Geofence{}
	}
	api.RespondJSON(w, http.StatusOK, geofences)
}

// Get returns a single geofence by ID.
// GET /api/geofences/{id}
func (h *GeofenceHandler) Get(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid geofence id")
		return
	}
	if !h.geofences.UserHasAccess(r.Context(), user, id) {
		api.RespondError(w, http.StatusForbidden, "access denied")
		return
	}
	geofence, err := h.geofences.GetByID(r.Context(), id)
	if err != nil {
		api.RespondError(w, http.StatusNotFound, "geofence not found")
		return
	}
	api.RespondJSON(w, http.StatusOK, geofence)
}

// Create adds a new geofence and associates it with the authenticated user.
// POST /api/geofences
func (h *GeofenceHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	var req geofenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		api.RespondError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := ValidateDisplayName(req.Name); err != nil {
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := ValidateDescription(req.Description); err != nil {
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Geometry == "" && req.Area == "" {
		api.RespondError(w, http.StatusBadRequest, "geometry or area is required")
		return
	}

	geofence := &model.Geofence{
		Name:        req.Name,
		Description: req.Description,
		Area:        req.Area,
		Geometry:    req.Geometry,
		CalendarID:  req.CalendarID,
		Attributes:  req.Attributes,
	}

	if err := h.geofences.Create(r.Context(), geofence); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to create geofence")
		return
	}

	// Associate with the creating user.
	if err := h.geofences.AssociateUser(r.Context(), user.ID, geofence.ID); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to associate geofence with user")
		return
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &user.ID, audit.ActionGeofenceCreate, audit.ResourceGeofence, &geofence.ID,
			map[string]interface{}{"name": geofence.Name})
	}

	api.RespondJSON(w, http.StatusCreated, geofence)
}

// Update modifies an existing geofence.
// PUT /api/geofences/{id}
func (h *GeofenceHandler) Update(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid geofence id")
		return
	}
	if !h.geofences.UserHasAccess(r.Context(), user, id) {
		api.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	existing, err := h.geofences.GetByID(r.Context(), id)
	if err != nil {
		api.RespondError(w, http.StatusNotFound, "geofence not found")
		return
	}

	var req geofenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != "" {
		if err := ValidateDisplayName(req.Name); err != nil {
			api.RespondError(w, http.StatusBadRequest, err.Error())
			return
		}
		existing.Name = req.Name
	}
	if err := ValidateDescription(req.Description); err != nil {
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Geometry != "" {
		existing.Geometry = req.Geometry
	}
	if req.Area != "" {
		existing.Area = req.Area
	}
	existing.Description = req.Description
	// Always apply calendarId from the request: a nil value clears the
	// association, a non-nil value sets it. This allows the frontend to
	// explicitly remove a calendar by sending calendarId: null.
	existing.CalendarID = req.CalendarID
	if req.Attributes != nil {
		existing.Attributes = req.Attributes
	}

	if err := h.geofences.Update(r.Context(), existing); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to update geofence")
		return
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &user.ID, audit.ActionGeofenceUpdate, audit.ResourceGeofence, &existing.ID,
			map[string]interface{}{"name": existing.Name})
	}

	api.RespondJSON(w, http.StatusOK, existing)
}

// Delete removes a geofence by ID.
// DELETE /api/geofences/{id}
func (h *GeofenceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid geofence id")
		return
	}
	if !h.geofences.UserHasAccess(r.Context(), user, id) {
		api.RespondError(w, http.StatusForbidden, "access denied")
		return
	}
	if err := h.geofences.Delete(r.Context(), id); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to delete geofence")
		return
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &user.ID, audit.ActionGeofenceDelete, audit.ResourceGeofence, &id, nil)
	}

	w.WriteHeader(http.StatusNoContent)
}

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
	if req.Name == "" {
		return &oas.CreateGeofenceBadRequest{Error: "name is required"}, nil
	}
	if err := ValidateDisplayName(req.Name); err != nil {
		return &oas.CreateGeofenceBadRequest{Error: err.Error()}, nil
	}
	desc, _ := req.Description.Get()
	if err := ValidateDescription(desc); err != nil {
		return &oas.CreateGeofenceBadRequest{Error: err.Error()}, nil
	}
	geom, _ := req.Geometry.Get()
	if req.Area == "" && geom == "" {
		return &oas.CreateGeofenceBadRequest{Error: "geometry or area is required"}, nil
	}

	var calID *int64
	if v, ok := req.CalendarId.Get(); ok {
		calID = &v
	}
	var attrs map[string]interface{}
	if req.Attributes.Set {
		attrs = rawToAttrs(map[string]jx.Raw(req.Attributes.Value))
	}

	geofence := &model.Geofence{
		Name:        req.Name,
		Description: desc,
		Area:        req.Area,
		Geometry:    geom,
		CalendarID:  calID,
		Attributes:  attrs,
	}

	if err := h.cfg.Geofences.Create(ctx, geofence); err != nil {
		return &oas.CreateGeofenceBadRequest{Error: "failed to create geofence"}, nil
	}
	if err := h.cfg.Geofences.AssociateUser(ctx, user.ID, geofence.ID); err != nil {
		return &oas.CreateGeofenceBadRequest{Error: "failed to associate geofence with user"}, nil
	}

	if h.cfg.AuditLogger != nil {
		h.cfg.AuditLogger.Log(ctx, &user.ID,
			audit.ActionGeofenceCreate, audit.ResourceGeofence, &geofence.ID,
			map[string]interface{}{"name": geofence.Name}, "", "")
	}
	out := geofenceToOAS(geofence)
	return &out, nil
}

// UpdateGeofence modifies an existing geofence.
func (h *Handler) UpdateGeofence(ctx context.Context, req *oas.GeofenceInput, params oas.UpdateGeofenceParams) (oas.UpdateGeofenceRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.UpdateGeofenceUnauthorized{Error: "unauthorized"}, nil
	}
	if !h.cfg.Geofences.UserHasAccess(ctx, user, params.ID) {
		return &oas.UpdateGeofenceForbidden{Error: "access denied"}, nil
	}
	existing, err := h.cfg.Geofences.GetByID(ctx, params.ID)
	if err != nil {
		return &oas.UpdateGeofenceNotFound{Error: "geofence not found"}, nil
	}

	updated := *existing
	if req.Name != "" {
		if err := ValidateDisplayName(req.Name); err != nil {
			return &oas.UpdateGeofenceBadRequest{Error: err.Error()}, nil
		}
		updated.Name = req.Name
	}
	desc, _ := req.Description.Get()
	if err := ValidateDescription(desc); err != nil {
		return &oas.UpdateGeofenceBadRequest{Error: err.Error()}, nil
	}
	updated.Description = desc
	if geom, ok := req.Geometry.Get(); ok && geom != "" {
		updated.Geometry = geom
	}
	if req.Area != "" {
		updated.Area = req.Area
	}
	if req.CalendarId.Set {
		if v, ok := req.CalendarId.Get(); ok {
			updated.CalendarID = &v
		} else {
			updated.CalendarID = nil
		}
	}
	if req.Attributes.Set {
		updated.Attributes = rawToAttrs(map[string]jx.Raw(req.Attributes.Value))
	}

	if err := h.cfg.Geofences.Update(ctx, &updated); err != nil {
		return &oas.UpdateGeofenceBadRequest{Error: "failed to update geofence"}, nil
	}

	if h.cfg.AuditLogger != nil {
		h.cfg.AuditLogger.Log(ctx, &user.ID,
			audit.ActionGeofenceUpdate, audit.ResourceGeofence, &updated.ID,
			map[string]interface{}{"name": updated.Name}, "", "")
	}
	out := geofenceToOAS(&updated)
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

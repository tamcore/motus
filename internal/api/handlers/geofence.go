package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
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
		existing.Name = req.Name
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

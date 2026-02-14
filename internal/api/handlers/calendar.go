package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/calendar"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// CalendarHandler handles calendar CRUD endpoints.
type CalendarHandler struct {
	calendars repository.CalendarRepo
	audit     *audit.Logger
}

// NewCalendarHandler creates a new calendar handler.
func NewCalendarHandler(calendars repository.CalendarRepo) *CalendarHandler {
	return &CalendarHandler{calendars: calendars}
}

// SetAuditLogger configures audit logging for calendar events.
func (h *CalendarHandler) SetAuditLogger(logger *audit.Logger) {
	h.audit = logger
}

type calendarRequest struct {
	Name string `json:"name"`
	Data string `json:"data"` // iCalendar (RFC 5545) data
}

// List returns all calendars for the authenticated user.
// GET /api/calendars
func (h *CalendarHandler) List(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	calendars, err := h.calendars.GetByUser(r.Context(), user.ID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to list calendars")
		return
	}
	if calendars == nil {
		calendars = []*model.Calendar{}
	}
	api.RespondJSON(w, http.StatusOK, calendars)
}

// AdminListAll returns all calendars in the system with owner info (admin only).
// GET /api/admin/calendars
func (h *CalendarHandler) AdminListAll(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAdmin(w, r); !ok {
		return
	}
	calendars, err := h.calendars.GetAll(r.Context())
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to list calendars")
		return
	}
	if calendars == nil {
		calendars = []*model.Calendar{}
	}
	api.RespondJSON(w, http.StatusOK, calendars)
}

// Create adds a new calendar for the authenticated user.
// POST /api/calendars
func (h *CalendarHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	var req calendarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		api.RespondError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Data == "" {
		api.RespondError(w, http.StatusBadRequest, "data is required")
		return
	}

	// Validate iCalendar data.
	if err := calendar.Validate(req.Data); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid iCalendar data: "+err.Error())
		return
	}

	cal := &model.Calendar{
		UserID: user.ID,
		Name:   req.Name,
		Data:   req.Data,
	}

	if err := h.calendars.Create(r.Context(), cal); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to create calendar")
		return
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &user.ID, audit.ActionCalendarCreate, audit.ResourceCalendar, &cal.ID,
			map[string]interface{}{"name": cal.Name})
	}

	api.RespondJSON(w, http.StatusCreated, cal)
}

// Update modifies an existing calendar.
// PUT /api/calendars/{id}
func (h *CalendarHandler) Update(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid calendar id")
		return
	}
	if !h.calendars.UserHasAccess(r.Context(), user, id) {
		api.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	existing, err := h.calendars.GetByID(r.Context(), id)
	if err != nil {
		api.RespondError(w, http.StatusNotFound, "calendar not found")
		return
	}

	var req calendarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Data != "" {
		// Validate updated iCalendar data.
		if err := calendar.Validate(req.Data); err != nil {
			api.RespondError(w, http.StatusBadRequest, "invalid iCalendar data: "+err.Error())
			return
		}
		existing.Data = req.Data
	}

	if err := h.calendars.Update(r.Context(), existing); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to update calendar")
		return
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &user.ID, audit.ActionCalendarUpdate, audit.ResourceCalendar, &existing.ID,
			map[string]interface{}{"name": existing.Name})
	}

	api.RespondJSON(w, http.StatusOK, existing)
}

// Delete removes a calendar by ID.
// DELETE /api/calendars/{id}
func (h *CalendarHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid calendar id")
		return
	}
	if !h.calendars.UserHasAccess(r.Context(), user, id) {
		api.RespondError(w, http.StatusForbidden, "access denied")
		return
	}
	if err := h.calendars.Delete(r.Context(), id); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to delete calendar")
		return
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &user.ID, audit.ActionCalendarDelete, audit.ResourceCalendar, &id, nil)
	}

	w.WriteHeader(http.StatusNoContent)
}

// Check tests if the current time matches a calendar's schedule.
// GET /api/calendars/{id}/check
func (h *CalendarHandler) Check(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid calendar id")
		return
	}
	if !h.calendars.UserHasAccess(r.Context(), user, id) {
		api.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	cal, err := h.calendars.GetByID(r.Context(), id)
	if err != nil {
		api.RespondError(w, http.StatusNotFound, "calendar not found")
		return
	}

	now := time.Now().UTC()
	active, err := calendar.IsActiveAt(cal.Data, now)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to check calendar schedule")
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"calendarId": cal.ID,
		"name":       cal.Name,
		"active":     active,
		"checkedAt":  now,
	})
}

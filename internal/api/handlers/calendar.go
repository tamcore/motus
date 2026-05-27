package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
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
	if err := ValidateDisplayName(req.Name); err != nil {
		api.RespondError(w, http.StatusBadRequest, err.Error())
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
		if err := ValidateDisplayName(req.Name); err != nil {
			api.RespondError(w, http.StatusBadRequest, err.Error())
			return
		}
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

// --- ogen Handler methods ---

// ListCalendars returns all calendars for the authenticated user.
func (h *Handler) ListCalendars(ctx context.Context) (oas.ListCalendarsRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.Error{Error: "unauthorized"}, nil
	}
	calendars, err := h.cfg.Calendars.GetByUser(ctx, user.ID)
	if err != nil {
		return &oas.Error{Error: "failed to list calendars"}, nil
	}
	if calendars == nil {
		calendars = []*model.Calendar{}
	}
	result := make(oas.ListCalendarsOKApplicationJSON, len(calendars))
	for i, c := range calendars {
		result[i] = calendarToOAS(c)
	}
	return &result, nil
}

// CreateCalendar adds a new calendar for the authenticated user.
func (h *Handler) CreateCalendar(ctx context.Context, req *oas.CalendarInput) (oas.CreateCalendarRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.CreateCalendarUnauthorized{Error: "unauthorized"}, nil
	}
	if req.Name == "" {
		return &oas.CreateCalendarBadRequest{Error: "name is required"}, nil
	}
	if err := ValidateDisplayName(req.Name); err != nil {
		return &oas.CreateCalendarBadRequest{Error: err.Error()}, nil
	}
	if req.Data == "" {
		return &oas.CreateCalendarBadRequest{Error: "data is required"}, nil
	}
	if err := calendar.Validate(req.Data); err != nil {
		return &oas.CreateCalendarBadRequest{Error: "invalid iCalendar data: " + err.Error()}, nil
	}

	cal := &model.Calendar{
		UserID: user.ID,
		Name:   req.Name,
		Data:   req.Data,
	}
	if err := h.cfg.Calendars.Create(ctx, cal); err != nil {
		return &oas.CreateCalendarBadRequest{Error: "failed to create calendar"}, nil
	}

	if h.cfg.AuditLogger != nil {
		h.cfg.AuditLogger.Log(ctx, &user.ID,
			audit.ActionCalendarCreate, audit.ResourceCalendar, &cal.ID,
			map[string]interface{}{"name": cal.Name}, "", "")
	}
	out := calendarToOAS(cal)
	return &out, nil
}

// UpdateCalendar modifies an existing calendar.
func (h *Handler) UpdateCalendar(ctx context.Context, req *oas.CalendarInput, params oas.UpdateCalendarParams) (oas.UpdateCalendarRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.UpdateCalendarUnauthorized{Error: "unauthorized"}, nil
	}
	if !h.cfg.Calendars.UserHasAccess(ctx, user, params.ID) {
		return &oas.UpdateCalendarNotFound{Error: "calendar not found"}, nil
	}
	existing, err := h.cfg.Calendars.GetByID(ctx, params.ID)
	if err != nil {
		return &oas.UpdateCalendarNotFound{Error: "calendar not found"}, nil
	}

	updated := *existing
	if req.Name != "" {
		if err := ValidateDisplayName(req.Name); err != nil {
			return &oas.UpdateCalendarBadRequest{Error: err.Error()}, nil
		}
		updated.Name = req.Name
	}
	if req.Data != "" {
		if err := calendar.Validate(req.Data); err != nil {
			return &oas.UpdateCalendarBadRequest{Error: "invalid iCalendar data: " + err.Error()}, nil
		}
		updated.Data = req.Data
	}

	if err := h.cfg.Calendars.Update(ctx, &updated); err != nil {
		return &oas.UpdateCalendarBadRequest{Error: "failed to update calendar"}, nil
	}

	if h.cfg.AuditLogger != nil {
		h.cfg.AuditLogger.Log(ctx, &user.ID,
			audit.ActionCalendarUpdate, audit.ResourceCalendar, &updated.ID,
			map[string]interface{}{"name": updated.Name}, "", "")
	}
	out := calendarToOAS(&updated)
	return &out, nil
}

// DeleteCalendar removes a calendar by ID.
func (h *Handler) DeleteCalendar(ctx context.Context, params oas.DeleteCalendarParams) (oas.DeleteCalendarRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.DeleteCalendarUnauthorized{Error: "unauthorized"}, nil
	}
	if !h.cfg.Calendars.UserHasAccess(ctx, user, params.ID) {
		return &oas.DeleteCalendarNotFound{Error: "calendar not found"}, nil
	}
	if err := h.cfg.Calendars.Delete(ctx, params.ID); err != nil {
		return &oas.DeleteCalendarNotFound{Error: "failed to delete calendar"}, nil
	}
	if h.cfg.AuditLogger != nil {
		id := params.ID
		h.cfg.AuditLogger.Log(ctx, &user.ID,
			audit.ActionCalendarDelete, audit.ResourceCalendar, &id,
			nil, "", "")
	}
	return &oas.DeleteCalendarNoContent{}, nil
}

// CheckCalendar tests if the current time matches the calendar's schedule.
func (h *Handler) CheckCalendar(ctx context.Context, params oas.CheckCalendarParams) (oas.CheckCalendarRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.CheckCalendarUnauthorized{Error: "unauthorized"}, nil
	}
	if !h.cfg.Calendars.UserHasAccess(ctx, user, params.ID) {
		return &oas.CheckCalendarNotFound{Error: "calendar not found"}, nil
	}
	cal, err := h.cfg.Calendars.GetByID(ctx, params.ID)
	if err != nil {
		return &oas.CheckCalendarNotFound{Error: "calendar not found"}, nil
	}

	now := time.Now().UTC()
	active, err := calendar.IsActiveAt(cal.Data, now)
	if err != nil {
		return &oas.CheckCalendarNotFound{Error: "failed to check calendar schedule"}, nil
	}

	return &oas.CalendarCheckResult{
		Active: active,
	}, nil
}

// AdminListCalendars returns all calendars in the system (admin only).
func (h *Handler) AdminListCalendars(ctx context.Context) (oas.AdminListCalendarsRes, error) {
	if _, err := requireAdminCtx(ctx); err != nil {
		return &oas.AdminListCalendarsForbidden{Error: err.Error()}, nil
	}
	calendars, err := h.cfg.Calendars.GetAll(ctx)
	if err != nil {
		return &oas.AdminListCalendarsForbidden{Error: "failed to list calendars"}, nil
	}
	if calendars == nil {
		calendars = []*model.Calendar{}
	}
	result := make(oas.AdminListCalendarsOKApplicationJSON, len(calendars))
	for i, c := range calendars {
		result[i] = calendarToOAS(c)
	}
	return &result, nil
}

package handlers

import (
	"context"
	"time"

	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/calendar"
	"github.com/tamcore/motus/internal/model"
)

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

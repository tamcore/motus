package handlers

import (
	"context"
	"errors"
	"time"

	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/calendar"
	"github.com/tamcore/motus/internal/services"
)

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
	return new(mapSlice[oas.ListCalendarsOKApplicationJSON](calendars, calendarToOAS)), nil
}

// CreateCalendar adds a new calendar for the authenticated user.
func (h *Handler) CreateCalendar(ctx context.Context, req *oas.CalendarInput) (oas.CreateCalendarRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.CreateCalendarUnauthorized{Error: "unauthorized"}, nil
	}
	cal, err := h.cfg.CalendarService.CreateForUser(ctx, user, services.CalendarInput{
		Name: req.Name,
		Data: req.Data,
	})
	if err != nil {
		return &oas.CreateCalendarBadRequest{Error: services.PublicMessage(err, "failed to create calendar")}, nil
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
	updated, err := h.cfg.CalendarService.UpdateForUser(ctx, user, params.ID, services.CalendarInput{
		Name: req.Name,
		Data: req.Data,
	})
	if errors.Is(err, services.ErrNotFound) {
		return &oas.UpdateCalendarNotFound{Error: err.Error()}, nil
	}
	if err != nil {
		return &oas.UpdateCalendarBadRequest{Error: services.PublicMessage(err, "failed to update calendar")}, nil
	}
	out := calendarToOAS(updated)
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
	h.cfg.AuditLogger.Log(ctx, &user.ID,
		audit.ActionCalendarDelete, audit.ResourceCalendar, &params.ID,
		nil)
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
	return new(mapSlice[oas.AdminListCalendarsOKApplicationJSON](calendars, calendarToOAS)), nil
}

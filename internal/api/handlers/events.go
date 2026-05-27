package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// EventHandler handles event query endpoints.
type EventHandler struct {
	events  repository.EventRepo
	devices repository.DeviceRepo
}

// NewEventHandler creates a new event handler.
func NewEventHandler(events repository.EventRepo, devices repository.DeviceRepo) *EventHandler {
	return &EventHandler{events: events, devices: devices}
}

// List returns recent events for the authenticated user's devices.
// GET /api/events?deviceId=123
func (h *EventHandler) List(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())

	// Optional device filter.
	deviceIDStr := r.URL.Query().Get("deviceId")
	if deviceIDStr != "" {
		deviceID, err := strconv.ParseInt(deviceIDStr, 10, 64)
		if err != nil {
			api.RespondError(w, http.StatusBadRequest, "invalid deviceId")
			return
		}
		if !h.devices.UserHasAccess(r.Context(), user, deviceID) {
			api.RespondError(w, http.StatusForbidden, "access denied")
			return
		}
		events, err := h.events.GetByDevice(r.Context(), deviceID, 100)
		if err != nil {
			api.RespondError(w, http.StatusInternalServerError, "failed to get events")
			return
		}
		if events == nil {
			events = []*model.Event{}
		}
		api.RespondJSON(w, http.StatusOK, events)
		return
	}

	// All events for user's devices.
	events, err := h.events.GetByUser(r.Context(), user.ID, 100)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to get events")
		return
	}
	if events == nil {
		events = []*model.Event{}
	}
	api.RespondJSON(w, http.StatusOK, events)
}

// --- ogen Handler methods ---

// listEventsShared contains the shared logic for ListEvents and ReportEvents.
// It filters events by the provided parameters and returns OAS-typed events.
func (h *Handler) listEventsShared(ctx context.Context, deviceID oas.OptInt64, eventType oas.OptString, from, to oas.OptDateTime) ([]*model.Event, error) {
	user := api.UserFromContext(ctx)

	var deviceIDs []int64
	if id, ok := deviceID.Get(); ok && id > 0 {
		if !h.cfg.Devices.UserHasAccess(ctx, user, id) {
			return nil, nil
		}
		deviceIDs = []int64{id}
	}

	var eventTypes []string
	if t, ok := eventType.Get(); ok && t != "" {
		eventTypes = []string{t}
	}

	fromTime := time.Now().Add(-24 * time.Hour)
	toTime := time.Now()
	if t, ok := from.Get(); ok {
		fromTime = t
	}
	if t, ok := to.Get(); ok {
		toTime = t
	}

	events, err := h.cfg.Events.GetByFilters(ctx, user.ID, deviceIDs, eventTypes, fromTime, toTime)
	return events, err
}

// ListEvents implements oas.Handler for GET /api/events.
func (h *Handler) ListEvents(ctx context.Context, params oas.ListEventsParams) (oas.ListEventsRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.Error{Error: "unauthorized"}, nil
	}

	// Access check for the requested device.
	if id, ok := params.DeviceId.Get(); ok && id > 0 {
		if !h.cfg.Devices.UserHasAccess(ctx, user, id) {
			return &oas.Error{Error: "access denied"}, nil
		}
	}

	events, err := h.listEventsShared(ctx, params.DeviceId, params.Type, params.From, params.To)
	if err != nil {
		return &oas.Error{Error: "failed to get events"}, nil
	}
	result := make(oas.ListEventsOKApplicationJSON, len(events))
	for i, e := range events {
		result[i] = eventToOAS(e)
	}
	return &result, nil
}

// ReportEvents implements oas.Handler for GET /api/reports/events (Traccar-compat alias).
func (h *Handler) ReportEvents(ctx context.Context, params oas.ReportEventsParams) (oas.ReportEventsRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.Error{Error: "unauthorized"}, nil
	}

	// Access check for the requested device.
	if id, ok := params.DeviceId.Get(); ok && id > 0 {
		if !h.cfg.Devices.UserHasAccess(ctx, user, id) {
			return &oas.Error{Error: "access denied"}, nil
		}
	}

	events, err := h.listEventsShared(ctx, params.DeviceId, params.Type, params.From, params.To)
	if err != nil {
		return &oas.Error{Error: "failed to get events"}, nil
	}
	result := make(oas.ReportEventsOKApplicationJSON, len(events))
	for i, e := range events {
		result[i] = eventToOAS(e)
	}
	return &result, nil
}

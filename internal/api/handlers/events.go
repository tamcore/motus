package handlers

import (
	"net/http"
	"strconv"

	"github.com/tamcore/motus/internal/api"
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

package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/storage/repository"
)

// ReportsHandler handles report endpoints for Traccar compatibility.
type ReportsHandler struct {
	events  repository.EventRepo
	devices repository.DeviceRepo
}

// NewReportsHandler creates a new reports handler.
func NewReportsHandler(events repository.EventRepo, devices repository.DeviceRepo) *ReportsHandler {
	return &ReportsHandler{events: events, devices: devices}
}

// GetEvents returns events filtered by query parameters (Traccar format).
// GET /api/reports/events?deviceId[]=1&deviceId[]=2&type[]=geofenceEnter&from=...&to=...
func (h *ReportsHandler) GetEvents(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	if user == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Parse device IDs (repeatable parameter).
	var deviceIDs []int64
	for _, idStr := range r.URL.Query()["deviceId[]"] {
		if id, err := strconv.ParseInt(idStr, 10, 64); err == nil {
			deviceIDs = append(deviceIDs, id)
		}
	}
	// Also support non-array form: deviceId=1
	if idStr := r.URL.Query().Get("deviceId"); idStr != "" {
		if id, err := strconv.ParseInt(idStr, 10, 64); err == nil {
			deviceIDs = append(deviceIDs, id)
		}
	}

	// Parse event types (repeatable parameter).
	eventTypes := r.URL.Query()["type[]"]
	if t := r.URL.Query().Get("type"); t != "" {
		eventTypes = append(eventTypes, t)
	}

	// Parse time range.
	var from, to time.Time
	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		from, _ = time.Parse(time.RFC3339, fromStr)
	}
	if from.IsZero() {
		from = time.Now().Add(-24 * time.Hour)
	}

	if toStr := r.URL.Query().Get("to"); toStr != "" {
		to, _ = time.Parse(time.RFC3339, toStr)
	}
	if to.IsZero() {
		to = time.Now()
	}

	// Verify access to requested devices.
	for _, deviceID := range deviceIDs {
		if !h.devices.UserHasAccess(r.Context(), user, deviceID) {
			api.RespondError(w, http.StatusForbidden, "access denied to device")
			return
		}
	}

	events, err := h.events.GetByFilters(r.Context(), user.ID, deviceIDs, eventTypes, from, to)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to get events")
		return
	}

	api.RespondJSON(w, http.StatusOK, events)
}

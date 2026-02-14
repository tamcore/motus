package handlers

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
)

// DeviceCacheInvalidator can invalidate cached user-device access entries.
// Implemented by websocket.Hub. Using an interface avoids an import cycle
// between the handlers and websocket packages.
type DeviceCacheInvalidator interface {
	InvalidateDevice(deviceID int64)
}

// SetCacheInvalidator configures the handler to invalidate the WebSocket
// user-device access cache when device assignments change. If not set,
// the cache will still expire naturally via TTL.
func (h *UserHandler) SetCacheInvalidator(ci DeviceCacheInvalidator) {
	h.cacheInvalidator = ci
}

// AdminListAllDevices returns all devices in the system (admin only).
// GET /api/admin/devices
func (h *UserHandler) AdminListAllDevices(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAdmin(w, r); !ok {
		return
	}

	devices, err := h.devices.GetAllWithOwners(r.Context())
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to list devices")
		return
	}
	if devices == nil {
		devices = []model.Device{}
	}
	if prefix := effectivePrefix(r, h.uniqueIDPrefix); prefix != "" {
		for i := range devices {
			devices[i].UniqueID = prefix + devices[i].UniqueID
		}
	}
	api.RespondJSON(w, http.StatusOK, devices)
}

// ListDevices returns the devices assigned to a user.
// GET /api/users/{id}/devices
func (h *UserHandler) ListDevices(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAdmin(w, r); !ok {
		return
	}

	userID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	// Verify user exists.
	if _, err := h.users.GetByID(r.Context(), userID); err != nil {
		api.RespondError(w, http.StatusNotFound, "user not found")
		return
	}

	devices, err := h.devices.GetByUser(r.Context(), userID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to list devices")
		return
	}
	if devices == nil {
		devices = []*model.Device{}
	}
	model.ApplyUniqueIDPrefix(devices, effectivePrefix(r, h.uniqueIDPrefix))
	api.RespondJSON(w, http.StatusOK, devices)
}

// AssignDevice associates a device with a user.
// POST /api/users/{id}/devices/{deviceId}
func (h *UserHandler) AssignDevice(w http.ResponseWriter, r *http.Request) {
	admin, ok := requireAdmin(w, r)
	if !ok {
		return
	}

	userID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	deviceID, err := strconv.ParseInt(chi.URLParam(r, "deviceId"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid device id")
		return
	}

	// Verify user exists.
	if _, err := h.users.GetByID(r.Context(), userID); err != nil {
		api.RespondError(w, http.StatusNotFound, "user not found")
		return
	}

	// Verify device exists.
	if _, err := h.devices.GetByID(r.Context(), deviceID); err != nil {
		api.RespondError(w, http.StatusNotFound, "device not found")
		return
	}

	if err := h.users.AssignDevice(r.Context(), userID, deviceID); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to assign device")
		return
	}

	// Invalidate cached access list so WebSocket broadcasts pick up the change.
	if h.cacheInvalidator != nil {
		h.cacheInvalidator.InvalidateDevice(deviceID)
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &admin.ID, audit.ActionDeviceAssign, audit.ResourceDevice, &deviceID,
			map[string]interface{}{"targetUserId": userID})
	}

	w.WriteHeader(http.StatusNoContent)
}

// UnassignDevice removes a device association from a user.
// DELETE /api/users/{id}/devices/{deviceId}
func (h *UserHandler) UnassignDevice(w http.ResponseWriter, r *http.Request) {
	admin, ok := requireAdmin(w, r)
	if !ok {
		return
	}

	userID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	deviceID, err := strconv.ParseInt(chi.URLParam(r, "deviceId"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid device id")
		return
	}

	if err := h.users.UnassignDevice(r.Context(), userID, deviceID); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to unassign device")
		return
	}

	// Invalidate cached access list so WebSocket broadcasts pick up the change.
	if h.cacheInvalidator != nil {
		h.cacheInvalidator.InvalidateDevice(deviceID)
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &admin.ID, audit.ActionDeviceUnassign, audit.ResourceDevice, &deviceID,
			map[string]interface{}{"targetUserId": userID})
	}

	w.WriteHeader(http.StatusNoContent)
}

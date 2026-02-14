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
	"github.com/tamcore/motus/internal/validation"
)

// DeviceHandler handles device CRUD endpoints.
type DeviceHandler struct {
	devices        repository.DeviceRepo
	audit          *audit.Logger
	uniqueIDPrefix string
}

// NewDeviceHandler creates a new device handler.
func NewDeviceHandler(devices repository.DeviceRepo, uniqueIDPrefix string) *DeviceHandler {
	return &DeviceHandler{devices: devices, uniqueIDPrefix: uniqueIDPrefix}
}

// effectivePrefix returns the unique ID prefix only for API key authenticated
// requests (e.g. Home Assistant). Session cookie requests (web UI) get no
// prefix so identifiers display without modification.
func effectivePrefix(r *http.Request, prefix string) string {
	if api.ApiKeyFromContext(r.Context()) != nil {
		return prefix
	}
	return ""
}

// SetAuditLogger configures audit logging for device events.
func (h *DeviceHandler) SetAuditLogger(logger *audit.Logger) {
	h.audit = logger
}

// List returns all devices for the authenticated user.
// GET /api/devices
func (h *DeviceHandler) List(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	devices, err := h.devices.GetByUser(r.Context(), user.ID)
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

// Get returns a single device by ID.
// GET /api/devices/{id}
func (h *DeviceHandler) Get(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	deviceID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid device id")
		return
	}
	if !h.devices.UserHasAccess(r.Context(), user, deviceID) {
		api.RespondError(w, http.StatusForbidden, "access denied")
		return
	}
	device, err := h.devices.GetByID(r.Context(), deviceID)
	if err != nil {
		api.RespondError(w, http.StatusNotFound, "device not found")
		return
	}
	model.ApplyUniqueIDPrefix([]*model.Device{device}, effectivePrefix(r, h.uniqueIDPrefix))
	api.RespondJSON(w, http.StatusOK, device)
}

type createDeviceRequest struct {
	UniqueID   string   `json:"uniqueId"`
	Name       string   `json:"name"`
	Protocol   string   `json:"protocol"`
	SpeedLimit *float64 `json:"speedLimit,omitempty"`
	Phone      *string  `json:"phone,omitempty"`
	Model      *string  `json:"model,omitempty"`
	Category   *string  `json:"category,omitempty"`
	Mileage    *float64 `json:"mileage,omitempty"`
}

// Create adds a new device and associates it with the authenticated user.
// POST /api/devices
func (h *DeviceHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	var req createDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validation.ValidateDeviceUniqueID(req.UniqueID); err != nil {
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validation.ValidateName(req.Name); err != nil {
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}

	device := &model.Device{
		UniqueID:   req.UniqueID,
		Name:       req.Name,
		Protocol:   req.Protocol,
		SpeedLimit: req.SpeedLimit,
		Phone:      req.Phone,
		Model:      req.Model,
		Category:   req.Category,
		Mileage:    req.Mileage,
		Status:     "unknown",
	}
	if device.Mileage != nil && *device.Mileage < 0 {
		api.RespondError(w, http.StatusBadRequest, "mileage must be non-negative")
		return
	}
	if err := h.devices.Create(r.Context(), device, user.ID); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to create device")
		return
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &user.ID, audit.ActionDeviceCreate, audit.ResourceDevice, &device.ID,
			map[string]interface{}{"name": device.Name, "uniqueId": device.UniqueID})
	}

	model.ApplyUniqueIDPrefix([]*model.Device{device}, effectivePrefix(r, h.uniqueIDPrefix))
	api.RespondJSON(w, http.StatusOK, device)
}

// Update modifies an existing device.
// PUT /api/devices/{id}
func (h *DeviceHandler) Update(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	deviceID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid device id")
		return
	}
	if !h.devices.UserHasAccess(r.Context(), user, deviceID) {
		api.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	device, err := h.devices.GetByID(r.Context(), deviceID)
	if err != nil {
		api.RespondError(w, http.StatusNotFound, "device not found")
		return
	}

	var req createDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name != "" {
		if err := validation.ValidateName(req.Name); err != nil {
			api.RespondError(w, http.StatusBadRequest, err.Error())
			return
		}
		device.Name = req.Name
	}
	if req.Protocol != "" {
		device.Protocol = req.Protocol
	}
	if req.SpeedLimit != nil {
		device.SpeedLimit = req.SpeedLimit
	}
	if req.Phone != nil {
		device.Phone = req.Phone
	}
	if req.Model != nil {
		device.Model = req.Model
	}
	if req.Category != nil {
		device.Category = req.Category
	}
	if req.Mileage != nil {
		if *req.Mileage < 0 {
			api.RespondError(w, http.StatusBadRequest, "mileage must be non-negative")
			return
		}
		device.Mileage = req.Mileage
		device.PendingMileage = 0
	}

	if err := h.devices.Update(r.Context(), device); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to update device")
		return
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &user.ID, audit.ActionDeviceUpdate, audit.ResourceDevice, &device.ID,
			map[string]interface{}{"name": device.Name})
	}

	model.ApplyUniqueIDPrefix([]*model.Device{device}, effectivePrefix(r, h.uniqueIDPrefix))
	api.RespondJSON(w, http.StatusOK, device)
}

// Delete removes a device by ID.
// DELETE /api/devices/{id}
func (h *DeviceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	deviceID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid device id")
		return
	}
	if !h.devices.UserHasAccess(r.Context(), user, deviceID) {
		api.RespondError(w, http.StatusForbidden, "access denied")
		return
	}
	if err := h.devices.Delete(r.Context(), deviceID); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to delete device")
		return
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &user.ID, audit.ActionDeviceDelete, audit.ResourceDevice, &deviceID, nil)
	}

	w.WriteHeader(http.StatusNoContent)
}

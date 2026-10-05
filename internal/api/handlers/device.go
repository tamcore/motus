package handlers

import (
	"context"

	"github.com/go-faster/jx"
	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/validation"
)

// deviceOut converts d for a response. API-key requests (Traccar clients such
// as Home Assistant) get the configured unique-id prefix to avoid collisions.
func (h *Handler) deviceOut(ctx context.Context, d *model.Device) oas.Device {
	out := deviceToOAS(d)
	if api.ApiKeyFromContext(ctx) != nil {
		out.UniqueId = h.cfg.UniqueIDPrefix + out.UniqueId
	}
	return out
}

// ListDevices returns all devices for the authenticated user.
func (h *Handler) ListDevices(ctx context.Context) (oas.ListDevicesRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.Error{Error: "unauthorized"}, nil
	}
	devices, err := h.cfg.Devices.GetByUser(ctx, user.ID)
	if err != nil {
		return &oas.Error{Error: "failed to list devices"}, nil
	}
	return new(mapSlice[oas.ListDevicesOKApplicationJSON](devices, func(d *model.Device) oas.Device {
		return h.deviceOut(ctx, d)
	})), nil
}

// GetDevice returns a single device by ID.
func (h *Handler) GetDevice(ctx context.Context, params oas.GetDeviceParams) (oas.GetDeviceRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.GetDeviceUnauthorized{Error: "unauthorized"}, nil
	}
	if !h.cfg.Devices.UserHasAccess(ctx, user, params.ID) {
		return &oas.GetDeviceForbidden{Error: "access denied"}, nil
	}
	device, err := h.cfg.Devices.GetByID(ctx, params.ID)
	if err != nil {
		return &oas.GetDeviceNotFound{Error: "device not found"}, nil
	}
	return new(h.deviceOut(ctx, device)), nil
}

// CreateDevice creates a new device and associates it with the authenticated user.
func (h *Handler) CreateDevice(ctx context.Context, req *oas.DeviceInput) (oas.CreateDeviceRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.CreateDeviceUnauthorized{Error: "unauthorized"}, nil
	}
	if err := validation.ValidateDeviceUniqueID(req.UniqueId); err != nil {
		return &oas.CreateDeviceBadRequest{Error: err.Error()}, nil
	}
	if err := validation.ValidateName(req.Name); err != nil {
		return &oas.CreateDeviceBadRequest{Error: err.Error()}, nil
	}
	device := applyDeviceInputFields(&model.Device{UniqueID: req.UniqueId, Name: req.Name, Status: "unknown"}, req)
	if err := h.cfg.Devices.Create(ctx, device, user.ID); err != nil {
		return &oas.CreateDeviceBadRequest{Error: "failed to create device"}, nil
	}
	h.cfg.AuditLogger.Log(ctx, &user.ID,
		audit.ActionDeviceCreate, audit.ResourceDevice, &device.ID,
		map[string]any{"name": device.Name, "uniqueId": device.UniqueID})
	return new(h.deviceOut(ctx, device)), nil
}

// UpdateDevice modifies an existing device.
func (h *Handler) UpdateDevice(ctx context.Context, req *oas.DeviceInput, params oas.UpdateDeviceParams) (oas.UpdateDeviceRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.UpdateDeviceUnauthorized{Error: "unauthorized"}, nil
	}
	if !h.cfg.Devices.UserHasAccess(ctx, user, params.ID) {
		return &oas.UpdateDeviceForbidden{Error: "access denied"}, nil
	}
	device, err := h.cfg.Devices.GetByID(ctx, params.ID)
	if err != nil {
		return &oas.UpdateDeviceNotFound{Error: "device not found"}, nil
	}
	updated := applyDeviceInputFields(device, req)
	if req.UniqueId != "" && req.UniqueId != device.UniqueID {
		if err := validation.ValidateDeviceUniqueID(req.UniqueId); err != nil {
			return &oas.UpdateDeviceBadRequest{Error: err.Error()}, nil
		}
		updated.UniqueID = req.UniqueId
	}
	if req.Name != "" && req.Name != device.Name {
		if err := validation.ValidateName(req.Name); err != nil {
			return &oas.UpdateDeviceBadRequest{Error: err.Error()}, nil
		}
		updated.Name = req.Name
	}
	device = updated
	if err := h.cfg.Devices.Update(ctx, device); err != nil {
		return &oas.UpdateDeviceBadRequest{Error: "failed to update device"}, nil
	}
	h.cfg.AuditLogger.Log(ctx, &user.ID,
		audit.ActionDeviceUpdate, audit.ResourceDevice, &device.ID,
		map[string]any{"name": device.Name})
	return new(h.deviceOut(ctx, device)), nil
}

// DeleteDevice removes a device by ID.
func (h *Handler) DeleteDevice(ctx context.Context, params oas.DeleteDeviceParams) (oas.DeleteDeviceRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.DeleteDeviceUnauthorized{Error: "unauthorized"}, nil
	}
	if !h.cfg.Devices.UserHasAccess(ctx, user, params.ID) {
		return &oas.DeleteDeviceForbidden{Error: "access denied"}, nil
	}
	if err := h.cfg.Devices.Delete(ctx, params.ID); err != nil {
		return &oas.DeleteDeviceForbidden{Error: "failed to delete device"}, nil
	}
	h.cfg.AuditLogger.Log(ctx, &user.ID,
		audit.ActionDeviceDelete, audit.ResourceDevice, &params.ID,
		nil)
	return &oas.DeleteDeviceNoContent{}, nil
}

// AdminListDevices returns all devices (admin only).
func (h *Handler) AdminListDevices(ctx context.Context) (oas.AdminListDevicesRes, error) {
	if _, err := requireAdminCtx(ctx); err != nil {
		return &oas.AdminListDevicesForbidden{Error: err.Error()}, nil
	}
	devices, err := h.cfg.Devices.GetAllWithOwners(ctx)
	if err != nil {
		return &oas.AdminListDevicesForbidden{Error: "failed to list devices"}, nil
	}
	result := make(oas.AdminListDevicesOKApplicationJSON, len(devices))
	for i := range devices {
		result[i] = deviceToOAS(&devices[i])
	}
	return &result, nil
}

// AdminListUserDevices returns all devices for a specific user (admin only).
func (h *Handler) AdminListUserDevices(ctx context.Context, params oas.AdminListUserDevicesParams) (oas.AdminListUserDevicesRes, error) {
	if _, err := requireAdminCtx(ctx); err != nil {
		return &oas.AdminListUserDevicesForbidden{Error: err.Error()}, nil
	}
	devices, err := h.cfg.Devices.GetByUser(ctx, params.ID)
	if err != nil {
		return &oas.AdminListUserDevicesNotFound{Error: "user or devices not found"}, nil
	}
	return new(mapSlice[oas.AdminListUserDevicesOKApplicationJSON](devices, deviceToOAS)), nil
}

// AdminAssignDevice assigns a device to a user (admin only).
func (h *Handler) AdminAssignDevice(ctx context.Context, params oas.AdminAssignDeviceParams) (oas.AdminAssignDeviceRes, error) {
	admin, err := requireAdminCtx(ctx)
	if err != nil {
		return &oas.AdminAssignDeviceForbidden{Error: err.Error()}, nil
	}
	if err := h.cfg.Users.AssignDevice(ctx, params.ID, params.DeviceId); err != nil {
		return &oas.AdminAssignDeviceNotFound{Error: "user or device not found"}, nil
	}
	// Invalidate the hub's cached access list so WebSocket broadcasts pick up
	// the new assignment immediately instead of waiting for the cache TTL.
	if h.cfg.Hub != nil {
		h.cfg.Hub.InvalidateDevice(params.DeviceId)
	}
	h.cfg.AuditLogger.Log(ctx, &admin.ID,
		audit.ActionDeviceAssign, audit.ResourceDevice, &params.DeviceId,
		map[string]any{"userId": params.ID})
	return &oas.AdminAssignDeviceNoContent{}, nil
}

// AdminUnassignDevice removes a device assignment from a user (admin only).
func (h *Handler) AdminUnassignDevice(ctx context.Context, params oas.AdminUnassignDeviceParams) (oas.AdminUnassignDeviceRes, error) {
	admin, err := requireAdminCtx(ctx)
	if err != nil {
		return &oas.AdminUnassignDeviceForbidden{Error: err.Error()}, nil
	}
	if err := h.cfg.Users.UnassignDevice(ctx, params.ID, params.DeviceId); err != nil {
		return &oas.AdminUnassignDeviceNotFound{Error: "user or device not found"}, nil
	}
	// Invalidate the hub's cached access list so WebSocket broadcasts stop
	// reaching the unassigned user immediately instead of after the cache TTL.
	if h.cfg.Hub != nil {
		h.cfg.Hub.InvalidateDevice(params.DeviceId)
	}
	h.cfg.AuditLogger.Log(ctx, &admin.ID,
		audit.ActionDeviceUnassign, audit.ResourceDevice, &params.DeviceId,
		map[string]any{"userId": params.ID})
	return &oas.AdminUnassignDeviceNoContent{}, nil
}

// applyDeviceInputFields applies the set fields of req onto a copy of d.
func applyDeviceInputFields(d *model.Device, req *oas.DeviceInput) *model.Device {
	clone := *d
	if v, ok := req.Phone.Get(); ok {
		clone.Phone = &v
	}
	if v, ok := req.Model.Get(); ok {
		clone.Model = &v
	}
	if v, ok := req.Contact.Get(); ok {
		clone.Contact = &v
	}
	if v, ok := req.Category.Get(); ok {
		clone.Category = &v
	}
	if v, ok := req.Protocol.Get(); ok {
		clone.Protocol = v
	}
	if req.CalendarId.Set {
		if v, ok := req.CalendarId.Get(); ok {
			clone.CalendarID = &v
		} else {
			clone.CalendarID = nil
		}
	}
	if v, ok := req.SpeedLimit.Get(); ok {
		clone.SpeedLimit = &v
	}
	if v, ok := req.Disabled.Get(); ok {
		clone.Disabled = v
	}
	if req.Mileage.Set {
		if v, ok := req.Mileage.Get(); ok {
			clone.Mileage = &v
		} else {
			clone.Mileage = nil
		}
	}
	if req.Attributes.Set {
		clone.Attributes = rawToAttrs(map[string]jx.Raw(req.Attributes.Value))
	}
	return &clone
}

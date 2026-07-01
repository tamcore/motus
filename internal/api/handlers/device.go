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

// ─── ogen Handler methods ───────────────────────────────────────────────────

// effectivePrefixCtx is the context-based variant of effectivePrefix.
// It returns the prefix only when the request was authenticated via API key.
func effectivePrefixCtx(ctx context.Context, prefix string) string {
	if api.ApiKeyFromContext(ctx) != nil {
		return prefix
	}
	return ""
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
	if devices == nil {
		devices = []*model.Device{}
	}
	prefix := effectivePrefixCtx(ctx, h.cfg.UniqueIDPrefix)
	model.ApplyUniqueIDPrefix(devices, prefix)
	result := make(oas.ListDevicesOKApplicationJSON, len(devices))
	for i, d := range devices {
		result[i] = deviceToOAS(d)
	}
	return &result, nil
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
	prefix := effectivePrefixCtx(ctx, h.cfg.UniqueIDPrefix)
	model.ApplyUniqueIDPrefix([]*model.Device{device}, prefix)
	out := deviceToOAS(device)
	return &out, nil
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
	device := oasInputToDevice(req)
	if device.Mileage != nil && *device.Mileage < 0 {
		return &oas.CreateDeviceBadRequest{Error: "mileage must be non-negative"}, nil
	}
	if err := h.cfg.Devices.Create(ctx, device, user.ID); err != nil {
		return &oas.CreateDeviceBadRequest{Error: "failed to create device"}, nil
	}
	h.cfg.AuditLogger.Log(ctx, &user.ID,
		audit.ActionDeviceCreate, audit.ResourceDevice, &device.ID,
		map[string]any{"name": device.Name, "uniqueId": device.UniqueID},
		"", "")
	prefix := effectivePrefixCtx(ctx, h.cfg.UniqueIDPrefix)
	model.ApplyUniqueIDPrefix([]*model.Device{device}, prefix)
	out := deviceToOAS(device)
	return &out, nil
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
	// Apply fields from the input onto the existing device (immutable merge).
	if req.UniqueId != "" && req.UniqueId != device.UniqueID {
		if err := validation.ValidateDeviceUniqueID(req.UniqueId); err != nil {
			return &oas.UpdateDeviceBadRequest{Error: err.Error()}, nil
		}
		device = &model.Device{
			ID:             device.ID,
			UniqueID:       req.UniqueId,
			Name:           device.Name,
			Protocol:       device.Protocol,
			Status:         device.Status,
			SpeedLimit:     device.SpeedLimit,
			LastUpdate:     device.LastUpdate,
			PositionID:     device.PositionID,
			GroupID:        device.GroupID,
			Phone:          device.Phone,
			Model:          device.Model,
			Contact:        device.Contact,
			Category:       device.Category,
			CalendarID:     device.CalendarID,
			ExpirationTime: device.ExpirationTime,
			Disabled:       device.Disabled,
			Mileage:        device.Mileage,
			PendingMileage: device.PendingMileage,
			Attributes:     device.Attributes,
			CreatedAt:      device.CreatedAt,
			UpdatedAt:      device.UpdatedAt,
		}
	}
	if req.Name != "" && req.Name != device.Name {
		if err := validation.ValidateName(req.Name); err != nil {
			return &oas.UpdateDeviceBadRequest{Error: err.Error()}, nil
		}
		device = cloneDeviceWithName(device, req.Name)
	}
	device = applyDeviceInputFields(device, req)
	if err := h.cfg.Devices.Update(ctx, device); err != nil {
		return &oas.UpdateDeviceBadRequest{Error: "failed to update device"}, nil
	}
	h.cfg.AuditLogger.Log(ctx, &user.ID,
		audit.ActionDeviceUpdate, audit.ResourceDevice, &device.ID,
		map[string]any{"name": device.Name},
		"", "")
	prefix := effectivePrefixCtx(ctx, h.cfg.UniqueIDPrefix)
	model.ApplyUniqueIDPrefix([]*model.Device{device}, prefix)
	out := deviceToOAS(device)
	return &out, nil
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
	deviceID := params.ID
	h.cfg.AuditLogger.Log(ctx, &user.ID,
		audit.ActionDeviceDelete, audit.ResourceDevice, &deviceID,
		nil, "", "")
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
	if devices == nil {
		devices = []*model.Device{}
	}
	result := make(oas.AdminListUserDevicesOKApplicationJSON, len(devices))
	for i, d := range devices {
		result[i] = deviceToOAS(d)
	}
	return &result, nil
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
		map[string]any{"userId": params.ID},
		"", "")
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
		map[string]any{"userId": params.ID},
		"", "")
	return &oas.AdminUnassignDeviceNoContent{}, nil
}

// ─── helpers ────────────────────────────────────────────────────────────────

// cloneDeviceWithName returns a shallow copy of d with the name replaced.
func cloneDeviceWithName(d *model.Device, name string) *model.Device {
	clone := *d
	clone.Name = name
	return &clone
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
	if req.Attributes.Set {
		clone.Attributes = rawToAttrs(map[string]jx.Raw(req.Attributes.Value))
	}
	return &clone
}

package handlers

import (
	"context"
	"slices"

	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
)

// isValidCommandType checks if a command type is in the supported allowlist.
func isValidCommandType(t string) bool {
	return slices.Contains(model.SupportedCommandTypes(), t)
}

// --- ogen Handler methods ---

// oasCommandInputToModel converts an oas.CommandInput to a model.Command.
func oasCommandInputToModel(req *oas.CommandInput) *model.Command {
	return &model.Command{
		DeviceID:   req.DeviceId,
		Type:       req.Type,
		Attributes: oasCommandAttrsToModel(req.Attributes),
		Status:     model.CommandStatusPending,
	}
}

// CreateCommand implements oas.Handler for POST /api/commands.
// Queues a command for later delivery to the device.
func (h *Handler) CreateCommand(ctx context.Context, req *oas.CommandInput) (oas.CreateCommandRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.CreateCommandUnauthorized{Error: "unauthorized"}, nil
	}
	if req.DeviceId == 0 || req.Type == "" {
		return &oas.CreateCommandBadRequest{Error: "deviceId and type are required"}, nil
	}
	if !isValidCommandType(req.Type) {
		return &oas.CreateCommandBadRequest{Error: "invalid command type"}, nil
	}
	if !h.cfg.Devices.UserHasAccess(ctx, user, req.DeviceId) {
		return &oas.CreateCommandBadRequest{Error: "access denied"}, nil
	}

	cmd := oasCommandInputToModel(req)
	if err := h.cfg.Commands.Create(ctx, cmd); err != nil {
		return &oas.CreateCommandBadRequest{Error: "failed to create command"}, nil
	}
	out := commandToOAS(cmd)
	return &out, nil
}

// ListCommands implements oas.Handler for GET /api/commands.
// Returns the most recent commands for a device.
func (h *Handler) ListCommands(ctx context.Context, params oas.ListCommandsParams) (oas.ListCommandsRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.Error{Error: "unauthorized"}, nil
	}

	deviceID, hasDevice := params.DeviceId.Get()
	if !hasDevice || deviceID == 0 {
		return &oas.Error{Error: "deviceId is required"}, nil
	}
	if !h.cfg.Devices.UserHasAccess(ctx, user, deviceID) {
		return &oas.Error{Error: "access denied"}, nil
	}

	const defaultLimit = 10
	commands, err := h.cfg.Commands.ListByDevice(ctx, deviceID, defaultLimit)
	if err != nil {
		return &oas.Error{Error: "failed to list commands"}, nil
	}
	result := make(oas.ListCommandsOKApplicationJSON, len(commands))
	for i, c := range commands {
		result[i] = commandToOAS(c)
	}
	return &result, nil
}

// GetCommandTypes implements oas.Handler for GET /api/commands/types.
// Returns the list of supported command types.
func (h *Handler) GetCommandTypes(ctx context.Context) (oas.GetCommandTypesRes, error) {
	types := model.SupportedCommandTypes()
	result := make(oas.GetCommandTypesOKApplicationJSON, len(types))
	for i, t := range types {
		result[i] = oas.CommandType{Type: t}
	}
	return &result, nil
}

// SendCommand implements oas.Handler for POST /api/commands/send.
// Creates a command and attempts immediate delivery to a live device connection.
func (h *Handler) SendCommand(ctx context.Context, req *oas.SendCommandRequest) (oas.SendCommandRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.SendCommandUnauthorized{Error: "unauthorized"}, nil
	}
	if req.DeviceId == 0 || req.Type == "" {
		return &oas.SendCommandBadRequest{Error: "deviceId and type are required"}, nil
	}
	if !isValidCommandType(req.Type) {
		return &oas.SendCommandBadRequest{Error: "invalid command type"}, nil
	}

	attrs := oasCommandAttrsToModel(req.Attributes)

	// Custom commands require a non-empty "text" attribute.
	if req.Type == model.CommandCustom {
		text, _ := attrs["text"].(string)
		if text == "" {
			return &oas.SendCommandBadRequest{Error: "custom commands require a non-empty 'text' attribute"}, nil
		}
	}
	if !h.cfg.Devices.UserHasAccess(ctx, user, req.DeviceId) {
		return &oas.SendCommandBadRequest{Error: "access denied"}, nil
	}

	device, err := h.cfg.Devices.GetByID(ctx, req.DeviceId)
	if err != nil {
		return &oas.SendCommandNotFound{Error: "device not found"}, nil
	}

	// Encode the command payload.
	var payload []byte
	if req.Type == model.CommandCustom {
		text, _ := attrs["text"].(string)
		payload = []byte(text)
	} else if h.cfg.EncoderRegistry != nil {
		enc := h.cfg.EncoderRegistry.Get(device.Protocol)
		if enc == nil {
			return &oas.SendCommandBadRequest{Error: "no encoder for device protocol: " + device.Protocol}, nil
		}
		modelCmd := &model.Command{Type: req.Type, Attributes: attrs}
		payload, err = enc.EncodeCommand(modelCmd, device.UniqueID)
		if err != nil {
			return &oas.SendCommandBadRequest{Error: "encode command: " + err.Error()}, nil
		}
	}

	// Save command as pending before dispatching (device can respond within ms).
	cmd := &model.Command{
		DeviceID:   req.DeviceId,
		Type:       req.Type,
		Attributes: attrs,
		Status:     model.CommandStatusPending,
	}
	if err := h.cfg.Commands.Create(ctx, cmd); err != nil {
		return &oas.SendCommandBadRequest{Error: "failed to create command"}, nil
	}

	// Attempt immediate delivery if device is online.
	online := h.cfg.DeviceRegistry != nil && h.cfg.DeviceRegistry.IsOnline(device.UniqueID)
	if online && payload != nil {
		if updErr := h.cfg.Commands.UpdateStatus(ctx, cmd.ID, model.CommandStatusSent); updErr == nil {
			if h.cfg.DeviceRegistry.Send(device.UniqueID, payload) {
				cmd.Status = model.CommandStatusSent
			} else {
				_ = h.cfg.Commands.UpdateStatus(ctx, cmd.ID, model.CommandStatusPending)
			}
		}
	}

	if h.cfg.AuditLogger != nil {
		details := map[string]any{
			"commandType":   cmd.Type,
			"commandStatus": cmd.Status,
			"deviceName":    device.Name,
		}
		h.cfg.AuditLogger.Log(ctx, &user.ID,
			audit.ActionCommandSend, audit.ResourceCommand, &cmd.ID, details, "", "")
	}

	out := commandToOAS(cmd)
	return &out, nil
}

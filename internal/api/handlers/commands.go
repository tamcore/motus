package handlers

import (
	"context"
	"errors"
	"slices"

	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/protocol"
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
// Without deviceId it returns all command types; with deviceId only those
// the device's protocol can receive (Traccar-compatible).
func (h *Handler) GetCommandTypes(ctx context.Context, params oas.GetCommandTypesParams) (oas.GetCommandTypesRes, error) {
	types := model.SupportedCommandTypes()
	if deviceID, ok := params.DeviceId.Get(); ok {
		user := api.UserFromContext(ctx)
		if user == nil {
			return &oas.GetCommandTypesUnauthorized{Error: "unauthorized"}, nil
		}
		if !h.cfg.Devices.UserHasAccess(ctx, user, deviceID) {
			return &oas.GetCommandTypesForbidden{Error: "access denied"}, nil
		}
		device, err := h.cfg.Devices.GetByID(ctx, deviceID)
		if err != nil {
			return &oas.GetCommandTypesNotFound{Error: "device not found"}, nil
		}
		types = h.cfg.EncoderRegistry.SupportedCommands(device.Protocol)
	}
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
	// Validate, encode, persist as pending and deliver immediately when the
	// device is connected — the same path notification command rules use.
	submitter := &protocol.CommandSubmitter{
		Commands: h.cfg.Commands,
		Encoders: h.cfg.EncoderRegistry,
		Registry: h.cfg.DeviceRegistry,
	}
	cmd, err := submitter.Submit(ctx, device, req.Type, attrs)
	switch {
	case errors.Is(err, protocol.ErrCommandUnsupported):
		return &oas.SendCommandBadRequest{
			Error: "command type " + req.Type + " is not supported by device protocol " + device.Protocol,
		}, nil
	case errors.Is(err, protocol.ErrNoEncoder):
		return &oas.SendCommandBadRequest{Error: "no encoder for device protocol: " + device.Protocol}, nil
	case errors.Is(err, protocol.ErrCommandEncode):
		return &oas.SendCommandBadRequest{Error: err.Error()}, nil
	case err != nil:
		return &oas.SendCommandBadRequest{Error: "failed to create command"}, nil
	}

	details := map[string]any{
		"commandType":   cmd.Type,
		"commandStatus": cmd.Status,
		"deviceName":    device.Name,
	}
	h.cfg.AuditLogger.Log(ctx, &user.ID,
		audit.ActionCommandSend, audit.ResourceCommand, &cmd.ID, details, "", "")

	out := commandToOAS(cmd)
	return &out, nil
}

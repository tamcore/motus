package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-faster/jx"
	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/protocol"
	"github.com/tamcore/motus/internal/storage/repository"
)

// CommandHandler handles command API endpoints.
type CommandHandler struct {
	commands repository.CommandRepo
	devices  repository.DeviceRepo
	registry *protocol.DeviceRegistry
	encoders *protocol.EncoderRegistry
	audit    *audit.Logger
}

// NewCommandHandler creates a new command handler.
func NewCommandHandler(
	commands repository.CommandRepo,
	devices repository.DeviceRepo,
	registry *protocol.DeviceRegistry,
	encoders *protocol.EncoderRegistry,
) *CommandHandler {
	return &CommandHandler{
		commands: commands,
		devices:  devices,
		registry: registry,
		encoders: encoders,
	}
}

// SetAuditLogger configures audit logging for command events.
func (h *CommandHandler) SetAuditLogger(logger *audit.Logger) {
	h.audit = logger
}

type commandRequest struct {
	DeviceID   int64                  `json:"deviceId"`
	Type       string                 `json:"type"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

// isValidCommandType checks if a command type is in the supported allowlist.
func isValidCommandType(t string) bool {
	for _, valid := range model.SupportedCommandTypes() {
		if t == valid {
			return true
		}
	}
	return false
}

// Create queues a command for later delivery to the device.
// POST /api/commands
func (h *CommandHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	var req commandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DeviceID == 0 || req.Type == "" {
		api.RespondError(w, http.StatusBadRequest, "deviceId and type are required")
		return
	}
	if !isValidCommandType(req.Type) {
		api.RespondError(w, http.StatusBadRequest, "invalid command type")
		return
	}
	if !h.devices.UserHasAccess(r.Context(), user, req.DeviceID) {
		api.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	cmd := &model.Command{
		DeviceID:   req.DeviceID,
		Type:       req.Type,
		Attributes: req.Attributes,
		Status:     model.CommandStatusPending,
	}
	if err := h.commands.Create(r.Context(), cmd); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to create command")
		return
	}
	api.RespondJSON(w, http.StatusOK, cmd)
}

// Send creates a command and attempts immediate delivery to a live device connection.
// POST /api/commands/send
func (h *CommandHandler) Send(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	var req commandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DeviceID == 0 || req.Type == "" {
		api.RespondError(w, http.StatusBadRequest, "deviceId and type are required")
		return
	}
	if !isValidCommandType(req.Type) {
		api.RespondError(w, http.StatusBadRequest, "invalid command type")
		return
	}
	// Custom commands require a non-empty "text" attribute.
	if req.Type == model.CommandCustom {
		text, _ := req.Attributes["text"].(string)
		if text == "" {
			api.RespondError(w, http.StatusBadRequest, "custom commands require a non-empty 'text' attribute")
			return
		}
	}
	if !h.devices.UserHasAccess(r.Context(), user, req.DeviceID) {
		api.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	// Look up the device to get its uniqueID and protocol.
	device, err := h.devices.GetByID(r.Context(), req.DeviceID)
	if err != nil {
		api.RespondError(w, http.StatusNotFound, "device not found")
		return
	}

	// Encode the command bytes.
	var payload []byte
	if req.Type == model.CommandCustom {
		text, _ := req.Attributes["text"].(string)
		payload = []byte(text)
	} else if h.encoders != nil {
		enc := h.encoders.Get(device.Protocol)
		if enc == nil {
			api.RespondError(w, http.StatusBadRequest, "no encoder for device protocol: "+device.Protocol)
			return
		}
		cmd := &model.Command{Type: req.Type, Attributes: req.Attributes}
		var encErr error
		payload, encErr = enc.EncodeCommand(cmd, device.UniqueID)
		if encErr != nil {
			api.RespondError(w, http.StatusBadRequest, "encode command: "+encErr.Error())
			return
		}
	}

	// Save the command as "pending" first so the DB row exists before the
	// payload reaches the device. (The device can respond within milliseconds
	// of receiving the command; if the row is inserted after the TCP write,
	// GetLatestSentByDevice won't find it.)
	cmd := &model.Command{
		DeviceID:   req.DeviceID,
		Type:       req.Type,
		Attributes: req.Attributes,
		Status:     model.CommandStatusPending,
	}
	if err := h.commands.Create(r.Context(), cmd); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to create command")
		return
	}

	// Attempt immediate delivery if device is online.
	online := h.registry != nil && h.registry.IsOnline(device.UniqueID)
	if online && payload != nil {
		// Update status before dispatching so GetLatestSentByDevice finds it.
		if updErr := h.commands.UpdateStatus(r.Context(), cmd.ID, model.CommandStatusSent); updErr == nil {
			if h.registry.Send(device.UniqueID, payload) {
				cmd.Status = model.CommandStatusSent
			} else {
				// Channel full or device disconnected — revert to pending.
				_ = h.commands.UpdateStatus(r.Context(), cmd.ID, model.CommandStatusPending)
			}
		}
	}

	// Audit log.
	if h.audit != nil {
		details := map[string]interface{}{
			"commandType":   cmd.Type,
			"commandStatus": cmd.Status,
			"deviceName":    device.Name,
		}
		h.audit.LogFromRequest(r, &user.ID, audit.ActionCommandSend, audit.ResourceCommand, &cmd.ID, details)
	}

	if cmd.Status == model.CommandStatusPending {
		api.RespondJSON(w, http.StatusAccepted, cmd)
		return
	}
	api.RespondJSON(w, http.StatusOK, cmd)
}

// List returns the most recent commands for a device.
// GET /api/commands?deviceId={id}&limit={n}
func (h *CommandHandler) List(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())

	deviceIDStr := r.URL.Query().Get("deviceId")
	if deviceIDStr == "" {
		api.RespondError(w, http.StatusBadRequest, "deviceId is required")
		return
	}
	deviceID, err := strconv.ParseInt(deviceIDStr, 10, 64)
	if err != nil || deviceID <= 0 {
		api.RespondError(w, http.StatusBadRequest, "invalid deviceId")
		return
	}

	if !h.devices.UserHasAccess(r.Context(), user, deviceID) {
		api.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	commands, err := h.commands.ListByDevice(r.Context(), deviceID, limit)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to list commands")
		return
	}
	if commands == nil {
		commands = []*model.Command{}
	}
	api.RespondJSON(w, http.StatusOK, commands)
}

// GetTypes returns the list of supported command types.
// GET /api/commands/types
func (h *CommandHandler) GetTypes(w http.ResponseWriter, r *http.Request) {
	types := make([]map[string]string, 0)
	for _, t := range model.SupportedCommandTypes() {
		types = append(types, map[string]string{"type": t})
	}
	api.RespondJSON(w, http.StatusOK, types)
}

// --- ogen Handler methods ---

// oasCommandInputToModel converts an oas.CommandInput to a model.Command.
func oasCommandInputToModel(req *oas.CommandInput) *model.Command {
	var attrs map[string]interface{}
	if req.Attributes.Set {
		attrs = rawToAttrs(map[string]jx.Raw(req.Attributes.Value))
	}
	return &model.Command{
		DeviceID:   req.DeviceId,
		Type:       req.Type,
		Attributes: attrs,
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

	// Convert attributes for custom command text check.
	var attrs map[string]interface{}
	if req.Attributes.Set {
		attrs = rawToAttrs(map[string]jx.Raw(req.Attributes.Value))
	}

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
		details := map[string]interface{}{
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

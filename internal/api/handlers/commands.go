package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/tamcore/motus/internal/api"
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

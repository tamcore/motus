package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/notification"
	"github.com/tamcore/motus/internal/services"
	"github.com/tamcore/motus/internal/storage/repository"
)

// NotificationHandler handles notification rule CRUD endpoints.
type NotificationHandler struct {
	notifications       repository.NotificationRepo
	notificationService *services.NotificationService
	audit               *audit.Logger
}

// NewNotificationHandler creates a new notification handler.
func NewNotificationHandler(
	notifications repository.NotificationRepo,
	notificationService *services.NotificationService,
) *NotificationHandler {
	return &NotificationHandler{
		notifications:       notifications,
		notificationService: notificationService,
	}
}

// SetAuditLogger configures audit logging for notification events.
func (h *NotificationHandler) SetAuditLogger(logger *audit.Logger) {
	h.audit = logger
}

type notificationRuleRequest struct {
	Name       string                 `json:"name"`
	EventTypes []string               `json:"eventTypes"`
	Channel    string                 `json:"channel"`
	Config     map[string]interface{} `json:"config"`
	Template   string                 `json:"template"`
	Enabled    bool                   `json:"enabled"`
}

// validEventTypes lists the event types the notification system supports.
var validEventTypes = map[string]bool{
	"geofenceEnter": true,
	"geofenceExit":  true,
	"deviceOnline":  true,
	"deviceOffline": true,
	"overspeed":     true,
	"motion":        true,
	"deviceIdle":    true,
	"ignitionOn":    true,
	"ignitionOff":   true,
	"alarm":         true,
	"tripCompleted": true,
}

// validChannels lists the notification delivery channels.
var validChannels = map[string]bool{
	"webhook": true,
}

// List returns all notification rules for the authenticated user.
// GET /api/notifications
func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	if user == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	rules, err := h.notifications.GetByUser(r.Context(), user.ID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to get notification rules")
		return
	}
	if rules == nil {
		rules = []*model.NotificationRule{}
	}
	api.RespondJSON(w, http.StatusOK, rules)
}

// AdminListAll returns all notification rules in the system with owner info (admin only).
// GET /api/admin/notifications
func (h *NotificationHandler) AdminListAll(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAdmin(w, r); !ok {
		return
	}
	rules, err := h.notifications.GetAll(r.Context())
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to list notification rules")
		return
	}
	if rules == nil {
		rules = []*model.NotificationRule{}
	}
	api.RespondJSON(w, http.StatusOK, rules)
}

// Create adds a new notification rule for the authenticated user.
// POST /api/notifications
func (h *NotificationHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	if user == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req notificationRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		api.RespondError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(req.EventTypes) == 0 {
		api.RespondError(w, http.StatusBadRequest, "at least one event type is required")
		return
	}
	for _, et := range req.EventTypes {
		if !validEventTypes[et] {
			api.RespondError(w, http.StatusBadRequest, fmt.Sprintf("invalid event type: %s", et))
			return
		}
	}
	if !validChannels[req.Channel] {
		api.RespondError(w, http.StatusBadRequest, "invalid channel")
		return
	}
	if req.Template == "" {
		api.RespondError(w, http.StatusBadRequest, "template is required")
		return
	}

	// Validate webhook URL to prevent SSRF attacks.
	if req.Channel == "webhook" {
		webhookURL, _ := req.Config["webhookUrl"].(string)
		if err := notification.ValidateWebhookURL(webhookURL); err != nil {
			api.RespondError(w, http.StatusBadRequest, fmt.Sprintf("invalid webhook URL: %v", err))
			return
		}
	}

	rule := &model.NotificationRule{
		UserID:     user.ID,
		Name:       req.Name,
		EventTypes: req.EventTypes,
		Channel:    req.Channel,
		Config:     req.Config,
		Template:   req.Template,
		Enabled:    req.Enabled,
	}

	if err := h.notifications.Create(r.Context(), rule); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to create notification rule")
		return
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &user.ID, audit.ActionNotifCreate, audit.ResourceNotification, &rule.ID,
			map[string]interface{}{"name": rule.Name, "eventTypes": rule.EventTypes, "channel": rule.Channel})
	}

	api.RespondJSON(w, http.StatusCreated, rule)
}

// Update modifies an existing notification rule.
// PUT /api/notifications/{id}
func (h *NotificationHandler) Update(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	if user == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid notification rule id")
		return
	}

	var req notificationRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		api.RespondError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(req.EventTypes) == 0 {
		api.RespondError(w, http.StatusBadRequest, "at least one event type is required")
		return
	}
	for _, et := range req.EventTypes {
		if !validEventTypes[et] {
			api.RespondError(w, http.StatusBadRequest, fmt.Sprintf("invalid event type: %s", et))
			return
		}
	}
	if !validChannels[req.Channel] {
		api.RespondError(w, http.StatusBadRequest, "invalid channel")
		return
	}

	// Validate webhook URL to prevent SSRF attacks.
	if req.Channel == "webhook" {
		webhookURL, _ := req.Config["webhookUrl"].(string)
		if err := notification.ValidateWebhookURL(webhookURL); err != nil {
			api.RespondError(w, http.StatusBadRequest, fmt.Sprintf("invalid webhook URL: %v", err))
			return
		}
	}

	// Verify the rule exists and belongs to the authenticated user before updating.
	existing, err := h.notifications.GetByID(r.Context(), id)
	if err != nil {
		api.RespondError(w, http.StatusNotFound, "notification rule not found")
		return
	}
	if existing.UserID != user.ID && !user.IsAdmin() {
		api.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	rule := &model.NotificationRule{
		ID:         id,
		UserID:     existing.UserID,
		Name:       req.Name,
		EventTypes: req.EventTypes,
		Channel:    req.Channel,
		Config:     req.Config,
		Template:   req.Template,
		Enabled:    req.Enabled,
	}

	if err := h.notifications.Update(r.Context(), rule); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to update notification rule")
		return
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &user.ID, audit.ActionNotifUpdate, audit.ResourceNotification, &id,
			map[string]interface{}{"name": rule.Name, "eventTypes": rule.EventTypes, "channel": rule.Channel})
	}

	api.RespondJSON(w, http.StatusOK, rule)
}

// Delete removes a notification rule.
// DELETE /api/notifications/{id}
func (h *NotificationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	if user == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid notification rule id")
		return
	}

	existing, err := h.notifications.GetByID(r.Context(), id)
	if err != nil {
		api.RespondError(w, http.StatusNotFound, "notification rule not found")
		return
	}
	if existing.UserID != user.ID && !user.IsAdmin() {
		api.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	if err := h.notifications.Delete(r.Context(), id); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to delete notification rule")
		return
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &user.ID, audit.ActionNotifDelete, audit.ResourceNotification, &id, nil)
	}

	w.WriteHeader(http.StatusNoContent)
}

// Test sends a test notification for a rule.
// POST /api/notifications/{id}/test
func (h *NotificationHandler) Test(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	if user == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid notification rule id")
		return
	}

	rule, err := h.notifications.GetByID(r.Context(), id)
	if err != nil {
		api.RespondError(w, http.StatusNotFound, "notification rule not found")
		return
	}

	// Verify the rule belongs to the authenticated user.
	if rule.UserID != user.ID && !user.IsAdmin() {
		api.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	statusCode, err := h.notificationService.SendTestNotification(r.Context(), rule)
	if err != nil {
		api.RespondJSON(w, http.StatusOK, map[string]interface{}{
			"status":       "failed",
			"error":        err.Error(),
			"responseCode": statusCode,
		})
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"status":       "sent",
		"responseCode": statusCode,
	})
}

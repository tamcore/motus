package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-faster/jx"
	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
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

// --- ogen Handler methods ---

// ListNotifications returns all notification rules for the authenticated user.
func (h *Handler) ListNotifications(ctx context.Context) (oas.ListNotificationsRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.Error{Error: "unauthorized"}, nil
	}
	rules, err := h.cfg.Notifications.GetByUser(ctx, user.ID)
	if err != nil {
		return &oas.Error{Error: "failed to list notification rules"}, nil
	}
	if rules == nil {
		rules = []*model.NotificationRule{}
	}
	result := make(oas.ListNotificationsOKApplicationJSON, len(rules))
	for i, r := range rules {
		result[i] = notificationRuleToOAS(r)
	}
	return &result, nil
}

// CreateNotification adds a new notification rule for the authenticated user.
func (h *Handler) CreateNotification(ctx context.Context, req *oas.NotificationRuleInput) (oas.CreateNotificationRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.CreateNotificationUnauthorized{Error: "unauthorized"}, nil
	}
	if req.Name == "" {
		return &oas.CreateNotificationBadRequest{Error: "name is required"}, nil
	}
	if len(req.EventTypes) == 0 {
		return &oas.CreateNotificationBadRequest{Error: "at least one event type is required"}, nil
	}
	for _, et := range req.EventTypes {
		if !validEventTypes[et] {
			return &oas.CreateNotificationBadRequest{Error: fmt.Sprintf("invalid event type: %s", et)}, nil
		}
	}
	if !validChannels[req.Channel] {
		return &oas.CreateNotificationBadRequest{Error: "invalid channel"}, nil
	}
	tmpl, _ := req.Template.Get()
	if tmpl == "" {
		return &oas.CreateNotificationBadRequest{Error: "template is required"}, nil
	}

	cfg := rawToAttrs(map[string]jx.Raw(req.Config))
	if req.Channel == "webhook" {
		webhookURL, _ := cfg["webhookUrl"].(string)
		if err := notification.ValidateWebhookURL(webhookURL); err != nil {
			return &oas.CreateNotificationBadRequest{Error: fmt.Sprintf("invalid webhook URL: %v", err)}, nil
		}
	}

	enabled, _ := req.Enabled.Get()
	rule := &model.NotificationRule{
		UserID:     user.ID,
		Name:       req.Name,
		EventTypes: req.EventTypes,
		Channel:    req.Channel,
		Config:     cfg,
		Template:   tmpl,
		Enabled:    enabled,
	}
	if err := h.cfg.Notifications.Create(ctx, rule); err != nil {
		return &oas.CreateNotificationBadRequest{Error: "failed to create notification rule"}, nil
	}

	if h.cfg.AuditLogger != nil {
		h.cfg.AuditLogger.Log(ctx, &user.ID,
			audit.ActionNotifCreate, audit.ResourceNotification, &rule.ID,
			map[string]interface{}{"name": rule.Name, "eventTypes": rule.EventTypes, "channel": rule.Channel},
			"", "")
	}
	out := notificationRuleToOAS(rule)
	return &out, nil
}

// UpdateNotification modifies an existing notification rule.
func (h *Handler) UpdateNotification(ctx context.Context, req *oas.NotificationRuleInput, params oas.UpdateNotificationParams) (oas.UpdateNotificationRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.UpdateNotificationUnauthorized{Error: "unauthorized"}, nil
	}
	if req.Name == "" {
		return &oas.UpdateNotificationBadRequest{Error: "name is required"}, nil
	}
	if len(req.EventTypes) == 0 {
		return &oas.UpdateNotificationBadRequest{Error: "at least one event type is required"}, nil
	}
	for _, et := range req.EventTypes {
		if !validEventTypes[et] {
			return &oas.UpdateNotificationBadRequest{Error: fmt.Sprintf("invalid event type: %s", et)}, nil
		}
	}
	if !validChannels[req.Channel] {
		return &oas.UpdateNotificationBadRequest{Error: "invalid channel"}, nil
	}

	cfg := rawToAttrs(map[string]jx.Raw(req.Config))
	if req.Channel == "webhook" {
		webhookURL, _ := cfg["webhookUrl"].(string)
		if err := notification.ValidateWebhookURL(webhookURL); err != nil {
			return &oas.UpdateNotificationBadRequest{Error: fmt.Sprintf("invalid webhook URL: %v", err)}, nil
		}
	}

	existing, err := h.cfg.Notifications.GetByID(ctx, params.ID)
	if err != nil {
		return &oas.UpdateNotificationNotFound{Error: "notification rule not found"}, nil
	}
	if existing.UserID != user.ID && !user.IsAdmin() {
		return &oas.UpdateNotificationForbidden{Error: "access denied"}, nil
	}

	tmpl, _ := req.Template.Get()
	enabled, _ := req.Enabled.Get()
	rule := &model.NotificationRule{
		ID:         params.ID,
		UserID:     existing.UserID,
		Name:       req.Name,
		EventTypes: req.EventTypes,
		Channel:    req.Channel,
		Config:     cfg,
		Template:   tmpl,
		Enabled:    enabled,
	}
	if err := h.cfg.Notifications.Update(ctx, rule); err != nil {
		return &oas.UpdateNotificationBadRequest{Error: "failed to update notification rule"}, nil
	}

	if h.cfg.AuditLogger != nil {
		h.cfg.AuditLogger.Log(ctx, &user.ID,
			audit.ActionNotifUpdate, audit.ResourceNotification, &params.ID,
			map[string]interface{}{"name": rule.Name, "eventTypes": rule.EventTypes, "channel": rule.Channel},
			"", "")
	}
	out := notificationRuleToOAS(rule)
	return &out, nil
}

// DeleteNotification removes a notification rule.
func (h *Handler) DeleteNotification(ctx context.Context, params oas.DeleteNotificationParams) (oas.DeleteNotificationRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.DeleteNotificationUnauthorized{Error: "unauthorized"}, nil
	}
	existing, err := h.cfg.Notifications.GetByID(ctx, params.ID)
	if err != nil {
		return &oas.DeleteNotificationNotFound{Error: "notification rule not found"}, nil
	}
	if existing.UserID != user.ID && !user.IsAdmin() {
		return &oas.DeleteNotificationForbidden{Error: "access denied"}, nil
	}
	if err := h.cfg.Notifications.Delete(ctx, params.ID); err != nil {
		return &oas.DeleteNotificationForbidden{Error: "failed to delete notification rule"}, nil
	}
	if h.cfg.AuditLogger != nil {
		id := params.ID
		h.cfg.AuditLogger.Log(ctx, &user.ID,
			audit.ActionNotifDelete, audit.ResourceNotification, &id,
			nil, "", "")
	}
	return &oas.DeleteNotificationNoContent{}, nil
}

// NotificationLogs returns recent delivery logs for a notification rule.
func (h *Handler) NotificationLogs(ctx context.Context, params oas.NotificationLogsParams) (oas.NotificationLogsRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.NotificationLogsUnauthorized{Error: "unauthorized"}, nil
	}
	rule, err := h.cfg.Notifications.GetByID(ctx, params.ID)
	if err != nil {
		return &oas.NotificationLogsNotFound{Error: "notification rule not found"}, nil
	}
	if rule.UserID != user.ID && !user.IsAdmin() {
		return &oas.NotificationLogsForbidden{Error: "access denied"}, nil
	}
	logs, err := h.cfg.Notifications.GetLogsByRule(ctx, params.ID, 50)
	if err != nil {
		return &oas.NotificationLogsNotFound{Error: "failed to get notification logs"}, nil
	}
	if logs == nil {
		logs = []*model.NotificationLog{}
	}
	result := make(oas.NotificationLogsOKApplicationJSON, len(logs))
	for i, l := range logs {
		result[i] = notificationLogToOAS(l)
	}
	return &result, nil
}

// TestNotification sends a test notification for a rule.
func (h *Handler) TestNotification(ctx context.Context, params oas.TestNotificationParams) (oas.TestNotificationRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.TestNotificationUnauthorized{Error: "unauthorized"}, nil
	}
	rule, err := h.cfg.Notifications.GetByID(ctx, params.ID)
	if err != nil {
		return &oas.TestNotificationNotFound{Error: "notification rule not found"}, nil
	}
	if rule.UserID != user.ID && !user.IsAdmin() {
		return &oas.TestNotificationForbidden{Error: "access denied"}, nil
	}
	if _, err := h.cfg.NotificationService.SendTestNotification(ctx, rule); err != nil {
		return &oas.TestNotificationNotFound{Error: err.Error()}, nil
	}
	return &oas.TestNotificationNoContent{}, nil
}

// AdminListNotifications returns all notification rules in the system (admin only).
func (h *Handler) AdminListNotifications(ctx context.Context) (oas.AdminListNotificationsRes, error) {
	if _, err := requireAdminCtx(ctx); err != nil {
		return &oas.AdminListNotificationsForbidden{Error: err.Error()}, nil
	}
	rules, err := h.cfg.Notifications.GetAll(ctx)
	if err != nil {
		return &oas.AdminListNotificationsForbidden{Error: "failed to list notification rules"}, nil
	}
	if rules == nil {
		rules = []*model.NotificationRule{}
	}
	result := make(oas.AdminListNotificationsOKApplicationJSON, len(rules))
	for i, r := range rules {
		result[i] = notificationRuleToOAS(r)
	}
	return &result, nil
}

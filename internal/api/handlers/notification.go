package handlers

import (
	"context"
	"fmt"

	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/notification"
)

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

	cfg, cfgErr := oasNotificationConfigToModel(req.Config)
	if cfgErr != nil {
		return &oas.CreateNotificationBadRequest{Error: cfgErr.Error()}, nil
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
			map[string]any{"name": rule.Name, "eventTypes": rule.EventTypes, "channel": rule.Channel},
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

	cfg, cfgErr := oasNotificationConfigToModel(req.Config)
	if cfgErr != nil {
		return &oas.UpdateNotificationBadRequest{Error: cfgErr.Error()}, nil
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
			map[string]any{"name": rule.Name, "eventTypes": rule.EventTypes, "channel": rule.Channel},
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

// oasNotificationConfigToModel converts a typed oas.NotificationRuleConfig to a model attribute map.
func oasNotificationConfigToModel(config oas.NotificationRuleConfig) (map[string]any, error) {
	if wh, ok := config.GetNotificationConfigWebhook(); ok {
		webhookURL := wh.WebhookUrl.String()
		if err := notification.ValidateWebhookURL(webhookURL); err != nil {
			return nil, fmt.Errorf("invalid webhook URL: %v", err)
		}
		cfg := map[string]any{"webhookUrl": webhookURL}
		if wh.Headers.Set && len(wh.Headers.Value) > 0 {
			hmap := make(map[string]any, len(wh.Headers.Value))
			for k, v := range wh.Headers.Value {
				hmap[k] = v
			}
			cfg["headers"] = hmap
		}
		return cfg, nil
	}
	return nil, fmt.Errorf("unsupported notification channel config")
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

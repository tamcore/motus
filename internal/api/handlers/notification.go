package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/services"
)

// notificationRuleInputFromOAS decodes a create/update request. The rule
// itself is validated by services.NotificationRuleService.
func notificationRuleInputFromOAS(req *oas.NotificationRuleInput) (services.NotificationRuleInput, error) {
	cfg, err := notificationConfigToModel(req.Channel, req.Config)
	if err != nil {
		return services.NotificationRuleInput{}, err
	}
	tmpl, _ := req.Template.Get()
	enabled, _ := req.Enabled.Get()
	return services.NotificationRuleInput{
		Name:        req.Name,
		EventTypes:  req.EventTypes,
		Channel:     req.Channel,
		Config:      cfg,
		Template:    tmpl,
		Enabled:     enabled,
		GeofenceIDs: req.GeofenceIds,
	}, nil
}

// notificationConfigToModel converts the typed config of channel to the
// stored config map. Unknown channels yield nil (rejected by the service).
func notificationConfigToModel(channel string, config oas.NotificationRuleConfig) (map[string]any, error) {
	switch channel {
	case model.NotificationChannelCommand:
		c, ok := config.GetNotificationConfigCommand()
		if !ok {
			return nil, errors.New("config does not match channel command")
		}
		return notificationCommandConfigToModel(c)
	case model.NotificationChannelWebhook:
		wh, ok := config.GetNotificationConfigWebhook()
		if !ok {
			return nil, errors.New("config does not match channel webhook")
		}
		cfg := map[string]any{"webhookUrl": wh.WebhookUrl.String()}
		if len(wh.Headers.Value) > 0 {
			headers := make(map[string]any, len(wh.Headers.Value))
			for k, v := range wh.Headers.Value {
				headers[k] = v
			}
			cfg["headers"] = headers
		}
		return cfg, nil
	default:
		return nil, nil
	}
}

// notificationCommandConfigToModel validates command attributes and converts
// them to the stored config map {commandType, attributes?}.
func notificationCommandConfigToModel(c oas.NotificationConfigCommand) (map[string]any, error) {
	if c.Attributes.Set && string(c.Attributes.Value.Type) != c.CommandType {
		return nil, fmt.Errorf("command attributes of type %s do not match command type %s",
			c.Attributes.Value.Type, c.CommandType)
	}
	attrs := oasCommandAttrsToModel(c.Attributes)
	if err := validateCommandAttrs(c.CommandType, attrs); err != nil {
		return nil, err
	}
	cfg := map[string]any{"commandType": c.CommandType}
	if len(attrs) > 0 {
		cfg["attributes"] = attrs
	}
	return cfg, nil
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
	in, err := notificationRuleInputFromOAS(req)
	if err != nil {
		return &oas.CreateNotificationBadRequest{Error: err.Error()}, nil
	}
	rule, err := h.cfg.NotificationRules.CreateForUser(ctx, user, in)
	if err != nil {
		return &oas.CreateNotificationBadRequest{Error: services.PublicMessage(err, "failed to create notification rule")}, nil
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
	in, err := notificationRuleInputFromOAS(req)
	if err != nil {
		return &oas.UpdateNotificationBadRequest{Error: err.Error()}, nil
	}
	rule, err := h.cfg.NotificationRules.UpdateForUser(ctx, user, params.ID, in)
	switch {
	case errors.Is(err, services.ErrNotFound):
		return &oas.UpdateNotificationNotFound{Error: err.Error()}, nil
	case errors.Is(err, services.ErrAccessDenied):
		return &oas.UpdateNotificationForbidden{Error: err.Error()}, nil
	case err != nil:
		return &oas.UpdateNotificationBadRequest{Error: services.PublicMessage(err, "failed to update notification rule")}, nil
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
	err := h.cfg.NotificationRules.DeleteForUser(ctx, user, params.ID)
	switch {
	case errors.Is(err, services.ErrNotFound):
		return &oas.DeleteNotificationNotFound{Error: err.Error()}, nil
	case err != nil:
		return &oas.DeleteNotificationForbidden{Error: services.PublicMessage(err, "failed to delete notification rule")}, nil
	}
	return &oas.DeleteNotificationNoContent{}, nil
}

// NotificationLogs returns recent delivery logs for a notification rule.
func (h *Handler) NotificationLogs(ctx context.Context, params oas.NotificationLogsParams) (oas.NotificationLogsRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.NotificationLogsUnauthorized{Error: "unauthorized"}, nil
	}
	if _, err := h.cfg.NotificationRules.GetForUser(ctx, user, params.ID); err != nil {
		if errors.Is(err, services.ErrAccessDenied) {
			return &oas.NotificationLogsForbidden{Error: err.Error()}, nil
		}
		return &oas.NotificationLogsNotFound{Error: err.Error()}, nil
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
	rule, err := h.cfg.NotificationRules.GetForUser(ctx, user, params.ID)
	if err != nil {
		if errors.Is(err, services.ErrAccessDenied) {
			return &oas.TestNotificationForbidden{Error: err.Error()}, nil
		}
		return &oas.TestNotificationNotFound{Error: err.Error()}, nil
	}
	if _, err := h.cfg.NotificationService.SendTestNotification(ctx, rule); err != nil {
		if errors.Is(err, services.ErrTestNotSupported) {
			return &oas.TestNotificationBadRequest{Error: err.Error()}, nil
		}
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

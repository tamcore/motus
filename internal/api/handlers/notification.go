package handlers

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/notification"
	"github.com/tamcore/motus/internal/services"
)

// validChannels lists the notification delivery channels.
var validChannels = map[string]bool{
	model.NotificationChannelWebhook: true,
	model.NotificationChannelCommand: true,
}

// notificationRuleFromInput validates a create/update request and returns the
// rule fields it describes. ID, UserID, timestamps and the geofence filter
// (see resolveRuleGeofenceIDs) are left to the caller. On invalid input it
// returns a client-facing error message. requireTemplate enforces a non-empty
// template for webhook rules (create only, matching the historical update
// behaviour).
func notificationRuleFromInput(req *oas.NotificationRuleInput, requireTemplate bool) (*model.NotificationRule, string) {
	if req.Name == "" {
		return nil, "name is required"
	}
	if len(req.EventTypes) == 0 {
		return nil, "at least one event type is required"
	}
	for _, et := range req.EventTypes {
		if !model.IsNotificationEventType(et) {
			return nil, fmt.Sprintf("invalid event type: %s", et)
		}
	}
	if !validChannels[req.Channel] {
		return nil, "invalid channel"
	}

	tmpl, _ := req.Template.Get()
	var cfg map[string]any
	switch req.Channel {
	case model.NotificationChannelCommand:
		cmdCfg, ok := req.Config.GetNotificationConfigCommand()
		if !ok {
			return nil, "config does not match channel command"
		}
		var err error
		if cfg, err = notificationCommandConfigToModel(cmdCfg); err != nil {
			return nil, err.Error()
		}
		if err := model.ValidateCommandEventTypes(cmdCfg.CommandType, req.EventTypes); err != nil {
			return nil, err.Error()
		}
		tmpl = "" // command rules have no message
	default:
		if _, ok := req.Config.GetNotificationConfigWebhook(); !ok {
			return nil, "config does not match channel " + req.Channel
		}
		if requireTemplate && tmpl == "" {
			return nil, "template is required"
		}
		var err error
		if cfg, err = oasNotificationConfigToModel(req.Config); err != nil {
			return nil, err.Error()
		}
	}

	enabled, _ := req.Enabled.Get()
	return &model.NotificationRule{
		Name:       req.Name,
		EventTypes: req.EventTypes,
		Channel:    req.Channel,
		Config:     cfg,
		Template:   tmpl,
		Enabled:    enabled,
	}, ""
}

// resolveRuleGeofenceIDs returns the normalized geofence filter (sorted,
// unique; empty = all geofences) for a create (existing == nil) or update.
//
// On update, an absent geofenceIds field (nil) keeps the stored filter, while
// an explicit [] clears it. IDs already stored in the rule are accepted as-is
// even if the geofence was deleted or is no longer accessible, so the rule
// stays editable; only newly added IDs must be accessible to the user.
func (h *Handler) resolveRuleGeofenceIDs(ctx context.Context, user *model.User, eventTypes []string, requested []int64, existing *model.NotificationRule) ([]int64, string) {
	ids := requested
	var stored []int64
	if existing != nil {
		stored = existing.GeofenceIDs
		if requested == nil {
			ids = stored
		}
	}
	if len(ids) == 0 {
		return []int64{}, ""
	}
	if !slices.ContainsFunc(eventTypes, model.IsGeofenceEventType) {
		if requested == nil {
			// The inherited filter has no effect without geofence events.
			return []int64{}, ""
		}
		return nil, "geofenceIds require a geofenceEnter or geofenceExit event type"
	}
	out := slices.Compact(slices.Sorted(slices.Values(ids)))
	for _, id := range out {
		if slices.Contains(stored, id) {
			continue
		}
		if id <= 0 || h.cfg.Geofences == nil || !h.cfg.Geofences.UserHasAccess(ctx, user, id) {
			return nil, fmt.Sprintf("geofence %d not found or access denied", id)
		}
	}
	return out, ""
}

// notificationCommandConfigToModel validates a command-channel config and
// converts it to the stored config map {commandType, attributes?}.
func notificationCommandConfigToModel(c oas.NotificationConfigCommand) (map[string]any, error) {
	if !slices.Contains(model.NotificationCommandTypes(), c.CommandType) {
		return nil, fmt.Errorf("invalid command type for notification rule: %q", c.CommandType)
	}
	if c.Attributes.Set && string(c.Attributes.Value.Type) != c.CommandType {
		return nil, fmt.Errorf("command attributes of type %s do not match command type %s",
			c.Attributes.Value.Type, c.CommandType)
	}
	attrs := oasCommandAttrsToModel(c.Attributes)
	switch c.CommandType {
	case model.CommandPositionPeriodic:
		if _, ok := attrs["frequency"].(int); !ok {
			return nil, fmt.Errorf("positionPeriodic requires a frequency attribute")
		}
	case model.CommandSosNumber:
		if p, _ := attrs["phoneNumber"].(string); p == "" {
			return nil, fmt.Errorf("sosNumber requires a phoneNumber attribute")
		}
	case model.CommandSetSpeedAlarm:
		if s, ok := attrs["speed"].(float64); !ok || s < 0 {
			return nil, fmt.Errorf("setSpeedAlarm requires a speed attribute >= 0")
		}
	case model.CommandCustom:
		if t, _ := attrs["text"].(string); t == "" {
			return nil, fmt.Errorf("custom commands require a non-empty 'text' attribute")
		}
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
	rule, msg := notificationRuleFromInput(req, true)
	if msg != "" {
		return &oas.CreateNotificationBadRequest{Error: msg}, nil
	}
	if rule.GeofenceIDs, msg = h.resolveRuleGeofenceIDs(ctx, user, req.EventTypes, req.GeofenceIds, nil); msg != "" {
		return &oas.CreateNotificationBadRequest{Error: msg}, nil
	}
	rule.UserID = user.ID
	if err := h.cfg.Notifications.Create(ctx, rule); err != nil {
		return &oas.CreateNotificationBadRequest{Error: "failed to create notification rule"}, nil
	}

	h.cfg.AuditLogger.Log(ctx, &user.ID,
		audit.ActionNotifCreate, audit.ResourceNotification, &rule.ID,
		map[string]any{"name": rule.Name, "eventTypes": rule.EventTypes, "channel": rule.Channel},
		"", "")
	out := notificationRuleToOAS(rule)
	return &out, nil
}

// UpdateNotification modifies an existing notification rule.
func (h *Handler) UpdateNotification(ctx context.Context, req *oas.NotificationRuleInput, params oas.UpdateNotificationParams) (oas.UpdateNotificationRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.UpdateNotificationUnauthorized{Error: "unauthorized"}, nil
	}
	rule, msg := notificationRuleFromInput(req, false)
	if msg != "" {
		return &oas.UpdateNotificationBadRequest{Error: msg}, nil
	}

	existing, err := h.cfg.Notifications.GetByID(ctx, params.ID)
	if err != nil {
		return &oas.UpdateNotificationNotFound{Error: "notification rule not found"}, nil
	}
	if !user.CanManage(existing.UserID) {
		return &oas.UpdateNotificationForbidden{Error: "access denied"}, nil
	}
	if rule.GeofenceIDs, msg = h.resolveRuleGeofenceIDs(ctx, user, req.EventTypes, req.GeofenceIds, existing); msg != "" {
		return &oas.UpdateNotificationBadRequest{Error: msg}, nil
	}

	rule.ID = params.ID
	rule.UserID = existing.UserID
	if err := h.cfg.Notifications.Update(ctx, rule); err != nil {
		return &oas.UpdateNotificationBadRequest{Error: "failed to update notification rule"}, nil
	}

	h.cfg.AuditLogger.Log(ctx, &user.ID,
		audit.ActionNotifUpdate, audit.ResourceNotification, &params.ID,
		map[string]any{"name": rule.Name, "eventTypes": rule.EventTypes, "channel": rule.Channel},
		"", "")
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
	if !user.CanManage(existing.UserID) {
		return &oas.DeleteNotificationForbidden{Error: "access denied"}, nil
	}
	if err := h.cfg.Notifications.Delete(ctx, params.ID); err != nil {
		return &oas.DeleteNotificationForbidden{Error: "failed to delete notification rule"}, nil
	}
	h.cfg.AuditLogger.Log(ctx, &user.ID,
		audit.ActionNotifDelete, audit.ResourceNotification, &params.ID,
		nil, "", "")
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
	if !user.CanManage(rule.UserID) {
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
	if !user.CanManage(rule.UserID) {
		return &oas.TestNotificationForbidden{Error: "access denied"}, nil
	}
	if _, err := h.cfg.NotificationService.SendTestNotification(ctx, rule); err != nil {
		if errors.Is(err, services.ErrTestNotSupported) {
			return &oas.TestNotificationBadRequest{Error: err.Error()}, nil
		}
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

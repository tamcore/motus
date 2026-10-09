package services

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/notification"
	"github.com/tamcore/motus/internal/storage/repository"
)

// NotificationRuleService bundles notification rule validation, ownership,
// geofence/device filter resolution and audit logging so the OAS handler and MCP
// tools share identical behaviour.
type NotificationRuleService struct {
	repo        repository.NotificationRepo
	geofences   repository.GeofenceRepo
	devices     repository.DeviceRepo
	auditLogger *audit.Logger
}

// NewNotificationRuleService returns a NotificationRuleService. auditLogger
// may be nil (audit entries are silently skipped).
func NewNotificationRuleService(repo repository.NotificationRepo, geofences repository.GeofenceRepo, devices repository.DeviceRepo, auditLogger *audit.Logger) *NotificationRuleService {
	return &NotificationRuleService{repo: repo, geofences: geofences, devices: devices, auditLogger: auditLogger}
}

// NotificationRuleInput holds all rule fields. Config is the stored channel
// config: {webhookUrl, headers?} or {commandType, attributes?}. On update a nil
// GeofenceIDs/DeviceIDs keeps the stored filter; an empty slice clears it.
type NotificationRuleInput struct {
	Name        string
	EventTypes  []string
	Channel     string
	Config      map[string]any
	Template    string
	Enabled     bool
	GeofenceIDs []int64
	DeviceIDs   []int64
}

// validateRuleInput checks in and returns the template to store. Webhook
// rules need a template on create only (historical update behaviour).
func validateRuleInput(in NotificationRuleInput, isCreate bool) (string, error) {
	if in.Name == "" {
		return "", errors.New("name is required")
	}
	if len(in.EventTypes) == 0 {
		return "", errors.New("at least one event type is required")
	}
	for _, et := range in.EventTypes {
		if !model.IsNotificationEventType(et) {
			return "", fmt.Errorf("invalid event type: %s", et)
		}
	}
	switch in.Channel {
	case model.NotificationChannelCommand:
		cmdType, _ := in.Config["commandType"].(string)
		if !slices.Contains(model.NotificationCommandTypes(), cmdType) {
			return "", fmt.Errorf("invalid command type for notification rule: %q", cmdType)
		}
		return "", model.ValidateCommandEventTypes(cmdType, in.EventTypes)
	case model.NotificationChannelWebhook:
		if isCreate && in.Template == "" {
			return "", errors.New("template is required")
		}
		webhookURL, _ := in.Config["webhookUrl"].(string)
		if err := notification.ValidateWebhookURL(webhookURL); err != nil {
			return "", fmt.Errorf("invalid webhook URL: %v", err)
		}
		return in.Template, nil
	default:
		return "", errors.New("invalid channel")
	}
}

// resolveIDFilter returns the normalized ID filter (sorted, unique; empty =
// all). On update a nil ids inherits stored. IDs already in stored are
// accepted as-is even if the resource was deleted or is no longer
// accessible, so the rule stays editable; only newly added IDs must pass
// hasAccess. kind names the resource in errors.
func resolveIDFilter(ids, stored []int64, kind string, hasAccess func(int64) bool) ([]int64, error) {
	if ids == nil {
		ids = stored
	}
	if len(ids) == 0 {
		return []int64{}, nil
	}
	out := slices.Compact(slices.Sorted(slices.Values(ids)))
	for _, id := range out {
		if slices.Contains(stored, id) {
			continue
		}
		if id <= 0 || !hasAccess(id) {
			return nil, fmt.Errorf("%s %d not found or access denied", kind, id)
		}
	}
	return out, nil
}

// resolveGeofenceIDs returns the geofence filter of in. It requires a
// geofence event type; an inherited filter is dropped without one.
func (s *NotificationRuleService) resolveGeofenceIDs(ctx context.Context, user *model.User, in NotificationRuleInput, existing *model.NotificationRule) ([]int64, error) {
	var stored []int64
	if existing != nil {
		stored = existing.GeofenceIDs
	}
	if !slices.ContainsFunc(in.EventTypes, model.IsGeofenceEventType) {
		if len(in.GeofenceIDs) > 0 {
			return nil, errors.New("geofenceIds require a geofenceEnter or geofenceExit event type")
		}
		// The inherited filter has no effect without geofence events.
		return []int64{}, nil
	}
	return resolveIDFilter(in.GeofenceIDs, stored, "geofence", func(id int64) bool {
		return s.geofences != nil && s.geofences.UserHasAccess(ctx, user, id)
	})
}

// resolveDeviceIDs returns the device filter of in (any event type).
func (s *NotificationRuleService) resolveDeviceIDs(ctx context.Context, user *model.User, in NotificationRuleInput, existing *model.NotificationRule) ([]int64, error) {
	var stored []int64
	if existing != nil {
		stored = existing.DeviceIDs
	}
	return resolveIDFilter(in.DeviceIDs, stored, "device", func(id int64) bool {
		return s.devices != nil && s.devices.UserHasAccess(ctx, user, id)
	})
}

// buildRule resolves the geofence and device filters of the validated in and
// returns the rule it describes.
func (s *NotificationRuleService) buildRule(ctx context.Context, user *model.User, in NotificationRuleInput, tmpl string, existing *model.NotificationRule) (*model.NotificationRule, error) {
	geofenceIDs, err := s.resolveGeofenceIDs(ctx, user, in, existing)
	if err != nil {
		return nil, invalid(err)
	}
	deviceIDs, err := s.resolveDeviceIDs(ctx, user, in, existing)
	if err != nil {
		return nil, invalid(err)
	}
	return &model.NotificationRule{
		Name:        in.Name,
		EventTypes:  in.EventTypes,
		Channel:     in.Channel,
		Config:      in.Config,
		Template:    tmpl,
		Enabled:     in.Enabled,
		GeofenceIDs: geofenceIDs,
		DeviceIDs:   deviceIDs,
	}, nil
}

// GetForUser returns a rule user may manage.
func (s *NotificationRuleService) GetForUser(ctx context.Context, user *model.User, ruleID int64) (*model.NotificationRule, error) {
	rule, err := s.repo.GetByID(ctx, ruleID)
	if err != nil || rule == nil {
		return nil, invalid(fmt.Errorf("notification rule %w", ErrNotFound))
	}
	if !user.CanManage(rule.UserID) {
		return nil, invalid(ErrAccessDenied)
	}
	return rule, nil
}

func (s *NotificationRuleService) logAudit(ctx context.Context, user *model.User, action string, ruleID int64, rule *model.NotificationRule) {
	var details map[string]any
	if rule != nil {
		details = map[string]any{"name": rule.Name, "eventTypes": rule.EventTypes, "channel": rule.Channel}
	}
	s.auditLogger.Log(ctx, &user.ID, action, audit.ResourceNotification, &ruleID, details)
}

// CreateForUser validates, persists, and audits a new rule owned by user.
func (s *NotificationRuleService) CreateForUser(ctx context.Context, user *model.User, in NotificationRuleInput) (*model.NotificationRule, error) {
	tmpl, err := validateRuleInput(in, true)
	if err != nil {
		return nil, invalid(err)
	}
	rule, err := s.buildRule(ctx, user, in, tmpl, nil)
	if err != nil {
		return nil, err
	}
	rule.UserID = user.ID
	if err := s.repo.Create(ctx, rule); err != nil {
		return nil, fmt.Errorf("create notification rule: %w", err)
	}
	s.logAudit(ctx, user, audit.ActionNotifCreate, rule.ID, rule)
	return rule, nil
}

// UpdateForUser replaces a rule user may manage and emits an audit entry.
func (s *NotificationRuleService) UpdateForUser(ctx context.Context, user *model.User, ruleID int64, in NotificationRuleInput) (*model.NotificationRule, error) {
	tmpl, err := validateRuleInput(in, false)
	if err != nil {
		return nil, invalid(err)
	}
	existing, err := s.GetForUser(ctx, user, ruleID)
	if err != nil {
		return nil, err
	}
	rule, err := s.buildRule(ctx, user, in, tmpl, existing)
	if err != nil {
		return nil, err
	}
	rule.ID = ruleID
	rule.UserID = existing.UserID
	if err := s.repo.Update(ctx, rule); err != nil {
		return nil, fmt.Errorf("update notification rule: %w", err)
	}
	s.logAudit(ctx, user, audit.ActionNotifUpdate, ruleID, rule)
	return rule, nil
}

// DeleteForUser deletes a rule user may manage and emits an audit entry.
func (s *NotificationRuleService) DeleteForUser(ctx context.Context, user *model.User, ruleID int64) error {
	if _, err := s.GetForUser(ctx, user, ruleID); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, ruleID); err != nil {
		return fmt.Errorf("delete notification rule: %w", err)
	}
	s.logAudit(ctx, user, audit.ActionNotifDelete, ruleID, nil)
	return nil
}

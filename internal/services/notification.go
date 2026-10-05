package services

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/metrics"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/notification"
	"github.com/tamcore/motus/internal/storage/repository"
)

// Notification log statuses.
const (
	notificationStatusSent   = "sent"
	notificationStatusQueued = "queued"
	notificationStatusFailed = "failed"
)

// ErrTestNotSupported is returned by SendTestNotification for channels that
// cannot be tested without side effects (sending a real device command).
var ErrTestNotSupported = errors.New("test notifications are not supported for command rules")

// CommandSubmitter queues a command for a device and delivers it immediately
// when the device is connected (implemented by protocol.CommandSubmitter, the
// same path POST /api/commands/send uses).
type CommandSubmitter interface {
	Submit(ctx context.Context, device *model.Device, cmdType string, attrs map[string]any) (*model.Command, error)
}

// NotificationService processes events and dispatches notifications
// to matching rules.
type NotificationService struct {
	notificationRepo repository.NotificationRepo
	deviceRepo       repository.DeviceRepo
	geofenceRepo     repository.GeofenceRepo
	positionRepo     repository.PositionRepo
	sender           *notification.Sender
	commands         CommandSubmitter
	commandQueue     *deviceQueue
	auditLogger      *audit.Logger
	logger           *slog.Logger
}

// NewNotificationService creates a new notification service. commands
// enables the "command" channel (without it, command rules log a failed
// delivery); auditLogger records commands sent by command rules. Both may be
// nil.
func NewNotificationService(
	notificationRepo repository.NotificationRepo,
	deviceRepo repository.DeviceRepo,
	geofenceRepo repository.GeofenceRepo,
	positionRepo repository.PositionRepo,
	commands CommandSubmitter,
	auditLogger *audit.Logger,
) *NotificationService {
	return &NotificationService{
		notificationRepo: notificationRepo,
		deviceRepo:       deviceRepo,
		geofenceRepo:     geofenceRepo,
		positionRepo:     positionRepo,
		sender:           notification.NewSender(),
		commands:         commands,
		commandQueue:     newDeviceQueue(),
		auditLogger:      auditLogger,
		logger:           slog.Default(),
	}
}

// ProcessEvent finds matching notification rules for the event and sends
// notifications asynchronously. It looks up the users who own the device
// and checks each user's enabled rules for the event type.
func (s *NotificationService) ProcessEvent(ctx context.Context, event *model.Event) error {
	device, err := s.deviceRepo.GetByID(ctx, event.DeviceID)
	if err != nil {
		return err
	}

	userIDs, err := s.deviceRepo.GetUserIDs(ctx, device.ID)
	if err != nil {
		return err
	}

	for _, userID := range userIDs {
		rules, err := s.notificationRepo.GetByEventType(ctx, userID, event.Type)
		if err != nil {
			s.logger.Error("failed to get notification rules",
				slog.Int64("userID", userID),
				slog.String("eventType", event.Type),
				slog.Any("error", err),
			)
			continue
		}

		for _, rule := range rules {
			// Apply the rule's geofence filter (empty = all geofences).
			if !rule.MatchesEvent(event) {
				continue
			}
			isCommand := rule.Channel == model.NotificationChannelCommand
			send := s.sendNotification
			if isCommand {
				send = s.sendCommand
			}
			// WithoutCancel inherits trace spans from the event context but is
			// not cancelled when ProcessEvent returns. The timeout context is
			// created when the job runs so its lifetime is scoped to the job.
			job := func() {
				ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
				defer cancel()
				send(ctx, rule, event, device)
			}
			if isCommand {
				// Device commands run one at a time per device, in event
				// order: an earlier command (e.g. "left home -> 20 s") must
				// never be submitted after a later one ("back home -> 300 s"),
				// or the device would keep the away interval at home.
				s.commandQueue.enqueue(device.ID, job)
				continue
			}
			// Webhooks are independent and run in their own goroutine to
			// avoid blocking the event processing pipeline.
			go job()
		}
	}

	return nil
}

// SendTestNotification sends a test notification for a rule using sample data.
func (s *NotificationService) SendTestNotification(ctx context.Context, rule *model.NotificationRule) (int, error) {
	if rule.Channel == model.NotificationChannelCommand {
		return 0, ErrTestNotSupported
	}
	templateCtx := &notification.TemplateContext{
		Device: &model.Device{
			ID:       0,
			Name:     "Test Device",
			UniqueID: "000000000",
			Status:   "online",
		},
		Event: &model.Event{
			ID:        0,
			Type:      rule.EventTypes[0],
			Timestamp: time.Now().UTC(),
		},
		Geofence: &model.Geofence{
			ID:   0,
			Name: "Test Geofence",
		},
		Position: &model.Position{
			Latitude:  52.520008,
			Longitude: 13.404954,
		},
	}

	return s.sender.Send(ctx, rule, templateCtx)
}

func (s *NotificationService) sendNotification(ctx context.Context, rule *model.NotificationRule, event *model.Event, device *model.Device) {
	templateCtx := &notification.TemplateContext{
		Device: device,
		Event:  event,
	}

	// Enrich context with geofence details if the event references one.
	if event.GeofenceID != nil {
		geofence, err := s.geofenceRepo.GetByID(ctx, *event.GeofenceID)
		if err == nil {
			templateCtx.Geofence = geofence
		}
	}

	// Enrich context with position details if the event references one.
	if event.PositionID != nil {
		position, err := s.positionRepo.GetByID(ctx, *event.PositionID)
		if err == nil {
			templateCtx.Position = position
		}
	}

	sentAt := time.Now().UTC()
	responseCode, err := s.sender.Send(ctx, rule, templateCtx)

	// Record the delivery attempt in the notification log.
	logEntry := &model.NotificationLog{
		RuleID:       rule.ID,
		EventID:      &event.ID,
		SentAt:       &sentAt,
		ResponseCode: responseCode,
	}

	if err != nil {
		logEntry.Status = notificationStatusFailed
		logEntry.Error = err.Error()
		s.logger.Error("notification failed",
			slog.String("ruleName", rule.Name),
			slog.Int64("eventID", event.ID),
			slog.Any("error", err),
		)
	} else {
		logEntry.Status = notificationStatusSent
		s.logger.Info("notification sent",
			slog.String("ruleName", rule.Name),
			slog.Int64("eventID", event.ID),
			slog.String("channel", rule.Channel),
		)
	}

	if logErr := s.notificationRepo.LogDelivery(ctx, logEntry); logErr != nil {
		s.logger.Error("failed to log notification delivery", slog.Any("error", logErr))
	}
}

// sendCommand executes a command-channel rule: it sends the configured
// command to the device that triggered the event. The rule owner has access
// to the device by construction (rules are looked up per device user). The
// command goes through the shared submit path, so offline devices receive it
// from the pending queue when they reconnect.
func (s *NotificationService) sendCommand(ctx context.Context, rule *model.NotificationRule, event *model.Event, device *model.Device) {
	sentAt := time.Now().UTC()
	logEntry := &model.NotificationLog{RuleID: rule.ID, EventID: &event.ID, SentAt: &sentAt}

	cmd, err := s.submitRuleCommand(ctx, rule, device)
	switch {
	case err != nil:
		logEntry.Status = notificationStatusFailed
		logEntry.Error = err.Error()
		metrics.NotificationsSent.WithLabelValues(rule.Channel, "error").Inc()
		s.logger.Error("notification command failed",
			slog.String("ruleName", rule.Name),
			slog.Int64("eventID", event.ID),
			slog.Int64("deviceID", device.ID),
			slog.Any("error", err),
		)
	default:
		logEntry.Status = notificationStatusQueued
		if cmd.Status == model.CommandStatusSent {
			logEntry.Status = notificationStatusSent
		}
		metrics.NotificationsSent.WithLabelValues(rule.Channel, "success").Inc()
		s.logger.Info("notification command submitted",
			slog.String("ruleName", rule.Name),
			slog.Int64("eventID", event.ID),
			slog.Int64("deviceID", device.ID),
			slog.String("commandType", cmd.Type),
			slog.String("commandStatus", cmd.Status),
		)
		ownerID := rule.UserID
		s.auditLogger.Log(ctx, &ownerID, audit.ActionCommandSend, audit.ResourceCommand, &cmd.ID,
			map[string]any{
				"commandType":        cmd.Type,
				"commandStatus":      cmd.Status,
				"deviceName":         device.Name,
				"notificationRuleId": rule.ID,
			})
	}

	if logErr := s.notificationRepo.LogDelivery(ctx, logEntry); logErr != nil {
		s.logger.Error("failed to log notification delivery", slog.Any("error", logErr))
	}
}

// submitRuleCommand builds the rule's command for device and submits it.
func (s *NotificationService) submitRuleCommand(ctx context.Context, rule *model.NotificationRule, device *model.Device) (*model.Command, error) {
	if s.commands == nil {
		return nil, errors.New("command delivery is not available")
	}
	cmd, err := rule.DeviceCommand(device.ID)
	if err != nil {
		return nil, err
	}
	return s.commands.Submit(ctx, device, cmd.Type, cmd.Attributes)
}

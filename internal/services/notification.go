package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/notification"
	"github.com/tamcore/motus/internal/storage/repository"
)

// NotificationService processes events and dispatches notifications
// to matching rules.
type NotificationService struct {
	notificationRepo repository.NotificationRepo
	deviceRepo       repository.DeviceRepo
	geofenceRepo     repository.GeofenceRepo
	positionRepo     repository.PositionRepo
	sender           *notification.Sender
	logger           *slog.Logger
	audit            *audit.Logger
}

// NewNotificationService creates a new notification service.
func NewNotificationService(
	notificationRepo repository.NotificationRepo,
	deviceRepo repository.DeviceRepo,
	geofenceRepo repository.GeofenceRepo,
	positionRepo repository.PositionRepo,
) *NotificationService {
	return &NotificationService{
		notificationRepo: notificationRepo,
		deviceRepo:       deviceRepo,
		geofenceRepo:     geofenceRepo,
		positionRepo:     positionRepo,
		sender:           notification.NewSender(),
		logger:           slog.Default(),
	}
}

// SetLogger configures the structured logger for this service.
func (s *NotificationService) SetLogger(l *slog.Logger) {
	if l != nil {
		s.logger = l
	}
}

// SetAuditLogger configures audit logging for notification delivery.
func (s *NotificationService) SetAuditLogger(l *audit.Logger) {
	s.audit = l
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
			// Send each notification in its own goroutine to avoid blocking
			// the event processing pipeline. The context and its cancel are
			// created inside the goroutine so the timer lifetime is scoped
			// to the goroutine, not the outer loop iteration.
			go func(r *model.NotificationRule, uid int64) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				s.sendNotification(ctx, r, event, device, uid)
			}(rule, userID)
		}
	}

	return nil
}

// SendTestNotification sends a test notification for a rule using sample data.
func (s *NotificationService) SendTestNotification(ctx context.Context, rule *model.NotificationRule) (int, error) {
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

func (s *NotificationService) sendNotification(ctx context.Context, rule *model.NotificationRule, event *model.Event, device *model.Device, userID int64) {
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
		logEntry.Status = "failed"
		logEntry.Error = err.Error()
		s.logger.Error("notification failed",
			slog.String("ruleName", rule.Name),
			slog.Int64("eventID", event.ID),
			slog.Any("error", err),
		)

		// Audit log the failed notification.
		if s.audit != nil {
			uid := userID
			s.audit.Log(ctx, &uid, audit.ActionNotifFailed, audit.ResourceNotification, &rule.ID,
				map[string]interface{}{
					"ruleName":     rule.Name,
					"eventType":    event.Type,
					"channel":      rule.Channel,
					"deviceId":     device.ID,
					"error":        err.Error(),
					"responseCode": responseCode,
				}, "", "")
		}
	} else {
		logEntry.Status = "sent"
		s.logger.Info("notification sent",
			slog.String("ruleName", rule.Name),
			slog.Int64("eventID", event.ID),
			slog.String("channel", rule.Channel),
		)

		// Audit log the successful notification.
		if s.audit != nil {
			uid := userID
			s.audit.Log(ctx, &uid, audit.ActionNotifSent, audit.ResourceNotification, &rule.ID,
				map[string]interface{}{
					"ruleName":     rule.Name,
					"eventType":    event.Type,
					"channel":      rule.Channel,
					"deviceId":     device.ID,
					"responseCode": responseCode,
				}, "", "")
		}
	}

	if logErr := s.notificationRepo.LogDelivery(ctx, logEntry); logErr != nil {
		s.logger.Error("failed to log notification delivery", slog.Any("error", logErr))
	}
}

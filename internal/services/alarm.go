package services

import (
	"context"
	"log/slog"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/websocket"
)

// AlarmService detects hardware alarm conditions reported in the H02 flags
// word (bits 0, 1, 2, 18, 19) and emits an "alarm" event for each position
// that carries a non-empty "alarm" attribute. Unlike transition-based services
// (ignition, motion), alarms fire on every position that reports an active
// alarm — there is no deduplication, matching Traccar's behaviour.
type AlarmService struct {
	eventRepo           repository.EventRepo
	hub                 *websocket.Hub
	notificationService *NotificationService
	logger              *slog.Logger
}

// NewAlarmService creates a new alarm detection service.
func NewAlarmService(
	eventRepo repository.EventRepo,
	hub *websocket.Hub,
	notificationService *NotificationService,
) *AlarmService {
	return &AlarmService{
		eventRepo:           eventRepo,
		hub:                 hub,
		notificationService: notificationService,
		logger:              slog.Default(),
	}
}

// SetLogger configures the structured logger for this service.
func (s *AlarmService) SetLogger(l *slog.Logger) {
	if l != nil {
		s.logger = l
	}
}

// CheckAlarm reads the "alarm" attribute written by the H02 decoder and emits
// an alarm event when one is present. Positions without an "alarm" attribute
// (no active alarm, or non-H02 protocols) are silently skipped.
func (s *AlarmService) CheckAlarm(ctx context.Context, position *model.Position) error {
	alarmType, ok := alarmFromAttributes(position.Attributes)
	if !ok {
		return nil
	}

	event := &model.Event{
		DeviceID:   position.DeviceID,
		Type:       "alarm",
		PositionID: &position.ID,
		Timestamp:  position.Timestamp,
		Attributes: map[string]interface{}{
			"alarm": alarmType,
		},
	}

	if err := s.eventRepo.Create(ctx, event); err != nil {
		return err
	}

	s.logger.Info("alarm event detected",
		slog.Int64("deviceID", position.DeviceID),
		slog.String("alarm", alarmType),
	)

	if s.hub != nil {
		s.hub.BroadcastEvent(event)
	}

	if s.notificationService != nil {
		if err := s.notificationService.ProcessEvent(ctx, event); err != nil {
			s.logger.Error("failed to process notifications for alarm event",
				slog.Int64("eventID", event.ID),
				slog.Any("error", err),
			)
		}
	}

	return nil
}

// alarmFromAttributes extracts the alarm type string from a position's
// attribute map. Returns ("", false) when absent or wrong type.
func alarmFromAttributes(attrs map[string]interface{}) (string, bool) {
	if attrs == nil {
		return "", false
	}
	v, exists := attrs["alarm"]
	if !exists {
		return "", false
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}

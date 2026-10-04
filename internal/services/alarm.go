package services

import (
	"context"
	"log/slog"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/websocket"
)

// AlarmService detects hardware alarm conditions reported in the H02 flags
// word (bits 0, 1, 2, 18, 19) and emits an "alarm" event for each position
// that carries a non-empty "alarm" attribute. Unlike transition-based services
// (ignition, motion), alarms fire on every position that reports an active
// alarm — there is no deduplication, matching Traccar's behaviour.
type AlarmService struct {
	eventEmitter
}

// NewAlarmService creates a new alarm detection service.
func NewAlarmService(
	eventRepo eventStore,
	hub *websocket.Hub,
	notificationService *NotificationService,
	logger *slog.Logger,
) *AlarmService {
	return &AlarmService{newEventEmitter(eventRepo, hub, notificationService, logger)}
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
		Attributes: map[string]any{
			"alarm": alarmType,
		},
	}
	return s.emit(ctx, event, "alarm event detected",
		slog.Int64("deviceID", position.DeviceID),
		slog.String("alarm", alarmType),
	)
}

// alarmFromAttributes extracts the alarm type string from a position's
// attribute map. Returns ("", false) when absent or wrong type.
func alarmFromAttributes(attrs map[string]any) (string, bool) {
	s, _ := attrs["alarm"].(string)
	return s, s != ""
}

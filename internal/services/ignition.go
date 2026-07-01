package services

import (
	"context"
	"log/slog"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/websocket"
)

// IgnitionService detects ACC/ignition state changes and emits ignitionOn /
// ignitionOff events. It tracks the ignition state on the device record itself
// (ignition_on + last_ignition_time columns) rather than comparing consecutive
// positions. This approach is immune to out-of-order and duplicate positions
// that the H02 tracker frequently sends.
type IgnitionService struct {
	deviceRepo          repository.DeviceRepo
	eventRepo           repository.EventRepo
	hub                 *websocket.Hub
	notificationService *NotificationService
	logger              *slog.Logger
}

// NewIgnitionService creates a new ignition detection service.
func NewIgnitionService(
	deviceRepo repository.DeviceRepo,
	eventRepo repository.EventRepo,
	hub *websocket.Hub,
	notificationService *NotificationService,
) *IgnitionService {
	return &IgnitionService{
		deviceRepo:          deviceRepo,
		eventRepo:           eventRepo,
		hub:                 hub,
		notificationService: notificationService,
		logger:              slog.Default(),
	}
}

// SetLogger configures the structured logger for this service.
func (s *IgnitionService) SetLogger(l *slog.Logger) {
	if l != nil {
		s.logger = l
	}
}

// CheckIgnition compares the ignition attribute of the current position with
// the device's tracked ignition state. An event is emitted only when the
// authoritative state changes. Positions older than the last state change are
// silently skipped to handle out-of-order arrivals.
func (s *IgnitionService) CheckIgnition(ctx context.Context, position *model.Position) error {
	// Only act on positions that carry explicit ignition data.
	currIgnition, ok := ignitionFromAttributes(position.Attributes)
	if !ok {
		return nil
	}

	device, err := s.deviceRepo.GetByID(ctx, position.DeviceID)
	if err != nil {
		return err
	}

	// Skip out-of-order positions: if the device already has a newer ignition
	// state change recorded, this position is stale.
	if device.LastIgnitionTime != nil && position.Timestamp.Before(*device.LastIgnitionTime) {
		return nil
	}

	// No state change — update last_ignition_time if the position is newer
	// (keeps the guard timestamp fresh) but don't fire an event.
	if currIgnition == device.IgnitionOn {
		if currIgnition && (device.LastIgnitionTime == nil || !position.Timestamp.Before(*device.LastIgnitionTime)) {
			_ = s.deviceRepo.UpdateIgnitionState(ctx, device.ID, true, position.Timestamp)
		}
		return nil
	}

	// State changed: update device and fire event.
	if err := s.deviceRepo.UpdateIgnitionState(ctx, device.ID, currIgnition, position.Timestamp); err != nil {
		return err
	}

	eventType := "ignitionOff"
	if currIgnition {
		eventType = "ignitionOn"
	}

	event := &model.Event{
		DeviceID:   position.DeviceID,
		Type:       eventType,
		PositionID: &position.ID,
		Timestamp:  position.Timestamp,
		Attributes: map[string]any{
			"ignition": currIgnition,
		},
	}

	if err := s.eventRepo.Create(ctx, event); err != nil {
		return err
	}

	s.logger.Info("ignition event detected",
		slog.Int64("deviceID", position.DeviceID),
		slog.String("event", eventType),
	)

	if s.hub != nil {
		s.hub.BroadcastEvent(event)
	}

	if s.notificationService != nil {
		if err := s.notificationService.ProcessEvent(ctx, event); err != nil {
			s.logger.Error("failed to process notifications for ignition event",
				slog.Int64("eventID", event.ID),
				slog.Any("error", err),
			)
		}
	}

	return nil
}

// ignitionFromAttributes extracts the ignition boolean from a position's
// attribute map. Returns (value, true) when the key is present, (false, false)
// when absent or of an unexpected type.
func ignitionFromAttributes(attrs map[string]any) (bool, bool) {
	if attrs == nil {
		return false, false
	}
	v, exists := attrs["ignition"]
	if !exists {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

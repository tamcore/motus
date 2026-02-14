package services

import (
	"context"
	"log/slog"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/websocket"
)

// IgnitionService detects ACC/ignition state changes and emits ignitionOn /
// ignitionOff events. It relies on the "ignition" boolean attribute written by
// the H02 protocol decoder (flags bit 10). Positions from protocols that do
// not set this attribute are silently ignored.
type IgnitionService struct {
	positionRepo        repository.PositionRepo
	eventRepo           repository.EventRepo
	hub                 *websocket.Hub
	notificationService *NotificationService
	logger              *slog.Logger
}

// NewIgnitionService creates a new ignition detection service.
func NewIgnitionService(
	positionRepo repository.PositionRepo,
	eventRepo repository.EventRepo,
	hub *websocket.Hub,
	notificationService *NotificationService,
) *IgnitionService {
	return &IgnitionService{
		positionRepo:        positionRepo,
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
// the previous one and emits an ignitionOn or ignitionOff event on a
// transition. Positions without an "ignition" attribute (non-H02 protocols)
// are skipped.
func (s *IgnitionService) CheckIgnition(ctx context.Context, position *model.Position) error {
	// Only act on positions that carry explicit ignition data.
	currIgnition, ok := ignitionFromAttributes(position.Attributes)
	if !ok {
		return nil
	}

	prev, err := s.positionRepo.GetPreviousByDevice(ctx, position.DeviceID, position.Timestamp)
	if err != nil || prev == nil {
		return nil // No previous position to compare; skip.
	}

	prevIgnition, ok := ignitionFromAttributes(prev.Attributes)
	if !ok {
		return nil // Previous position has no ignition data; can't determine transition.
	}

	if currIgnition == prevIgnition {
		return nil // No change.
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
		Attributes: map[string]interface{}{
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
func ignitionFromAttributes(attrs map[string]interface{}) (bool, bool) {
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

package services

import (
	"context"
	"log/slog"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/websocket"
)

// OverspeedService detects when a device exceeds its configured speed limit
// and creates overspeed events.
type OverspeedService struct {
	eventRepo           repository.EventRepo
	hub                 *websocket.Hub
	notificationService *NotificationService
	logger              *slog.Logger
}

// NewOverspeedService creates a new overspeed detection service.
func NewOverspeedService(
	eventRepo repository.EventRepo,
	hub *websocket.Hub,
	notificationService *NotificationService,
) *OverspeedService {
	return &OverspeedService{
		eventRepo:           eventRepo,
		hub:                 hub,
		notificationService: notificationService,
		logger:              slog.Default(),
	}
}

// SetLogger configures the structured logger for this service.
func (s *OverspeedService) SetLogger(l *slog.Logger) {
	if l != nil {
		s.logger = l
	}
}

// CheckOverspeed compares the position speed against the device speed limit.
// If the speed exceeds the limit, an overspeed event is created and
// notifications are triggered.
func (s *OverspeedService) CheckOverspeed(ctx context.Context, position *model.Position, device *model.Device) error {
	if device.SpeedLimit == nil || position.Speed == nil {
		return nil
	}

	if *position.Speed <= *device.SpeedLimit {
		return nil
	}

	event := &model.Event{
		DeviceID:   device.ID,
		Type:       "overspeed",
		PositionID: &position.ID,
		Timestamp:  position.Timestamp,
		Attributes: map[string]interface{}{
			"speed": *position.Speed,
			"limit": *device.SpeedLimit,
		},
	}

	if err := s.eventRepo.Create(ctx, event); err != nil {
		return err
	}

	s.logger.Warn("overspeed event detected",
		slog.Int64("deviceID", device.ID),
		slog.Float64("speed", *position.Speed),
		slog.Float64("limit", *device.SpeedLimit),
	)

	// Broadcast the event via WebSocket.
	if s.hub != nil {
		s.hub.BroadcastEvent(event)
	}

	// Trigger notifications for this event.
	if s.notificationService != nil {
		if err := s.notificationService.ProcessEvent(ctx, event); err != nil {
			s.logger.Error("failed to process notifications for overspeed event",
				slog.Int64("eventID", event.ID),
				slog.Any("error", err),
			)
		}
	}

	return nil
}

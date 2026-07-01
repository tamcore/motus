package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/websocket"
)

// MotionThreshold is the minimum speed in km/h to consider a device in motion.
const MotionThreshold = 5.0

// MotionDedupWindow suppresses repeat motion events when a device's reported
// speed oscillates around MotionThreshold during a single trip. Without this,
// a vehicle driving at ~5 km/h emits a motion event for every threshold
// crossing, flooding the notification webhook.
const MotionDedupWindow = 5 * time.Minute

// MotionService detects when a device transitions from stationary to moving
// and creates motion events.
type MotionService struct {
	positionRepo        repository.PositionRepo
	eventRepo           repository.EventRepo
	hub                 *websocket.Hub
	notificationService *NotificationService
	logger              *slog.Logger
}

// NewMotionService creates a new motion detection service.
func NewMotionService(
	positionRepo repository.PositionRepo,
	eventRepo repository.EventRepo,
	hub *websocket.Hub,
	notificationService *NotificationService,
) *MotionService {
	return &MotionService{
		positionRepo:        positionRepo,
		eventRepo:           eventRepo,
		hub:                 hub,
		notificationService: notificationService,
		logger:              slog.Default(),
	}
}

// SetLogger configures the structured logger for this service.
func (s *MotionService) SetLogger(l *slog.Logger) {
	if l != nil {
		s.logger = l
	}
}

// CheckMotion compares the current position speed with the previous position
// to detect when a device starts moving (crosses the motion threshold).
func (s *MotionService) CheckMotion(ctx context.Context, position *model.Position) error {
	// Get the previous position to compare speed.
	prev, err := s.positionRepo.GetPreviousByDevice(ctx, position.DeviceID, position.Timestamp)
	if err != nil || prev == nil {
		return nil // No previous position to compare; skip.
	}

	prevSpeed := 0.0
	if prev.Speed != nil {
		prevSpeed = *prev.Speed
	}

	currSpeed := 0.0
	if position.Speed != nil {
		currSpeed = *position.Speed
	}

	// Motion started: previous speed was below threshold, current speed meets or exceeds it.
	if prevSpeed < MotionThreshold && currSpeed >= MotionThreshold {
		// Suppress duplicates from speed oscillation around the threshold:
		// only fire a new motion event if no motion event has been recorded
		// for this device within MotionDedupWindow.
		recent, err := s.eventRepo.GetRecentByDeviceAndType(ctx, position.DeviceID, "motion", 1)
		if err == nil && len(recent) > 0 && position.Timestamp.Sub(recent[0].Timestamp) < MotionDedupWindow {
			return nil
		}

		event := &model.Event{
			DeviceID:   position.DeviceID,
			Type:       "motion",
			PositionID: &position.ID,
			Timestamp:  position.Timestamp,
			Attributes: map[string]any{
				"speed":         currSpeed,
				"previousSpeed": prevSpeed,
			},
		}

		if err := s.eventRepo.Create(ctx, event); err != nil {
			return err
		}

		s.logger.Info("motion event detected",
			slog.Int64("deviceID", position.DeviceID),
			slog.Float64("speed", currSpeed),
			slog.Float64("previousSpeed", prevSpeed),
		)

		// Broadcast the event via WebSocket.
		if s.hub != nil {
			s.hub.BroadcastEvent(event)
		}

		// Trigger notifications for this event.
		if s.notificationService != nil {
			if err := s.notificationService.ProcessEvent(ctx, event); err != nil {
				s.logger.Error("failed to process notifications for motion event",
					slog.Int64("eventID", event.ID),
					slog.Any("error", err),
				)
			}
		}
	}

	return nil
}

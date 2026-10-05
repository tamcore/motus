package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/websocket"
)

// EventDedupWindow suppresses repeat motion and geofence enter/exit events.
// A device's speed oscillating around model.MotionThreshold, or its position
// oscillating across a geofence boundary (GPS jitter, duplicate timestamps,
// interleaved H02 streams), would otherwise flood the notification webhook.
const EventDedupWindow = 5 * time.Minute

// MotionService detects when a device transitions from stationary to moving
// and creates motion events.
type MotionService struct {
	eventEmitter
	positionRepo repository.PositionRepo
}

// NewMotionService creates a new motion detection service.
func NewMotionService(
	positionRepo repository.PositionRepo,
	eventRepo eventStore,
	hub *websocket.Hub,
	notificationService *NotificationService,
	logger *slog.Logger,
) *MotionService {
	return &MotionService{
		eventEmitter: newEventEmitter(eventRepo, hub, notificationService, logger),
		positionRepo: positionRepo,
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

	prevSpeed := prev.SpeedOrZero()
	currSpeed := position.SpeedOrZero()

	// Motion started: previous speed was below threshold, current speed meets or exceeds it.
	if prevSpeed < model.MotionThreshold && currSpeed >= model.MotionThreshold {
		// Suppress duplicates from speed oscillation around the threshold:
		// only fire a new motion event if no motion event has been recorded
		// for this device within EventDedupWindow.
		recent, err := s.eventRepo.GetRecentByDeviceAndType(ctx, position.DeviceID, "motion", 1)
		if err == nil && len(recent) > 0 && position.Timestamp.Sub(recent[0].Timestamp) < EventDedupWindow {
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

		return s.emit(ctx, event, "motion event detected",
			slog.Int64("deviceID", position.DeviceID),
			slog.Float64("speed", currSpeed),
			slog.Float64("previousSpeed", prevSpeed),
		)
	}

	return nil
}

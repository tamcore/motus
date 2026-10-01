package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/tamcore/motus/internal/geo"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/websocket"
)

const (
	// MileageSpeedThreshold is the minimum speed in km/h for distance
	// accumulation. Positions below this are considered stationary.
	MileageSpeedThreshold = 5.0

	// MinStopDuration is how long a device must be stopped before a trip is
	// considered complete and pending mileage is committed.
	MinStopDuration = 5 * time.Minute

	// MaxReasonableDistanceKm caps the distance between two consecutive
	// positions to filter GPS jumps / teleportation artifacts.
	MaxReasonableDistanceKm = 50.0
)

// MileageService tracks device mileage by accumulating distance from GPS
// positions during motion and committing the total when a trip completes.
type MileageService struct {
	eventEmitter
	positionRepo mileagePositionStore
	deviceRepo   mileageDeviceStore
}

type mileagePositionStore interface {
	GetLatestByDevice(ctx context.Context, deviceID int64) (*model.Position, error)
	GetPreviousByDevice(ctx context.Context, deviceID int64, beforeTime time.Time) (*model.Position, error)
	GetLastMovingPosition(ctx context.Context, deviceID int64, speedThreshold float64) (*model.Position, error)
}

type mileageDeviceStore interface {
	Update(ctx context.Context, d *model.Device) error
}

// NewMileageService creates a new mileage tracking service.
func NewMileageService(
	positionRepo mileagePositionStore,
	deviceRepo mileageDeviceStore,
	eventRepo eventStore,
	hub *websocket.Hub,
	notificationService *NotificationService,
	logger *slog.Logger,
) *MileageService {
	return &MileageService{
		eventEmitter: newEventEmitter(eventRepo, hub, notificationService, logger),
		positionRepo: positionRepo,
		deviceRepo:   deviceRepo,
	}
}

// ProcessPosition handles mileage accumulation and trip completion for a
// single incoming position. Called from HandlePosition after the position
// is stored and the device is loaded.
func (s *MileageService) ProcessPosition(ctx context.Context, pos *model.Position, device *model.Device) error {
	if device.Mileage == nil {
		return nil
	}

	currSpeed := 0.0
	if pos.Speed != nil {
		currSpeed = *pos.Speed
	}

	prev, err := s.positionRepo.GetPreviousByDevice(ctx, pos.DeviceID, pos.Timestamp)
	if err != nil || prev == nil {
		return nil // No previous position; nothing to accumulate.
	}

	// Accumulate distance when moving.
	if currSpeed >= MileageSpeedThreshold {
		dist := geo.HaversineDistance(
			prev.Latitude, prev.Longitude,
			pos.Latitude, pos.Longitude,
		)
		if dist < MaxReasonableDistanceKm && dist > 0.001 {
			device.PendingMileage += dist
			// Persist the accumulated pending mileage. HandlePosition reloads
			// the device from the DB on every incoming position, so without
			// this write the in-memory accumulator would be reset to zero
			// before the next frame and trip completion would never fire.
			if err := s.deviceRepo.Update(ctx, device); err != nil {
				s.logger.Error("failed to persist pending mileage",
					slog.Int64("deviceID", device.ID),
					slog.Any("error", err),
				)
			}
		}
		return nil // Still moving; don't check for trip completion.
	}

	// Device is stopped. If there's pending mileage, check if the stop
	// has been sustained long enough to commit.
	if device.PendingMileage <= 0 {
		return nil
	}

	lastMoving, err := s.positionRepo.GetLastMovingPosition(ctx, pos.DeviceID, MileageSpeedThreshold)
	if err != nil || lastMoving == nil {
		return nil
	}

	stopDuration := pos.Timestamp.Sub(lastMoving.Timestamp)
	if stopDuration < MinStopDuration {
		return nil // Brief stop (traffic light); don't commit yet.
	}

	return s.commitMileage(ctx, pos, device)
}

// CommitPendingMileage is called by the periodic fallback (IdleService) for
// devices that stopped sending positions after parking.
func (s *MileageService) CommitPendingMileage(ctx context.Context, device *model.Device) error {
	if device.Mileage == nil || device.PendingMileage <= 0 {
		return nil
	}

	pos, err := s.positionRepo.GetLatestByDevice(ctx, device.ID)
	if err != nil || pos == nil {
		return nil
	}

	return s.commitMileage(ctx, pos, device)
}

func (s *MileageService) commitMileage(ctx context.Context, pos *model.Position, device *model.Device) error {
	tripDistance := device.PendingMileage
	*device.Mileage += tripDistance
	device.PendingMileage = 0

	if err := s.deviceRepo.Update(ctx, device); err != nil {
		return err
	}

	event := &model.Event{
		DeviceID:   device.ID,
		Type:       "tripCompleted",
		PositionID: &pos.ID,
		Timestamp:  pos.Timestamp,
		Attributes: map[string]any{
			"distance": tripDistance,
			"mileage":  *device.Mileage,
		},
	}

	err := s.emit(ctx, event, "trip completed",
		slog.Int64("deviceID", device.ID),
		slog.Float64("distanceKm", tripDistance),
		slog.Float64("mileageKm", *device.Mileage),
	)
	if err != nil {
		s.logger.Error("failed to create tripCompleted event",
			slog.Int64("deviceID", device.ID),
			slog.Any("error", err),
		)
	}
	return nil
}

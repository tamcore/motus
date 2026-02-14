package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/tamcore/motus/internal/geo"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
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
	positionRepo        repository.PositionRepo
	deviceRepo          repository.DeviceRepo
	eventRepo           repository.EventRepo
	hub                 *websocket.Hub
	notificationService *NotificationService
	logger              *slog.Logger
}

// NewMileageService creates a new mileage tracking service.
func NewMileageService(
	positionRepo repository.PositionRepo,
	deviceRepo repository.DeviceRepo,
	eventRepo repository.EventRepo,
	hub *websocket.Hub,
	notificationService *NotificationService,
) *MileageService {
	return &MileageService{
		positionRepo:        positionRepo,
		deviceRepo:          deviceRepo,
		eventRepo:           eventRepo,
		hub:                 hub,
		notificationService: notificationService,
		logger:              slog.Default(),
	}
}

// SetLogger configures the structured logger for this service.
func (s *MileageService) SetLogger(l *slog.Logger) {
	if l != nil {
		s.logger = l
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
		Attributes: map[string]interface{}{
			"distance": tripDistance,
			"mileage":  *device.Mileage,
		},
	}

	if err := s.eventRepo.Create(ctx, event); err != nil {
		s.logger.Error("failed to create tripCompleted event",
			slog.Int64("deviceID", device.ID),
			slog.Any("error", err),
		)
		return nil // Non-fatal: mileage is already committed.
	}

	s.logger.Info("trip completed",
		slog.Int64("deviceID", device.ID),
		slog.Float64("distanceKm", tripDistance),
		slog.Float64("mileageKm", *device.Mileage),
	)

	if s.hub != nil {
		s.hub.BroadcastEvent(event)
	}

	if s.notificationService != nil {
		if err := s.notificationService.ProcessEvent(ctx, event); err != nil {
			s.logger.Error("failed to process notifications for trip event",
				slog.Int64("eventID", event.ID),
				slog.Any("error", err),
			)
		}
	}

	return nil
}

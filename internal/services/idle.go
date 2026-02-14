package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/websocket"
)

const (
	// IdleThreshold is how long a device must be stationary before an idle event is created.
	IdleThreshold = 30 * time.Minute

	// IdleSpeedThreshold is the maximum speed in km/h to consider a device stationary.
	IdleSpeedThreshold = 1.0

	// IdleCheckInterval is how often the background service checks for idle devices.
	IdleCheckInterval = 5 * time.Minute
)

// AddressGeocoder provides reverse geocoding for idle/stopped positions.
// The returned address is stored in the position's address field in the database.
type AddressGeocoder interface {
	Lookup(ctx context.Context, lat, lon float64) string
}

// AddressUpdater persists a geocoded address on a stored position.
type AddressUpdater interface {
	UpdateAddress(ctx context.Context, positionID int64, address string) error
}

// IdleService detects devices that have been stationary for longer than
// the idle threshold and creates deviceIdle events. It runs as a background
// service, polling at a configured interval.
type IdleService struct {
	deviceRepo          repository.DeviceRepo
	positionRepo        repository.PositionRepo
	eventRepo           repository.EventRepo
	hub                 *websocket.Hub
	notificationService *NotificationService
	mileageService      *MileageService
	geocoder            AddressGeocoder
	addressUpdater      AddressUpdater
	logger              *slog.Logger
}

// NewIdleService creates a new idle detection service.
func NewIdleService(
	deviceRepo repository.DeviceRepo,
	positionRepo repository.PositionRepo,
	eventRepo repository.EventRepo,
	hub *websocket.Hub,
	notificationService *NotificationService,
) *IdleService {
	return &IdleService{
		deviceRepo:          deviceRepo,
		positionRepo:        positionRepo,
		eventRepo:           eventRepo,
		hub:                 hub,
		notificationService: notificationService,
		logger:              slog.Default(),
	}
}

// SetLogger configures the structured logger for this service.
func (s *IdleService) SetLogger(l *slog.Logger) {
	if l != nil {
		s.logger = l
	}
}

// SetGeocoder configures reverse geocoding for idle positions. When set, the
// service will geocode the stop location and persist the address in the
// position's address field in the database.
func (s *IdleService) SetGeocoder(geocoder AddressGeocoder, updater AddressUpdater) {
	s.geocoder = geocoder
	s.addressUpdater = updater
}

// SetMileageService configures the mileage tracker so the idle service can
// commit pending mileage for devices that stop sending positions after parking.
func (s *IdleService) SetMileageService(ms *MileageService) {
	s.mileageService = ms
}

// Start begins the idle detection loop. It blocks until the context is cancelled.
func (s *IdleService) Start(ctx context.Context) {
	ticker := time.NewTicker(IdleCheckInterval)
	defer ticker.Stop()

	s.logger.Info("idle detection service started",
		slog.String("threshold", IdleThreshold.String()),
		slog.String("checkInterval", IdleCheckInterval.String()),
	)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("idle detection service stopped")
			return
		case <-ticker.C:
			if err := s.CheckIdle(ctx); err != nil {
				s.logger.Error("error checking idle devices", slog.Any("error", err))
			}
		}
	}
}

// CheckIdle scans all devices and creates deviceIdle events for any device
// whose latest position is below the speed threshold and older than the
// idle threshold. It deduplicates by checking if an idle event was already
// created recently for the same idle period.
func (s *IdleService) CheckIdle(ctx context.Context) error {
	devices, err := s.deviceRepo.GetAll(ctx)
	if err != nil {
		return err
	}

	for i := range devices {
		device := &devices[i]

		// Get latest position for this device.
		position, err := s.positionRepo.GetLatestByDevice(ctx, device.ID)
		if err != nil || position == nil {
			continue
		}

		// Check if the device is stationary.
		speed := 0.0
		if position.Speed != nil {
			speed = *position.Speed
		}

		if speed >= IdleSpeedThreshold {
			continue // Device is moving; not idle.
		}

		timeSincePosition := time.Since(position.Timestamp)

		// Fallback mileage commit: if the device has pending mileage and has
		// been stopped long enough (MinStopDuration from mileage service),
		// commit the trip distance even if the device stopped sending positions.
		if s.mileageService != nil && device.PendingMileage > 0 && timeSincePosition > MinStopDuration {
			if err := s.mileageService.CommitPendingMileage(ctx, device); err != nil {
				s.logger.Error("failed to commit pending mileage",
					slog.Int64("deviceID", device.ID),
					slog.Any("error", err),
				)
			}
		}

		if timeSincePosition <= IdleThreshold {
			continue // Not idle long enough.
		}

		// Deduplicate: check if we already created an idle event for this period.
		recentEvents, err := s.eventRepo.GetRecentByDeviceAndType(ctx, device.ID, "deviceIdle", 1)
		if err == nil && len(recentEvents) > 0 {
			lastIdleEvent := recentEvents[0]
			if time.Since(lastIdleEvent.Timestamp) < IdleThreshold {
				continue // Already created event for this idle period.
			}
		}

		event := &model.Event{
			DeviceID:   device.ID,
			Type:       "deviceIdle",
			PositionID: &position.ID,
			Timestamp:  time.Now().UTC(),
			Attributes: map[string]interface{}{
				"idleDuration": timeSincePosition.Minutes(),
			},
		}

		if err := s.eventRepo.Create(ctx, event); err != nil {
			s.logger.Error("failed to create idle event",
				slog.Int64("deviceID", device.ID),
				slog.Any("error", err),
			)
			continue
		}

		s.logger.Info("idle event detected",
			slog.Int64("deviceID", device.ID),
			slog.Float64("idleDurationMin", timeSincePosition.Minutes()),
		)

		// Geocode the stop location and store the address on the position.
		// This only runs when the idle event is first created (not on
		// subsequent checks), so the geocoder rate limit is respected.
		if s.geocoder != nil && s.addressUpdater != nil && position.Address == nil {
			addr := s.geocoder.Lookup(ctx, position.Latitude, position.Longitude)
			if err := s.addressUpdater.UpdateAddress(ctx, position.ID, addr); err != nil {
				s.logger.Error("failed to store geocoded address",
					slog.Int64("positionID", position.ID),
					slog.Any("error", err),
				)
			} else {
				s.logger.Debug("stored geocoded address for idle position",
					slog.Int64("positionID", position.ID),
					slog.String("address", addr),
				)
			}
		}

		// Broadcast the event via WebSocket.
		if s.hub != nil {
			s.hub.BroadcastEvent(event)
		}

		// Trigger notifications for this event.
		if s.notificationService != nil {
			if err := s.notificationService.ProcessEvent(ctx, event); err != nil {
				s.logger.Error("failed to process notifications for idle event",
					slog.Int64("eventID", event.ID),
					slog.Any("error", err),
				)
			}
		}
	}

	return nil
}

package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/tamcore/motus/internal/geocoding"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/ticker"
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

// IdleService detects devices that have been stationary for longer than
// the idle threshold and creates deviceIdle events. It runs as a background
// service, polling at a configured interval.
type IdleService struct {
	eventEmitter
	deviceRepo     repository.DeviceRepo
	positionRepo   repository.PositionRepo
	mileageService *MileageService
	geocoder       *geocoding.CachedGeocoder
}

// NewIdleService creates a new idle detection service.
func NewIdleService(
	deviceRepo repository.DeviceRepo,
	positionRepo repository.PositionRepo,
	eventRepo eventStore,
	hub *websocket.Hub,
	notificationService *NotificationService,
	mileageService *MileageService,
	logger *slog.Logger,
) *IdleService {
	return &IdleService{
		eventEmitter:   newEventEmitter(eventRepo, hub, notificationService, logger),
		deviceRepo:     deviceRepo,
		positionRepo:   positionRepo,
		mileageService: mileageService,
	}
}

// SetGeocoder configures reverse geocoding for idle positions. When set, the
// service will geocode the stop location and persist the address in the
// position's address field in the database.
func (s *IdleService) SetGeocoder(geocoder *geocoding.CachedGeocoder) {
	s.geocoder = geocoder
}

// Start begins the idle detection loop. It blocks until the context is cancelled.
func (s *IdleService) Start(ctx context.Context) {
	s.logger.Info("idle detection service started",
		slog.String("threshold", IdleThreshold.String()),
		slog.String("checkInterval", IdleCheckInterval.String()),
	)
	ticker.Every(ctx, IdleCheckInterval, func() {
		if err := s.CheckIdle(ctx); err != nil {
			s.logger.Error("error checking idle devices", slog.Any("error", err))
		}
	})
	s.logger.Info("idle detection service stopped")
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
		if position.SpeedOrZero() >= IdleSpeedThreshold {
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

		// Deduplicate: only emit a new deviceIdle event if the device has sent
		// a fresh position since the last idle event. Compare position IDs
		// rather than timestamps because a CheckIdle run creates the event
		// with Timestamp=time.Now() while the latest stored position keeps
		// its (older) GPS fix time — a timestamp comparison would block all
		// subsequent emissions until a position with a future timestamp
		// arrives.
		recentEvents, err := s.eventRepo.GetRecentByDeviceAndType(ctx, device.ID, "deviceIdle", 1)
		if err == nil && len(recentEvents) > 0 {
			lastIdleEvent := recentEvents[0]
			if lastIdleEvent.PositionID != nil && *lastIdleEvent.PositionID == position.ID {
				continue // Same latest position as last idle event — no new movement.
			}
		}

		event := &model.Event{
			DeviceID:   device.ID,
			Type:       "deviceIdle",
			PositionID: &position.ID,
			Timestamp:  time.Now().UTC(),
			Attributes: map[string]any{
				"idleDuration": timeSincePosition.Minutes(),
			},
		}

		// Geocode the stop location and store the address on the position.
		// This only runs for a new idle event (not on subsequent checks), so
		// the geocoder rate limit is respected.
		if s.geocoder != nil && position.Address == nil {
			addr := s.geocoder.Lookup(ctx, position.Latitude, position.Longitude)
			if err := s.positionRepo.UpdateAddress(ctx, position.ID, addr); err != nil {
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

		err = s.emit(ctx, event, "idle event detected",
			slog.Int64("deviceID", device.ID),
			slog.Float64("idleDurationMin", timeSincePosition.Minutes()),
		)
		if err != nil {
			s.logger.Error("failed to create idle event",
				slog.Int64("deviceID", device.ID),
				slog.Any("error", err),
			)
		}
	}

	return nil
}

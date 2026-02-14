package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/websocket"
)

// DeviceTimeoutService monitors devices and marks them offline after inactivity.
type DeviceTimeoutService struct {
	deviceRepo repository.DeviceRepo
	hub        *websocket.Hub
	timeout    time.Duration
	interval   time.Duration
	logger     *slog.Logger
}

// NewDeviceTimeoutService creates a new device timeout service.
// timeout is how long a device can be silent before being marked offline.
// interval is how often the service checks for timed-out devices.
func NewDeviceTimeoutService(
	deviceRepo repository.DeviceRepo,
	hub *websocket.Hub,
	timeout, interval time.Duration,
) *DeviceTimeoutService {
	return &DeviceTimeoutService{
		deviceRepo: deviceRepo,
		hub:        hub,
		timeout:    timeout,
		interval:   interval,
		logger:     slog.Default(),
	}
}

// SetLogger configures the structured logger for this service.
func (s *DeviceTimeoutService) SetLogger(l *slog.Logger) {
	if l != nil {
		s.logger = l
	}
}

// Start begins monitoring devices for timeouts. It blocks until the
// context is cancelled.
func (s *DeviceTimeoutService) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.logger.Info("device timeout service started",
		slog.String("timeout", s.timeout.String()),
		slog.String("checkInterval", s.interval.String()),
	)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("device timeout service stopped")
			return
		case <-ticker.C:
			if err := s.checkTimeouts(ctx); err != nil {
				s.logger.Error("error checking device timeouts", slog.Any("error", err))
			}
		}
	}
}

func (s *DeviceTimeoutService) checkTimeouts(ctx context.Context) error {
	cutoff := time.Now().UTC().Add(-s.timeout)

	// Only fetch devices that are online/moving and past the timeout cutoff,
	// avoiding loading every device into memory.
	devices, err := s.deviceRepo.GetTimedOut(ctx, cutoff)
	if err != nil {
		return err
	}

	updated := 0

	for i := range devices {
		device := &devices[i]
		device.Status = "offline"
		if err := s.deviceRepo.Update(ctx, device); err != nil {
			s.logger.Error("failed to mark device offline",
				slog.String("uniqueID", device.UniqueID),
				slog.Int64("deviceID", device.ID),
				slog.Any("error", err),
			)
			continue
		}
		if device.LastUpdate != nil {
			s.logger.Info("device marked offline",
				slog.String("uniqueID", device.UniqueID),
				slog.String("lastSeen", device.LastUpdate.Format(time.RFC3339)),
			)
		} else {
			s.logger.Info("device marked offline",
				slog.String("uniqueID", device.UniqueID),
				slog.String("lastSeen", "never"),
			)
		}
		// Broadcast the status change via WebSocket.
		s.hub.BroadcastDeviceStatus(device)
		updated++
	}

	if updated > 0 {
		s.logger.Info("marked devices offline due to timeout",
			slog.Int("count", updated),
		)
	}

	return nil
}

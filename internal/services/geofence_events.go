package services

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/tamcore/motus/internal/calendar"
	"github.com/tamcore/motus/internal/metrics"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/websocket"
)

// GeofenceEventService detects geofence enter/exit events by comparing
// a new position against the previous one for the same device.
type GeofenceEventService struct {
	eventEmitter
	geofenceRepo repository.GeofenceRepo
	positionRepo repository.PositionRepo
	calendarRepo repository.CalendarRepo
	// now returns the current time. Defaults to time.Now; injectable for testing.
	now func() time.Time
}

// NewGeofenceEventService creates a new geofence event detection service.
func NewGeofenceEventService(
	geofenceRepo repository.GeofenceRepo,
	eventRepo eventStore,
	positionRepo repository.PositionRepo,
	calendarRepo repository.CalendarRepo,
	hub *websocket.Hub,
	notificationService *NotificationService,
	logger *slog.Logger,
) *GeofenceEventService {
	return &GeofenceEventService{
		eventEmitter: newEventEmitter(eventRepo, hub, notificationService, logger),
		geofenceRepo: geofenceRepo,
		positionRepo: positionRepo,
		calendarRepo: calendarRepo,
		now:          time.Now,
	}
}

// CheckGeofences determines whether the given position triggers any
// geofence enter or exit events. It compares the current position's
// geofence containment against the previous position's containment,
// using a device-scoped union so that shared devices emit one event row
// per physical transition regardless of how many users own the device.
func (s *GeofenceEventService) CheckGeofences(ctx context.Context, position *model.Position) error {
	currentGeofences, err := s.geofenceRepo.CheckContainmentForDevice(ctx, position.DeviceID, position.Latitude, position.Longitude)
	if err != nil {
		return err
	}

	// Expose current geofence membership for Home Assistant / Traccar clients.
	position.GeofenceIDs = currentGeofences

	prevPosition, err := s.positionRepo.GetPreviousByDevice(ctx, position.DeviceID, position.Timestamp)
	if err != nil {
		return err
	}
	if prevPosition == nil {
		// Genuinely first position for this device: emit enters for all containing
		// geofences. The dedup window in createEvent suppresses repeats caused by
		// duplicate timestamps falling into this branch.
		for _, gid := range currentGeofences {
			s.createEvent(ctx, position, gid, "geofenceEnter")
		}
		return nil
	}

	// Use the geofence membership stored on the previous position (written by the
	// protocol handler via UpdateGeofenceIDs). This avoids re-evaluating the prior
	// location against the current (possibly edited) polygon, which would produce
	// spurious enter/exit events after a geofence shape change.
	// Fall back to live recomputation only when no stored membership exists
	// (e.g. very first position after a new deployment, or test helpers that
	// skip the UpdateGeofenceIDs step).
	var prevGeofences []int64
	if len(prevPosition.GeofenceIDs) > 0 {
		prevGeofences = prevPosition.GeofenceIDs
	} else {
		prevGeofences, err = s.geofenceRepo.CheckContainmentForDevice(ctx, position.DeviceID, prevPosition.Latitude, prevPosition.Longitude)
		if err != nil {
			return err
		}
	}

	// Attached geofences limit evaluation to themselves (opt-in filter). The
	// stored membership may predate the attachment, so drop geofences that are
	// no longer evaluated instead of reporting a spurious exit for them.
	if len(prevGeofences) > 0 {
		attached, err := s.geofenceRepo.GetDeviceGeofenceIDs(ctx, position.DeviceID)
		if err != nil {
			return err
		}
		if len(attached) > 0 {
			prevGeofences = slices.DeleteFunc(slices.Clone(prevGeofences), func(gid int64) bool {
				return !slices.Contains(attached, gid)
			})
		}
	}

	for _, gid := range currentGeofences {
		if !slices.Contains(prevGeofences, gid) {
			s.createEvent(ctx, position, gid, "geofenceEnter")
		}
	}

	for _, gid := range prevGeofences {
		if !slices.Contains(currentGeofences, gid) {
			s.createEvent(ctx, position, gid, "geofenceExit")
		}
	}

	return nil
}

func (s *GeofenceEventService) createEvent(ctx context.Context, position *model.Position, geofenceID int64, eventType string) {
	// Suppress duplicate transitions within the dedup window (GPS jitter,
	// duplicate timestamps, or repeated first-position enters).
	recent, err := s.eventRepo.GetRecentByDeviceAndType(ctx, position.DeviceID, eventType, 10)
	if err == nil {
		for _, e := range recent {
			if e.GeofenceID != nil && *e.GeofenceID == geofenceID &&
				position.Timestamp.Sub(e.Timestamp) < EventDedupWindow {
				return
			}
		}
	}

	// Check if this geofence has a calendar restriction.
	if !s.isGeofenceActiveNow(ctx, geofenceID) {
		s.logger.Debug("geofence event suppressed by calendar",
			slog.Int64("geofenceID", geofenceID),
			slog.String("eventType", eventType),
			slog.Int64("deviceID", position.DeviceID),
		)
		return
	}

	event := &model.Event{
		DeviceID:   position.DeviceID,
		GeofenceID: &geofenceID,
		Type:       eventType,
		PositionID: &position.ID,
		Timestamp:  position.Timestamp,
	}

	err = s.emit(ctx, event, "geofence event detected",
		slog.Int64("deviceID", position.DeviceID),
		slog.Int64("geofenceID", geofenceID),
		slog.String("eventType", eventType),
	)
	if err != nil {
		s.logger.Error("failed to create geofence event",
			slog.String("eventType", eventType),
			slog.Int64("deviceID", position.DeviceID),
			slog.Int64("geofenceID", geofenceID),
			slog.Any("error", err),
		)
		return
	}
	metrics.GeofenceEvents.WithLabelValues(eventType).Inc()
}

// isGeofenceActiveNow checks whether a geofence should trigger events at the
// current time. If the geofence has no calendar_id, it is always active.
// If it has a calendar_id and the calendar is active now, it returns true.
// If the calendar is not active or cannot be loaded, it returns false.
func (s *GeofenceEventService) isGeofenceActiveNow(ctx context.Context, geofenceID int64) bool {
	geofence, err := s.geofenceRepo.GetByID(ctx, geofenceID)
	if err != nil {
		s.logger.Error("failed to get geofence for calendar check",
			slog.Int64("geofenceID", geofenceID),
			slog.Any("error", err),
		)
		return true // Fail open: allow event on error.
	}

	// No calendar restriction; geofence is always active.
	if geofence.CalendarID == nil {
		return true
	}

	cal, err := s.calendarRepo.GetByID(ctx, *geofence.CalendarID)
	if err != nil {
		s.logger.Error("failed to load calendar for geofence",
			slog.Int64("geofenceID", geofenceID),
			slog.Int64("calendarID", *geofence.CalendarID),
			slog.Any("error", err),
		)
		return true // Fail open: allow event on error.
	}

	active, err := calendar.IsActiveAt(cal.Data, s.now().UTC())
	if err != nil {
		s.logger.Error("failed to check calendar schedule",
			slog.Int64("calendarID", cal.ID),
			slog.Any("error", err),
		)
		return true // Fail open on parse errors.
	}

	return active
}

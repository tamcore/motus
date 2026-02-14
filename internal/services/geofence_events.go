package services

import (
	"context"
	"log/slog"
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
	geofenceRepo        repository.GeofenceRepo
	eventRepo           repository.EventRepo
	deviceRepo          repository.DeviceRepo
	positionRepo        repository.PositionRepo
	calendarRepo        repository.CalendarRepo
	hub                 *websocket.Hub
	notificationService *NotificationService
	logger              *slog.Logger
	// now returns the current time. Defaults to time.Now; injectable for testing.
	now func() time.Time
}

// NewGeofenceEventService creates a new geofence event detection service.
func NewGeofenceEventService(
	geofenceRepo repository.GeofenceRepo,
	eventRepo repository.EventRepo,
	deviceRepo repository.DeviceRepo,
	positionRepo repository.PositionRepo,
	hub *websocket.Hub,
	notificationService *NotificationService,
) *GeofenceEventService {
	return &GeofenceEventService{
		geofenceRepo:        geofenceRepo,
		eventRepo:           eventRepo,
		deviceRepo:          deviceRepo,
		positionRepo:        positionRepo,
		hub:                 hub,
		notificationService: notificationService,
		logger:              slog.Default(),
		now:                 time.Now,
	}
}

// SetLogger configures the structured logger for this service.
func (s *GeofenceEventService) SetLogger(l *slog.Logger) {
	if l != nil {
		s.logger = l
	}
}

// SetCalendarRepo sets the calendar repository for time-based geofence filtering.
// When set, geofences with a calendar_id will only trigger events when the
// current time matches the calendar schedule.
func (s *GeofenceEventService) SetCalendarRepo(repo repository.CalendarRepo) {
	s.calendarRepo = repo
}

// CheckGeofences determines whether the given position triggers any
// geofence enter or exit events. It compares the current position's
// geofence containment against the previous position's containment.
func (s *GeofenceEventService) CheckGeofences(ctx context.Context, position *model.Position) error {
	// Find users associated with this device to scope geofence checks.
	userIDs, err := s.deviceRepo.GetUserIDs(ctx, position.DeviceID)
	if err != nil {
		return err
	}
	if len(userIDs) == 0 {
		return nil // Device has no users; no geofences to check.
	}

	// Check geofences for each user that owns this device.
	for _, userID := range userIDs {
		if err := s.checkForUser(ctx, position, userID); err != nil {
			s.logger.Error("geofence check failed for user",
				slog.Int64("userID", userID),
				slog.Int64("deviceID", position.DeviceID),
				slog.Any("error", err),
			)
		}
	}

	return nil
}

func (s *GeofenceEventService) checkForUser(ctx context.Context, position *model.Position, userID int64) error {
	// Which geofences contain the current position?
	currentGeofences, err := s.geofenceRepo.CheckContainment(ctx, userID, position.Latitude, position.Longitude)
	if err != nil {
		return err
	}

	// Update position with current geofence IDs for Home Assistant/Traccar compatibility.
	// This allows clients to query which geofence(s) currently contain the device.
	position.GeofenceIDs = currentGeofences

	// Get the previous position to compare.
	prevPosition, err := s.positionRepo.GetPreviousByDevice(ctx, position.DeviceID, position.Timestamp)
	if err != nil || prevPosition == nil {
		// First position for this device -- treat all current geofences as enter events.
		for _, gid := range currentGeofences {
			s.createEvent(ctx, position, gid, "geofenceEnter")
		}
		return nil
	}

	// Which geofences contained the previous position?
	prevGeofences, err := s.geofenceRepo.CheckContainment(ctx, userID, prevPosition.Latitude, prevPosition.Longitude)
	if err != nil {
		return err
	}

	// Detect enter events: in current but not in previous.
	for _, gid := range currentGeofences {
		if !containsID(prevGeofences, gid) {
			s.createEvent(ctx, position, gid, "geofenceEnter")
		}
	}

	// Detect exit events: in previous but not in current.
	for _, gid := range prevGeofences {
		if !containsID(currentGeofences, gid) {
			s.createEvent(ctx, position, gid, "geofenceExit")
		}
	}

	return nil
}

func (s *GeofenceEventService) createEvent(ctx context.Context, position *model.Position, geofenceID int64, eventType string) {
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

	if err := s.eventRepo.Create(ctx, event); err != nil {
		s.logger.Error("failed to create geofence event",
			slog.String("eventType", eventType),
			slog.Int64("deviceID", position.DeviceID),
			slog.Int64("geofenceID", geofenceID),
			slog.Any("error", err),
		)
		return
	}

	metrics.GeofenceEvents.WithLabelValues(eventType).Inc()
	s.logger.Info("geofence event detected",
		slog.Int64("deviceID", position.DeviceID),
		slog.Int64("geofenceID", geofenceID),
		slog.String("eventType", eventType),
	)

	// Broadcast the event via WebSocket.
	if s.hub != nil {
		s.hub.BroadcastEvent(event)
	}

	// Trigger notifications for this event.
	if s.notificationService != nil {
		if err := s.notificationService.ProcessEvent(ctx, event); err != nil {
			s.logger.Error("failed to process notifications for event",
				slog.Int64("eventID", event.ID),
				slog.Any("error", err),
			)
		}
	}
}

// isGeofenceActiveNow checks whether a geofence should trigger events at the
// current time. If the geofence has no calendar_id, it is always active.
// If it has a calendar_id and the calendar is active now, it returns true.
// If the calendar is not active or cannot be loaded, it returns false.
func (s *GeofenceEventService) isGeofenceActiveNow(ctx context.Context, geofenceID int64) bool {
	// No calendar repo configured; all geofences are always active.
	if s.calendarRepo == nil {
		return true
	}

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

func containsID(ids []int64, target int64) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

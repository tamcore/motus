package services

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
	"github.com/tamcore/motus/internal/websocket"
)

const businessHoursICal = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Motus//Test//EN
BEGIN:VEVENT
DTSTART:20260101T080000Z
DTEND:20260101T180000Z
RRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR
SUMMARY:Business Hours
END:VEVENT
END:VCALENDAR`

// setupCalendarGeofenceTest creates the full test environment for calendar-geofence tests.
func setupCalendarGeofenceTest(t *testing.T) (
	*GeofenceEventService,
	*repository.GeofenceRepository,
	*repository.EventRepository,
	*repository.DeviceRepository,
	*repository.PositionRepository,
	*repository.UserRepository,
	*repository.CalendarRepository,
) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	geoRepo := repository.NewGeofenceRepository(pool)
	eventRepo := repository.NewEventRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	posRepo := repository.NewPositionRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	calRepo := repository.NewCalendarRepository(pool)
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })

	svc := NewGeofenceEventService(geoRepo, eventRepo, deviceRepo, posRepo, hub, nil)
	svc.SetCalendarRepo(calRepo)

	return svc, geoRepo, eventRepo, deviceRepo, posRepo, userRepo, calRepo
}

func TestGeofenceCalendar_NoCalendar_AlwaysTriggers(t *testing.T) {
	svc, geoRepo, eventRepo, deviceRepo, posRepo, userRepo, _ := setupCalendarGeofenceTest(t)
	ctx := context.Background()

	user := &model.User{Email: "cal-nocal@example.com", PasswordHash: "hash", Name: "No Cal"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "cal-nocal-dev", Name: "No Cal Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// Geofence without calendar_id -- should always trigger.
	g := &model.Geofence{Name: "Always Active Fence", Geometry: testGeoJSON}
	_ = geoRepo.Create(ctx, g)
	_ = geoRepo.AssociateUser(ctx, user.ID, g.ID)

	pos := &model.Position{
		DeviceID: device.ID, Latitude: 52.52, Longitude: 13.37,
		Timestamp: time.Now().UTC(),
	}
	_ = posRepo.Create(ctx, pos)

	if err := svc.CheckGeofences(ctx, pos); err != nil {
		t.Fatalf("CheckGeofences failed: %v", err)
	}

	events, _ := eventRepo.GetByDevice(ctx, device.ID, 100)
	if len(events) != 1 {
		t.Fatalf("expected 1 enter event, got %d", len(events))
	}
	if events[0].Type != "geofenceEnter" {
		t.Errorf("expected 'geofenceEnter', got %q", events[0].Type)
	}
}

func TestGeofenceCalendar_ActiveCalendar_Triggers(t *testing.T) {
	svc, geoRepo, eventRepo, deviceRepo, posRepo, userRepo, calRepo := setupCalendarGeofenceTest(t)
	ctx := context.Background()

	user := &model.User{Email: "cal-active@example.com", PasswordHash: "hash", Name: "Active Cal"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "cal-active-dev", Name: "Active Cal Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// Create a calendar that is active at the test time.
	cal := &model.Calendar{UserID: user.ID, Name: "Business Hours", Data: businessHoursICal}
	_ = calRepo.Create(ctx, cal)

	// Create geofence with calendar_id.
	g := &model.Geofence{Name: "Cal Active Fence", Geometry: testGeoJSON, CalendarID: &cal.ID}
	_ = geoRepo.Create(ctx, g)
	_ = geoRepo.AssociateUser(ctx, user.ID, g.ID)

	// Set the clock to a Wednesday at 10:00 UTC (business hours).
	wednesday10am := time.Date(2026, 1, 14, 10, 0, 0, 0, time.UTC) // Jan 14, 2026 is a Wednesday
	svc.now = func() time.Time { return wednesday10am }

	pos := &model.Position{
		DeviceID: device.ID, Latitude: 52.52, Longitude: 13.37,
		Timestamp: wednesday10am,
	}
	_ = posRepo.Create(ctx, pos)

	if err := svc.CheckGeofences(ctx, pos); err != nil {
		t.Fatalf("CheckGeofences failed: %v", err)
	}

	events, _ := eventRepo.GetByDevice(ctx, device.ID, 100)
	if len(events) != 1 {
		t.Fatalf("expected 1 enter event (calendar active), got %d", len(events))
	}
}

func TestGeofenceCalendar_InactiveCalendar_Suppresses(t *testing.T) {
	svc, geoRepo, eventRepo, deviceRepo, posRepo, userRepo, calRepo := setupCalendarGeofenceTest(t)
	ctx := context.Background()

	user := &model.User{Email: "cal-inactive@example.com", PasswordHash: "hash", Name: "Inactive Cal"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "cal-inactive-dev", Name: "Inactive Cal Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// Create a business hours calendar.
	cal := &model.Calendar{UserID: user.ID, Name: "Business Hours", Data: businessHoursICal}
	_ = calRepo.Create(ctx, cal)

	// Create geofence with calendar_id.
	g := &model.Geofence{Name: "Cal Inactive Fence", Geometry: testGeoJSON, CalendarID: &cal.ID}
	_ = geoRepo.Create(ctx, g)
	_ = geoRepo.AssociateUser(ctx, user.ID, g.ID)

	// Set the clock to a Saturday at 10:00 UTC (outside business hours).
	saturday10am := time.Date(2026, 1, 17, 10, 0, 0, 0, time.UTC) // Jan 17, 2026 is a Saturday
	svc.now = func() time.Time { return saturday10am }

	pos := &model.Position{
		DeviceID: device.ID, Latitude: 52.52, Longitude: 13.37,
		Timestamp: saturday10am,
	}
	_ = posRepo.Create(ctx, pos)

	if err := svc.CheckGeofences(ctx, pos); err != nil {
		t.Fatalf("CheckGeofences failed: %v", err)
	}

	// No events should be created because the calendar says it's not active.
	events, _ := eventRepo.GetByDevice(ctx, device.ID, 100)
	if len(events) != 0 {
		t.Fatalf("expected 0 events (calendar inactive), got %d", len(events))
	}
}

func TestGeofenceCalendar_OutsideHours_Suppresses(t *testing.T) {
	svc, geoRepo, eventRepo, deviceRepo, posRepo, userRepo, calRepo := setupCalendarGeofenceTest(t)
	ctx := context.Background()

	user := &model.User{Email: "cal-hours@example.com", PasswordHash: "hash", Name: "Hours Cal"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "cal-hours-dev", Name: "Hours Cal Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// Create a business hours calendar.
	cal := &model.Calendar{UserID: user.ID, Name: "Business Hours", Data: businessHoursICal}
	_ = calRepo.Create(ctx, cal)

	// Create geofence with calendar_id.
	g := &model.Geofence{Name: "Cal Hours Fence", Geometry: testGeoJSON, CalendarID: &cal.ID}
	_ = geoRepo.Create(ctx, g)
	_ = geoRepo.AssociateUser(ctx, user.ID, g.ID)

	// Set the clock to a Wednesday at 22:00 UTC (after business hours).
	wednesday22pm := time.Date(2026, 1, 14, 22, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return wednesday22pm }

	pos := &model.Position{
		DeviceID: device.ID, Latitude: 52.52, Longitude: 13.37,
		Timestamp: wednesday22pm,
	}
	_ = posRepo.Create(ctx, pos)

	if err := svc.CheckGeofences(ctx, pos); err != nil {
		t.Fatalf("CheckGeofences failed: %v", err)
	}

	events, _ := eventRepo.GetByDevice(ctx, device.ID, 100)
	if len(events) != 0 {
		t.Fatalf("expected 0 events (outside business hours), got %d", len(events))
	}
}

func TestGeofenceCalendar_ExitAlsoSuppressed(t *testing.T) {
	svc, geoRepo, eventRepo, deviceRepo, posRepo, userRepo, calRepo := setupCalendarGeofenceTest(t)
	ctx := context.Background()

	user := &model.User{Email: "cal-exit@example.com", PasswordHash: "hash", Name: "Cal Exit"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "cal-exit-dev", Name: "Cal Exit Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	cal := &model.Calendar{UserID: user.ID, Name: "Business Hours", Data: businessHoursICal}
	_ = calRepo.Create(ctx, cal)

	g := &model.Geofence{Name: "Cal Exit Fence", Geometry: testGeoJSON, CalendarID: &cal.ID}
	_ = geoRepo.Create(ctx, g)
	_ = geoRepo.AssociateUser(ctx, user.ID, g.ID)

	// Step 1: Enter during business hours (should trigger).
	wednesday10am := time.Date(2026, 1, 14, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return wednesday10am }

	pos1 := &model.Position{
		DeviceID: device.ID, Latitude: 52.52, Longitude: 13.37,
		Timestamp: wednesday10am,
	}
	_ = posRepo.Create(ctx, pos1)
	_ = svc.CheckGeofences(ctx, pos1)

	// Step 2: Exit on Saturday (should be suppressed).
	saturday10am := time.Date(2026, 1, 17, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return saturday10am }

	pos2 := &model.Position{
		DeviceID: device.ID, Latitude: 52.55, Longitude: 13.50,
		Timestamp: saturday10am,
	}
	_ = posRepo.Create(ctx, pos2)
	_ = svc.CheckGeofences(ctx, pos2)

	events, _ := eventRepo.GetByDevice(ctx, device.ID, 100)

	// Should have 1 enter event (from Wednesday), but 0 exit events (Saturday suppressed).
	if len(events) != 1 {
		t.Fatalf("expected 1 event (enter only, exit suppressed), got %d", len(events))
	}
	if events[0].Type != "geofenceEnter" {
		t.Errorf("expected 'geofenceEnter', got %q", events[0].Type)
	}
}

func TestGeofenceCalendar_NoCalendarRepo_AlwaysTriggers(t *testing.T) {
	// When calendarRepo is nil (not configured), all geofences should trigger.
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	geoRepo := repository.NewGeofenceRepository(pool)
	eventRepo := repository.NewEventRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	posRepo := repository.NewPositionRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	calRepo := repository.NewCalendarRepository(pool)
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })

	// Create service WITHOUT setting calendar repo.
	svc := NewGeofenceEventService(geoRepo, eventRepo, deviceRepo, posRepo, hub, nil)

	ctx := context.Background()

	user := &model.User{Email: "cal-norepo@example.com", PasswordHash: "hash", Name: "No Repo"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "cal-norepo-dev", Name: "No Repo Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// Create a calendar and link geofence to it.
	cal := &model.Calendar{UserID: user.ID, Name: "Business Hours", Data: businessHoursICal}
	_ = calRepo.Create(ctx, cal)

	g := &model.Geofence{Name: "Has Calendar", Geometry: testGeoJSON, CalendarID: &cal.ID}
	_ = geoRepo.Create(ctx, g)
	_ = geoRepo.AssociateUser(ctx, user.ID, g.ID)

	pos := &model.Position{
		DeviceID: device.ID, Latitude: 52.52, Longitude: 13.37,
		Timestamp: time.Now().UTC(),
	}
	_ = posRepo.Create(ctx, pos)

	if err := svc.CheckGeofences(ctx, pos); err != nil {
		t.Fatalf("CheckGeofences failed: %v", err)
	}

	// Should still trigger because calendarRepo is nil (fail open).
	events, _ := eventRepo.GetByDevice(ctx, device.ID, 100)
	if len(events) != 1 {
		t.Fatalf("expected 1 event (no calendar repo = always trigger), got %d", len(events))
	}
}

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

func setupGeofenceService(t *testing.T) (
	*GeofenceEventService,
	*repository.GeofenceRepository,
	*repository.EventRepository,
	*repository.DeviceRepository,
	*repository.PositionRepository,
	*repository.UserRepository,
) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	geoRepo := repository.NewGeofenceRepository(pool)
	eventRepo := repository.NewEventRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	posRepo := repository.NewPositionRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })

	svc := NewGeofenceEventService(geoRepo, eventRepo, deviceRepo, posRepo, hub, nil)
	return svc, geoRepo, eventRepo, deviceRepo, posRepo, userRepo
}

const testGeoJSON = `{"type":"Polygon","coordinates":[[[13.35,52.51],[13.35,52.53],[13.40,52.53],[13.40,52.51],[13.35,52.51]]]}`

func TestGeofenceEvent_FirstPosition_EnterEvents(t *testing.T) {
	svc, geoRepo, eventRepo, deviceRepo, posRepo, userRepo := setupGeofenceService(t)
	ctx := context.Background()

	// Setup: user, device, geofence.
	user := &model.User{Email: "geoevt@example.com", PasswordHash: "hash", Name: "Geo Evt"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "geoevt-dev", Name: "Geo Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	g := &model.Geofence{Name: "Test Fence", Geometry: testGeoJSON}
	_ = geoRepo.Create(ctx, g)
	_ = geoRepo.AssociateUser(ctx, user.ID, g.ID)

	// Position inside the geofence.
	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Timestamp: time.Now().UTC(),
	}
	_ = posRepo.Create(ctx, pos)

	// First position should generate enter events for all containing geofences.
	if err := svc.CheckGeofences(ctx, pos); err != nil {
		t.Fatalf("CheckGeofences failed: %v", err)
	}

	events, err := eventRepo.GetByDevice(ctx, device.ID, 100)
	if err != nil {
		t.Fatalf("GetByDevice failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 enter event, got %d", len(events))
	}
	if events[0].Type != "geofenceEnter" {
		t.Errorf("expected type 'geofenceEnter', got %q", events[0].Type)
	}
}

func TestGeofenceEvent_NoGeofence_NoEvents(t *testing.T) {
	svc, _, eventRepo, deviceRepo, posRepo, userRepo := setupGeofenceService(t)
	ctx := context.Background()

	user := &model.User{Email: "geoevt-no@example.com", PasswordHash: "hash", Name: "No Geo"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "nogeo-dev", Name: "No Geo Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// No geofences created. Position should not generate events.
	pos := &model.Position{
		DeviceID: device.ID, Latitude: 52.52, Longitude: 13.37,
		Timestamp: time.Now().UTC(),
	}
	_ = posRepo.Create(ctx, pos)

	if err := svc.CheckGeofences(ctx, pos); err != nil {
		t.Fatalf("CheckGeofences failed: %v", err)
	}

	events, _ := eventRepo.GetByDevice(ctx, device.ID, 100)
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestGeofenceEvent_ExitDetection(t *testing.T) {
	svc, geoRepo, eventRepo, deviceRepo, posRepo, userRepo := setupGeofenceService(t)
	ctx := context.Background()

	user := &model.User{Email: "geoexit@example.com", PasswordHash: "hash", Name: "Geo Exit"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "geoexit-dev", Name: "Exit Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	g := &model.Geofence{Name: "Exit Fence", Geometry: testGeoJSON}
	_ = geoRepo.Create(ctx, g)
	_ = geoRepo.AssociateUser(ctx, user.ID, g.ID)

	now := time.Now().UTC()

	// First position: inside the geofence.
	pos1 := &model.Position{
		DeviceID: device.ID, Latitude: 52.52, Longitude: 13.37,
		Timestamp: now.Add(-5 * time.Minute),
	}
	_ = posRepo.Create(ctx, pos1)
	_ = svc.CheckGeofences(ctx, pos1)

	// Second position: outside the geofence.
	pos2 := &model.Position{
		DeviceID: device.ID, Latitude: 52.55, Longitude: 13.50,
		Timestamp: now,
	}
	_ = posRepo.Create(ctx, pos2)
	_ = svc.CheckGeofences(ctx, pos2)

	events, _ := eventRepo.GetByDevice(ctx, device.ID, 100)
	if len(events) != 2 {
		t.Fatalf("expected 2 events (enter + exit), got %d", len(events))
	}

	// Events are ordered DESC by timestamp.
	typeMap := map[string]bool{}
	for _, e := range events {
		typeMap[e.Type] = true
	}
	if !typeMap["geofenceEnter"] {
		t.Error("expected geofenceEnter event")
	}
	if !typeMap["geofenceExit"] {
		t.Error("expected geofenceExit event")
	}
}

func TestGeofenceEvent_DeviceWithNoUsers(t *testing.T) {
	svc, _, _, _, _, _ := setupGeofenceService(t)
	ctx := context.Background()

	// Create a position for a device with no user association.
	// This exercises the early return path for no users.
	pos := &model.Position{
		DeviceID: 99999, // Non-existent device.
		Latitude: 52.52, Longitude: 13.37,
		Timestamp: time.Now().UTC(),
	}

	// Should not error even though device doesn't exist; it just returns early.
	// (The GetUserIDs call will fail, which is handled.)
	_ = svc.CheckGeofences(ctx, pos)
}

func TestContainsID(t *testing.T) {
	tests := []struct {
		name   string
		ids    []int64
		target int64
		want   bool
	}{
		{"found", []int64{1, 2, 3}, 2, true},
		{"not found", []int64{1, 2, 3}, 4, false},
		{"empty slice", []int64{}, 1, false},
		{"nil slice", nil, 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsID(tt.ids, tt.target)
			if got != tt.want {
				t.Errorf("containsID(%v, %d) = %v, want %v", tt.ids, tt.target, got, tt.want)
			}
		})
	}
}

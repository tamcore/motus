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

	svc := NewGeofenceEventService(geoRepo, eventRepo, posRepo, hub, nil)
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

// TestCheckGeofences_ShapeEditDoesNotEmitSpuriousEvent verifies that resizing
// a geofence does not produce a spurious geofenceEnter when the device remains
// inside the resized shape. Without the fix, CheckGeofences re-evaluates the
// previous position against the NEW polygon, finds "not inside", then compares
// with the current position that IS inside → wrongly emits geofenceEnter.
func TestCheckGeofences_ShapeEditDoesNotEmitSpuriousEvent(t *testing.T) {
	svc, geoRepo, eventRepo, deviceRepo, posRepo, userRepo := setupGeofenceService(t)
	ctx := context.Background()

	user := &model.User{Email: "spurious@example.com", PasswordHash: "h", Name: "Spurious"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	device := &model.Device{UniqueID: "spurious-dev", Name: "Spurious", Status: "online"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatal(err)
	}

	// Large polygon covering (52.51-52.65, 13.35-13.40).
	largePolygon := `{"type":"Polygon","coordinates":[[[13.35,52.51],[13.35,52.65],[13.40,52.65],[13.40,52.51],[13.35,52.51]]]}`
	g := &model.Geofence{Name: "Large Fence", Geometry: largePolygon}
	if err := geoRepo.Create(ctx, g); err != nil {
		t.Fatal(err)
	}
	if err := geoRepo.AssociateUser(ctx, user.ID, g.ID); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()

	// pos1: inside the large polygon.
	pos1 := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Timestamp: now.Add(-5 * time.Minute),
	}
	if err := posRepo.Create(ctx, pos1); err != nil {
		t.Fatal(err)
	}
	if err := svc.CheckGeofences(ctx, pos1); err != nil {
		t.Fatal(err)
	}
	// Simulate what the protocol handler does: persist computed GeofenceIDs.
	if len(pos1.GeofenceIDs) > 0 {
		if err := posRepo.UpdateGeofenceIDs(ctx, pos1.ID, pos1.GeofenceIDs); err != nil {
			t.Fatalf("UpdateGeofenceIDs failed: %v", err)
		}
	}

	// Shrink the polygon to cover only (52.58-52.65, 13.35-13.40) — pos1 is now outside.
	smallPolygon := `{"type":"Polygon","coordinates":[[[13.35,52.58],[13.35,52.65],[13.40,52.65],[13.40,52.58],[13.35,52.58]]]}`
	g.Geometry = smallPolygon
	if err := geoRepo.Update(ctx, g); err != nil {
		t.Fatalf("Update geofence failed: %v", err)
	}

	// pos2: inside the NEW (smaller) polygon. Device hasn't moved relative to
	// the new boundary; only the boundary moved.
	pos2 := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.60,
		Longitude: 13.37,
		Timestamp: now,
	}
	if err := posRepo.Create(ctx, pos2); err != nil {
		t.Fatal(err)
	}
	if err := svc.CheckGeofences(ctx, pos2); err != nil {
		t.Fatal(err)
	}

	// Expect exactly 1 event total: the initial geofenceEnter from pos1.
	// No spurious geofenceEnter should be emitted for pos2.
	events, err := eventRepo.GetByDevice(ctx, device.ID, 100)
	if err != nil {
		t.Fatalf("GetByDevice failed: %v", err)
	}
	if len(events) != 1 {
		types := make([]string, len(events))
		for i, e := range events {
			types[i] = e.Type
		}
		t.Errorf("expected 1 event (geofenceEnter from pos1), got %d: %v", len(events), types)
	}
	if len(events) == 1 && events[0].Type != "geofenceEnter" {
		t.Errorf("expected geofenceEnter, got %q", events[0].Type)
	}
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

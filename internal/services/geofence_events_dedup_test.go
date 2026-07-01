package services

import (
	"context"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
)

// testGeoJSONEast is a polygon disjoint from testGeoJSON, used to verify
// that the dedup window does NOT shadow events for a different geofence.
const testGeoJSONEast = `{"type":"Polygon","coordinates":[[[13.45,52.51],[13.45,52.53],[13.50,52.53],[13.50,52.51],[13.45,52.51]]]}`

// TestGeofenceEvent_BoundaryJitterIsDeduplicated verifies the regression: a
// device whose position alternates across a geofence boundary (GPS jitter)
// emits exactly one enter and one exit instead of one event per oscillation.
func TestGeofenceEvent_BoundaryJitterIsDeduplicated(t *testing.T) {
	svc, geoRepo, eventRepo, deviceRepo, posRepo, userRepo := setupGeofenceService(t)
	ctx := context.Background()

	user := &model.User{Email: "jitter@example.com", PasswordHash: "h", Name: "J"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	device := &model.Device{UniqueID: "jitter-dev", Name: "Jitter", Status: "online"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatal(err)
	}
	g := &model.Geofence{Name: "J", Geometry: testGeoJSON}
	if err := geoRepo.Create(ctx, g); err != nil {
		t.Fatal(err)
	}
	if err := geoRepo.AssociateUser(ctx, user.ID, g.ID); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	for i := range 10 {
		var lat, lon float64
		if i%2 == 0 {
			lat, lon = 52.52, 13.37 // inside testGeoJSON
		} else {
			lat, lon = 52.55, 13.50 // outside testGeoJSON
		}
		pos := &model.Position{
			DeviceID:  device.ID,
			Latitude:  lat,
			Longitude: lon,
			Timestamp: now.Add(time.Duration(i) * time.Second),
		}
		if err := posRepo.Create(ctx, pos); err != nil {
			t.Fatalf("create pos %d: %v", i, err)
		}
		if err := svc.CheckGeofences(ctx, pos); err != nil {
			t.Fatalf("check pos %d: %v", i, err)
		}
	}

	events, err := eventRepo.GetByDevice(ctx, device.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	enterCount, exitCount := 0, 0
	for _, e := range events {
		switch e.Type {
		case "geofenceEnter":
			enterCount++
		case "geofenceExit":
			exitCount++
		}
	}
	if enterCount != 1 {
		t.Errorf("expected exactly 1 geofenceEnter under boundary jitter, got %d", enterCount)
	}
	if exitCount != 1 {
		t.Errorf("expected exactly 1 geofenceExit under boundary jitter, got %d", exitCount)
	}
}

// TestGeofenceEvent_DuplicateTimestampDoesNotReEnter verifies that a position
// with the same timestamp as the previous one (a common occurrence with
// devices reporting time at second resolution) does not trigger a spurious
// re-enter via the strict `<` lookup falling into the "no previous" branch.
func TestGeofenceEvent_DuplicateTimestampDoesNotReEnter(t *testing.T) {
	svc, geoRepo, eventRepo, deviceRepo, posRepo, userRepo := setupGeofenceService(t)
	ctx := context.Background()

	user := &model.User{Email: "dupts@example.com", PasswordHash: "h", Name: "DupTS"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	device := &model.Device{UniqueID: "dupts-dev", Name: "DupTS", Status: "online"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatal(err)
	}
	g := &model.Geofence{Name: "DupTS", Geometry: testGeoJSON}
	if err := geoRepo.Create(ctx, g); err != nil {
		t.Fatal(err)
	}
	if err := geoRepo.AssociateUser(ctx, user.ID, g.ID); err != nil {
		t.Fatal(err)
	}

	t0 := time.Now().UTC()

	pos1 := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Timestamp: t0,
	}
	if err := posRepo.Create(ctx, pos1); err != nil {
		t.Fatal(err)
	}
	if err := svc.CheckGeofences(ctx, pos1); err != nil {
		t.Fatal(err)
	}

	pos2 := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Timestamp: t0, // same timestamp as pos1 — strict `<` returns no prev
	}
	if err := posRepo.Create(ctx, pos2); err != nil {
		t.Fatal(err)
	}
	if err := svc.CheckGeofences(ctx, pos2); err != nil {
		t.Fatal(err)
	}

	events, err := eventRepo.GetByDevice(ctx, device.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	enterCount := 0
	for _, e := range events {
		if e.Type == "geofenceEnter" {
			enterCount++
		}
	}
	if enterCount != 1 {
		t.Errorf("expected exactly 1 geofenceEnter under duplicate timestamps, got %d", enterCount)
	}
}

// TestGeofenceEvent_SharedDeviceProducesOneEvent verifies that a device
// shared with multiple users (where both users are associated with the same
// geofence) produces exactly one event row per physical transition — not one
// row per user-share. Notification fan-out happens at dispatch, not by
// duplicating event rows.
func TestGeofenceEvent_SharedDeviceProducesOneEvent(t *testing.T) {
	svc, geoRepo, eventRepo, deviceRepo, posRepo, userRepo := setupGeofenceService(t)
	ctx := context.Background()

	userA := &model.User{Email: "share-a@example.com", PasswordHash: "h", Name: "A"}
	if err := userRepo.Create(ctx, userA); err != nil {
		t.Fatal(err)
	}
	userB := &model.User{Email: "share-b@example.com", PasswordHash: "h", Name: "B"}
	if err := userRepo.Create(ctx, userB); err != nil {
		t.Fatal(err)
	}

	device := &model.Device{UniqueID: "shared-dev", Name: "Shared", Status: "online"}
	if err := deviceRepo.Create(ctx, device, userA.ID); err != nil {
		t.Fatal(err)
	}
	if err := userRepo.AssignDevice(ctx, userB.ID, device.ID); err != nil {
		t.Fatal(err)
	}

	g := &model.Geofence{Name: "Shared", Geometry: testGeoJSON}
	if err := geoRepo.Create(ctx, g); err != nil {
		t.Fatal(err)
	}
	if err := geoRepo.AssociateUser(ctx, userA.ID, g.ID); err != nil {
		t.Fatal(err)
	}
	if err := geoRepo.AssociateUser(ctx, userB.ID, g.ID); err != nil {
		t.Fatal(err)
	}

	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Timestamp: time.Now().UTC(),
	}
	if err := posRepo.Create(ctx, pos); err != nil {
		t.Fatal(err)
	}
	if err := svc.CheckGeofences(ctx, pos); err != nil {
		t.Fatal(err)
	}

	events, err := eventRepo.GetByDevice(ctx, device.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	enterCount := 0
	for _, e := range events {
		if e.Type == "geofenceEnter" {
			enterCount++
		}
	}
	if enterCount != 1 {
		t.Errorf("expected exactly 1 geofenceEnter for a shared device, got %d", enterCount)
	}
}

// TestGeofenceEvent_NewSessionAfterWindowFiresAgain verifies that a
// legitimate new transition for the same geofence well outside the dedup
// window does fire — the dedup is a window, not a one-shot.
func TestGeofenceEvent_NewSessionAfterWindowFiresAgain(t *testing.T) {
	svc, geoRepo, eventRepo, deviceRepo, posRepo, userRepo := setupGeofenceService(t)
	ctx := context.Background()

	user := &model.User{Email: "session@example.com", PasswordHash: "h", Name: "Session"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	device := &model.Device{UniqueID: "session-dev", Name: "Session", Status: "online"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatal(err)
	}
	g := &model.Geofence{Name: "S", Geometry: testGeoJSON}
	if err := geoRepo.Create(ctx, g); err != nil {
		t.Fatal(err)
	}
	if err := geoRepo.AssociateUser(ctx, user.ID, g.ID); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()

	pos1 := &model.Position{ // inside, t-10min
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Timestamp: now.Add(-10 * time.Minute),
	}
	_ = posRepo.Create(ctx, pos1)
	_ = svc.CheckGeofences(ctx, pos1)

	pos2 := &model.Position{ // outside, t-9min
		DeviceID:  device.ID,
		Latitude:  52.55,
		Longitude: 13.50,
		Timestamp: now.Add(-9 * time.Minute),
	}
	_ = posRepo.Create(ctx, pos2)
	_ = svc.CheckGeofences(ctx, pos2)

	pos3 := &model.Position{ // inside again, t-1min — well outside the 2min window
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Timestamp: now.Add(-1 * time.Minute),
	}
	_ = posRepo.Create(ctx, pos3)
	if err := svc.CheckGeofences(ctx, pos3); err != nil {
		t.Fatal(err)
	}

	events, err := eventRepo.GetByDevice(ctx, device.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	enterCount, exitCount := 0, 0
	for _, e := range events {
		switch e.Type {
		case "geofenceEnter":
			enterCount++
		case "geofenceExit":
			exitCount++
		}
	}
	if enterCount != 2 {
		t.Errorf("expected 2 geofenceEnter events (one per parking session), got %d", enterCount)
	}
	if exitCount != 1 {
		t.Errorf("expected 1 geofenceExit event, got %d", exitCount)
	}
}

// TestGeofenceEvent_DifferentGeofencesNotShadowed verifies that the dedup
// window only suppresses repeats for the SAME (device, geofence) pair — a
// transition into a different geofence within the window must still fire.
func TestGeofenceEvent_DifferentGeofencesNotShadowed(t *testing.T) {
	svc, geoRepo, eventRepo, deviceRepo, posRepo, userRepo := setupGeofenceService(t)
	ctx := context.Background()

	user := &model.User{Email: "twofences@example.com", PasswordHash: "h", Name: "TF"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	device := &model.Device{UniqueID: "twofences-dev", Name: "TwoFences", Status: "online"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatal(err)
	}

	gA := &model.Geofence{Name: "A", Geometry: testGeoJSON}
	if err := geoRepo.Create(ctx, gA); err != nil {
		t.Fatal(err)
	}
	if err := geoRepo.AssociateUser(ctx, user.ID, gA.ID); err != nil {
		t.Fatal(err)
	}
	gB := &model.Geofence{Name: "B", Geometry: testGeoJSONEast}
	if err := geoRepo.Create(ctx, gB); err != nil {
		t.Fatal(err)
	}
	if err := geoRepo.AssociateUser(ctx, user.ID, gB.ID); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()

	pos1 := &model.Position{ // inside A
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Timestamp: now.Add(-30 * time.Second),
	}
	_ = posRepo.Create(ctx, pos1)
	_ = svc.CheckGeofences(ctx, pos1)

	pos2 := &model.Position{ // inside B (outside A)
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.47,
		Timestamp: now,
	}
	_ = posRepo.Create(ctx, pos2)
	if err := svc.CheckGeofences(ctx, pos2); err != nil {
		t.Fatal(err)
	}

	events, err := eventRepo.GetByDevice(ctx, device.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	entersByGeo := map[int64]int{}
	exitsByGeo := map[int64]int{}
	for _, e := range events {
		if e.GeofenceID == nil {
			continue
		}
		switch e.Type {
		case "geofenceEnter":
			entersByGeo[*e.GeofenceID]++
		case "geofenceExit":
			exitsByGeo[*e.GeofenceID]++
		}
	}
	if entersByGeo[gA.ID] != 1 {
		t.Errorf("expected 1 enter for geofence A, got %d", entersByGeo[gA.ID])
	}
	if entersByGeo[gB.ID] != 1 {
		t.Errorf("expected 1 enter for geofence B (must NOT be suppressed by recent enter for A), got %d", entersByGeo[gB.ID])
	}
	if exitsByGeo[gA.ID] != 1 {
		t.Errorf("expected 1 exit for geofence A, got %d", exitsByGeo[gA.ID])
	}
}

// TestGeofenceEvent_ExitTwoMinutesApartSuppressed verifies that two geofenceExit
// events for the same geofence 2m10s apart are suppressed under the 5-minute
// dedup window. This reproduces the real Kuga bug where the old 2-minute window
// let the second exit through.
func TestGeofenceEvent_ExitTwoMinutesApartSuppressed(t *testing.T) {
	svc, geoRepo, eventRepo, deviceRepo, posRepo, userRepo := setupGeofenceService(t)
	ctx := context.Background()

	user := &model.User{Email: "twomin@example.com", PasswordHash: "h", Name: "TwoMin"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	device := &model.Device{UniqueID: "twomin-dev", Name: "TwoMin", Status: "online"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatal(err)
	}
	g := &model.Geofence{Name: "TwoMin", Geometry: testGeoJSON}
	if err := geoRepo.Create(ctx, g); err != nil {
		t.Fatal(err)
	}
	if err := geoRepo.AssociateUser(ctx, user.ID, g.ID); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()

	// Seed: device is inside the geofence
	posInside := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Timestamp: now.Add(-5 * time.Minute),
	}
	_ = posRepo.Create(ctx, posInside)
	_ = svc.CheckGeofences(ctx, posInside)

	// First exit at t=0
	posExit1 := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.55,
		Longitude: 13.50,
		Timestamp: now,
	}
	_ = posRepo.Create(ctx, posExit1)
	_ = svc.CheckGeofences(ctx, posExit1)

	// Interleaved stationary position back inside (simulates H02 heartbeat)
	posHeartbeat := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Timestamp: now.Add(1 * time.Minute),
	}
	_ = posRepo.Create(ctx, posHeartbeat)
	_ = svc.CheckGeofences(ctx, posHeartbeat)

	// Second exit at t=2m10s — should be suppressed under the 5-min window
	posExit2 := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.55,
		Longitude: 13.50,
		Timestamp: now.Add(2*time.Minute + 10*time.Second),
	}
	_ = posRepo.Create(ctx, posExit2)
	if err := svc.CheckGeofences(ctx, posExit2); err != nil {
		t.Fatal(err)
	}

	events, err := eventRepo.GetByDevice(ctx, device.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	exitCount := 0
	for _, e := range events {
		if e.Type == "geofenceExit" {
			exitCount++
		}
	}
	if exitCount != 1 {
		t.Errorf("expected exactly 1 geofenceExit (second should be suppressed within 5-min window), got %d", exitCount)
	}
}

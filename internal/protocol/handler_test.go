package protocol

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

// mockGeofenceChecker is a test implementation of GeofenceChecker.
type mockGeofenceChecker struct {
	called bool
}

func (m *mockGeofenceChecker) CheckGeofences(_ context.Context, _ *model.Position) error {
	m.called = true
	return nil
}

func TestNewPositionHandler(t *testing.T) {
	h := NewPositionHandler(nil, nil, nil, nil)
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestPositionHandler_HandlePosition_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })

	ctx := context.Background()

	// Create user and device.
	user := &model.User{Email: "handler@example.com", PasswordHash: "hash", Name: "Handler Test"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	device := &model.Device{UniqueID: "handler-test-dev", Name: "Handler Device", Status: "offline"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	checker := &mockGeofenceChecker{}
	handler := NewPositionHandler(posRepo, deviceRepo, hub, checker)

	speed := 15.0
	course := 270.0
	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Speed:     &speed,
		Course:    &course,
		Timestamp: time.Now().UTC(),
	}

	if err := handler.HandlePosition(ctx, pos); err != nil {
		t.Fatalf("HandlePosition failed: %v", err)
	}

	// Position should be stored.
	if pos.ID == 0 {
		t.Error("expected position to have an ID after creation")
	}

	// Device should be marked "moving" because speed (15.0) >= threshold (5.0).
	updatedDevice, err := deviceRepo.GetByID(ctx, device.ID)
	if err != nil {
		t.Fatalf("get device: %v", err)
	}
	if updatedDevice.Status != "online" {
		t.Errorf("expected device status 'online', got %q", updatedDevice.Status)
	}
	if updatedDevice.LastUpdate == nil {
		t.Error("expected last_update to be set")
	}

	// Position should have motion attribute set to true.
	if pos.Attributes == nil {
		t.Fatal("expected position attributes to be non-nil")
	}
	if motion, ok := pos.Attributes["motion"]; !ok || motion != true {
		t.Errorf("expected position attributes[motion]=true, got %v", pos.Attributes["motion"])
	}

	// Geofence checker should have been called.
	if !checker.called {
		t.Error("expected geofence checker to be called")
	}
}

func TestPositionHandler_HandlePosition_StationaryDevice(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })

	ctx := context.Background()

	user := &model.User{Email: "stationary@example.com", PasswordHash: "hash", Name: "Stationary Test"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	device := &model.Device{UniqueID: "stationary-dev", Name: "Stationary Device", Status: "offline"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	handler := NewPositionHandler(posRepo, deviceRepo, hub, nil)

	// Speed below threshold: device should be "online" (not "moving").
	speed := 2.0
	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Speed:     &speed,
		Timestamp: time.Now().UTC(),
	}

	if err := handler.HandlePosition(ctx, pos); err != nil {
		t.Fatalf("HandlePosition failed: %v", err)
	}

	updatedDevice, err := deviceRepo.GetByID(ctx, device.ID)
	if err != nil {
		t.Fatalf("get device: %v", err)
	}
	if updatedDevice.Status != "online" {
		t.Errorf("expected device status 'online' for low speed, got %q", updatedDevice.Status)
	}

	// Position should have motion=false.
	if motion, ok := pos.Attributes["motion"]; !ok || motion != false {
		t.Errorf("expected position attributes[motion]=false, got %v", pos.Attributes["motion"])
	}
}

func TestPositionHandler_HandlePosition_NilSpeed(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })

	ctx := context.Background()

	user := &model.User{Email: "nilspeed@example.com", PasswordHash: "hash", Name: "Nil Speed Test"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	device := &model.Device{UniqueID: "nilspeed-dev", Name: "Nil Speed Device", Status: "offline"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	handler := NewPositionHandler(posRepo, deviceRepo, hub, nil)

	// Nil speed should be treated as stationary.
	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Speed:     nil,
		Timestamp: time.Now().UTC(),
	}

	if err := handler.HandlePosition(ctx, pos); err != nil {
		t.Fatalf("HandlePosition failed: %v", err)
	}

	updatedDevice, err := deviceRepo.GetByID(ctx, device.ID)
	if err != nil {
		t.Fatalf("get device: %v", err)
	}
	if updatedDevice.Status != "online" {
		t.Errorf("expected device status 'online' for nil speed, got %q", updatedDevice.Status)
	}

	if motion, ok := pos.Attributes["motion"]; !ok || motion != false {
		t.Errorf("expected position attributes[motion]=false for nil speed, got %v", pos.Attributes["motion"])
	}
}

func TestPositionHandler_HandlePosition_ExactThreshold(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })

	ctx := context.Background()

	user := &model.User{Email: "threshold@example.com", PasswordHash: "hash", Name: "Threshold Test"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	device := &model.Device{UniqueID: "threshold-dev", Name: "Threshold Device", Status: "offline"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	handler := NewPositionHandler(posRepo, deviceRepo, hub, nil)

	// Speed exactly at threshold (5.0): should be classified as moving.
	speed := motionSpeedThreshold
	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Speed:     &speed,
		Timestamp: time.Now().UTC(),
	}

	if err := handler.HandlePosition(ctx, pos); err != nil {
		t.Fatalf("HandlePosition failed: %v", err)
	}

	updatedDevice, err := deviceRepo.GetByID(ctx, device.ID)
	if err != nil {
		t.Fatalf("get device: %v", err)
	}
	if updatedDevice.Status != "online" {
		t.Errorf("expected device status 'online' at exact threshold, got %q", updatedDevice.Status)
	}

	if motion, ok := pos.Attributes["motion"]; !ok || motion != true {
		t.Errorf("expected position attributes[motion]=true at exact threshold, got %v", pos.Attributes["motion"])
	}
}

func TestPositionHandler_HandlePosition_MotionAttributePersisted(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })

	ctx := context.Background()

	user := &model.User{Email: "persist@example.com", PasswordHash: "hash", Name: "Persist Test"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	device := &model.Device{UniqueID: "persist-dev", Name: "Persist Device", Status: "offline"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	handler := NewPositionHandler(posRepo, deviceRepo, hub, nil)

	speed := 25.0
	pos := &model.Position{
		DeviceID:   device.ID,
		Latitude:   52.52,
		Longitude:  13.37,
		Speed:      &speed,
		Timestamp:  time.Now().UTC(),
		Attributes: map[string]interface{}{"flags": "FFFFFBFF"},
	}

	if err := handler.HandlePosition(ctx, pos); err != nil {
		t.Fatalf("HandlePosition failed: %v", err)
	}

	// Retrieve the position from DB and verify the motion attribute was persisted.
	stored, err := posRepo.GetLatestByDevice(ctx, device.ID)
	if err != nil {
		t.Fatalf("get latest position: %v", err)
	}
	if stored.Attributes == nil {
		t.Fatal("expected stored position attributes to be non-nil")
	}
	if motion, ok := stored.Attributes["motion"]; !ok {
		t.Error("expected 'motion' attribute to be persisted in stored position")
	} else if motion != true {
		t.Errorf("expected stored position attributes[motion]=true, got %v", motion)
	}
	// Existing attributes should also be preserved.
	if flags, ok := stored.Attributes["flags"]; !ok || flags != "FFFFFBFF" {
		t.Errorf("expected stored position attributes[flags]='FFFFFBFF', got %v", stored.Attributes["flags"])
	}
}

func TestPositionHandler_HandlePosition_MovingThenStopping(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })

	ctx := context.Background()

	user := &model.User{Email: "transition@example.com", PasswordHash: "hash", Name: "Transition Test"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	device := &model.Device{UniqueID: "transition-dev", Name: "Transition Device", Status: "offline"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	handler := NewPositionHandler(posRepo, deviceRepo, hub, nil)

	// First position: moving.
	speed := 30.0
	pos1 := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Speed:     &speed,
		Timestamp: time.Now().UTC(),
	}
	if err := handler.HandlePosition(ctx, pos1); err != nil {
		t.Fatalf("HandlePosition(moving) failed: %v", err)
	}

	d1, _ := deviceRepo.GetByID(ctx, device.ID)
	if d1.Status != "online" {
		t.Errorf("expected status 'online' after fast position, got %q", d1.Status)
	}

	// Second position: stopped.
	zeroSpeed := 0.0
	pos2 := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Speed:     &zeroSpeed,
		Timestamp: time.Now().UTC(),
	}
	if err := handler.HandlePosition(ctx, pos2); err != nil {
		t.Fatalf("HandlePosition(stopped) failed: %v", err)
	}

	d2, _ := deviceRepo.GetByID(ctx, device.ID)
	if d2.Status != "online" {
		t.Errorf("expected status 'online' after stop, got %q", d2.Status)
	}

	// Verify motion attributes on each position.
	if pos1.Attributes["motion"] != true {
		t.Errorf("pos1 attributes[motion] should be true, got %v", pos1.Attributes["motion"])
	}
	if pos2.Attributes["motion"] != false {
		t.Errorf("pos2 attributes[motion] should be false, got %v", pos2.Attributes["motion"])
	}
}

func TestPositionHandler_HandlePosition_NoGeofenceChecker(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })

	ctx := context.Background()

	user := &model.User{Email: "nochecker@example.com", PasswordHash: "hash", Name: "No Checker"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "nochecker-dev", Name: "No Checker Device", Status: "offline"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// nil geofence checker should be handled gracefully.
	handler := NewPositionHandler(posRepo, deviceRepo, hub, nil)

	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Timestamp: time.Now().UTC(),
	}

	if err := handler.HandlePosition(ctx, pos); err != nil {
		t.Fatalf("HandlePosition failed: %v", err)
	}

	if pos.ID == 0 {
		t.Error("expected position to have an ID after creation")
	}
}

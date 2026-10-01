package protocol

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
	"github.com/tamcore/motus/internal/websocket"
)

type mockMotionChecker struct {
	called bool
	err    error
}

func (m *mockMotionChecker) CheckMotion(_ context.Context, _ *model.Position) error {
	m.called = true
	return m.err
}

func TestPositionHandler_AddCheck(t *testing.T) {
	h := NewPositionHandler(nil, nil, nil, nil)
	h.AddCheck("motion", (&mockMotionChecker{}).CheckMotion)
	h.AddCheck("alarm", (&mockMotionChecker{}).CheckMotion)
	if len(h.checks) != 2 || h.checks[0].name != "motion" || h.checks[1].name != "alarm" {
		t.Errorf("checks = %+v", h.checks)
	}
}

func TestPositionHandler_HandlePosition_WithCheckers(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })

	ctx := context.Background()

	user := &model.User{Email: "checkers@example.com", PasswordHash: "hash", Name: "Checkers Test"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	device := &model.Device{UniqueID: "checkers-dev", Name: "Checkers Device", Status: "offline"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	geoChecker := &mockGeofenceChecker{}
	motionChecker := &mockMotionChecker{}

	handler := NewPositionHandler(posRepo, deviceRepo, hub, geoChecker)
	handler.AddCheck("motion", motionChecker.CheckMotion)

	speed := 120.0
	course := 90.0
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

	if !geoChecker.called {
		t.Error("expected geofence checker to be called")
	}
	if !motionChecker.called {
		t.Error("expected motion checker to be called")
	}
}

func TestPositionHandler_HandlePosition_CheckerErrors(t *testing.T) {
	// Checker errors should be logged but not cause HandlePosition to fail.
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })

	ctx := context.Background()

	user := &model.User{Email: "errcheckers@example.com", PasswordHash: "hash", Name: "Error Checkers"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "err-checkers-dev", Name: "Error Checkers Device", Status: "offline"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// All checkers return errors.
	geoChecker := &mockGeofenceChecker{}
	motionChecker := &mockMotionChecker{err: errors.New("motion check failed")}

	handler := NewPositionHandler(posRepo, deviceRepo, hub, geoChecker)
	handler.AddCheck("motion", motionChecker.CheckMotion)

	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Timestamp: time.Now().UTC(),
	}

	// HandlePosition should NOT return an error even when checkers fail.
	if err := handler.HandlePosition(ctx, pos); err != nil {
		t.Fatalf("HandlePosition should not fail when checkers error: %v", err)
	}

	// Checkers were still called.
	if !motionChecker.called {
		t.Error("expected motion checker to be called")
	}
}

// mockGeofenceCheckerWithError returns an error.
type mockGeofenceCheckerWithError struct {
	called bool
}

func (m *mockGeofenceCheckerWithError) CheckGeofences(_ context.Context, _ *model.Position) error {
	m.called = true
	return errors.New("geofence check failed")
}

func TestPositionHandler_HandlePosition_GeofenceError(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })

	ctx := context.Background()

	user := &model.User{Email: "geoerr@example.com", PasswordHash: "hash", Name: "Geo Error"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "geoerr-dev", Name: "Geo Error Device", Status: "offline"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	geoChecker := &mockGeofenceCheckerWithError{}
	handler := NewPositionHandler(posRepo, deviceRepo, hub, geoChecker)

	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Timestamp: time.Now().UTC(),
	}

	// Geofence error should not propagate.
	if err := handler.HandlePosition(ctx, pos); err != nil {
		t.Fatalf("HandlePosition should not fail when geofence check errors: %v", err)
	}
	if !geoChecker.called {
		t.Error("expected geofence checker to be called")
	}
}

func TestPositionHandler_SetAddressLookup(t *testing.T) {
	h := NewPositionHandler(nil, nil, nil, nil)
	h.SetAddressLookup(nil) // nil is valid
	if h.addressLookup != nil {
		t.Error("expected address lookup to be nil")
	}
}

func TestPositionHandler_SetLogger(t *testing.T) {
	h := NewPositionHandler(nil, nil, nil, nil)
	h.SetLogger(nil) // nil should not change the logger
	custom := slog.Default()
	h.SetLogger(custom)
	if h.logger != custom {
		t.Error("expected logger to be set")
	}
}

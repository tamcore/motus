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

func setupIdleService(t *testing.T) (
	*IdleService,
	*repository.EventRepository,
	*repository.DeviceRepository,
	*repository.PositionRepository,
	*repository.UserRepository,
) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	eventRepo := repository.NewEventRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	posRepo := repository.NewPositionRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })

	svc := NewIdleService(deviceRepo, posRepo, eventRepo, hub, nil)
	return svc, eventRepo, deviceRepo, posRepo, userRepo
}

func TestIdle_DeviceIdleLongEnough(t *testing.T) {
	svc, eventRepo, deviceRepo, posRepo, userRepo := setupIdleService(t)
	ctx := context.Background()

	user := &model.User{Email: "idle@example.com", PasswordHash: "hash", Name: "Idle"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "idle-dev", Name: "Idle Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// Position from 45 minutes ago with zero speed (beyond idle threshold of 30m).
	zeroSpeed := 0.0
	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Speed:     &zeroSpeed,
		Timestamp: time.Now().UTC().Add(-45 * time.Minute),
	}
	_ = posRepo.Create(ctx, pos)

	err := svc.CheckIdle(ctx)
	if err != nil {
		t.Fatalf("CheckIdle failed: %v", err)
	}

	events, err := eventRepo.GetByDevice(ctx, device.ID, 100)
	if err != nil {
		t.Fatalf("GetByDevice failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 idle event, got %d", len(events))
	}
	if events[0].Type != "deviceIdle" {
		t.Errorf("expected type 'deviceIdle', got %q", events[0].Type)
	}
	if events[0].Attributes["idleDuration"] == nil {
		t.Error("expected idleDuration attribute to be set")
	}
}

func TestIdle_DeviceNotIdleLongEnough(t *testing.T) {
	svc, eventRepo, deviceRepo, posRepo, userRepo := setupIdleService(t)
	ctx := context.Background()

	user := &model.User{Email: "notidle@example.com", PasswordHash: "hash", Name: "Not Idle"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "notidle-dev", Name: "Not Idle Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// Position from 10 minutes ago (below idle threshold of 30m).
	zeroSpeed := 0.0
	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Speed:     &zeroSpeed,
		Timestamp: time.Now().UTC().Add(-10 * time.Minute),
	}
	_ = posRepo.Create(ctx, pos)

	err := svc.CheckIdle(ctx)
	if err != nil {
		t.Fatalf("CheckIdle failed: %v", err)
	}

	events, _ := eventRepo.GetByDevice(ctx, device.ID, 100)
	if len(events) != 0 {
		t.Errorf("expected 0 events for device not idle long enough, got %d", len(events))
	}
}

func TestIdle_DeviceMoving(t *testing.T) {
	svc, eventRepo, deviceRepo, posRepo, userRepo := setupIdleService(t)
	ctx := context.Background()

	user := &model.User{Email: "moving@example.com", PasswordHash: "hash", Name: "Moving"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "moving-dev", Name: "Moving Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// Position from 45 minutes ago but speed is above idle threshold.
	speed := 10.0
	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Speed:     &speed,
		Timestamp: time.Now().UTC().Add(-45 * time.Minute),
	}
	_ = posRepo.Create(ctx, pos)

	err := svc.CheckIdle(ctx)
	if err != nil {
		t.Fatalf("CheckIdle failed: %v", err)
	}

	events, _ := eventRepo.GetByDevice(ctx, device.ID, 100)
	if len(events) != 0 {
		t.Errorf("expected 0 events for moving device, got %d", len(events))
	}
}

func TestIdle_Deduplication(t *testing.T) {
	svc, eventRepo, deviceRepo, posRepo, userRepo := setupIdleService(t)
	ctx := context.Background()

	user := &model.User{Email: "dedup@example.com", PasswordHash: "hash", Name: "Dedup"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "dedup-dev", Name: "Dedup Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// Position from 45 minutes ago.
	zeroSpeed := 0.0
	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Speed:     &zeroSpeed,
		Timestamp: time.Now().UTC().Add(-45 * time.Minute),
	}
	_ = posRepo.Create(ctx, pos)

	// First check should create event.
	err := svc.CheckIdle(ctx)
	if err != nil {
		t.Fatalf("first CheckIdle failed: %v", err)
	}

	events, _ := eventRepo.GetByDevice(ctx, device.ID, 100)
	if len(events) != 1 {
		t.Fatalf("expected 1 event after first check, got %d", len(events))
	}

	// Second check should NOT create a duplicate (event was just created).
	err = svc.CheckIdle(ctx)
	if err != nil {
		t.Fatalf("second CheckIdle failed: %v", err)
	}

	events, _ = eventRepo.GetByDevice(ctx, device.ID, 100)
	if len(events) != 1 {
		t.Errorf("expected still 1 event after second check (dedup), got %d", len(events))
	}
}

func TestIdle_NoPositions(t *testing.T) {
	svc, eventRepo, deviceRepo, _, userRepo := setupIdleService(t)
	ctx := context.Background()

	user := &model.User{Email: "nopos@example.com", PasswordHash: "hash", Name: "No Pos"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "nopos-dev", Name: "No Position Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// No positions created for this device.
	err := svc.CheckIdle(ctx)
	if err != nil {
		t.Fatalf("CheckIdle failed: %v", err)
	}

	events, _ := eventRepo.GetByDevice(ctx, device.ID, 100)
	if len(events) != 0 {
		t.Errorf("expected 0 events for device with no positions, got %d", len(events))
	}
}

func TestIdle_NilSpeedTreatedAsZero(t *testing.T) {
	svc, eventRepo, deviceRepo, posRepo, userRepo := setupIdleService(t)
	ctx := context.Background()

	user := &model.User{Email: "nilidle@example.com", PasswordHash: "hash", Name: "Nil Idle"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "nilidle-dev", Name: "Nil Idle Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// Position from 45 minutes ago with nil speed (treated as 0).
	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Timestamp: time.Now().UTC().Add(-45 * time.Minute),
	}
	_ = posRepo.Create(ctx, pos)

	err := svc.CheckIdle(ctx)
	if err != nil {
		t.Fatalf("CheckIdle failed: %v", err)
	}

	events, _ := eventRepo.GetByDevice(ctx, device.ID, 100)
	if len(events) != 1 {
		t.Fatalf("expected 1 idle event for nil speed, got %d", len(events))
	}
}

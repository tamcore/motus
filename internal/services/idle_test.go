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
) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	eventRepo := repository.NewEventRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	posRepo := repository.NewPositionRepository(pool)
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })

	svc := NewIdleService(deviceRepo, posRepo, eventRepo, hub, nil, nil, nil)
	return svc, eventRepo, deviceRepo, posRepo
}

func TestIdle_DeviceIdleLongEnough(t *testing.T) {
	svc, _, _, posRepo := setupIdleService(t)
	ctx := context.Background()

	user := testutil.CreateUser(t, "idle@example.com")

	device := testutil.CreateDevice(t, user.ID, "idle-dev")

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

	events, err := deviceEvents(t, device.ID)
	if err != nil {
		t.Fatalf("deviceEvents failed: %v", err)
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
	svc, _, _, posRepo := setupIdleService(t)
	ctx := context.Background()

	user := testutil.CreateUser(t, "notidle@example.com")

	device := testutil.CreateDevice(t, user.ID, "notidle-dev")

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

	events, _ := deviceEvents(t, device.ID)
	if len(events) != 0 {
		t.Errorf("expected 0 events for device not idle long enough, got %d", len(events))
	}
}

func TestIdle_DeviceMoving(t *testing.T) {
	svc, _, _, posRepo := setupIdleService(t)
	ctx := context.Background()

	user := testutil.CreateUser(t, "moving@example.com")

	device := testutil.CreateDevice(t, user.ID, "moving-dev")

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

	events, _ := deviceEvents(t, device.ID)
	if len(events) != 0 {
		t.Errorf("expected 0 events for moving device, got %d", len(events))
	}
}

func TestIdle_Deduplication(t *testing.T) {
	svc, _, _, posRepo := setupIdleService(t)
	ctx := context.Background()

	user := testutil.CreateUser(t, "dedup@example.com")

	device := testutil.CreateDevice(t, user.ID, "dedup-dev")

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

	events, _ := deviceEvents(t, device.ID)
	if len(events) != 1 {
		t.Fatalf("expected 1 event after first check, got %d", len(events))
	}

	// Second check should NOT create a duplicate (event was just created).
	err = svc.CheckIdle(ctx)
	if err != nil {
		t.Fatalf("second CheckIdle failed: %v", err)
	}

	events, _ = deviceEvents(t, device.ID)
	if len(events) != 1 {
		t.Errorf("expected still 1 event after second check (dedup), got %d", len(events))
	}
}

// TestIdle_LongParkOnlyOneEvent verifies the regression fix: a device that
// has been parked for many idle-threshold periods still only emits ONE
// deviceIdle event total, instead of one every IdleThreshold (which spammed
// webhook subscribers for hours).
func TestIdle_LongParkOnlyOneEvent(t *testing.T) {
	svc, _, _, posRepo := setupIdleService(t)
	ctx := context.Background()

	user := testutil.CreateUser(t, "longpark@example.com")

	device := testutil.CreateDevice(t, user.ID, "longpark-dev")

	zeroSpeed := 0.0
	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Speed:     &zeroSpeed,
		Timestamp: time.Now().UTC().Add(-12 * time.Hour),
	}
	_ = posRepo.Create(ctx, pos)

	for i := range 10 {
		if err := svc.CheckIdle(ctx); err != nil {
			t.Fatalf("CheckIdle iter %d failed: %v", i, err)
		}
	}

	events, _ := deviceEvents(t, device.ID)
	idleCount := 0
	for _, e := range events {
		if e.Type == "deviceIdle" {
			idleCount++
		}
	}
	if idleCount != 1 {
		t.Errorf("expected exactly 1 deviceIdle event for long-parked device, got %d", idleCount)
	}
}

// TestIdle_NewPositionTriggersNewIdleEvent verifies that after the device
// briefly moves (sends a fresh position) and then parks again, a second
// deviceIdle event is correctly emitted.
func TestIdle_NewPositionTriggersNewIdleEvent(t *testing.T) {
	svc, _, _, posRepo := setupIdleService(t)
	ctx := context.Background()

	user := testutil.CreateUser(t, "park2@example.com")

	device := testutil.CreateDevice(t, user.ID, "park2-dev")

	zeroSpeed := 0.0
	pos1 := &model.Position{
		DeviceID: device.ID,
		Latitude: 52.52, Longitude: 13.37,
		Speed:     &zeroSpeed,
		Timestamp: time.Now().UTC().Add(-2 * time.Hour),
	}
	_ = posRepo.Create(ctx, pos1)
	if err := svc.CheckIdle(ctx); err != nil {
		t.Fatal(err)
	}

	pos2 := &model.Position{
		DeviceID: device.ID,
		Latitude: 52.53, Longitude: 13.38,
		Speed:     &zeroSpeed,
		Timestamp: time.Now().UTC().Add(-45 * time.Minute),
	}
	_ = posRepo.Create(ctx, pos2)
	if err := svc.CheckIdle(ctx); err != nil {
		t.Fatal(err)
	}

	events, _ := deviceEvents(t, device.ID)
	idleCount := 0
	for _, e := range events {
		if e.Type == "deviceIdle" {
			idleCount++
		}
	}
	if idleCount != 2 {
		t.Errorf("expected 2 deviceIdle events (one per parking session), got %d", idleCount)
	}
}

func TestIdle_NoPositions(t *testing.T) {
	svc, _, _, _ := setupIdleService(t)
	ctx := context.Background()

	user := testutil.CreateUser(t, "nopos@example.com")

	device := testutil.CreateDevice(t, user.ID, "nopos-dev")

	// No positions created for this device.
	err := svc.CheckIdle(ctx)
	if err != nil {
		t.Fatalf("CheckIdle failed: %v", err)
	}

	events, _ := deviceEvents(t, device.ID)
	if len(events) != 0 {
		t.Errorf("expected 0 events for device with no positions, got %d", len(events))
	}
}

func TestIdle_NilSpeedTreatedAsZero(t *testing.T) {
	svc, _, _, posRepo := setupIdleService(t)
	ctx := context.Background()

	user := testutil.CreateUser(t, "nilidle@example.com")

	device := testutil.CreateDevice(t, user.ID, "nilidle-dev")

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

	events, _ := deviceEvents(t, device.ID)
	if len(events) != 1 {
		t.Fatalf("expected 1 idle event for nil speed, got %d", len(events))
	}
}

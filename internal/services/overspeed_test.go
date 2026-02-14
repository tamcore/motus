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

func setupOverspeedService(t *testing.T) (
	*OverspeedService,
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

	svc := NewOverspeedService(eventRepo, hub, nil)
	return svc, eventRepo, deviceRepo, posRepo, userRepo
}

func TestOverspeed_ExceedsLimit(t *testing.T) {
	svc, eventRepo, deviceRepo, posRepo, userRepo := setupOverspeedService(t)
	ctx := context.Background()

	// Setup user and device with speed limit.
	user := &model.User{Email: "overspeed@example.com", PasswordHash: "hash", Name: "Overspeed"}
	_ = userRepo.Create(ctx, user)

	speedLimit := 60.0
	device := &model.Device{UniqueID: "overspeed-dev", Name: "Speed Device", Status: "online", SpeedLimit: &speedLimit}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// Position with speed above limit.
	speed := 85.0
	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Speed:     &speed,
		Timestamp: time.Now().UTC(),
	}
	_ = posRepo.Create(ctx, pos)

	err := svc.CheckOverspeed(ctx, pos, device)
	if err != nil {
		t.Fatalf("CheckOverspeed failed: %v", err)
	}

	events, err := eventRepo.GetByDevice(ctx, device.ID, 100)
	if err != nil {
		t.Fatalf("GetByDevice failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 overspeed event, got %d", len(events))
	}
	if events[0].Type != "overspeed" {
		t.Errorf("expected type 'overspeed', got %q", events[0].Type)
	}
	if events[0].Attributes["speed"] != 85.0 {
		t.Errorf("expected speed attribute 85.0, got %v", events[0].Attributes["speed"])
	}
	if events[0].Attributes["limit"] != 60.0 {
		t.Errorf("expected limit attribute 60.0, got %v", events[0].Attributes["limit"])
	}
}

func TestOverspeed_UnderLimit(t *testing.T) {
	svc, eventRepo, deviceRepo, posRepo, userRepo := setupOverspeedService(t)
	ctx := context.Background()

	user := &model.User{Email: "underlimit@example.com", PasswordHash: "hash", Name: "Under Limit"}
	_ = userRepo.Create(ctx, user)

	speedLimit := 60.0
	device := &model.Device{UniqueID: "underlimit-dev", Name: "Slow Device", Status: "online", SpeedLimit: &speedLimit}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// Position with speed under limit.
	speed := 50.0
	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Speed:     &speed,
		Timestamp: time.Now().UTC(),
	}
	_ = posRepo.Create(ctx, pos)

	err := svc.CheckOverspeed(ctx, pos, device)
	if err != nil {
		t.Fatalf("CheckOverspeed failed: %v", err)
	}

	events, _ := eventRepo.GetByDevice(ctx, device.ID, 100)
	if len(events) != 0 {
		t.Errorf("expected 0 events for under-limit speed, got %d", len(events))
	}
}

func TestOverspeed_NoSpeedLimit(t *testing.T) {
	svc, eventRepo, deviceRepo, posRepo, userRepo := setupOverspeedService(t)
	ctx := context.Background()

	user := &model.User{Email: "nolimit@example.com", PasswordHash: "hash", Name: "No Limit"}
	_ = userRepo.Create(ctx, user)

	// Device without speed limit.
	device := &model.Device{UniqueID: "nolimit-dev", Name: "No Limit Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	speed := 200.0
	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Speed:     &speed,
		Timestamp: time.Now().UTC(),
	}
	_ = posRepo.Create(ctx, pos)

	err := svc.CheckOverspeed(ctx, pos, device)
	if err != nil {
		t.Fatalf("CheckOverspeed failed: %v", err)
	}

	events, _ := eventRepo.GetByDevice(ctx, device.ID, 100)
	if len(events) != 0 {
		t.Errorf("expected 0 events for device with no speed limit, got %d", len(events))
	}
}

func TestOverspeed_NilSpeed(t *testing.T) {
	svc, eventRepo, deviceRepo, posRepo, userRepo := setupOverspeedService(t)
	ctx := context.Background()

	user := &model.User{Email: "nilspeed@example.com", PasswordHash: "hash", Name: "Nil Speed"}
	_ = userRepo.Create(ctx, user)

	speedLimit := 60.0
	device := &model.Device{UniqueID: "nilspeed-dev", Name: "Nil Speed Device", Status: "online", SpeedLimit: &speedLimit}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// Position with no speed data.
	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Timestamp: time.Now().UTC(),
	}
	_ = posRepo.Create(ctx, pos)

	err := svc.CheckOverspeed(ctx, pos, device)
	if err != nil {
		t.Fatalf("CheckOverspeed failed: %v", err)
	}

	events, _ := eventRepo.GetByDevice(ctx, device.ID, 100)
	if len(events) != 0 {
		t.Errorf("expected 0 events for nil speed, got %d", len(events))
	}
}

func TestOverspeed_ExactLimit(t *testing.T) {
	svc, eventRepo, deviceRepo, posRepo, userRepo := setupOverspeedService(t)
	ctx := context.Background()

	user := &model.User{Email: "exact@example.com", PasswordHash: "hash", Name: "Exact Limit"}
	_ = userRepo.Create(ctx, user)

	speedLimit := 60.0
	device := &model.Device{UniqueID: "exact-dev", Name: "Exact Device", Status: "online", SpeedLimit: &speedLimit}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// Speed exactly at the limit should NOT trigger an event.
	speed := 60.0
	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Speed:     &speed,
		Timestamp: time.Now().UTC(),
	}
	_ = posRepo.Create(ctx, pos)

	err := svc.CheckOverspeed(ctx, pos, device)
	if err != nil {
		t.Fatalf("CheckOverspeed failed: %v", err)
	}

	events, _ := eventRepo.GetByDevice(ctx, device.ID, 100)
	if len(events) != 0 {
		t.Errorf("expected 0 events for speed exactly at limit, got %d", len(events))
	}
}

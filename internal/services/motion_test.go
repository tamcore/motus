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

func setupMotionService(t *testing.T) (
	*MotionService,
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

	svc := NewMotionService(posRepo, eventRepo, hub, nil)
	return svc, eventRepo, deviceRepo, posRepo, userRepo
}

func TestMotion_StartedMoving(t *testing.T) {
	svc, eventRepo, deviceRepo, posRepo, userRepo := setupMotionService(t)
	ctx := context.Background()

	user := &model.User{Email: "motion@example.com", PasswordHash: "hash", Name: "Motion"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "motion-dev", Name: "Motion Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	now := time.Now().UTC()

	// First position: stationary (speed below threshold).
	slowSpeed := 2.0
	pos1 := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Speed:     &slowSpeed,
		Timestamp: now.Add(-5 * time.Minute),
	}
	_ = posRepo.Create(ctx, pos1)

	// Second position: moving (speed above threshold).
	fastSpeed := 15.0
	pos2 := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.53,
		Longitude: 13.38,
		Speed:     &fastSpeed,
		Timestamp: now,
	}
	_ = posRepo.Create(ctx, pos2)

	err := svc.CheckMotion(ctx, pos2)
	if err != nil {
		t.Fatalf("CheckMotion failed: %v", err)
	}

	events, err := eventRepo.GetByDevice(ctx, device.ID, 100)
	if err != nil {
		t.Fatalf("GetByDevice failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 motion event, got %d", len(events))
	}
	if events[0].Type != "motion" {
		t.Errorf("expected type 'motion', got %q", events[0].Type)
	}
}

func TestMotion_AlreadyMoving(t *testing.T) {
	svc, eventRepo, deviceRepo, posRepo, userRepo := setupMotionService(t)
	ctx := context.Background()

	user := &model.User{Email: "already@example.com", PasswordHash: "hash", Name: "Already Moving"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "already-dev", Name: "Already Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	now := time.Now().UTC()

	// Both positions above threshold: no motion event.
	speed1 := 20.0
	pos1 := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Speed:     &speed1,
		Timestamp: now.Add(-5 * time.Minute),
	}
	_ = posRepo.Create(ctx, pos1)

	speed2 := 25.0
	pos2 := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.53,
		Longitude: 13.38,
		Speed:     &speed2,
		Timestamp: now,
	}
	_ = posRepo.Create(ctx, pos2)

	err := svc.CheckMotion(ctx, pos2)
	if err != nil {
		t.Fatalf("CheckMotion failed: %v", err)
	}

	events, _ := eventRepo.GetByDevice(ctx, device.ID, 100)
	if len(events) != 0 {
		t.Errorf("expected 0 events for already moving device, got %d", len(events))
	}
}

func TestMotion_StillStationary(t *testing.T) {
	svc, eventRepo, deviceRepo, posRepo, userRepo := setupMotionService(t)
	ctx := context.Background()

	user := &model.User{Email: "still@example.com", PasswordHash: "hash", Name: "Still"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "still-dev", Name: "Still Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	now := time.Now().UTC()

	// Both positions below threshold: no event.
	speed1 := 1.0
	pos1 := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Speed:     &speed1,
		Timestamp: now.Add(-5 * time.Minute),
	}
	_ = posRepo.Create(ctx, pos1)

	speed2 := 2.0
	pos2 := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Speed:     &speed2,
		Timestamp: now,
	}
	_ = posRepo.Create(ctx, pos2)

	err := svc.CheckMotion(ctx, pos2)
	if err != nil {
		t.Fatalf("CheckMotion failed: %v", err)
	}

	events, _ := eventRepo.GetByDevice(ctx, device.ID, 100)
	if len(events) != 0 {
		t.Errorf("expected 0 events for stationary device, got %d", len(events))
	}
}

func TestMotion_NoPreviousPosition(t *testing.T) {
	svc, eventRepo, deviceRepo, posRepo, userRepo := setupMotionService(t)
	ctx := context.Background()

	user := &model.User{Email: "noprev@example.com", PasswordHash: "hash", Name: "No Prev"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "noprev-dev", Name: "No Prev Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// First ever position with high speed: no event since no previous.
	speed := 50.0
	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Speed:     &speed,
		Timestamp: time.Now().UTC(),
	}
	_ = posRepo.Create(ctx, pos)

	err := svc.CheckMotion(ctx, pos)
	if err != nil {
		t.Fatalf("CheckMotion failed: %v", err)
	}

	events, _ := eventRepo.GetByDevice(ctx, device.ID, 100)
	if len(events) != 0 {
		t.Errorf("expected 0 events for first position, got %d", len(events))
	}
}

func TestMotion_NilSpeedTreatedAsZero(t *testing.T) {
	svc, eventRepo, deviceRepo, posRepo, userRepo := setupMotionService(t)
	ctx := context.Background()

	user := &model.User{Email: "nilmotion@example.com", PasswordHash: "hash", Name: "Nil Motion"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "nilmotion-dev", Name: "Nil Motion Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	now := time.Now().UTC()

	// Previous position with nil speed (treated as 0).
	pos1 := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Timestamp: now.Add(-5 * time.Minute),
	}
	_ = posRepo.Create(ctx, pos1)

	// Current position with speed above threshold.
	fastSpeed := 10.0
	pos2 := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.53,
		Longitude: 13.38,
		Speed:     &fastSpeed,
		Timestamp: now,
	}
	_ = posRepo.Create(ctx, pos2)

	err := svc.CheckMotion(ctx, pos2)
	if err != nil {
		t.Fatalf("CheckMotion failed: %v", err)
	}

	events, _ := eventRepo.GetByDevice(ctx, device.ID, 100)
	if len(events) != 1 {
		t.Fatalf("expected 1 motion event for nil->fast transition, got %d", len(events))
	}
	if events[0].Type != "motion" {
		t.Errorf("expected type 'motion', got %q", events[0].Type)
	}
}

func TestMotion_ThresholdBoundary(t *testing.T) {
	svc, eventRepo, deviceRepo, posRepo, userRepo := setupMotionService(t)
	ctx := context.Background()

	user := &model.User{Email: "boundary@example.com", PasswordHash: "hash", Name: "Boundary"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "boundary-dev", Name: "Boundary Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	now := time.Now().UTC()

	// Previous position: just below threshold.
	prevSpeed := 4.9
	pos1 := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Speed:     &prevSpeed,
		Timestamp: now.Add(-5 * time.Minute),
	}
	_ = posRepo.Create(ctx, pos1)

	// Current position: exactly at threshold (should trigger).
	exactSpeed := MotionThreshold
	pos2 := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.53,
		Longitude: 13.38,
		Speed:     &exactSpeed,
		Timestamp: now,
	}
	_ = posRepo.Create(ctx, pos2)

	err := svc.CheckMotion(ctx, pos2)
	if err != nil {
		t.Fatalf("CheckMotion failed: %v", err)
	}

	events, _ := eventRepo.GetByDevice(ctx, device.ID, 100)
	if len(events) != 1 {
		t.Fatalf("expected 1 motion event at exact threshold, got %d", len(events))
	}
}

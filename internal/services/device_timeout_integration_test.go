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

func TestDeviceTimeoutService_CheckTimeouts_IntegrationTest(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })

	ctx := context.Background()

	// Create a user and devices.
	user := &model.User{Email: "timeout-integ@example.com", PasswordHash: "hash", Name: "Timeout Integ"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Device 1: online with recent last_seen (should stay online).
	recentTime := time.Now().UTC()
	d1 := &model.Device{UniqueID: "timeout-recent", Name: "Recent Device", Status: "online"}
	if err := deviceRepo.Create(ctx, d1, user.ID); err != nil {
		t.Fatalf("create device 1: %v", err)
	}
	d1.LastUpdate = &recentTime
	if err := deviceRepo.Update(ctx, d1); err != nil {
		t.Fatalf("update device 1: %v", err)
	}

	// Device 2: online with old last_seen (should be marked offline).
	oldTime := time.Now().UTC().Add(-10 * time.Minute)
	d2 := &model.Device{UniqueID: "timeout-old", Name: "Old Device", Status: "online"}
	if err := deviceRepo.Create(ctx, d2, user.ID); err != nil {
		t.Fatalf("create device 2: %v", err)
	}
	d2.LastUpdate = &oldTime
	if err := deviceRepo.Update(ctx, d2); err != nil {
		t.Fatalf("update device 2: %v", err)
	}

	// Device 3: already offline (should be skipped).
	d3 := &model.Device{UniqueID: "timeout-offline", Name: "Already Offline", Status: "offline"}
	if err := deviceRepo.Create(ctx, d3, user.ID); err != nil {
		t.Fatalf("create device 3: %v", err)
	}

	// Create the service with a 5-minute timeout.
	svc := NewDeviceTimeoutService(deviceRepo, hub, 5*time.Minute, 1*time.Minute)

	// Run checkTimeouts.
	if err := svc.checkTimeouts(ctx); err != nil {
		t.Fatalf("checkTimeouts failed: %v", err)
	}

	// Verify: device 1 should still be online.
	got1, err := deviceRepo.GetByID(ctx, d1.ID)
	if err != nil {
		t.Fatalf("get device 1: %v", err)
	}
	if got1.Status != "online" {
		t.Errorf("device 1: expected status 'online', got %q", got1.Status)
	}

	// Verify: device 2 should now be offline.
	got2, err := deviceRepo.GetByID(ctx, d2.ID)
	if err != nil {
		t.Fatalf("get device 2: %v", err)
	}
	if got2.Status != "offline" {
		t.Errorf("device 2: expected status 'offline', got %q", got2.Status)
	}

	// Verify: device 3 should still be offline (unchanged).
	got3, err := deviceRepo.GetByID(ctx, d3.ID)
	if err != nil {
		t.Fatalf("get device 3: %v", err)
	}
	if got3.Status != "offline" {
		t.Errorf("device 3: expected status 'offline', got %q", got3.Status)
	}
}

func TestDeviceTimeoutService_CheckTimeouts_NilLastUpdate(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })

	ctx := context.Background()

	user := &model.User{Email: "timeout-nil@example.com", PasswordHash: "hash", Name: "Nil LastUpdate"}
	_ = userRepo.Create(ctx, user)

	// Device with nil last_seen and status online should be marked offline.
	d := &model.Device{UniqueID: "timeout-nil-ls", Name: "Nil LastUpdate Device", Status: "online"}
	_ = deviceRepo.Create(ctx, d, user.ID)

	svc := NewDeviceTimeoutService(deviceRepo, hub, 5*time.Minute, 1*time.Minute)

	if err := svc.checkTimeouts(ctx); err != nil {
		t.Fatalf("checkTimeouts failed: %v", err)
	}

	got, _ := deviceRepo.GetByID(ctx, d.ID)
	if got.Status != "offline" {
		t.Errorf("expected status 'offline' for nil last_seen, got %q", got.Status)
	}
}

func TestDeviceTimeoutService_Start_CancelsGracefully(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })

	// Use short interval so the ticker fires quickly.
	svc := NewDeviceTimeoutService(deviceRepo, hub, 5*time.Minute, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		svc.Start(ctx)
		close(done)
	}()

	// Let the service run for a bit, then cancel.
	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Service stopped as expected.
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for service to stop")
	}
}

func TestDeviceTimeoutService_CheckTimeouts_MovingDevice(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })

	ctx := context.Background()

	user := &model.User{Email: "timeout-moving@example.com", PasswordHash: "hash", Name: "Timeout Moving"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Device with "moving" status and recent last_update should stay "moving".
	recentTime := time.Now().UTC()
	d1 := &model.Device{UniqueID: "timeout-moving-recent", Name: "Moving Recent", Status: "moving"}
	if err := deviceRepo.Create(ctx, d1, user.ID); err != nil {
		t.Fatalf("create device 1: %v", err)
	}
	d1.LastUpdate = &recentTime
	if err := deviceRepo.Update(ctx, d1); err != nil {
		t.Fatalf("update device 1: %v", err)
	}

	// Device with "moving" status and old last_update should be marked offline.
	oldTime := time.Now().UTC().Add(-10 * time.Minute)
	d2 := &model.Device{UniqueID: "timeout-moving-old", Name: "Moving Old", Status: "moving"}
	if err := deviceRepo.Create(ctx, d2, user.ID); err != nil {
		t.Fatalf("create device 2: %v", err)
	}
	d2.LastUpdate = &oldTime
	if err := deviceRepo.Update(ctx, d2); err != nil {
		t.Fatalf("update device 2: %v", err)
	}

	svc := NewDeviceTimeoutService(deviceRepo, hub, 5*time.Minute, 1*time.Minute)

	if err := svc.checkTimeouts(ctx); err != nil {
		t.Fatalf("checkTimeouts failed: %v", err)
	}

	// Device 1 (recent moving) should still be "moving".
	got1, _ := deviceRepo.GetByID(ctx, d1.ID)
	if got1.Status != "moving" {
		t.Errorf("device 1: expected status 'moving', got %q", got1.Status)
	}

	// Device 2 (old moving) should now be "offline".
	got2, _ := deviceRepo.GetByID(ctx, d2.ID)
	if got2.Status != "offline" {
		t.Errorf("device 2: expected status 'offline', got %q", got2.Status)
	}
}

func TestNewDeviceTimeoutService(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	deviceRepo := repository.NewDeviceRepository(pool)
	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })

	svc := NewDeviceTimeoutService(deviceRepo, hub, 10*time.Minute, 2*time.Minute)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if svc.timeout != 10*time.Minute {
		t.Errorf("expected timeout 10m, got %v", svc.timeout)
	}
	if svc.interval != 2*time.Minute {
		t.Errorf("expected interval 2m, got %v", svc.interval)
	}
}

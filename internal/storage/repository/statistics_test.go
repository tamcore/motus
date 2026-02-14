package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func TestStatisticsRepository_GetPlatformStats_Empty(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewStatisticsRepository(pool)

	stats, err := repo.GetPlatformStats(context.Background())
	if err != nil {
		t.Fatalf("GetPlatformStats failed: %v", err)
	}

	if stats.TotalUsers != 0 {
		t.Errorf("expected TotalUsers=0, got %d", stats.TotalUsers)
	}
	if stats.TotalDevices != 0 {
		t.Errorf("expected TotalDevices=0, got %d", stats.TotalDevices)
	}
	if stats.TotalPositions != 0 {
		t.Errorf("expected TotalPositions=0, got %d", stats.TotalPositions)
	}
	if stats.TotalEvents != 0 {
		t.Errorf("expected TotalEvents=0, got %d", stats.TotalEvents)
	}
	if stats.DevicesByStatus == nil {
		t.Error("expected DevicesByStatus to be non-nil (empty map)")
	}
	if len(stats.DevicesByStatus) != 0 {
		t.Errorf("expected empty DevicesByStatus, got %v", stats.DevicesByStatus)
	}
}

func TestStatisticsRepository_GetPlatformStats_WithData(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	ctx := context.Background()
	userRepo := repository.NewUserRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	posRepo := repository.NewPositionRepository(pool)
	eventRepo := repository.NewEventRepository(pool)
	statsRepo := repository.NewStatisticsRepository(pool)

	// Create 2 users.
	u1 := &model.User{Email: "stats1@example.com", PasswordHash: "hash", Name: "Stats1"}
	u2 := &model.User{Email: "stats2@example.com", PasswordHash: "hash", Name: "Stats2"}
	if err := userRepo.Create(ctx, u1); err != nil {
		t.Fatalf("create user 1: %v", err)
	}
	if err := userRepo.Create(ctx, u2); err != nil {
		t.Fatalf("create user 2: %v", err)
	}

	// Create 3 devices with different statuses.
	d1 := &model.Device{UniqueID: "stats-d1", Name: "D1", Status: "online"}
	d2 := &model.Device{UniqueID: "stats-d2", Name: "D2", Status: "offline"}
	d3 := &model.Device{UniqueID: "stats-d3", Name: "D3", Status: "offline"}
	if err := deviceRepo.Create(ctx, d1, u1.ID); err != nil {
		t.Fatalf("create device 1: %v", err)
	}
	if err := deviceRepo.Create(ctx, d2, u1.ID); err != nil {
		t.Fatalf("create device 2: %v", err)
	}
	if err := deviceRepo.Create(ctx, d3, u2.ID); err != nil {
		t.Fatalf("create device 3: %v", err)
	}

	// Insert 5 positions.
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		pos := &model.Position{
			DeviceID:  d1.ID,
			Latitude:  52.5 + float64(i)*0.01,
			Longitude: 13.4,
			Timestamp: now.Add(-time.Duration(i) * time.Minute),
		}
		if err := posRepo.Create(ctx, pos); err != nil {
			t.Fatalf("create position %d: %v", i, err)
		}
	}

	// Insert 2 events.
	for i := 0; i < 2; i++ {
		e := &model.Event{
			DeviceID:  d1.ID,
			Type:      "geofenceEnter",
			Timestamp: now,
		}
		if err := eventRepo.Create(ctx, e); err != nil {
			t.Fatalf("create event %d: %v", i, err)
		}
	}

	stats, err := statsRepo.GetPlatformStats(ctx)
	if err != nil {
		t.Fatalf("GetPlatformStats failed: %v", err)
	}

	if stats.TotalUsers != 2 {
		t.Errorf("expected TotalUsers=2, got %d", stats.TotalUsers)
	}
	if stats.TotalDevices != 3 {
		t.Errorf("expected TotalDevices=3, got %d", stats.TotalDevices)
	}
	if stats.TotalPositions != 5 {
		t.Errorf("expected TotalPositions=5, got %d", stats.TotalPositions)
	}
	if stats.TotalEvents != 2 {
		t.Errorf("expected TotalEvents=2, got %d", stats.TotalEvents)
	}

	// Verify device status distribution.
	if stats.DevicesByStatus["online"] != 1 {
		t.Errorf("expected 1 online device, got %d", stats.DevicesByStatus["online"])
	}
	if stats.DevicesByStatus["offline"] != 2 {
		t.Errorf("expected 2 offline devices, got %d", stats.DevicesByStatus["offline"])
	}
}

func TestStatisticsRepository_GetUserStats_Empty(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	ctx := context.Background()
	userRepo := repository.NewUserRepository(pool)
	statsRepo := repository.NewStatisticsRepository(pool)

	// Create a user with no devices/positions/events.
	u := &model.User{Email: "usrstats@example.com", PasswordHash: "hash", Name: "UserStats"}
	if err := userRepo.Create(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}

	stats, err := statsRepo.GetUserStats(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserStats failed: %v", err)
	}

	if stats.UserID != u.ID {
		t.Errorf("expected UserID=%d, got %d", u.ID, stats.UserID)
	}
	if stats.DevicesOwned != 0 {
		t.Errorf("expected DevicesOwned=0, got %d", stats.DevicesOwned)
	}
	if stats.TotalPositions != 0 {
		t.Errorf("expected TotalPositions=0, got %d", stats.TotalPositions)
	}
	if stats.LastLogin != nil {
		t.Errorf("expected LastLogin=nil (no sessions), got %v", stats.LastLogin)
	}
	if stats.EventsTriggered != 0 {
		t.Errorf("expected EventsTriggered=0, got %d", stats.EventsTriggered)
	}
	if stats.GeofencesOwned != 0 {
		t.Errorf("expected GeofencesOwned=0, got %d", stats.GeofencesOwned)
	}
}

func TestStatisticsRepository_GetUserStats_WithData(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	ctx := context.Background()
	userRepo := repository.NewUserRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	posRepo := repository.NewPositionRepository(pool)
	eventRepo := repository.NewEventRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	statsRepo := repository.NewStatisticsRepository(pool)

	// Create a user.
	u := &model.User{Email: "fullstats@example.com", PasswordHash: "hash", Name: "FullStats"}
	if err := userRepo.Create(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Create 2 devices assigned to the user.
	d1 := &model.Device{UniqueID: "full-d1", Name: "Full D1", Status: "online"}
	d2 := &model.Device{UniqueID: "full-d2", Name: "Full D2", Status: "offline"}
	if err := deviceRepo.Create(ctx, d1, u.ID); err != nil {
		t.Fatalf("create device 1: %v", err)
	}
	if err := deviceRepo.Create(ctx, d2, u.ID); err != nil {
		t.Fatalf("create device 2: %v", err)
	}

	// Insert 5 positions for d1.
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		pos := &model.Position{
			DeviceID:  d1.ID,
			Latitude:  52.5,
			Longitude: 13.4,
			Timestamp: now.Add(-time.Duration(i) * time.Minute),
		}
		if err := posRepo.Create(ctx, pos); err != nil {
			t.Fatalf("create position %d: %v", i, err)
		}
	}

	// Insert 3 events for d1.
	for i := 0; i < 3; i++ {
		e := &model.Event{
			DeviceID:  d1.ID,
			Type:      "geofenceEnter",
			Timestamp: now,
		}
		if err := eventRepo.Create(ctx, e); err != nil {
			t.Fatalf("create event %d: %v", i, err)
		}
	}

	// Create a session (LastLogin should be set).
	if _, err := sessionRepo.Create(ctx, u.ID); err != nil {
		t.Fatalf("create session: %v", err)
	}

	stats, err := statsRepo.GetUserStats(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserStats failed: %v", err)
	}

	if stats.DevicesOwned != 2 {
		t.Errorf("expected DevicesOwned=2, got %d", stats.DevicesOwned)
	}
	if stats.TotalPositions != 5 {
		t.Errorf("expected TotalPositions=5, got %d", stats.TotalPositions)
	}
	if stats.EventsTriggered != 3 {
		t.Errorf("expected EventsTriggered=3, got %d", stats.EventsTriggered)
	}
	if stats.LastLogin == nil {
		t.Error("expected LastLogin to be set after creating a session")
	}
}

func TestStatisticsRepository_GetUserStats_NonExistentUser(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	statsRepo := repository.NewStatisticsRepository(pool)

	// Non-existent user ID — should return zeros (not an error).
	stats, err := statsRepo.GetUserStats(context.Background(), 99999)
	if err != nil {
		t.Fatalf("GetUserStats returned unexpected error: %v", err)
	}
	if stats.DevicesOwned != 0 {
		t.Errorf("expected DevicesOwned=0 for non-existent user, got %d", stats.DevicesOwned)
	}
	if stats.TotalPositions != 0 {
		t.Errorf("expected TotalPositions=0 for non-existent user, got %d", stats.TotalPositions)
	}
}

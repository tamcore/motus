package repository_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func createTestUser(t *testing.T, repo *repository.UserRepository) *model.User {
	t.Helper()
	user := &model.User{
		Email:        "device-test-" + time.Now().Format("20060102150405.000000000") + "@example.com",
		PasswordHash: "$2a$10$fakehash",
		Name:         "Device Test User",
	}
	if err := repo.Create(context.Background(), user); err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	return user
}

func TestDeviceRepository_Create(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := createTestUser(t, userRepo)

	device := &model.Device{
		UniqueID: "test-device-001",
		Name:     "Test Device",
		Protocol: "h02",
		Status:   "unknown",
	}

	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if device.ID == 0 {
		t.Error("expected device ID to be set")
	}
	if device.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestDeviceRepository_Create_DuplicateUniqueID(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := createTestUser(t, userRepo)

	d1 := &model.Device{UniqueID: "dup-001", Name: "Device 1", Status: "unknown"}
	d2 := &model.Device{UniqueID: "dup-001", Name: "Device 2", Status: "unknown"}

	if err := deviceRepo.Create(ctx, d1, user.ID); err != nil {
		t.Fatalf("first Create failed: %v", err)
	}
	if err := deviceRepo.Create(ctx, d2, user.ID); err == nil {
		t.Error("expected error for duplicate unique_id, got nil")
	}
}

func TestDeviceRepository_GetByID(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := createTestUser(t, userRepo)
	device := &model.Device{UniqueID: "getbyid-001", Name: "Get By ID", Protocol: "watch", Status: "unknown"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	found, err := deviceRepo.GetByID(ctx, device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found.UniqueID != "getbyid-001" {
		t.Errorf("expected uniqueId 'getbyid-001', got %q", found.UniqueID)
	}
	if found.Protocol != "watch" {
		t.Errorf("expected protocol 'watch', got %q", found.Protocol)
	}
}

func TestDeviceRepository_GetByID_NotFound(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	ctx := context.Background()

	_, err := deviceRepo.GetByID(ctx, 99999)
	if err == nil {
		t.Error("expected error for nonexistent ID")
	}
}

func TestDeviceRepository_GetByUniqueID(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := createTestUser(t, userRepo)
	device := &model.Device{UniqueID: "unique-find-001", Name: "Unique Find", Status: "unknown"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	found, err := deviceRepo.GetByUniqueID(ctx, "unique-find-001")
	if err != nil {
		t.Fatalf("GetByUniqueID failed: %v", err)
	}
	if found.ID != device.ID {
		t.Errorf("expected ID %d, got %d", device.ID, found.ID)
	}
}

func TestDeviceRepository_GetByUser(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := createTestUser(t, userRepo)

	d1 := &model.Device{UniqueID: "user-dev-1", Name: "Alpha Device", Status: "unknown"}
	d2 := &model.Device{UniqueID: "user-dev-2", Name: "Beta Device", Status: "unknown"}

	if err := deviceRepo.Create(ctx, d1, user.ID); err != nil {
		t.Fatalf("Create d1 failed: %v", err)
	}
	if err := deviceRepo.Create(ctx, d2, user.ID); err != nil {
		t.Fatalf("Create d2 failed: %v", err)
	}

	devices, err := deviceRepo.GetByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByUser failed: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
	// Should be ordered by name.
	if devices[0].Name != "Alpha Device" {
		t.Errorf("expected first device 'Alpha Device', got %q", devices[0].Name)
	}
}

func TestDeviceRepository_GetAll(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := createTestUser(t, userRepo)

	d1 := &model.Device{UniqueID: "all-1", Name: "Device A", Status: "online"}
	d2 := &model.Device{UniqueID: "all-2", Name: "Device B", Status: "offline"}
	if err := deviceRepo.Create(ctx, d1, user.ID); err != nil {
		t.Fatalf("Create d1 failed: %v", err)
	}
	if err := deviceRepo.Create(ctx, d2, user.ID); err != nil {
		t.Fatalf("Create d2 failed: %v", err)
	}

	devices, err := deviceRepo.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(devices) != 2 {
		t.Errorf("expected 2 devices, got %d", len(devices))
	}
}

func TestDeviceRepository_GetUserIDs(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := createTestUser(t, userRepo)
	device := &model.Device{UniqueID: "userid-dev", Name: "UserID Device", Status: "unknown"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	ids, err := deviceRepo.GetUserIDs(ctx, device.ID)
	if err != nil {
		t.Fatalf("GetUserIDs failed: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 user ID, got %d", len(ids))
	}
	if ids[0] != user.ID {
		t.Errorf("expected user ID %d, got %d", user.ID, ids[0])
	}
}

func TestDeviceRepository_UserHasAccess(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := createTestUser(t, userRepo)
	device := &model.Device{UniqueID: "access-dev", Name: "Access Device", Status: "unknown"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if !deviceRepo.UserHasAccess(ctx, &model.User{ID: user.ID}, device.ID) {
		t.Error("expected user to have access to device")
	}
	if deviceRepo.UserHasAccess(ctx, &model.User{ID: user.ID + 1}, device.ID) {
		t.Error("expected other user to NOT have access")
	}
}

func TestDeviceRepository_Update(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := createTestUser(t, userRepo)
	device := &model.Device{UniqueID: "update-dev", Name: "Before Update", Status: "unknown"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	now := time.Now().UTC()
	device.Name = "After Update"
	device.Status = "online"
	device.LastUpdate = &now

	if err := deviceRepo.Update(ctx, device); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	found, err := deviceRepo.GetByID(ctx, device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found.Name != "After Update" {
		t.Errorf("expected name 'After Update', got %q", found.Name)
	}
	if found.Status != "online" {
		t.Errorf("expected status 'online', got %q", found.Status)
	}
	if found.LastUpdate == nil {
		t.Error("expected LastUpdate to be set")
	}
}

func TestDeviceRepository_Delete(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := createTestUser(t, userRepo)
	device := &model.Device{UniqueID: "delete-dev", Name: "Delete Me", Status: "unknown"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := deviceRepo.Delete(ctx, device.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := deviceRepo.GetByID(ctx, device.ID)
	if err == nil {
		t.Error("expected error after deletion, got nil")
	}
}

func TestDeviceRepository_UpdateProtocol(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := createTestUser(t, userRepo)
	device := &model.Device{UniqueID: "proto-dev", Name: "Proto Device", Protocol: "watch", Status: "unknown"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := deviceRepo.UpdateProtocol(ctx, device.ID, "h02"); err != nil {
		t.Fatalf("UpdateProtocol failed: %v", err)
	}

	found, err := deviceRepo.GetByID(ctx, device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found.Protocol != "h02" {
		t.Errorf("Protocol: got %q, want %q", found.Protocol, "h02")
	}
	// Verify other fields are untouched.
	if found.Name != "Proto Device" {
		t.Errorf("Name changed unexpectedly: got %q", found.Name)
	}
	if found.Status != "unknown" {
		t.Errorf("Status changed unexpectedly: got %q", found.Status)
	}
}

func TestDeviceRepository_UpdateProtocol_Clear(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := createTestUser(t, userRepo)
	device := &model.Device{UniqueID: "proto-clear-dev", Name: "Clear Proto", Protocol: "h02", Status: "unknown"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := deviceRepo.UpdateProtocol(ctx, device.ID, ""); err != nil {
		t.Fatalf("UpdateProtocol to empty failed: %v", err)
	}

	found, err := deviceRepo.GetByID(ctx, device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found.Protocol != "" {
		t.Errorf("Protocol: got %q, want empty", found.Protocol)
	}
}

func TestDeviceRepository_MarkOnline(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := createTestUser(t, userRepo)
	device := &model.Device{UniqueID: "mark-online", Name: "Before", Status: "unknown", Disabled: true}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("Create: %v", err)
	}
	stale := *device
	stale.Name = "Renamed"
	if err := deviceRepo.Update(ctx, &stale); err != nil {
		t.Fatalf("Update: %v", err)
	}

	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	pos := &model.Position{DeviceID: device.ID, Timestamp: at, Valid: true, Latitude: 52.5, Longitude: 13.4}
	if err := repository.NewPositionRepository(pool).Create(ctx, pos); err != nil {
		t.Fatalf("Create position: %v", err)
	}
	got, err := deviceRepo.MarkOnline(ctx, device.ID, pos.ID, at, nil)
	if err != nil {
		t.Fatalf("MarkOnline: %v", err)
	}
	if got.Status != "online" || got.Disabled || got.LastUpdate == nil || !got.LastUpdate.Equal(at) {
		t.Errorf("MarkOnline = status %q disabled %v lastUpdate %v, want online, false, %v", got.Status, got.Disabled, got.LastUpdate, at)
	}
	if got.PositionID == nil || *got.PositionID != pos.ID {
		t.Errorf("PositionID = %v, want %d", got.PositionID, pos.ID)
	}
	if got.Name != "Renamed" {
		t.Errorf("Name = %q, want concurrent rename kept", got.Name)
	}
}

func TestDeviceRepository_MarkOnline_BatteryLevel(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	posRepo := repository.NewPositionRepository(pool)
	ctx := t.Context()

	user := createTestUser(t, repository.NewUserRepository(pool))
	device := &model.Device{UniqueID: "battery-level", Name: "Battery", Status: "unknown"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("Create: %v", err)
	}

	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	markOnline := func(battery *float64) *model.Device {
		t.Helper()
		pos := &model.Position{DeviceID: device.ID, Timestamp: at, Valid: true, Latitude: 52.5, Longitude: 13.4}
		if err := posRepo.Create(ctx, pos); err != nil {
			t.Fatalf("Create position: %v", err)
		}
		got, err := deviceRepo.MarkOnline(ctx, device.ID, pos.ID, at, battery)
		if err != nil {
			t.Fatalf("MarkOnline: %v", err)
		}
		return got
	}

	got := markOnline(new(42.0))
	if got.BatteryLevel == nil || *got.BatteryLevel != 42 {
		t.Fatalf("BatteryLevel = %v, want 42", got.BatteryLevel)
	}

	// A position without a battery reading keeps the last known level.
	got = markOnline(nil)
	if got.BatteryLevel == nil || *got.BatteryLevel != 42 {
		t.Errorf("BatteryLevel after position without battery = %v, want 42 kept", got.BatteryLevel)
	}

	// A user edit (full Update) must not clobber the protocol-reported level.
	got.Name = "Renamed"
	got.BatteryLevel = nil
	if err := deviceRepo.Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}

	devices, err := deviceRepo.GetByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByUser: %v", err)
	}
	if len(devices) != 1 || devices[0].BatteryLevel == nil || *devices[0].BatteryLevel != 42 {
		t.Errorf("GetByUser BatteryLevel = %+v, want 42", devices)
	}
	all, err := deviceRepo.GetAllWithOwners(ctx)
	if err != nil {
		t.Fatalf("GetAllWithOwners: %v", err)
	}
	if len(all) != 1 || all[0].BatteryLevel == nil || *all[0].BatteryLevel != 42 {
		t.Errorf("GetAllWithOwners BatteryLevel = %+v, want 42", all)
	}

	got = markOnline(new(15.0))
	if got.BatteryLevel == nil || *got.BatteryLevel != 15 {
		t.Errorf("BatteryLevel = %v, want 15", got.BatteryLevel)
	}
}

func TestDeviceRepository_SetIgnitionState(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	ctx := context.Background()
	user := createTestUser(t, repository.NewUserRepository(pool))
	device := &model.Device{UniqueID: "ignition-atomic", Name: "Ign", Status: "online"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("Create: %v", err)
	}

	t0 := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	steps := []struct {
		name        string
		on          bool
		ts          time.Time
		wantChanged bool
		wantOn      bool
		wantLast    time.Time
	}{
		{"off while off writes nothing", false, t0, false, false, time.Time{}},
		{"on is a change", true, t0, true, true, t0},
		{"on again refreshes the time", true, t0.Add(time.Minute), false, true, t0.Add(time.Minute)},
		{"older position is ignored", false, t0, false, true, t0.Add(time.Minute)},
		{"off is a change", false, t0.Add(2 * time.Minute), true, false, t0.Add(2 * time.Minute)},
		{"off again writes nothing", false, t0.Add(3 * time.Minute), false, false, t0.Add(2 * time.Minute)},
	}
	for _, s := range steps {
		changed, err := deviceRepo.SetIgnitionState(ctx, device.ID, s.on, s.ts)
		if err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		got, err := deviceRepo.GetByID(ctx, device.ID)
		if err != nil {
			t.Fatalf("%s: GetByID: %v", s.name, err)
		}
		var last time.Time
		if got.LastIgnitionTime != nil {
			last = got.LastIgnitionTime.UTC()
		}
		if changed != s.wantChanged || got.IgnitionOn != s.wantOn || !last.Equal(s.wantLast) {
			t.Errorf("%s: changed=%v on=%v last=%v, want %v %v %v", s.name, changed, got.IgnitionOn, last, s.wantChanged, s.wantOn, s.wantLast)
		}
	}
}

func TestDeviceRepository_SetIgnitionState_ConcurrentReportsChangeOnce(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	ctx := context.Background()
	user := createTestUser(t, repository.NewUserRepository(pool))
	device := &model.Device{UniqueID: "ignition-race", Name: "Ign", Status: "online"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("Create: %v", err)
	}

	ts := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	var changes atomic.Int32
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			changed, err := deviceRepo.SetIgnitionState(ctx, device.ID, true, ts)
			if err != nil {
				t.Errorf("SetIgnitionState: %v", err)
			}
			if changed {
				changes.Add(1)
			}
		})
	}
	wg.Wait()
	if n := changes.Load(); n != 1 {
		t.Errorf("concurrent off→on reported %d changes, want exactly 1", n)
	}
}

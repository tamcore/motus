package repository_test

import (
	"context"
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

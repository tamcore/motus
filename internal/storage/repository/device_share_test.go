package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func setupShareTest(t *testing.T) (*repository.DeviceShareRepository, *repository.UserRepository, *repository.DeviceRepository) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	return repository.NewDeviceShareRepository(pool),
		repository.NewUserRepository(pool),
		repository.NewDeviceRepository(pool)
}

func TestDeviceShareRepository_Create(t *testing.T) {
	shareRepo, userRepo, deviceRepo := setupShareTest(t)
	ctx := context.Background()

	user := &model.User{Email: "share-create@example.com", PasswordHash: "hash", Name: "Share Create"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	device := &model.Device{UniqueID: "share-dev-001", Name: "Share Dev", Status: "online"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	share := &model.DeviceShare{DeviceID: device.ID, CreatedBy: user.ID}
	if err := shareRepo.Create(ctx, share); err != nil {
		t.Fatalf("Create share failed: %v", err)
	}
	if share.ID == 0 {
		t.Error("expected non-zero share ID")
	}
	if share.Token == "" {
		t.Error("expected non-empty token")
	}
	if share.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestDeviceShareRepository_GetByToken(t *testing.T) {
	shareRepo, userRepo, deviceRepo := setupShareTest(t)
	ctx := context.Background()

	user := &model.User{Email: "share-get@example.com", PasswordHash: "hash", Name: "Share Get"}
	_ = userRepo.Create(ctx, user)
	device := &model.Device{UniqueID: "share-dev-002", Name: "Share Dev 2", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	share := &model.DeviceShare{DeviceID: device.ID, CreatedBy: user.ID}
	_ = shareRepo.Create(ctx, share)

	found, err := shareRepo.GetByToken(ctx, share.Token)
	if err != nil {
		t.Fatalf("GetByToken failed: %v", err)
	}
	if found.DeviceID != device.ID {
		t.Errorf("expected device ID %d, got %d", device.ID, found.DeviceID)
	}
}

func TestDeviceShareRepository_GetByToken_Expired(t *testing.T) {
	shareRepo, userRepo, deviceRepo := setupShareTest(t)
	ctx := context.Background()

	user := &model.User{Email: "share-exp@example.com", PasswordHash: "hash", Name: "Share Exp"}
	_ = userRepo.Create(ctx, user)
	device := &model.Device{UniqueID: "share-dev-003", Name: "Share Dev 3", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	expired := time.Now().Add(-1 * time.Hour)
	share := &model.DeviceShare{DeviceID: device.ID, CreatedBy: user.ID, ExpiresAt: &expired}
	_ = shareRepo.Create(ctx, share)

	_, err := shareRepo.GetByToken(ctx, share.Token)
	if err == nil {
		t.Error("expected error for expired token, got nil")
	}
}

func TestDeviceShareRepository_ListByDevice(t *testing.T) {
	shareRepo, userRepo, deviceRepo := setupShareTest(t)
	ctx := context.Background()

	user := &model.User{Email: "share-list@example.com", PasswordHash: "hash", Name: "Share List"}
	_ = userRepo.Create(ctx, user)
	device := &model.Device{UniqueID: "share-dev-004", Name: "Share Dev 4", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	for i := 0; i < 3; i++ {
		share := &model.DeviceShare{DeviceID: device.ID, CreatedBy: user.ID}
		_ = shareRepo.Create(ctx, share)
	}

	shares, err := shareRepo.ListByDevice(ctx, device.ID)
	if err != nil {
		t.Fatalf("ListByDevice failed: %v", err)
	}
	if len(shares) != 3 {
		t.Errorf("expected 3 shares, got %d", len(shares))
	}
}

func TestDeviceShareRepository_GetByID(t *testing.T) {
	shareRepo, userRepo, deviceRepo := setupShareTest(t)
	ctx := context.Background()

	user := &model.User{Email: "share-getid@example.com", PasswordHash: "hash", Name: "Share GetID"}
	_ = userRepo.Create(ctx, user)
	device := &model.Device{UniqueID: "share-dev-getid", Name: "Share Dev GetID", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	share := &model.DeviceShare{DeviceID: device.ID, CreatedBy: user.ID}
	_ = shareRepo.Create(ctx, share)

	found, err := shareRepo.GetByID(ctx, share.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found.ID != share.ID {
		t.Errorf("expected share ID %d, got %d", share.ID, found.ID)
	}
	if found.DeviceID != device.ID {
		t.Errorf("expected device ID %d, got %d", device.ID, found.DeviceID)
	}
	if found.Token != share.Token {
		t.Errorf("expected token %q, got %q", share.Token, found.Token)
	}
}

func TestDeviceShareRepository_GetByID_NotFound(t *testing.T) {
	shareRepo, _, _ := setupShareTest(t)
	ctx := context.Background()

	_, err := shareRepo.GetByID(ctx, 99999)
	if err == nil {
		t.Error("expected error for nonexistent share ID, got nil")
	}
}

func TestDeviceShareRepository_Delete(t *testing.T) {
	shareRepo, userRepo, deviceRepo := setupShareTest(t)
	ctx := context.Background()

	user := &model.User{Email: "share-del@example.com", PasswordHash: "hash", Name: "Share Del"}
	_ = userRepo.Create(ctx, user)
	device := &model.Device{UniqueID: "share-dev-005", Name: "Share Dev 5", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	share := &model.DeviceShare{DeviceID: device.ID, CreatedBy: user.ID}
	_ = shareRepo.Create(ctx, share)

	if err := shareRepo.Delete(ctx, share.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := shareRepo.GetByToken(ctx, share.Token)
	if err == nil {
		t.Error("expected error after deletion, got nil")
	}
}

func TestDeviceShareRepository_GetByToken_NotFound(t *testing.T) {
	shareRepo, _, _ := setupShareTest(t)
	ctx := context.Background()

	_, err := shareRepo.GetByToken(ctx, "nonexistent-token")
	if err == nil {
		t.Error("expected error for nonexistent token, got nil")
	}
}

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
	shareRepo, _, _ := setupShareTest(t)
	ctx := context.Background()

	user := testutil.CreateUser(t, "share-create@example.com")
	device := testutil.CreateDevice(t, user.ID, "share-dev-001")

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
	shareRepo, _, _ := setupShareTest(t)
	ctx := context.Background()

	user := testutil.CreateUser(t, "share-get@example.com")
	device := testutil.CreateDevice(t, user.ID, "share-dev-002")

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
	shareRepo, _, _ := setupShareTest(t)
	ctx := context.Background()

	user := testutil.CreateUser(t, "share-exp@example.com")
	device := testutil.CreateDevice(t, user.ID, "share-dev-003")

	expired := time.Now().Add(-1 * time.Hour)
	share := &model.DeviceShare{DeviceID: device.ID, CreatedBy: user.ID, ExpiresAt: &expired}
	_ = shareRepo.Create(ctx, share)

	_, err := shareRepo.GetByToken(ctx, share.Token)
	if err == nil {
		t.Error("expected error for expired token, got nil")
	}
}

func TestDeviceShareRepository_ListByDevice(t *testing.T) {
	shareRepo, _, _ := setupShareTest(t)
	ctx := context.Background()

	user := testutil.CreateUser(t, "share-list@example.com")
	device := testutil.CreateDevice(t, user.ID, "share-dev-004")

	for range 3 {
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
	shareRepo, _, _ := setupShareTest(t)
	ctx := context.Background()

	user := testutil.CreateUser(t, "share-getid@example.com")
	device := testutil.CreateDevice(t, user.ID, "share-dev-getid")

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
	shareRepo, _, _ := setupShareTest(t)
	ctx := context.Background()

	user := testutil.CreateUser(t, "share-del@example.com")
	device := testutil.CreateDevice(t, user.ID, "share-dev-005")

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

package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

type bookmarkFixture struct {
	pool    *pgxpool.Pool
	repo    *repository.TrailBookmarkRepository
	users   *repository.UserRepository
	devices *repository.DeviceRepository
	owner   *model.User
	device  *model.Device
}

func setupBookmarkFixture(t *testing.T) *bookmarkFixture {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	f := &bookmarkFixture{
		pool:    pool,
		repo:    repository.NewTrailBookmarkRepository(pool),
		users:   repository.NewUserRepository(pool),
		devices: repository.NewDeviceRepository(pool),
	}
	f.owner = &model.User{Email: "hiker@example.com", PasswordHash: "hash", Name: "Hiker", Role: model.RoleUser}
	if err := f.users.Create(ctx, f.owner); err != nil {
		t.Fatalf("create user: %v", err)
	}
	f.device = &model.Device{UniqueID: "bm-dev-1", Name: "Backpack", Status: "offline"}
	if err := f.devices.Create(ctx, f.device, f.owner.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}
	return f
}

func (f *bookmarkFixture) create(t *testing.T, userID, deviceID int64, name string, from time.Time) *model.TrailBookmark {
	t.Helper()
	b := &model.TrailBookmark{
		UserID:      userID,
		DeviceID:    deviceID,
		Name:        name,
		Description: "desc " + name,
		From:        from,
		To:          from.Add(3 * time.Hour),
	}
	if err := f.repo.Create(context.Background(), b); err != nil {
		t.Fatalf("create bookmark %q: %v", name, err)
	}
	return b
}

func TestTrailBookmarkRepository_CreateAndGet(t *testing.T) {
	f := setupBookmarkFixture(t)
	ctx := context.Background()
	from := time.Date(2026, 6, 6, 8, 0, 0, 0, time.UTC)

	b := f.create(t, f.owner.ID, f.device.ID, "Zugspitze", from)
	if b.ID == 0 || b.CreatedAt.IsZero() || b.UpdatedAt.IsZero() {
		t.Fatalf("expected id and timestamps to be set, got %+v", b)
	}

	got, err := f.repo.GetByID(ctx, b.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Zugspitze" || got.Description != "desc Zugspitze" {
		t.Errorf("unexpected name/description: %q / %q", got.Name, got.Description)
	}
	if !got.From.Equal(from) || !got.To.Equal(from.Add(3*time.Hour)) {
		t.Errorf("unexpected range: %v – %v", got.From, got.To)
	}
	if got.UserID != f.owner.ID || got.DeviceID != f.device.ID {
		t.Errorf("unexpected owner/device: %d / %d", got.UserID, got.DeviceID)
	}
	if got.DeviceName != "Backpack" {
		t.Errorf("DeviceName = %q, want Backpack", got.DeviceName)
	}
}

func TestTrailBookmarkRepository_GetByID_NotFound(t *testing.T) {
	f := setupBookmarkFixture(t)
	_, err := f.repo.GetByID(context.Background(), 999999)
	if !errors.Is(err, repository.ErrTrailBookmarkNotFound) {
		t.Fatalf("expected ErrTrailBookmarkNotFound, got %v", err)
	}
}

func TestTrailBookmarkRepository_CreateRejectsInvertedRange(t *testing.T) {
	f := setupBookmarkFixture(t)
	from := time.Date(2026, 6, 6, 8, 0, 0, 0, time.UTC)
	b := &model.TrailBookmark{
		UserID: f.owner.ID, DeviceID: f.device.ID, Name: "Backwards",
		From: from, To: from.Add(-time.Hour),
	}
	if err := f.repo.Create(context.Background(), b); err == nil {
		t.Fatal("expected check constraint violation for to <= from")
	}
}

func TestTrailBookmarkRepository_ListForUser(t *testing.T) {
	f := setupBookmarkFixture(t)
	ctx := context.Background()

	other := &model.User{Email: "other@example.com", PasswordHash: "hash", Name: "Other", Role: model.RoleUser}
	if err := f.users.Create(ctx, other); err != nil {
		t.Fatalf("create other user: %v", err)
	}
	dev2 := &model.Device{UniqueID: "bm-dev-2", Name: "Bike", Status: "offline"}
	if err := f.devices.Create(ctx, dev2, f.owner.ID); err != nil {
		t.Fatalf("create device 2: %v", err)
	}
	// The other user also has access to the device but their bookmark is private.
	if err := f.users.AssignDevice(ctx, other.ID, f.device.ID); err != nil {
		t.Fatalf("assign device: %v", err)
	}

	base := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	older := f.create(t, f.owner.ID, f.device.ID, "Older", base)
	newer := f.create(t, f.owner.ID, f.device.ID, "Newer", base.Add(48*time.Hour))
	bike := f.create(t, f.owner.ID, dev2.ID, "Ride", base.Add(24*time.Hour))
	f.create(t, other.ID, f.device.ID, "Not mine", base)

	all, err := f.repo.ListForUser(ctx, f.owner, nil)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	wantOrder := []int64{newer.ID, bike.ID, older.ID}
	if len(all) != len(wantOrder) {
		t.Fatalf("expected %d bookmarks, got %d", len(wantOrder), len(all))
	}
	for i, id := range wantOrder {
		if all[i].ID != id {
			t.Errorf("position %d: got id %d, want %d (newest range first)", i, all[i].ID, id)
		}
	}

	devID := dev2.ID
	byDevice, err := f.repo.ListForUser(ctx, f.owner, &devID)
	if err != nil {
		t.Fatalf("ListForUser(device): %v", err)
	}
	if len(byDevice) != 1 || byDevice[0].ID != bike.ID || byDevice[0].DeviceName != "Bike" {
		t.Errorf("unexpected device-filtered result: %+v", byDevice)
	}
}

func TestTrailBookmarkRepository_ListForUser_HidesUnassignedDevices(t *testing.T) {
	f := setupBookmarkFixture(t)
	ctx := context.Background()
	f.create(t, f.owner.ID, f.device.ID, "Hike", time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC))

	if err := f.users.UnassignDevice(ctx, f.owner.ID, f.device.ID); err != nil {
		t.Fatalf("unassign: %v", err)
	}
	list, err := f.repo.ListForUser(ctx, f.owner, nil)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected bookmarks of inaccessible devices to be hidden, got %d", len(list))
	}

	// Admins keep seeing their own bookmarks regardless of user_devices.
	admin := &model.User{ID: f.owner.ID, Role: model.RoleAdmin}
	list, err = f.repo.ListForUser(ctx, admin, nil)
	if err != nil {
		t.Fatalf("ListForUser(admin): %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected admin to see own bookmark, got %d", len(list))
	}
}

func TestTrailBookmarkRepository_Update(t *testing.T) {
	f := setupBookmarkFixture(t)
	ctx := context.Background()
	b := f.create(t, f.owner.ID, f.device.ID, "Draft", time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC))
	createdUpdatedAt := b.UpdatedAt

	b.Name = "Final"
	b.Description = ""
	b.From = time.Date(2026, 5, 2, 7, 0, 0, 0, time.UTC)
	b.To = time.Date(2026, 5, 2, 17, 30, 0, 0, time.UTC)
	if err := f.repo.Update(ctx, b); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := f.repo.GetByID(ctx, b.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Final" || got.Description != "" {
		t.Errorf("unexpected name/description after update: %q / %q", got.Name, got.Description)
	}
	if !got.From.Equal(b.From) || !got.To.Equal(b.To) {
		t.Errorf("unexpected range after update: %v – %v", got.From, got.To)
	}
	if got.UpdatedAt.Before(createdUpdatedAt) {
		t.Errorf("expected updated_at to advance")
	}
}

func TestTrailBookmarkRepository_Delete(t *testing.T) {
	f := setupBookmarkFixture(t)
	ctx := context.Background()
	b := f.create(t, f.owner.ID, f.device.ID, "Gone", time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC))

	if err := f.repo.Delete(ctx, b.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := f.repo.GetByID(ctx, b.ID); err == nil {
		t.Fatal("expected bookmark to be deleted")
	}
}

func TestTrailBookmarkRepository_CascadeOnUserAndDeviceDelete(t *testing.T) {
	f := setupBookmarkFixture(t)
	ctx := context.Background()
	from := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)

	byDevice := f.create(t, f.owner.ID, f.device.ID, "Device gone", from)
	if err := f.devices.Delete(ctx, f.device.ID); err != nil {
		t.Fatalf("delete device: %v", err)
	}
	if _, err := f.repo.GetByID(ctx, byDevice.ID); err == nil {
		t.Error("expected bookmark to be removed with its device")
	}

	dev2 := testutil.CreateDevice(t, f.owner.ID, "bm-dev-3")
	byUser := f.create(t, f.owner.ID, dev2.ID, "User gone", from)
	if err := f.users.Delete(ctx, f.owner.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if _, err := f.repo.GetByID(ctx, byUser.ID); err == nil {
		t.Error("expected bookmark to be removed with its user")
	}
}

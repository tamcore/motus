package repository_test

import (
	"context"
	"testing"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func TestUserRepository_Create(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{
		Email:        "test@example.com",
		PasswordHash: "$2a$10$fakehash",
		Name:         "Test User",
	}

	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if user.ID == 0 {
		t.Error("expected user ID to be set after Create")
	}
	if user.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set after Create")
	}
}

func TestUserRepository_Create_DuplicateEmail(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user1 := &model.User{Email: "dup@example.com", PasswordHash: "hash1", Name: "User 1"}
	user2 := &model.User{Email: "dup@example.com", PasswordHash: "hash2", Name: "User 2"}

	if err := repo.Create(ctx, user1); err != nil {
		t.Fatalf("first Create failed: %v", err)
	}

	if err := repo.Create(ctx, user2); err == nil {
		t.Error("expected error for duplicate email, got nil")
	}
}

func TestUserRepository_GetByEmail(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "find@example.com", PasswordHash: "hash", Name: "Findable"}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	found, err := repo.GetByEmail(ctx, "find@example.com")
	if err != nil {
		t.Fatalf("GetByEmail failed: %v", err)
	}
	if found.ID != user.ID {
		t.Errorf("expected ID %d, got %d", user.ID, found.ID)
	}
	if found.Name != "Findable" {
		t.Errorf("expected name 'Findable', got %q", found.Name)
	}
}

func TestUserRepository_GetByEmail_NotFound(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	_, err := repo.GetByEmail(ctx, "nonexistent@example.com")
	if err == nil {
		t.Error("expected error for nonexistent email, got nil")
	}
}

func TestUserRepository_GetByID(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "byid@example.com", PasswordHash: "hash", Name: "ByID"}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	found, err := repo.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found.Email != "byid@example.com" {
		t.Errorf("expected email 'byid@example.com', got %q", found.Email)
	}
}

func TestUserRepository_GetByID_NotFound(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	_, err := repo.GetByID(ctx, 99999)
	if err == nil {
		t.Error("expected error for nonexistent ID, got nil")
	}
}

func TestUserRepository_GenerateToken(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "token@example.com", PasswordHash: "hash", Name: "Token User"}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	token, err := repo.GenerateToken(ctx, user.ID)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}
	// Token should be 64 hex characters (32 bytes).
	if len(token) != 64 {
		t.Errorf("expected token length 64, got %d", len(token))
	}
}

func TestUserRepository_GetByToken(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "tokenget@example.com", PasswordHash: "hash", Name: "Token Get"}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	token, err := repo.GenerateToken(ctx, user.ID)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	found, err := repo.GetByToken(ctx, token)
	if err != nil {
		t.Fatalf("GetByToken failed: %v", err)
	}
	if found.ID != user.ID {
		t.Errorf("expected user ID %d, got %d", user.ID, found.ID)
	}
}

func TestUserRepository_GetByToken_NotFound(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	_, err := repo.GetByToken(ctx, "nonexistent-token")
	if err == nil {
		t.Error("expected error for nonexistent token, got nil")
	}
}

func TestUserRepository_Create_WithRole(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "admin@example.com", PasswordHash: "hash", Name: "Admin", Role: model.RoleAdmin}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create with role failed: %v", err)
	}

	found, err := repo.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found.Role != model.RoleAdmin {
		t.Errorf("expected role 'admin', got %q", found.Role)
	}
}

func TestUserRepository_Create_DefaultRole(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "default@example.com", PasswordHash: "hash", Name: "Default"}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if user.Role != model.RoleUser {
		t.Errorf("expected default role 'user', got %q", user.Role)
	}

	found, err := repo.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found.Role != model.RoleUser {
		t.Errorf("expected role 'user' from DB, got %q", found.Role)
	}
}

func TestUserRepository_ListAll(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	u1 := &model.User{Email: "alice@example.com", PasswordHash: "hash", Name: "Alice", Role: model.RoleAdmin}
	u2 := &model.User{Email: "bob@example.com", PasswordHash: "hash", Name: "Bob", Role: model.RoleUser}
	_ = repo.Create(ctx, u1)
	_ = repo.Create(ctx, u2)

	users, err := repo.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll failed: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	// Ordered by email: alice before bob.
	if users[0].Email != "alice@example.com" {
		t.Errorf("expected first user 'alice@example.com', got %q", users[0].Email)
	}
}

func TestUserRepository_Update(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "update@example.com", PasswordHash: "hash", Name: "Before", Role: model.RoleUser}
	_ = repo.Create(ctx, user)

	user.Name = "After"
	user.Role = model.RoleReadonly
	user.Email = "updated@example.com"
	if err := repo.Update(ctx, user); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	found, err := repo.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found.Name != "After" {
		t.Errorf("expected name 'After', got %q", found.Name)
	}
	if found.Role != model.RoleReadonly {
		t.Errorf("expected role 'readonly', got %q", found.Role)
	}
	if found.Email != "updated@example.com" {
		t.Errorf("expected email 'updated@example.com', got %q", found.Email)
	}
}

func TestUserRepository_UpdatePassword(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "pass@example.com", PasswordHash: "oldhash", Name: "Pass User"}
	_ = repo.Create(ctx, user)

	if err := repo.UpdatePassword(ctx, user.ID, "newhash"); err != nil {
		t.Fatalf("UpdatePassword failed: %v", err)
	}

	found, err := repo.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found.PasswordHash != "newhash" {
		t.Errorf("expected password hash 'newhash', got %q", found.PasswordHash)
	}
}

func TestUserRepository_Delete(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	repo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "del@example.com", PasswordHash: "hash", Name: "Delete Me"}
	_ = repo.Create(ctx, user)

	if err := repo.Delete(ctx, user.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := repo.GetByID(ctx, user.ID)
	if err == nil {
		t.Error("expected error after deletion, got nil")
	}
}

func TestUserRepository_DeviceAssignment(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	userRepo := repository.NewUserRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "devassign@example.com", PasswordHash: "hash", Name: "Dev Assign"}
	_ = userRepo.Create(ctx, user)

	// Create a device owned by a different user.
	owner := &model.User{Email: "owner@example.com", PasswordHash: "hash", Name: "Owner"}
	_ = userRepo.Create(ctx, owner)
	device := &model.Device{UniqueID: "dev-assign-001", Name: "Test Device", Status: "unknown"}
	_ = deviceRepo.Create(ctx, device, owner.ID)

	// Initially no devices for user.
	ids, err := userRepo.GetDevicesForUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetDevicesForUser failed: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 devices, got %d", len(ids))
	}

	// Assign device.
	if err := userRepo.AssignDevice(ctx, user.ID, device.ID); err != nil {
		t.Fatalf("AssignDevice failed: %v", err)
	}

	ids, err = userRepo.GetDevicesForUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetDevicesForUser failed: %v", err)
	}
	if len(ids) != 1 || ids[0] != device.ID {
		t.Errorf("expected [%d], got %v", device.ID, ids)
	}

	// Idempotent assign.
	if err := userRepo.AssignDevice(ctx, user.ID, device.ID); err != nil {
		t.Fatalf("duplicate AssignDevice failed: %v", err)
	}

	ids, _ = userRepo.GetDevicesForUser(ctx, user.ID)
	if len(ids) != 1 {
		t.Errorf("expected 1 device after duplicate assign, got %d", len(ids))
	}

	// Unassign device.
	if err := userRepo.UnassignDevice(ctx, user.ID, device.ID); err != nil {
		t.Fatalf("UnassignDevice failed: %v", err)
	}

	ids, _ = userRepo.GetDevicesForUser(ctx, user.ID)
	if len(ids) != 0 {
		t.Errorf("expected 0 devices after unassign, got %d", len(ids))
	}
}

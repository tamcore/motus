package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

// createTestUserWithEmail is a helper that inserts a user with a specific email and returns it.
func createTestUserWithEmail(t *testing.T, repo *repository.UserRepository, email string) *model.User {
	t.Helper()
	u := &model.User{
		Email:        email,
		PasswordHash: "$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ12",
		Name:         "Test User",
		Role:         model.RoleUser,
	}
	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return u
}

func TestApiKeyRepository_Create(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	keyRepo := repository.NewApiKeyRepository(pool)

	user := createTestUserWithEmail(t, userRepo, "apikey-create@example.com")

	key := &model.ApiKey{
		UserID:      user.ID,
		Name:        "Test Key",
		Permissions: model.PermissionFull,
	}

	err := keyRepo.Create(context.Background(), key)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if key.ID == 0 {
		t.Error("expected non-zero ID after create")
	}
	if key.Token == "" {
		t.Error("expected non-empty token after create")
	}
	if len(key.Token) != 64 {
		t.Errorf("expected 64-char hex token, got %d chars", len(key.Token))
	}
	if key.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
	if key.Permissions != model.PermissionFull {
		t.Errorf("expected permissions %q, got %q", model.PermissionFull, key.Permissions)
	}
}

func TestApiKeyRepository_Create_DefaultPermissions(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	keyRepo := repository.NewApiKeyRepository(pool)

	user := createTestUserWithEmail(t, userRepo, "apikey-default@example.com")

	key := &model.ApiKey{
		UserID: user.ID,
		Name:   "Default Perms Key",
		// Permissions left empty - should default to "full".
	}

	err := keyRepo.Create(context.Background(), key)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if key.Permissions != model.PermissionFull {
		t.Errorf("expected default permissions %q, got %q", model.PermissionFull, key.Permissions)
	}
}

func TestApiKeyRepository_Create_Readonly(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	keyRepo := repository.NewApiKeyRepository(pool)

	user := createTestUserWithEmail(t, userRepo, "apikey-readonly@example.com")

	key := &model.ApiKey{
		UserID:      user.ID,
		Name:        "Read-Only Key",
		Permissions: model.PermissionReadonly,
	}

	err := keyRepo.Create(context.Background(), key)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if key.Permissions != model.PermissionReadonly {
		t.Errorf("expected permissions %q, got %q", model.PermissionReadonly, key.Permissions)
	}
}

func TestApiKeyRepository_GetByToken(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	keyRepo := repository.NewApiKeyRepository(pool)

	user := createTestUserWithEmail(t, userRepo, "apikey-gettoken@example.com")

	key := &model.ApiKey{
		UserID:      user.ID,
		Name:        "Lookup Key",
		Permissions: model.PermissionFull,
	}
	if err := keyRepo.Create(context.Background(), key); err != nil {
		t.Fatalf("Create: %v", err)
	}

	found, err := keyRepo.GetByToken(context.Background(), key.Token)
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}

	if found.ID != key.ID {
		t.Errorf("expected ID %d, got %d", key.ID, found.ID)
	}
	if found.UserID != user.ID {
		t.Errorf("expected UserID %d, got %d", user.ID, found.UserID)
	}
	if found.Name != "Lookup Key" {
		t.Errorf("expected name %q, got %q", "Lookup Key", found.Name)
	}
	if found.Permissions != model.PermissionFull {
		t.Errorf("expected permissions %q, got %q", model.PermissionFull, found.Permissions)
	}
}

func TestApiKeyRepository_GetByToken_NotFound(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	keyRepo := repository.NewApiKeyRepository(pool)

	_, err := keyRepo.GetByToken(context.Background(), "nonexistent-token")
	if err == nil {
		t.Fatal("expected error for non-existent token, got nil")
	}
}

func TestApiKeyRepository_GetByID(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	keyRepo := repository.NewApiKeyRepository(pool)

	user := createTestUserWithEmail(t, userRepo, "apikey-getid@example.com")

	key := &model.ApiKey{
		UserID:      user.ID,
		Name:        "ID Lookup Key",
		Permissions: model.PermissionReadonly,
	}
	if err := keyRepo.Create(context.Background(), key); err != nil {
		t.Fatalf("Create: %v", err)
	}

	found, err := keyRepo.GetByID(context.Background(), key.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	if found.Name != "ID Lookup Key" {
		t.Errorf("expected name %q, got %q", "ID Lookup Key", found.Name)
	}
	if found.Permissions != model.PermissionReadonly {
		t.Errorf("expected permissions %q, got %q", model.PermissionReadonly, found.Permissions)
	}
}

func TestApiKeyRepository_ListByUser(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	keyRepo := repository.NewApiKeyRepository(pool)

	user := createTestUserWithEmail(t, userRepo, "apikey-list@example.com")

	// Create multiple keys.
	for i, name := range []string{"Key A", "Key B", "Key C"} {
		perm := model.PermissionFull
		if i == 2 {
			perm = model.PermissionReadonly
		}
		key := &model.ApiKey{
			UserID:      user.ID,
			Name:        name,
			Permissions: perm,
		}
		if err := keyRepo.Create(context.Background(), key); err != nil {
			t.Fatalf("Create key %q: %v", name, err)
		}
	}

	keys, err := keyRepo.ListByUser(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}

	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}

	// Verify order (DESC by created_at, so last created first).
	if keys[0].Name != "Key C" {
		t.Errorf("expected first key name %q, got %q", "Key C", keys[0].Name)
	}
}

func TestApiKeyRepository_ListByUser_Empty(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	keyRepo := repository.NewApiKeyRepository(pool)

	user := createTestUserWithEmail(t, userRepo, "apikey-listempty@example.com")

	keys, err := keyRepo.ListByUser(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}

	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

func TestApiKeyRepository_ListByUser_IsolatedPerUser(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	keyRepo := repository.NewApiKeyRepository(pool)

	user1 := createTestUserWithEmail(t, userRepo, "user1-apikey@example.com")
	user2 := createTestUserWithEmail(t, userRepo, "user2-apikey@example.com")

	// Create key for user1.
	if err := keyRepo.Create(context.Background(), &model.ApiKey{
		UserID: user1.ID, Name: "User1 Key", Permissions: model.PermissionFull,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Create key for user2.
	if err := keyRepo.Create(context.Background(), &model.ApiKey{
		UserID: user2.ID, Name: "User2 Key", Permissions: model.PermissionReadonly,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// List user1's keys.
	keys1, err := keyRepo.ListByUser(context.Background(), user1.ID)
	if err != nil {
		t.Fatalf("ListByUser user1: %v", err)
	}
	if len(keys1) != 1 {
		t.Errorf("expected 1 key for user1, got %d", len(keys1))
	}

	// List user2's keys.
	keys2, err := keyRepo.ListByUser(context.Background(), user2.ID)
	if err != nil {
		t.Fatalf("ListByUser user2: %v", err)
	}
	if len(keys2) != 1 {
		t.Errorf("expected 1 key for user2, got %d", len(keys2))
	}
}

func TestApiKeyRepository_Delete(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	keyRepo := repository.NewApiKeyRepository(pool)

	user := createTestUserWithEmail(t, userRepo, "apikey-delete@example.com")

	key := &model.ApiKey{
		UserID:      user.ID,
		Name:        "To Delete",
		Permissions: model.PermissionFull,
	}
	if err := keyRepo.Create(context.Background(), key); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Delete the key.
	if err := keyRepo.Delete(context.Background(), key.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify it is gone.
	_, err := keyRepo.GetByID(context.Background(), key.ID)
	if err == nil {
		t.Fatal("expected error after deletion, got nil")
	}
}

func TestApiKeyRepository_Delete_DoesNotAffectOtherKeys(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	keyRepo := repository.NewApiKeyRepository(pool)

	user := createTestUserWithEmail(t, userRepo, "apikey-delother@example.com")

	key1 := &model.ApiKey{UserID: user.ID, Name: "Keep", Permissions: model.PermissionFull}
	key2 := &model.ApiKey{UserID: user.ID, Name: "Delete", Permissions: model.PermissionFull}
	if err := keyRepo.Create(context.Background(), key1); err != nil {
		t.Fatalf("Create key1: %v", err)
	}
	if err := keyRepo.Create(context.Background(), key2); err != nil {
		t.Fatalf("Create key2: %v", err)
	}

	// Delete only key2.
	if err := keyRepo.Delete(context.Background(), key2.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// key1 should still exist.
	found, err := keyRepo.GetByID(context.Background(), key1.ID)
	if err != nil {
		t.Fatalf("GetByID after delete: %v", err)
	}
	if found.Name != "Keep" {
		t.Errorf("expected key1 name %q, got %q", "Keep", found.Name)
	}
}

func TestApiKeyRepository_UpdateLastUsed(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	keyRepo := repository.NewApiKeyRepository(pool)

	user := createTestUserWithEmail(t, userRepo, "apikey-lastused@example.com")

	key := &model.ApiKey{
		UserID:      user.ID,
		Name:        "Last Used Key",
		Permissions: model.PermissionFull,
	}
	if err := keyRepo.Create(context.Background(), key); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Initially, last_used_at should be nil.
	found, err := keyRepo.GetByID(context.Background(), key.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if found.LastUsedAt != nil {
		t.Error("expected nil LastUsedAt initially")
	}

	// Update last used.
	if err := keyRepo.UpdateLastUsed(context.Background(), key.ID); err != nil {
		t.Fatalf("UpdateLastUsed: %v", err)
	}

	// Verify it was updated.
	found, err = keyRepo.GetByID(context.Background(), key.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if found.LastUsedAt == nil {
		t.Error("expected non-nil LastUsedAt after update")
	}
}

func TestApiKeyRepository_CascadeDeleteOnUserDelete(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	keyRepo := repository.NewApiKeyRepository(pool)

	user := createTestUserWithEmail(t, userRepo, "apikey-cascade@example.com")

	key := &model.ApiKey{
		UserID:      user.ID,
		Name:        "Cascade Key",
		Permissions: model.PermissionFull,
	}
	if err := keyRepo.Create(context.Background(), key); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Delete the user - should cascade to api_keys.
	if err := userRepo.Delete(context.Background(), user.ID); err != nil {
		t.Fatalf("Delete user: %v", err)
	}

	// Verify the key is gone.
	_, err := keyRepo.GetByID(context.Background(), key.ID)
	if err == nil {
		t.Fatal("expected error after user deletion (cascade), got nil")
	}
}

func TestApiKeyRepository_Create_WithExpiration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	keyRepo := repository.NewApiKeyRepository(pool)

	user := createTestUserWithEmail(t, userRepo, "apikey-expiry@example.com")

	expiresAt := time.Now().Add(7 * 24 * time.Hour).UTC()
	key := &model.ApiKey{
		UserID:      user.ID,
		Name:        "Expiring Key",
		Permissions: model.PermissionFull,
		ExpiresAt:   &expiresAt,
	}

	err := keyRepo.Create(context.Background(), key)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Verify expiration was stored.
	found, err := keyRepo.GetByToken(context.Background(), key.Token)
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if found.ExpiresAt == nil {
		t.Fatal("expected non-nil ExpiresAt")
	}
	// Allow 1 second tolerance for DB rounding.
	diff := found.ExpiresAt.Sub(expiresAt)
	if diff < -1*time.Second || diff > 1*time.Second {
		t.Errorf("ExpiresAt %v differs from expected %v by %v", found.ExpiresAt, expiresAt, diff)
	}
}

func TestApiKeyRepository_Create_WithoutExpiration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	keyRepo := repository.NewApiKeyRepository(pool)

	user := createTestUserWithEmail(t, userRepo, "apikey-noexpiry@example.com")

	key := &model.ApiKey{
		UserID:      user.ID,
		Name:        "No Expiry Key",
		Permissions: model.PermissionFull,
		// ExpiresAt intentionally nil.
	}

	err := keyRepo.Create(context.Background(), key)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	found, err := keyRepo.GetByToken(context.Background(), key.Token)
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if found.ExpiresAt != nil {
		t.Errorf("expected nil ExpiresAt, got %v", found.ExpiresAt)
	}
}

func TestApiKeyRepository_ListByUser_IncludesExpiration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	keyRepo := repository.NewApiKeyRepository(pool)

	user := createTestUserWithEmail(t, userRepo, "apikey-listexpiry@example.com")

	// Create one with expiration, one without.
	expiresAt := time.Now().Add(24 * time.Hour).UTC()
	if err := keyRepo.Create(context.Background(), &model.ApiKey{
		UserID: user.ID, Name: "Expiring", Permissions: model.PermissionFull, ExpiresAt: &expiresAt,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := keyRepo.Create(context.Background(), &model.ApiKey{
		UserID: user.ID, Name: "Forever", Permissions: model.PermissionFull,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	keys, err := keyRepo.ListByUser(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}

	// Verify one has expiration and one does not (order is DESC by created_at).
	var hasExpiry, hasNoExpiry bool
	for _, k := range keys {
		if k.ExpiresAt != nil {
			hasExpiry = true
		} else {
			hasNoExpiry = true
		}
	}
	if !hasExpiry || !hasNoExpiry {
		t.Errorf("expected one key with expiry and one without; hasExpiry=%v, hasNoExpiry=%v", hasExpiry, hasNoExpiry)
	}
}

func TestApiKeyRepository_GetByID_IncludesExpiration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	keyRepo := repository.NewApiKeyRepository(pool)

	user := createTestUserWithEmail(t, userRepo, "apikey-getidexpiry@example.com")

	expiresAt := time.Now().Add(48 * time.Hour).UTC()
	key := &model.ApiKey{
		UserID:      user.ID,
		Name:        "Expiry By ID",
		Permissions: model.PermissionReadonly,
		ExpiresAt:   &expiresAt,
	}
	if err := keyRepo.Create(context.Background(), key); err != nil {
		t.Fatalf("Create: %v", err)
	}

	found, err := keyRepo.GetByID(context.Background(), key.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if found.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set")
	}
}

func TestApiKeyRepository_UniqueTokens(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	keyRepo := repository.NewApiKeyRepository(pool)

	user := createTestUserWithEmail(t, userRepo, "apikey-unique@example.com")

	// Create multiple keys and verify they all have different tokens.
	tokens := make(map[string]bool)
	for i := 0; i < 5; i++ {
		key := &model.ApiKey{
			UserID:      user.ID,
			Name:        "Key",
			Permissions: model.PermissionFull,
		}
		if err := keyRepo.Create(context.Background(), key); err != nil {
			t.Fatalf("Create key %d: %v", i, err)
		}
		if tokens[key.Token] {
			t.Fatalf("duplicate token generated: %s", key.Token)
		}
		tokens[key.Token] = true
	}
}

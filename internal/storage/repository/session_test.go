package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func TestSessionRepository_Create(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	sessionRepo := repository.NewSessionRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "session@example.com", PasswordHash: "hash", Name: "Session User"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("Create user failed: %v", err)
	}

	session, err := sessionRepo.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("Create session failed: %v", err)
	}

	if session.ID == "" {
		t.Error("expected non-empty session ID")
	}
	if session.UserID != user.ID {
		t.Errorf("expected user ID %d, got %d", user.ID, session.UserID)
	}
	if session.ExpiresAt.Before(session.CreatedAt) {
		t.Error("expected ExpiresAt to be after CreatedAt")
	}
}

func TestSessionRepository_GetByID(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	sessionRepo := repository.NewSessionRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "sess-get@example.com", PasswordHash: "hash", Name: "Sess Get"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("Create user failed: %v", err)
	}

	session, err := sessionRepo.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("Create session failed: %v", err)
	}

	found, err := sessionRepo.GetByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found.UserID != user.ID {
		t.Errorf("expected user ID %d, got %d", user.ID, found.UserID)
	}
}

func TestSessionRepository_GetByID_NotFound(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	sessionRepo := repository.NewSessionRepository(pool)
	ctx := context.Background()

	_, err := sessionRepo.GetByID(ctx, "nonexistent-session-id")
	if err == nil {
		t.Error("expected error for nonexistent session, got nil")
	}
}

func TestSessionRepository_CreateWithExpiry(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	sessionRepo := repository.NewSessionRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "sess-expiry@example.com", PasswordHash: "hash", Name: "Expiry User"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("Create user failed: %v", err)
	}

	expiry := time.Now().Add(10 * 365 * 24 * time.Hour)
	session, err := sessionRepo.CreateWithExpiry(ctx, user.ID, expiry, true)
	if err != nil {
		t.Fatalf("CreateWithExpiry failed: %v", err)
	}

	if !session.RememberMe {
		t.Error("expected RememberMe to be true")
	}

	// Verify the session is persisted with the correct remember_me value.
	found, err := sessionRepo.GetByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if !found.RememberMe {
		t.Error("expected RememberMe to be true after fetch")
	}
	// Expiry should be ~10 years from now.
	expiresIn := time.Until(found.ExpiresAt)
	if expiresIn < 10*365*24*time.Hour-24*time.Hour {
		t.Errorf("expected expiry ~10 years, got %v", expiresIn)
	}
}

func TestSessionRepository_CreateWithApiKey(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	sessionRepo := repository.NewSessionRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "sess-apikey@example.com", PasswordHash: "hash", Name: "API Key Session User"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("Create user failed: %v", err)
	}

	key := &model.ApiKey{UserID: user.ID, Name: "Test Key", Permissions: model.PermissionReadonly}
	if err := apiKeyRepo.Create(ctx, key); err != nil {
		t.Fatalf("Create api key failed: %v", err)
	}

	expiry := time.Now().Add(10 * 365 * 24 * time.Hour)
	session, err := sessionRepo.CreateWithApiKey(ctx, user.ID, key.ID, expiry, true)
	if err != nil {
		t.Fatalf("CreateWithApiKey failed: %v", err)
	}

	if session.ID == "" {
		t.Error("expected non-empty session ID")
	}
	if session.UserID != user.ID {
		t.Errorf("expected user ID %d, got %d", user.ID, session.UserID)
	}
	if session.ApiKeyID == nil {
		t.Fatal("expected non-nil ApiKeyID")
	}
	if *session.ApiKeyID != key.ID {
		t.Errorf("expected ApiKeyID %d, got %d", key.ID, *session.ApiKeyID)
	}

	// Verify GetByID returns the api_key_id.
	found, err := sessionRepo.GetByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found.ApiKeyID == nil {
		t.Fatal("expected non-nil ApiKeyID from GetByID")
	}
	if *found.ApiKeyID != key.ID {
		t.Errorf("expected ApiKeyID %d from GetByID, got %d", key.ID, *found.ApiKeyID)
	}
}

func TestSessionRepository_CreateWithApiKey_NilApiKeyID_OnGetByID(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	sessionRepo := repository.NewSessionRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "sess-no-apikey@example.com", PasswordHash: "hash", Name: "No API Key Session User"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("Create user failed: %v", err)
	}

	// Regular session creation -- no api_key_id.
	session, err := sessionRepo.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	found, err := sessionRepo.GetByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found.ApiKeyID != nil {
		t.Errorf("expected nil ApiKeyID for regular session, got %d", *found.ApiKeyID)
	}
}

func TestSessionRepository_Delete(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	sessionRepo := repository.NewSessionRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "sess-del@example.com", PasswordHash: "hash", Name: "Sess Del"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("Create user failed: %v", err)
	}

	session, err := sessionRepo.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("Create session failed: %v", err)
	}

	if err := sessionRepo.Delete(ctx, session.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = sessionRepo.GetByID(ctx, session.ID)
	if err == nil {
		t.Error("expected error after deletion, got nil")
	}
}

func TestSessionRepository_ListByUser(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	sessionRepo := repository.NewSessionRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "sess-list@example.com", PasswordHash: "hash", Name: "List User"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("Create user failed: %v", err)
	}

	// Create an API key to link to one session.
	key := &model.ApiKey{UserID: user.ID, Name: "Session Key", Permissions: model.PermissionFull}
	if err := apiKeyRepo.Create(ctx, key); err != nil {
		t.Fatalf("Create api key failed: %v", err)
	}

	// Session 1: regular session (created first, should appear last in DESC order).
	s1, err := sessionRepo.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("Create session 1 failed: %v", err)
	}

	// Session 2: sudo session.
	adminUser := &model.User{Email: "sess-list-admin@example.com", PasswordHash: "hash", Name: "Admin"}
	if err := userRepo.Create(ctx, adminUser); err != nil {
		t.Fatalf("Create admin user failed: %v", err)
	}
	s2, err := sessionRepo.CreateSudo(ctx, user.ID, adminUser.ID)
	if err != nil {
		t.Fatalf("Create sudo session failed: %v", err)
	}

	// Session 3: linked to API key (created last, should appear first in DESC order).
	expiry := time.Now().Add(48 * time.Hour)
	s3, err := sessionRepo.CreateWithApiKey(ctx, user.ID, key.ID, expiry, true)
	if err != nil {
		t.Fatalf("Create API key session failed: %v", err)
	}

	// Also create an expired session that should NOT be returned.
	pastExpiry := time.Now().Add(-1 * time.Hour)
	_, err = sessionRepo.CreateWithExpiry(ctx, user.ID, pastExpiry, false)
	if err != nil {
		t.Fatalf("Create expired session failed: %v", err)
	}

	sessions, err := sessionRepo.ListByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListByUser failed: %v", err)
	}

	// Should return 3 non-expired sessions (the expired one excluded).
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}

	// Verify DESC ordering: s3 (newest) first, s1 (oldest) last.
	if sessions[0].ID != s3.ID {
		t.Errorf("expected first session ID %s (API key session), got %s", s3.ID, sessions[0].ID)
	}
	if sessions[1].ID != s2.ID {
		t.Errorf("expected second session ID %s (sudo session), got %s", s2.ID, sessions[1].ID)
	}
	if sessions[2].ID != s1.ID {
		t.Errorf("expected third session ID %s (regular session), got %s", s1.ID, sessions[2].ID)
	}

	// The API key-linked session should have ApiKeyName populated.
	if sessions[0].ApiKeyName == nil {
		t.Fatal("expected non-nil ApiKeyName on API key-linked session")
	}
	if *sessions[0].ApiKeyName != "Session Key" {
		t.Errorf("expected ApiKeyName %q, got %q", "Session Key", *sessions[0].ApiKeyName)
	}

	// The other sessions should have nil ApiKeyName.
	if sessions[1].ApiKeyName != nil {
		t.Errorf("expected nil ApiKeyName on sudo session, got %q", *sessions[1].ApiKeyName)
	}
	if sessions[2].ApiKeyName != nil {
		t.Errorf("expected nil ApiKeyName on regular session, got %q", *sessions[2].ApiKeyName)
	}

	// Verify the sudo session fields.
	if !sessions[1].IsSudo {
		t.Error("expected IsSudo to be true on sudo session")
	}
}

func TestSessionRepository_ListByUser_Empty(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	sessionRepo := repository.NewSessionRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "sess-list-empty@example.com", PasswordHash: "hash", Name: "Empty User"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("Create user failed: %v", err)
	}

	sessions, err := sessionRepo.ListByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListByUser failed: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions for user with no sessions, got %d", len(sessions))
	}
}

func TestSessionRepository_ListByUser_ExcludesExpired(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	sessionRepo := repository.NewSessionRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "sess-list-expired@example.com", PasswordHash: "hash", Name: "Expired User"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("Create user failed: %v", err)
	}

	// Create a session that already expired (1 hour in the past).
	pastExpiry := time.Now().Add(-1 * time.Hour)
	_, err := sessionRepo.CreateWithExpiry(ctx, user.ID, pastExpiry, false)
	if err != nil {
		t.Fatalf("Create expired session failed: %v", err)
	}

	sessions, err := sessionRepo.ListByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListByUser failed: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions (all expired), got %d", len(sessions))
	}
}

func TestSessionRepository_CascadeDeleteApiKey(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	sessionRepo := repository.NewSessionRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "sess-cascade-apikey@example.com", PasswordHash: "hash", Name: "Cascade User"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("Create user failed: %v", err)
	}

	key := &model.ApiKey{UserID: user.ID, Name: "Cascade Key", Permissions: model.PermissionReadonly}
	if err := apiKeyRepo.Create(ctx, key); err != nil {
		t.Fatalf("Create api key failed: %v", err)
	}

	// Create a session linked to the API key.
	expiry := time.Now().Add(24 * time.Hour)
	session, err := sessionRepo.CreateWithApiKey(ctx, user.ID, key.ID, expiry, false)
	if err != nil {
		t.Fatalf("CreateWithApiKey failed: %v", err)
	}

	// Verify the session exists before deletion.
	found, err := sessionRepo.GetByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetByID before delete failed: %v", err)
	}
	if found.ID != session.ID {
		t.Fatalf("expected session ID %s, got %s", session.ID, found.ID)
	}

	// Delete the API key -- should cascade-delete the linked session.
	if err := apiKeyRepo.Delete(ctx, key.ID); err != nil {
		t.Fatalf("Delete api key failed: %v", err)
	}

	// The session should no longer exist.
	_, err = sessionRepo.GetByID(ctx, session.ID)
	if err == nil {
		t.Error("expected error after API key deletion (cascade), got nil")
	}
}

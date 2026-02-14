package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/middleware"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

// TestAuthMiddleware_ApiKey_FullAccess verifies that a valid API key with
// full permissions authenticates the user and sets both user and API key
// in the request context.
func TestAuthMiddleware_ApiKey_FullAccess(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	// Create a test user.
	user := &model.User{Email: "apikey-full@example.com", PasswordHash: "hash", Name: "API Key User"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Create a full-access API key.
	key := &model.ApiKey{UserID: user.ID, Name: "Full Key", Permissions: model.PermissionFull}
	if err := apiKeyRepo.Create(context.Background(), key); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	mw := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	var gotUser *model.User
	var gotKey *model.ApiKey
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = api.UserFromContext(r.Context())
		gotKey = api.ApiKeyFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.Header.Set("Authorization", "Bearer "+key.Token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotUser == nil {
		t.Fatal("expected user in context, got nil")
	}
	if gotUser.Email != "apikey-full@example.com" {
		t.Errorf("expected email 'apikey-full@example.com', got %q", gotUser.Email)
	}
	if gotKey == nil {
		t.Fatal("expected API key in context, got nil")
	}
	if gotKey.Permissions != model.PermissionFull {
		t.Errorf("expected full permissions, got %q", gotKey.Permissions)
	}
}

// TestAuthMiddleware_ApiKey_ReadonlyAccess verifies that a valid API key
// with readonly permissions sets the correct API key in context.
func TestAuthMiddleware_ApiKey_ReadonlyAccess(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	user := &model.User{Email: "apikey-readonly@example.com", PasswordHash: "hash", Name: "Readonly User"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	key := &model.ApiKey{UserID: user.ID, Name: "Readonly Key", Permissions: model.PermissionReadonly}
	if err := apiKeyRepo.Create(context.Background(), key); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	mw := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	var gotKey *model.ApiKey
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = api.ApiKeyFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.Header.Set("Authorization", "Bearer "+key.Token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotKey == nil {
		t.Fatal("expected API key in context, got nil")
	}
	if gotKey.Permissions != model.PermissionReadonly {
		t.Errorf("expected readonly permissions, got %q", gotKey.Permissions)
	}
}

// TestAuthMiddleware_ApiKey_InvalidToken verifies that an invalid API key
// token falls through to legacy token and then cookie auth, resulting
// in 401 when neither is present.
func TestAuthMiddleware_ApiKey_InvalidToken(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	mw := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.Header.Set("Authorization", "Bearer invalid-api-key-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// TestAuthMiddleware_LegacyToken_StillWorks verifies that the legacy user
// token (stored on the users table) still works for backward compatibility.
func TestAuthMiddleware_LegacyToken_StillWorks(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	user := &model.User{Email: "legacy-token@example.com", PasswordHash: "hash", Name: "Legacy User"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	token, err := userRepo.GenerateToken(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	mw := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	var gotUser *model.User
	var gotKey *model.ApiKey
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = api.UserFromContext(r.Context())
		gotKey = api.ApiKeyFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotUser == nil {
		t.Fatal("expected user in context, got nil")
	}
	if gotUser.Email != "legacy-token@example.com" {
		t.Errorf("expected email 'legacy-token@example.com', got %q", gotUser.Email)
	}
	// Legacy tokens should not set API key in context.
	if gotKey != nil {
		t.Error("expected nil API key for legacy token auth, got non-nil")
	}
}

// TestAuthMiddleware_ApiKey_PrioritizedOverLegacyToken verifies that when
// a token is found in api_keys, it takes priority over users.token lookup.
func TestAuthMiddleware_ApiKey_PrioritizedOverLegacyToken(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	user := &model.User{Email: "priority@example.com", PasswordHash: "hash", Name: "Priority User"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Create an API key.
	key := &model.ApiKey{UserID: user.ID, Name: "Priority Key", Permissions: model.PermissionReadonly}
	if err := apiKeyRepo.Create(context.Background(), key); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	mw := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	var gotKey *model.ApiKey
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = api.ApiKeyFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Use the API key token.
	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.Header.Set("Authorization", "Bearer "+key.Token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotKey == nil {
		t.Fatal("expected API key in context (api_keys should take priority), got nil")
	}
	if gotKey.Permissions != model.PermissionReadonly {
		t.Errorf("expected readonly permissions, got %q", gotKey.Permissions)
	}
}

// TestAuthMiddleware_ApiKey_UpdatesLastUsed verifies that the last_used_at
// timestamp is updated when an API key is used for authentication.
func TestAuthMiddleware_ApiKey_UpdatesLastUsed(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	user := &model.User{Email: "lastused@example.com", PasswordHash: "hash", Name: "Last Used User"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	key := &model.ApiKey{UserID: user.ID, Name: "Track Usage", Permissions: model.PermissionFull}
	if err := apiKeyRepo.Create(context.Background(), key); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Verify initially nil.
	initial, err := apiKeyRepo.GetByID(context.Background(), key.ID)
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	if initial.LastUsedAt != nil {
		t.Error("expected nil LastUsedAt before auth")
	}

	mw := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.Header.Set("Authorization", "Bearer "+key.Token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Wait for the async goroutine to complete the update.
	time.Sleep(200 * time.Millisecond)

	updated, err := apiKeyRepo.GetByID(context.Background(), key.ID)
	if err != nil {
		t.Fatalf("get key after auth: %v", err)
	}
	if updated.LastUsedAt == nil {
		t.Error("expected non-nil LastUsedAt after auth (may be racy if goroutine is slow)")
	}
}

// TestReadonlyKeyIntegration_BlocksPostWithRealAuth tests the full
// middleware chain: auth (with API key) -> RequireWriteAccess -> handler.
// A readonly key should be blocked from POST requests.
func TestReadonlyKeyIntegration_BlocksPostWithRealAuth(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	user := &model.User{Email: "readonly-post@example.com", PasswordHash: "hash", Name: "Readonly User"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	key := &model.ApiKey{UserID: user.ID, Name: "Readonly Key", Permissions: model.PermissionReadonly}
	if err := apiKeyRepo.Create(context.Background(), key); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Build full middleware chain: Auth -> RequireWriteAccess -> handler.
	authMW := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := authMW(middleware.RequireWriteAccess(inner))

	// POST should be blocked.
	req := httptest.NewRequest(http.MethodPost, "/api/devices", nil)
	req.Header.Set("Authorization", "Bearer "+key.Token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("POST: expected 403, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// GET should be allowed.
	req = httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.Header.Set("Authorization", "Bearer "+key.Token)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET: expected 200, got %d", rr.Code)
	}
}

// TestFullKeyIntegration_AllowsPostWithRealAuth tests the full middleware
// chain with a full-access API key. All methods should be allowed.
func TestFullKeyIntegration_AllowsPostWithRealAuth(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	user := &model.User{Email: "full-post@example.com", PasswordHash: "hash", Name: "Full Access User"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	key := &model.ApiKey{UserID: user.ID, Name: "Full Key", Permissions: model.PermissionFull}
	if err := apiKeyRepo.Create(context.Background(), key); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	authMW := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := authMW(middleware.RequireWriteAccess(inner))

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/devices", nil)
			req.Header.Set("Authorization", "Bearer "+key.Token)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("%s: expected 200, got %d", method, rr.Code)
			}
		})
	}
}

// TestKeyRevocation_BlocksAfterDelete verifies that a deleted API key
// can no longer be used for authentication.
func TestKeyRevocation_BlocksAfterDelete(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	user := &model.User{Email: "revoke@example.com", PasswordHash: "hash", Name: "Revoke User"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	key := &model.ApiKey{UserID: user.ID, Name: "Revocable Key", Permissions: model.PermissionFull}
	if err := apiKeyRepo.Create(context.Background(), key); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	mw := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First, verify it works.
	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.Header.Set("Authorization", "Bearer "+key.Token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 before revocation, got %d", rr.Code)
	}

	// Revoke (delete) the key.
	if err := apiKeyRepo.Delete(context.Background(), key.ID); err != nil {
		t.Fatalf("delete api key: %v", err)
	}

	// After revocation, auth should fail.
	req = httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.Header.Set("Authorization", "Bearer "+key.Token)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 after revocation, got %d", rr.Code)
	}
}

// TestSessionAuth_NoApiKeyInContext verifies that session cookie auth
// does not set an API key in context.
func TestSessionAuth_NoApiKeyInContext(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	user := &model.User{Email: "session-no-key@example.com", PasswordHash: "hash", Name: "Session User"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	session, err := sessionRepo.Create(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	mw := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	var gotKey *model.ApiKey
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = api.ApiKeyFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: session.ID})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotKey != nil {
		t.Error("expected nil API key for session auth, got non-nil")
	}
}

// TestSessionWithApiKey_ReadonlyEnforced verifies that when a session was
// created from a readonly API key (via CreateWithApiKey), the auth middleware
// restores the API key in context so RequireWriteAccess blocks writes.
// This is the critical security fix for the privilege escalation vulnerability
// where readonly API key -> session cookie bypassed write restrictions.
func TestSessionWithApiKey_ReadonlyEnforced(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	user := &model.User{Email: "readonly-session@example.com", PasswordHash: "hash", Name: "Readonly Session User"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Create a readonly API key.
	key := &model.ApiKey{UserID: user.ID, Name: "Readonly Key", Permissions: model.PermissionReadonly}
	if err := apiKeyRepo.Create(context.Background(), key); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Create a session linked to the readonly API key (simulates the
	// token-based session creation flow from GetCurrentSession).
	tokenExpiry := time.Now().Add(30 * 24 * time.Hour)
	session, err := sessionRepo.CreateWithApiKey(context.Background(), user.ID, key.ID, tokenExpiry, true)
	if err != nil {
		t.Fatalf("create session with api key: %v", err)
	}

	// Build the full middleware chain: Auth -> RequireWriteAccess -> handler.
	authMW := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := authMW(middleware.RequireWriteAccess(inner))

	// POST with cookie should be BLOCKED (403) because the session is
	// linked to a readonly API key.
	req := httptest.NewRequest(http.MethodPost, "/api/devices", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: session.ID})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("POST: expected 403 for readonly API key session, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// GET with cookie should still work.
	req = httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: session.ID})
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET: expected 200 for readonly API key session, got %d", rr.Code)
	}

	// DELETE with cookie should be BLOCKED.
	req = httptest.NewRequest(http.MethodDelete, "/api/devices", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: session.ID})
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("DELETE: expected 403 for readonly API key session, got %d", rr.Code)
	}
}

// TestSessionWithApiKey_FullAccessAllowed verifies that when a session was
// created from a full-access API key, write operations are permitted.
func TestSessionWithApiKey_FullAccessAllowed(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	user := &model.User{Email: "full-session@example.com", PasswordHash: "hash", Name: "Full Access Session User"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Create a full-access API key.
	key := &model.ApiKey{UserID: user.ID, Name: "Full Key", Permissions: model.PermissionFull}
	if err := apiKeyRepo.Create(context.Background(), key); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Create a session linked to the full-access API key.
	tokenExpiry := time.Now().Add(30 * 24 * time.Hour)
	session, err := sessionRepo.CreateWithApiKey(context.Background(), user.ID, key.ID, tokenExpiry, true)
	if err != nil {
		t.Fatalf("create session with api key: %v", err)
	}

	authMW := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := authMW(middleware.RequireWriteAccess(inner))

	// All methods should work with a full-access API key session.
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/devices", nil)
			req.AddCookie(&http.Cookie{Name: "session_id", Value: session.ID})
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("%s: expected 200, got %d; body: %s", method, rr.Code, rr.Body.String())
			}
		})
	}
}

// TestSessionWithApiKey_PasswordLoginUnrestricted verifies that a session
// created via password login (no API key) is unrestricted, even when the
// user also has readonly API keys.
func TestSessionWithApiKey_PasswordLoginUnrestricted(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	user := &model.User{Email: "password-login@example.com", PasswordHash: "hash", Name: "Password Login User"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// User has a readonly API key but logs in with password.
	readonlyKey := &model.ApiKey{UserID: user.ID, Name: "Readonly Key", Permissions: model.PermissionReadonly}
	if err := apiKeyRepo.Create(context.Background(), readonlyKey); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Session created via password login (no api_key_id).
	session, err := sessionRepo.Create(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	authMW := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := authMW(middleware.RequireWriteAccess(inner))

	// POST should be allowed (password login is unrestricted).
	req := httptest.NewRequest(http.MethodPost, "/api/devices", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: session.ID})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("POST: expected 200 for password-login session, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestSessionWithApiKey_DeletedKeyCascadesToSession verifies that deleting an
// API key also deletes all sessions that were created from it (ON DELETE CASCADE),
// causing subsequent requests with those sessions to return 401.
func TestSessionWithApiKey_DeletedKeyCascadesToSession(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	user := &model.User{Email: "deleted-key@example.com", PasswordHash: "hash", Name: "Deleted Key User"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	key := &model.ApiKey{UserID: user.ID, Name: "Will Be Deleted", Permissions: model.PermissionReadonly}
	if err := apiKeyRepo.Create(context.Background(), key); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	tokenExpiry := time.Now().Add(30 * 24 * time.Hour)
	session, err := sessionRepo.CreateWithApiKey(context.Background(), user.ID, key.ID, tokenExpiry, true)
	if err != nil {
		t.Fatalf("create session with api key: %v", err)
	}

	// Verify the session is initially restricted (readonly key → 403 on write).
	authMW := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := authMW(middleware.RequireWriteAccess(inner))

	req := httptest.NewRequest(http.MethodPost, "/api/devices", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: session.ID})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("POST before key deletion: expected 403, got %d", rr.Code)
	}

	// Delete the API key. The sessions FK is ON DELETE CASCADE (migration 00031),
	// so the session is deleted along with the key.
	if err := apiKeyRepo.Delete(context.Background(), key.ID); err != nil {
		t.Fatalf("delete api key: %v", err)
	}

	// The session no longer exists → authentication should fail with 401.
	req = httptest.NewRequest(http.MethodPost, "/api/devices", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: session.ID})
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("POST after key deletion: expected 401 (session deleted), got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestSessionWithApiKey_ContextHasCorrectApiKey verifies that the auth
// middleware correctly sets the API key in context when the session has an
// api_key_id, and that the key matches the original.
func TestSessionWithApiKey_ContextHasCorrectApiKey(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	user := &model.User{Email: "ctx-check@example.com", PasswordHash: "hash", Name: "Context Check User"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	key := &model.ApiKey{UserID: user.ID, Name: "Context Key", Permissions: model.PermissionReadonly}
	if err := apiKeyRepo.Create(context.Background(), key); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	tokenExpiry := time.Now().Add(30 * 24 * time.Hour)
	session, err := sessionRepo.CreateWithApiKey(context.Background(), user.ID, key.ID, tokenExpiry, true)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	mw := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	var gotUser *model.User
	var gotKey *model.ApiKey
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = api.UserFromContext(r.Context())
		gotKey = api.ApiKeyFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: session.ID})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotUser == nil {
		t.Fatal("expected user in context")
	}
	if gotUser.Email != "ctx-check@example.com" {
		t.Errorf("expected email 'ctx-check@example.com', got %q", gotUser.Email)
	}
	if gotKey == nil {
		t.Fatal("expected API key in context for session with api_key_id")
	}
	if gotKey.ID != key.ID {
		t.Errorf("expected API key ID %d, got %d", key.ID, gotKey.ID)
	}
	if gotKey.Permissions != model.PermissionReadonly {
		t.Errorf("expected permissions %q, got %q", model.PermissionReadonly, gotKey.Permissions)
	}
}

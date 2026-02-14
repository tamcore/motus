package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/middleware"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testutil.Cleanup()
	os.Exit(code)
}

// TestAuthMiddleware_ValidBearerToken verifies that a valid bearer token
// authenticates the user and passes control to the next handler.
func TestAuthMiddleware_ValidBearerToken(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)

	// Create a user and generate a token.
	user := &model.User{Email: "bearer@example.com", PasswordHash: "hash", Name: "Bearer User"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	token, err := userRepo.GenerateToken(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	mw := middleware.Auth(userRepo, sessionRepo, repository.NewApiKeyRepository(pool))
	var gotUser *model.User
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = api.UserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if gotUser == nil {
		t.Fatal("expected user in context, got nil")
	}
	if gotUser.Email != "bearer@example.com" {
		t.Errorf("expected email 'bearer@example.com', got %q", gotUser.Email)
	}
}

// TestAuthMiddleware_ValidSessionCookie verifies that a valid session cookie
// authenticates the user.
func TestAuthMiddleware_ValidSessionCookie(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)

	// Create a user and session.
	user := &model.User{Email: "session@example.com", PasswordHash: "hash", Name: "Session User"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	session, err := sessionRepo.Create(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	mw := middleware.Auth(userRepo, sessionRepo, repository.NewApiKeyRepository(pool))
	var gotUser *model.User
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = api.UserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: session.ID})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if gotUser == nil {
		t.Fatal("expected user in context, got nil")
	}
	if gotUser.Email != "session@example.com" {
		t.Errorf("expected email 'session@example.com', got %q", gotUser.Email)
	}
}

// TestAuthMiddleware_InvalidBearerFallsToSession verifies that an invalid
// bearer token falls through to session cookie auth.
func TestAuthMiddleware_InvalidBearerFallsToSession(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)

	// Create a user and session.
	user := &model.User{Email: "fallback@example.com", PasswordHash: "hash", Name: "Fallback User"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	session, err := sessionRepo.Create(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	mw := middleware.Auth(userRepo, sessionRepo, repository.NewApiKeyRepository(pool))
	var gotUser *model.User
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = api.UserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Invalid bearer token + valid session cookie.
	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-xyz")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: session.ID})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if gotUser == nil {
		t.Fatal("expected user in context (via session fallback), got nil")
	}
}

// TestAuthMiddleware_InvalidSessionCookie verifies that an invalid session
// cookie returns 401.
func TestAuthMiddleware_InvalidSessionCookie(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)

	mw := middleware.Auth(userRepo, sessionRepo, repository.NewApiKeyRepository(pool))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "nonexistent-session-id"})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

// TestAuthMiddleware_InvalidBearerToken verifies that an invalid bearer
// token with no cookie falls through to 401.
func TestAuthMiddleware_InvalidBearerToken(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)

	mw := middleware.Auth(userRepo, sessionRepo, repository.NewApiKeyRepository(pool))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.Header.Set("Authorization", "Bearer completely-invalid-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

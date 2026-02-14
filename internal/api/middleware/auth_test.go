package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/middleware"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// TestAuthMiddleware_NoCredentials verifies that requests without any
// authentication credentials receive a 401 response.
func TestAuthMiddleware_NoCredentials(t *testing.T) {
	userRepo := repository.NewUserRepository(nil)
	sessionRepo := repository.NewSessionRepository(nil)

	mw := middleware.Auth(userRepo, sessionRepo, repository.NewApiKeyRepository(nil))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}

	var body map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["error"] != "authentication required" {
		t.Errorf("expected error message 'authentication required', got %q", body["error"])
	}
}

// TestAuthMiddleware_EmptyBearerFallsThrough verifies that an empty bearer
// token falls through to cookie-based auth, which also fails without a cookie.
func TestAuthMiddleware_EmptyBearerFallsThrough(t *testing.T) {
	userRepo := repository.NewUserRepository(nil)
	sessionRepo := repository.NewSessionRepository(nil)

	mw := middleware.Auth(userRepo, sessionRepo, repository.NewApiKeyRepository(nil))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.Header.Set("Authorization", "Bearer ")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

// TestAuthMiddleware_WrongScheme verifies that non-Bearer auth schemes
// are rejected and fall through to cookie check.
func TestAuthMiddleware_WrongScheme(t *testing.T) {
	userRepo := repository.NewUserRepository(nil)
	sessionRepo := repository.NewSessionRepository(nil)

	mw := middleware.Auth(userRepo, sessionRepo, repository.NewApiKeyRepository(nil))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

// TestContextHelpers verifies that user context helpers round-trip correctly.
func TestContextHelpers(t *testing.T) {
	user := &model.User{ID: 42, Email: "test@example.com"}
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	// Without user in context.
	if got := api.UserFromContext(req.Context()); got != nil {
		t.Error("expected nil user from empty context")
	}

	// With user in context.
	ctx := api.ContextWithUser(req.Context(), user)
	got := api.UserFromContext(ctx)
	if got == nil {
		t.Fatal("expected non-nil user from context")
	}
	if got.ID != 42 {
		t.Errorf("expected user ID 42, got %d", got.ID)
	}
}

// TestApiKeyContextHelpers verifies that API key context helpers round-trip.
func TestApiKeyContextHelpers(t *testing.T) {
	key := &model.ApiKey{ID: 7, UserID: 42, Permissions: model.PermissionReadonly}
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	// Without API key in context.
	if got := api.ApiKeyFromContext(req.Context()); got != nil {
		t.Error("expected nil API key from empty context")
	}

	// With API key in context.
	ctx := api.ContextWithApiKey(req.Context(), key)
	got := api.ApiKeyFromContext(ctx)
	if got == nil {
		t.Fatal("expected non-nil API key from context")
	}
	if got.ID != 7 {
		t.Errorf("expected API key ID 7, got %d", got.ID)
	}
	if got.Permissions != model.PermissionReadonly {
		t.Errorf("expected permissions %q, got %q", model.PermissionReadonly, got.Permissions)
	}
}

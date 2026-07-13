package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	"github.com/tamcore/motus/internal/api/middleware"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

// TestRouter_Passkey_ReadonlyAndPublicExemptions verifies two routing guarantees
// that the demo-mode passkey behavior depends on:
//   - a read-only API key (as demo users hold) is NOT blocked by the readonly
//     write gate on passkey ceremony routes, and
//   - the passkey login-begin route is reachable without authentication.
//
// The WebAuthn engine is left nil, so a route that passes the middleware layer
// reaches the handler and returns 501. A 501 (not 403/401) therefore proves the
// middleware exemption is in effect.
func TestRouter_Passkey_ReadonlyAndPublicExemptions(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	ctx := context.Background()
	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	user := &model.User{Email: "pk-router@example.com", PasswordHash: "hash", Name: "PK Router"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	readonlyKey := &model.ApiKey{UserID: user.ID, Name: "RO", Permissions: model.PermissionReadonly}
	if err := apiKeyRepo.Create(ctx, readonlyKey); err != nil {
		t.Fatalf("create readonly key: %v", err)
	}

	// Mirror production wiring: LoadAuthContext populates auth context without
	// blocking, and the ogen SecurityHandler enforces auth per-operation. Using
	// the blocking Auth middleware here would 401 the public login route before
	// it ever reached the handler.
	routerCfg := api.RouterConfig{
		Auth:        middleware.LoadAuthContext(userRepo, sessionRepo, apiKeyRepo),
		WriteAccess: middleware.RequireWriteAccess,
	}
	handler := handlers.NewHandler(handlers.HandlerConfig{
		Users:    userRepo,
		Sessions: sessionRepo,
		ApiKeys:  apiKeyRepo,
		Passkeys: repository.NewPasskeyRepository(pool),
		// WebAuthn intentionally nil.
	})
	secHandler := handlers.NewSecurityHandler(sessionRepo, apiKeyRepo, userRepo)
	router := api.NewRouter(handler, secHandler, nil, routerCfg)

	t.Run("readonly_key_not_blocked_on_register_begin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/session/passkey/register/begin", nil)
		req.Header.Set("Authorization", "Bearer "+readonlyKey.Token)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code == http.StatusForbidden {
			t.Fatalf("readonly key was blocked by the write gate (403); expected exemption. body: %s", rr.Body.String())
		}
		if rr.Code != http.StatusNotImplemented {
			t.Errorf("expected 501 (WebAuthn disabled), got %d; body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("login_begin_reachable_without_auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/session/passkey/login/begin", nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code == http.StatusUnauthorized {
			t.Fatalf("login begin required auth (401); expected public access. body: %s", rr.Body.String())
		}
		if rr.Code != http.StatusNotImplemented {
			t.Errorf("expected 501 (WebAuthn disabled), got %d; body: %s", rr.Code, rr.Body.String())
		}
	})
}

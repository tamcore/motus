package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	"github.com/tamcore/motus/internal/model"
)

// Shared mocks (mockUserRepo, mockSessionRepo, …) live in mocks_test.go.

// TestGetCurrentSession_ApiKeyToken verifies that a token from the api_keys
// table authenticates successfully via the ?token= query parameter and that
// the session is created with the API key ID linked.
func TestGetCurrentSession_ApiKeyToken(t *testing.T) {
	testUser := &model.User{ID: 42, Email: "apikey@example.com", Name: "API Key User", Role: "user"}
	testApiKey := &model.ApiKey{ID: 1, UserID: 42, Token: "abc123apikey", Name: "test-key", Permissions: "full"}

	var createdWithApiKeyID int64
	users := &mockUserRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.User, error) {
			if id == 42 {
				return testUser, nil
			}
			return nil, errors.New("not found")
		},
	}
	sessions := &mockSessionRepo{
		createWithApiKeyFn: func(_ context.Context, userID int64, apiKeyID int64, _ time.Time, _ bool) (*model.Session, error) {
			createdWithApiKeyID = apiKeyID
			return &model.Session{ID: "mock-session-id", UserID: userID, ApiKeyID: &apiKeyID, ExpiresAt: time.Now().Add(30 * 24 * time.Hour)}, nil
		},
	}
	apiKeys := &mockApiKeyRepo{
		getByTokenFn: func(_ context.Context, token string) (*model.ApiKey, error) {
			if token == "abc123apikey" {
				return testApiKey, nil
			}
			return nil, errors.New("not found")
		},
	}

	h := handlers.NewSessionHandler(users, sessions, apiKeys)

	req := httptest.NewRequest(http.MethodGet, "/api/session?token=abc123apikey", nil)
	rr := httptest.NewRecorder()
	h.GetCurrentSession(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp["email"] != "apikey@example.com" {
		t.Errorf("expected email apikey@example.com, got %v", resp["email"])
	}

	// Verify CreateWithApiKey was called with the correct API key ID.
	if createdWithApiKeyID != 1 {
		t.Errorf("expected CreateWithApiKey called with apiKeyID=1, got %d", createdWithApiKeyID)
	}

	// Verify session cookie is set (for WebSocket compatibility).
	cookies := rr.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "session_id" && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Error("expected session_id cookie to be set for API key token auth")
	}
}

// TestGetCurrentSession_LegacyTokenFallback verifies that when the token
// is not found in api_keys, it falls back to the legacy users.token column.
func TestGetCurrentSession_LegacyTokenFallback(t *testing.T) {
	testUser := &model.User{ID: 7, Email: "legacy@example.com", Name: "Legacy User", Role: "user"}

	users := &mockUserRepo{
		getByTokenFn: func(_ context.Context, token string) (*model.User, error) {
			if token == "legacy-user-token" {
				return testUser, nil
			}
			return nil, errors.New("not found")
		},
	}
	sessions := &mockSessionRepo{}
	apiKeys := &mockApiKeyRepo{
		// API key lookup returns not found -- should fall back to legacy.
		getByTokenFn: func(_ context.Context, _ string) (*model.ApiKey, error) {
			return nil, errors.New("not found")
		},
	}

	h := handlers.NewSessionHandler(users, sessions, apiKeys)

	req := httptest.NewRequest(http.MethodGet, "/api/session?token=legacy-user-token", nil)
	rr := httptest.NewRecorder()
	h.GetCurrentSession(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp["email"] != "legacy@example.com" {
		t.Errorf("expected email legacy@example.com, got %v", resp["email"])
	}
}

// TestGetCurrentSession_InvalidTokenBothSources verifies that when the token
// is not found in either api_keys or users.token, a 401 is returned.
func TestGetCurrentSession_InvalidTokenBothSources(t *testing.T) {
	users := &mockUserRepo{
		getByTokenFn: func(_ context.Context, _ string) (*model.User, error) {
			return nil, errors.New("not found")
		},
	}
	sessions := &mockSessionRepo{}
	apiKeys := &mockApiKeyRepo{
		getByTokenFn: func(_ context.Context, _ string) (*model.ApiKey, error) {
			return nil, errors.New("not found")
		},
	}

	h := handlers.NewSessionHandler(users, sessions, apiKeys)

	req := httptest.NewRequest(http.MethodGet, "/api/session?token=nonexistent-token", nil)
	rr := httptest.NewRecorder()
	h.GetCurrentSession(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestGetCurrentSession_ApiKeyPriorityOverLegacy verifies that when the same
// token exists in both api_keys and users.token, the api_keys match takes
// priority (the legacy lookup is never called).
func TestGetCurrentSession_ApiKeyPriorityOverLegacy(t *testing.T) {
	apiKeyUser := &model.User{ID: 10, Email: "apikey-user@example.com", Name: "API Key User", Role: "user"}
	legacyUser := &model.User{ID: 20, Email: "legacy-user@example.com", Name: "Legacy User", Role: "user"}

	legacyLookupCalled := false

	users := &mockUserRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.User, error) {
			if id == 10 {
				return apiKeyUser, nil
			}
			return nil, errors.New("not found")
		},
		getByTokenFn: func(_ context.Context, _ string) (*model.User, error) {
			legacyLookupCalled = true
			return legacyUser, nil
		},
	}
	sessions := &mockSessionRepo{}
	apiKeys := &mockApiKeyRepo{
		getByTokenFn: func(_ context.Context, token string) (*model.ApiKey, error) {
			if token == "shared-token" {
				return &model.ApiKey{ID: 1, UserID: 10, Token: "shared-token", Name: "key", Permissions: "full"}, nil
			}
			return nil, errors.New("not found")
		},
	}

	h := handlers.NewSessionHandler(users, sessions, apiKeys)

	req := httptest.NewRequest(http.MethodGet, "/api/session?token=shared-token", nil)
	rr := httptest.NewRecorder()
	h.GetCurrentSession(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	// Should resolve to the API key user, not the legacy user.
	if resp["email"] != "apikey-user@example.com" {
		t.Errorf("expected email apikey-user@example.com (API key user), got %v", resp["email"])
	}

	if legacyLookupCalled {
		t.Error("legacy GetByToken should NOT have been called when API key matched")
	}
}

// TestGetCurrentSession_NilApiKeyRepo verifies graceful degradation when the
// apiKeys repository is nil (e.g., in tests that don't set it up).
func TestGetCurrentSession_NilApiKeyRepo(t *testing.T) {
	testUser := &model.User{ID: 5, Email: "nilrepo@example.com", Name: "Nil Repo User", Role: "user"}

	users := &mockUserRepo{
		getByTokenFn: func(_ context.Context, token string) (*model.User, error) {
			if token == "legacy-only-token" {
				return testUser, nil
			}
			return nil, errors.New("not found")
		},
	}
	sessions := &mockSessionRepo{}

	// Pass nil for apiKeys to test graceful degradation.
	h := handlers.NewSessionHandler(users, sessions, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/session?token=legacy-only-token", nil)
	rr := httptest.NewRecorder()
	h.GetCurrentSession(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 (legacy fallback), got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp["email"] != "nilrepo@example.com" {
		t.Errorf("expected email nilrepo@example.com, got %v", resp["email"])
	}
}

// TestGetCurrentSession_SessionCreateFails verifies that a 500 is returned
// when token auth succeeds but session creation fails.
func TestGetCurrentSession_SessionCreateFails(t *testing.T) {
	testUser := &model.User{ID: 42, Email: "fail@example.com", Name: "Fail User", Role: "user"}

	users := &mockUserRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.User, error) {
			if id == 42 {
				return testUser, nil
			}
			return nil, errors.New("not found")
		},
	}
	sessions := &mockSessionRepo{
		// The API key token path now uses CreateWithApiKey.
		createWithApiKeyFn: func(_ context.Context, _ int64, _ int64, _ time.Time, _ bool) (*model.Session, error) {
			return nil, errors.New("database error")
		},
	}
	apiKeys := &mockApiKeyRepo{
		getByTokenFn: func(_ context.Context, token string) (*model.ApiKey, error) {
			if token == "valid-key" {
				return &model.ApiKey{ID: 1, UserID: 42, Token: "valid-key", Name: "key", Permissions: "full"}, nil
			}
			return nil, errors.New("not found")
		},
	}

	h := handlers.NewSessionHandler(users, sessions, apiKeys)

	req := httptest.NewRequest(http.MethodGet, "/api/session?token=valid-key", nil)
	rr := httptest.NewRecorder()
	h.GetCurrentSession(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500 when session creation fails, got %d", rr.Code)
	}
}

// TestGetCurrentSession_LegacyToken_NoApiKeyLinked verifies that when a token
// is resolved via legacy users.token column (not an API key), the session is
// created using CreateWithExpiry (not CreateWithApiKey) with a 10-year expiry.
func TestGetCurrentSession_LegacyToken_NoApiKeyLinked(t *testing.T) {
	testUser := &model.User{ID: 7, Email: "legacy@example.com", Name: "Legacy User", Role: "user"}
	createWithExpiryCalled := false
	createWithApiKeyCalled := false

	users := &mockUserRepo{
		getByTokenFn: func(_ context.Context, token string) (*model.User, error) {
			if token == "legacy-token" {
				return testUser, nil
			}
			return nil, errors.New("not found")
		},
	}
	sessions := &mockSessionRepo{
		createWithExpiryFn: func(_ context.Context, userID int64, expiresAt time.Time, rememberMe bool) (*model.Session, error) {
			createWithExpiryCalled = true
			return &model.Session{ID: "mock-session-id", UserID: userID, RememberMe: rememberMe, ExpiresAt: expiresAt}, nil
		},
		createWithApiKeyFn: func(_ context.Context, _ int64, _ int64, _ time.Time, _ bool) (*model.Session, error) {
			createWithApiKeyCalled = true
			return nil, errors.New("should not be called")
		},
	}
	apiKeys := &mockApiKeyRepo{
		getByTokenFn: func(_ context.Context, _ string) (*model.ApiKey, error) {
			return nil, errors.New("not found")
		},
	}

	h := handlers.NewSessionHandler(users, sessions, apiKeys)

	req := httptest.NewRequest(http.MethodGet, "/api/session?token=legacy-token", nil)
	rr := httptest.NewRecorder()
	h.GetCurrentSession(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if !createWithExpiryCalled {
		t.Error("expected CreateWithExpiry to be called for legacy token")
	}
	if createWithApiKeyCalled {
		t.Error("CreateWithApiKey should NOT have been called for legacy token")
	}
}

// TestGetCurrentSession_TokenLoginExpiry verifies that token-based login
// (via ?token= query parameter) creates sessions with the remember-me
// expiry duration (sessionExpiryRememberMe = 10 years), so the session
// survives browser restarts.
func TestGetCurrentSession_TokenLoginExpiry(t *testing.T) {
	// expectedExpiry matches handlers.sessionExpiryRememberMe (30 days).
	expectedExpiry := 30 * 24 * time.Hour

	t.Run("api key token", func(t *testing.T) {
		testUser := &model.User{ID: 42, Email: "apikey@example.com", Name: "API Key User", Role: "user"}
		testApiKey := &model.ApiKey{ID: 1, UserID: 42, Token: "abc123apikey", Name: "test-key", Permissions: "full"}

		var capturedExpiry time.Time
		var capturedRememberMe bool
		users := &mockUserRepo{
			getByIDFn: func(_ context.Context, id int64) (*model.User, error) {
				if id == 42 {
					return testUser, nil
				}
				return nil, errors.New("not found")
			},
		}
		sessions := &mockSessionRepo{
			createWithApiKeyFn: func(_ context.Context, userID int64, apiKeyID int64, expiresAt time.Time, rememberMe bool) (*model.Session, error) {
				capturedExpiry = expiresAt
				capturedRememberMe = rememberMe
				return &model.Session{ID: "mock-session-id", UserID: userID, ApiKeyID: &apiKeyID, RememberMe: rememberMe, ExpiresAt: expiresAt}, nil
			},
		}
		apiKeys := &mockApiKeyRepo{
			getByTokenFn: func(_ context.Context, token string) (*model.ApiKey, error) {
				if token == "abc123apikey" {
					return testApiKey, nil
				}
				return nil, errors.New("not found")
			},
		}

		h := handlers.NewSessionHandler(users, sessions, apiKeys)
		req := httptest.NewRequest(http.MethodGet, "/api/session?token=abc123apikey", nil)
		rr := httptest.NewRecorder()
		h.GetCurrentSession(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d; body: %s", rr.Code, rr.Body.String())
		}

		expiresIn := time.Until(capturedExpiry)
		if expiresIn < expectedExpiry-time.Hour || expiresIn > expectedExpiry+time.Hour {
			t.Errorf("expected session expiry ~%v, got %v", expectedExpiry, expiresIn)
		}
		if !capturedRememberMe {
			t.Error("expected rememberMe=true for token login")
		}

		for _, c := range rr.Result().Cookies() {
			if c.Name == "session_id" {
				cookieExpiresIn := time.Until(c.Expires)
				if cookieExpiresIn < expectedExpiry-time.Hour {
					t.Errorf("expected cookie expiry ~%v, got %v", expectedExpiry, cookieExpiresIn)
				}
				return
			}
		}
		t.Error("expected session_id cookie to be set")
	})

	t.Run("legacy token", func(t *testing.T) {
		testUser := &model.User{ID: 7, Email: "legacy@example.com", Name: "Legacy User", Role: "user"}

		var capturedExpiry time.Time
		var capturedRememberMe bool
		users := &mockUserRepo{
			getByTokenFn: func(_ context.Context, token string) (*model.User, error) {
				if token == "legacy-tok-expiry" {
					return testUser, nil
				}
				return nil, errors.New("not found")
			},
		}
		sessions := &mockSessionRepo{
			createWithExpiryFn: func(_ context.Context, userID int64, expiresAt time.Time, rememberMe bool) (*model.Session, error) {
				capturedExpiry = expiresAt
				capturedRememberMe = rememberMe
				return &model.Session{ID: "mock-session-id", UserID: userID, RememberMe: rememberMe, ExpiresAt: expiresAt}, nil
			},
		}
		apiKeys := &mockApiKeyRepo{
			getByTokenFn: func(_ context.Context, _ string) (*model.ApiKey, error) {
				return nil, errors.New("not found")
			},
		}

		h := handlers.NewSessionHandler(users, sessions, apiKeys)
		req := httptest.NewRequest(http.MethodGet, "/api/session?token=legacy-tok-expiry", nil)
		rr := httptest.NewRecorder()
		h.GetCurrentSession(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d; body: %s", rr.Code, rr.Body.String())
		}

		expiresIn := time.Until(capturedExpiry)
		if expiresIn < expectedExpiry-time.Hour || expiresIn > expectedExpiry+time.Hour {
			t.Errorf("expected session expiry ~%v, got %v", expectedExpiry, expiresIn)
		}
		if !capturedRememberMe {
			t.Error("expected rememberMe=true for token login")
		}

		for _, c := range rr.Result().Cookies() {
			if c.Name == "session_id" {
				cookieExpiresIn := time.Until(c.Expires)
				if cookieExpiresIn < expectedExpiry-time.Hour {
					t.Errorf("expected cookie expiry ~%v, got %v", expectedExpiry, cookieExpiresIn)
				}
				return
			}
		}
		t.Error("expected session_id cookie to be set")
	})
}

// TestGetCurrentSession_ReadonlyApiKeyToken_LinksApiKey verifies that a
// readonly API key token creates a session linked to the API key, so that
// the auth middleware can later enforce read-only restrictions.
func TestGetCurrentSession_ReadonlyApiKeyToken_LinksApiKey(t *testing.T) {
	testUser := &model.User{ID: 42, Email: "readonly@example.com", Name: "Readonly User", Role: "user"}
	readonlyKey := &model.ApiKey{ID: 99, UserID: 42, Token: "readonly-key", Name: "readonly-key", Permissions: model.PermissionReadonly}

	var capturedApiKeyID int64
	users := &mockUserRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.User, error) {
			if id == 42 {
				return testUser, nil
			}
			return nil, errors.New("not found")
		},
	}
	sessions := &mockSessionRepo{
		createWithApiKeyFn: func(_ context.Context, userID int64, apiKeyID int64, expiresAt time.Time, _ bool) (*model.Session, error) {
			capturedApiKeyID = apiKeyID
			return &model.Session{ID: "readonly-session", UserID: userID, ApiKeyID: &apiKeyID, ExpiresAt: expiresAt}, nil
		},
	}
	apiKeys := &mockApiKeyRepo{
		getByTokenFn: func(_ context.Context, token string) (*model.ApiKey, error) {
			if token == "readonly-key" {
				return readonlyKey, nil
			}
			return nil, errors.New("not found")
		},
	}

	h := handlers.NewSessionHandler(users, sessions, apiKeys)

	req := httptest.NewRequest(http.MethodGet, "/api/session?token=readonly-key", nil)
	rr := httptest.NewRecorder()
	h.GetCurrentSession(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if capturedApiKeyID != 99 {
		t.Errorf("expected CreateWithApiKey called with apiKeyID=99, got %d", capturedApiKeyID)
	}
}

func TestLogoutAll_RevokesOtherSessions(t *testing.T) {
	user := &model.User{ID: 3, Email: "user@example.com", Role: "user"}

	var capturedUserID int64
	var capturedExceptID string
	sessions := &mockSessionRepo{
		deleteAllByUserFn: func(_ context.Context, userID int64, exceptID string) error {
			capturedUserID = userID
			capturedExceptID = exceptID
			return nil
		},
	}

	h := handlers.NewSessionHandler(&mockUserRepo{}, sessions, &mockApiKeyRepo{})
	req := httptest.NewRequest(http.MethodDelete, "/api/sessions", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "current-sess"})
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()
	h.LogoutAll(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if capturedUserID != 3 {
		t.Errorf("expected userID=3, got %d", capturedUserID)
	}
	if capturedExceptID != "current-sess" {
		t.Errorf("expected exceptID=current-sess, got %s", capturedExceptID)
	}
}

func TestLogoutAll_NoSessionCookie_Returns400(t *testing.T) {
	user := &model.User{ID: 3, Email: "user@example.com", Role: "user"}
	sessions := &mockSessionRepo{}

	h := handlers.NewSessionHandler(&mockUserRepo{}, sessions, &mockApiKeyRepo{})
	req := httptest.NewRequest(http.MethodDelete, "/api/sessions", nil)
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()
	h.LogoutAll(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when no session cookie, got %d", rr.Code)
	}
}

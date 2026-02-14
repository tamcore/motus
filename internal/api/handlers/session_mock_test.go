package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/api/handlers"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// mockUserRepo is a mock implementation of repository.UserRepo for
// unit testing without a database.
type mockUserRepo struct {
	createFn           func(ctx context.Context, user *model.User) error
	createOIDCUserFn   func(ctx context.Context, email, name, role, subject, issuer string) (*model.User, error)
	getByEmailFn       func(ctx context.Context, email string) (*model.User, error)
	getByIDFn          func(ctx context.Context, id int64) (*model.User, error)
	getByTokenFn       func(ctx context.Context, token string) (*model.User, error)
	getByOIDCSubjectFn func(ctx context.Context, subject, issuer string) (*model.User, error)
	setOIDCSubjectFn   func(ctx context.Context, userID int64, subject, issuer string) error
	listAllFn          func(ctx context.Context) ([]*model.User, error)
	updateFn           func(ctx context.Context, user *model.User) error
	updatePasswordFn   func(ctx context.Context, userID int64, hash string) error
	deleteFn           func(ctx context.Context, id int64) error
	getDevicesForUser  func(ctx context.Context, userID int64) ([]int64, error)
	assignDeviceFn     func(ctx context.Context, userID, deviceID int64) error
	unassignDeviceFn   func(ctx context.Context, userID, deviceID int64) error
	generateTokenFn    func(ctx context.Context, userID int64) (string, error)
}

var _ repository.UserRepo = (*mockUserRepo)(nil)

func (m *mockUserRepo) Create(ctx context.Context, user *model.User) error {
	if m.createFn != nil {
		return m.createFn(ctx, user)
	}
	return errors.New("not implemented")
}
func (m *mockUserRepo) CreateOIDCUser(ctx context.Context, email, name, role, subject, issuer string) (*model.User, error) {
	if m.createOIDCUserFn != nil {
		return m.createOIDCUserFn(ctx, email, name, role, subject, issuer)
	}
	return nil, errors.New("not implemented")
}
func (m *mockUserRepo) GetByOIDCSubject(ctx context.Context, subject, issuer string) (*model.User, error) {
	if m.getByOIDCSubjectFn != nil {
		return m.getByOIDCSubjectFn(ctx, subject, issuer)
	}
	return nil, errors.New("not found")
}
func (m *mockUserRepo) SetOIDCSubject(ctx context.Context, userID int64, subject, issuer string) error {
	if m.setOIDCSubjectFn != nil {
		return m.setOIDCSubjectFn(ctx, userID, subject, issuer)
	}
	return nil
}
func (m *mockUserRepo) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	if m.getByEmailFn != nil {
		return m.getByEmailFn(ctx, email)
	}
	return nil, errors.New("not found")
}
func (m *mockUserRepo) GetByID(ctx context.Context, id int64) (*model.User, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, errors.New("not found")
}
func (m *mockUserRepo) GetByToken(ctx context.Context, token string) (*model.User, error) {
	if m.getByTokenFn != nil {
		return m.getByTokenFn(ctx, token)
	}
	return nil, errors.New("not found")
}
func (m *mockUserRepo) ListAll(ctx context.Context) ([]*model.User, error) {
	if m.listAllFn != nil {
		return m.listAllFn(ctx)
	}
	return nil, nil
}
func (m *mockUserRepo) Update(ctx context.Context, user *model.User) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, user)
	}
	return errors.New("not implemented")
}
func (m *mockUserRepo) UpdatePassword(ctx context.Context, userID int64, hash string) error {
	if m.updatePasswordFn != nil {
		return m.updatePasswordFn(ctx, userID, hash)
	}
	return errors.New("not implemented")
}
func (m *mockUserRepo) Delete(ctx context.Context, id int64) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return errors.New("not implemented")
}
func (m *mockUserRepo) GetDevicesForUser(ctx context.Context, userID int64) ([]int64, error) {
	if m.getDevicesForUser != nil {
		return m.getDevicesForUser(ctx, userID)
	}
	return nil, nil
}
func (m *mockUserRepo) AssignDevice(ctx context.Context, userID, deviceID int64) error {
	if m.assignDeviceFn != nil {
		return m.assignDeviceFn(ctx, userID, deviceID)
	}
	return errors.New("not implemented")
}
func (m *mockUserRepo) UnassignDevice(ctx context.Context, userID, deviceID int64) error {
	if m.unassignDeviceFn != nil {
		return m.unassignDeviceFn(ctx, userID, deviceID)
	}
	return errors.New("not implemented")
}
func (m *mockUserRepo) GenerateToken(ctx context.Context, userID int64) (string, error) {
	if m.generateTokenFn != nil {
		return m.generateTokenFn(ctx, userID)
	}
	return "", errors.New("not implemented")
}

// mockSessionRepo is a mock implementation of repository.SessionRepo.
type mockSessionRepo struct {
	createFn           func(ctx context.Context, userID int64) (*model.Session, error)
	createWithExpiryFn func(ctx context.Context, userID int64, expiresAt time.Time, rememberMe bool) (*model.Session, error)
	createWithApiKeyFn func(ctx context.Context, userID int64, apiKeyID int64, expiresAt time.Time, rememberMe bool) (*model.Session, error)
	createSudoFn       func(ctx context.Context, targetUserID, originalUserID int64) (*model.Session, error)
	getByIDFn          func(ctx context.Context, id string) (*model.Session, error)
	getByIDPrefixFn    func(ctx context.Context, userID int64, prefix string) (*model.Session, error)
	deleteFn           func(ctx context.Context, id string) error
	listByUserFn       func(ctx context.Context, userID int64) ([]*model.Session, error)
}

var _ repository.SessionRepo = (*mockSessionRepo)(nil)

func (m *mockSessionRepo) Create(ctx context.Context, userID int64) (*model.Session, error) {
	if m.createFn != nil {
		return m.createFn(ctx, userID)
	}
	return &model.Session{ID: "mock-session-id", UserID: userID, ExpiresAt: time.Now().Add(24 * time.Hour)}, nil
}
func (m *mockSessionRepo) CreateWithExpiry(ctx context.Context, userID int64, expiresAt time.Time, rememberMe bool) (*model.Session, error) {
	if m.createWithExpiryFn != nil {
		return m.createWithExpiryFn(ctx, userID, expiresAt, rememberMe)
	}
	return &model.Session{ID: "mock-session-id", UserID: userID, ExpiresAt: expiresAt}, nil
}
func (m *mockSessionRepo) CreateWithApiKey(ctx context.Context, userID int64, apiKeyID int64, expiresAt time.Time, rememberMe bool) (*model.Session, error) {
	if m.createWithApiKeyFn != nil {
		return m.createWithApiKeyFn(ctx, userID, apiKeyID, expiresAt, rememberMe)
	}
	return &model.Session{ID: "mock-session-id", UserID: userID, ApiKeyID: &apiKeyID, RememberMe: rememberMe, ExpiresAt: expiresAt}, nil
}
func (m *mockSessionRepo) CreateSudo(ctx context.Context, targetUserID, originalUserID int64) (*model.Session, error) {
	if m.createSudoFn != nil {
		return m.createSudoFn(ctx, targetUserID, originalUserID)
	}
	return nil, errors.New("not implemented")
}
func (m *mockSessionRepo) GetByID(ctx context.Context, id string) (*model.Session, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, errors.New("not found")
}
func (m *mockSessionRepo) GetByIDPrefix(ctx context.Context, userID int64, prefix string) (*model.Session, error) {
	if m.getByIDPrefixFn != nil {
		return m.getByIDPrefixFn(ctx, userID, prefix)
	}
	return nil, errors.New("not found")
}
func (m *mockSessionRepo) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}
func (m *mockSessionRepo) ListByUser(ctx context.Context, userID int64) ([]*model.Session, error) {
	if m.listByUserFn != nil {
		return m.listByUserFn(ctx, userID)
	}
	return nil, nil
}

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
			return &model.Session{ID: "mock-session-id", UserID: userID, ApiKeyID: &apiKeyID, ExpiresAt: time.Now().Add(10 * 365 * 24 * time.Hour)}, nil
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
	// expectedExpiry matches handlers.sessionExpiryRememberMe (10 years = 87600 hours).
	expectedExpiry := 87600 * time.Hour

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

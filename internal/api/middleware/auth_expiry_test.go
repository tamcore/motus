package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/api/middleware"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// --- Mock repos for auth middleware tests ---

// mockUserRepo satisfies repository.UserRepo for middleware tests.
type mockUserRepo struct {
	getByIDFn    func(ctx context.Context, id int64) (*model.User, error)
	getByTokenFn func(ctx context.Context, token string) (*model.User, error)
}

var _ repository.UserRepo = (*mockUserRepo)(nil)

func (m *mockUserRepo) Create(_ context.Context, _ *model.User) error { return nil }
func (m *mockUserRepo) CreateOIDCUser(_ context.Context, _, _, _, _, _ string) (*model.User, error) {
	return nil, errors.New("not implemented")
}
func (m *mockUserRepo) Update(_ context.Context, _ *model.User) error { return nil }
func (m *mockUserRepo) Delete(_ context.Context, _ int64) error       { return nil }
func (m *mockUserRepo) ListAll(_ context.Context) ([]*model.User, error) {
	return nil, nil
}
func (m *mockUserRepo) UpdatePassword(_ context.Context, _ int64, _ string) error { return nil }
func (m *mockUserRepo) GetDevicesForUser(_ context.Context, _ int64) ([]int64, error) {
	return nil, nil
}
func (m *mockUserRepo) AssignDevice(_ context.Context, _, _ int64) error   { return nil }
func (m *mockUserRepo) UnassignDevice(_ context.Context, _, _ int64) error { return nil }
func (m *mockUserRepo) GenerateToken(_ context.Context, _ int64) (string, error) {
	return "", nil
}
func (m *mockUserRepo) GetByEmail(_ context.Context, _ string) (*model.User, error) {
	return nil, errors.New("not found")
}
func (m *mockUserRepo) GetByOIDCSubject(_ context.Context, _, _ string) (*model.User, error) {
	return nil, errors.New("not found")
}
func (m *mockUserRepo) SetOIDCSubject(_ context.Context, _ int64, _, _ string) error {
	return nil
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

// mockSessionRepo satisfies repository.SessionRepo for middleware tests.
type mockSessionRepo struct {
	getByIDFn      func(ctx context.Context, id string) (*model.Session, error)
	updateExpiryFn func(ctx context.Context, id string, expiresAt time.Time)
}

var _ repository.SessionRepo = (*mockSessionRepo)(nil)

func (m *mockSessionRepo) Create(_ context.Context, _ int64) (*model.Session, error) {
	return nil, nil
}
func (m *mockSessionRepo) CreateWithExpiry(_ context.Context, _ int64, _ time.Time, _ bool) (*model.Session, error) {
	return nil, nil
}
func (m *mockSessionRepo) CreateWithApiKey(_ context.Context, _ int64, _ int64, _ time.Time, _ bool) (*model.Session, error) {
	return nil, nil
}
func (m *mockSessionRepo) CreateSudo(_ context.Context, _, _ int64) (*model.Session, error) {
	return nil, nil
}
func (m *mockSessionRepo) Delete(_ context.Context, _ string) error { return nil }
func (m *mockSessionRepo) ListByUser(_ context.Context, _ int64) ([]*model.Session, error) {
	return nil, nil
}
func (m *mockSessionRepo) GetByID(ctx context.Context, id string) (*model.Session, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, errors.New("not found")
}
func (m *mockSessionRepo) GetByIDPrefix(_ context.Context, _ int64, _ string) (*model.Session, error) {
	return nil, errors.New("not found")
}
func (m *mockSessionRepo) UpdateLastSeen(_ context.Context, _, _, _ string) error { return nil }
func (m *mockSessionRepo) UpdateExpiry(ctx context.Context, id string, expiresAt time.Time) error {
	if m.updateExpiryFn != nil {
		m.updateExpiryFn(ctx, id, expiresAt)
	}
	return nil
}
func (m *mockSessionRepo) DeleteAllByUser(_ context.Context, _ int64, _ string) error { return nil }

// mockApiKeyRepo satisfies repository.ApiKeyRepo for middleware tests.
type mockApiKeyRepo struct {
	getByTokenFn     func(ctx context.Context, token string) (*model.ApiKey, error)
	getByIDFn        func(ctx context.Context, id int64) (*model.ApiKey, error)
	createFn         func(ctx context.Context, key *model.ApiKey) error
	listByUserFn     func(ctx context.Context, userID int64) ([]*model.ApiKey, error)
	deleteFn         func(ctx context.Context, id int64) error
	updateLastUsedFn func(ctx context.Context, id int64) error
}

var _ repository.ApiKeyRepo = (*mockApiKeyRepo)(nil)

func (m *mockApiKeyRepo) Create(ctx context.Context, key *model.ApiKey) error {
	if m.createFn != nil {
		return m.createFn(ctx, key)
	}
	return nil
}
func (m *mockApiKeyRepo) GetByToken(ctx context.Context, token string) (*model.ApiKey, error) {
	if m.getByTokenFn != nil {
		return m.getByTokenFn(ctx, token)
	}
	return nil, errors.New("not found")
}
func (m *mockApiKeyRepo) GetByID(ctx context.Context, id int64) (*model.ApiKey, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, errors.New("not found")
}
func (m *mockApiKeyRepo) ListByUser(ctx context.Context, userID int64) ([]*model.ApiKey, error) {
	if m.listByUserFn != nil {
		return m.listByUserFn(ctx, userID)
	}
	return nil, nil
}
func (m *mockApiKeyRepo) Delete(ctx context.Context, id int64) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}
func (m *mockApiKeyRepo) UpdateLastUsed(ctx context.Context, id int64) error {
	if m.updateLastUsedFn != nil {
		return m.updateLastUsedFn(ctx, id)
	}
	return nil
}

// --- Expiration tests ---

func TestAuthMiddleware_ExpiredBearerToken_Returns401(t *testing.T) {
	pastTime := time.Now().Add(-1 * time.Hour)
	apiKeyRepo := &mockApiKeyRepo{
		getByTokenFn: func(_ context.Context, _ string) (*model.ApiKey, error) {
			return &model.ApiKey{
				ID:          1,
				UserID:      1,
				Token:       "expired-token",
				Name:        "Expired Key",
				Permissions: model.PermissionFull,
				ExpiresAt:   &pastTime,
			}, nil
		},
	}
	userRepo := &mockUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			return &model.User{ID: 1, Email: "test@example.com"}, nil
		},
	}
	sessionRepo := &mockSessionRepo{}

	mw := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called for expired API key")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.Header.Set("Authorization", "Bearer expired-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestAuthMiddleware_ValidBearerToken_NotExpired(t *testing.T) {
	futureTime := time.Now().Add(24 * time.Hour)
	handlerCalled := false

	apiKeyRepo := &mockApiKeyRepo{
		getByTokenFn: func(_ context.Context, _ string) (*model.ApiKey, error) {
			return &model.ApiKey{
				ID:          1,
				UserID:      1,
				Token:       "valid-token",
				Name:        "Valid Key",
				Permissions: model.PermissionFull,
				ExpiresAt:   &futureTime,
			}, nil
		},
	}
	userRepo := &mockUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			return &model.User{ID: 1, Email: "test@example.com"}, nil
		},
	}
	sessionRepo := &mockSessionRepo{}

	mw := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if !handlerCalled {
		t.Error("expected handler to be called for valid non-expired API key")
	}
}

func TestAuthMiddleware_NilExpiresAt_NeverExpires(t *testing.T) {
	handlerCalled := false

	apiKeyRepo := &mockApiKeyRepo{
		getByTokenFn: func(_ context.Context, _ string) (*model.ApiKey, error) {
			return &model.ApiKey{
				ID:          1,
				UserID:      1,
				Token:       "forever-token",
				Name:        "Forever Key",
				Permissions: model.PermissionFull,
				ExpiresAt:   nil, // Never expires.
			}, nil
		},
	}
	userRepo := &mockUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			return &model.User{ID: 1, Email: "test@example.com"}, nil
		},
	}
	sessionRepo := &mockSessionRepo{}

	mw := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.Header.Set("Authorization", "Bearer forever-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if !handlerCalled {
		t.Error("expected handler to be called for key without expiration")
	}
}

// --- Session rolling tests ---

func TestAuthMiddleware_RememberMeSessionRollsWhenNearExpiry_Cookie(t *testing.T) {
	rolledCh := make(chan time.Time, 1)

	nearExpiry := time.Now().Add(10 * 24 * time.Hour) // < 15-day threshold
	sessionRepo := &mockSessionRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.Session, error) {
			return &model.Session{
				ID:         "sess-roll",
				UserID:     1,
				RememberMe: true,
				ExpiresAt:  nearExpiry,
			}, nil
		},
		updateExpiryFn: func(_ context.Context, _ string, expiresAt time.Time) {
			rolledCh <- expiresAt
		},
	}
	userRepo := &mockUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			return &model.User{ID: 1, Email: "test@example.com"}, nil
		},
	}

	mw := middleware.Auth(userRepo, sessionRepo, &mockApiKeyRepo{})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "sess-roll"})
	handler.ServeHTTP(httptest.NewRecorder(), req)

	select {
	case newExpiry := <-rolledCh:
		// New expiry should be ~30 days from now.
		if time.Until(newExpiry) < 29*24*time.Hour || time.Until(newExpiry) > 31*24*time.Hour {
			t.Errorf("expected new expiry ~30 days from now, got %v", time.Until(newExpiry))
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("expected UpdateExpiry to be called for near-expiry remember-me session (cookie path)")
	}
}

func TestAuthMiddleware_RememberMeSessionRollsWhenNearExpiry_XAuthToken(t *testing.T) {
	rolledCh := make(chan time.Time, 1)

	nearExpiry := time.Now().Add(5 * 24 * time.Hour) // < 15-day threshold
	sessionRepo := &mockSessionRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.Session, error) {
			return &model.Session{
				ID:         "sess-roll-xat",
				UserID:     1,
				RememberMe: true,
				ExpiresAt:  nearExpiry,
			}, nil
		},
		updateExpiryFn: func(_ context.Context, _ string, expiresAt time.Time) {
			rolledCh <- expiresAt
		},
	}
	userRepo := &mockUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			return &model.User{ID: 1, Email: "test@example.com"}, nil
		},
	}

	mw := middleware.Auth(userRepo, sessionRepo, &mockApiKeyRepo{})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.Header.Set("X-Auth-Token", "sess-roll-xat")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	select {
	case <-rolledCh:
		// Rolling fired — pass.
	case <-time.After(500 * time.Millisecond):
		t.Error("expected UpdateExpiry to be called for near-expiry remember-me session (X-Auth-Token path)")
	}
}

func TestAuthMiddleware_NonRememberMeSession_NotRolled(t *testing.T) {
	nearExpiry := time.Now().Add(5 * 24 * time.Hour)
	sessionRepo := &mockSessionRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.Session, error) {
			return &model.Session{
				ID:         "sess-noroll",
				UserID:     1,
				RememberMe: false, // not remember-me
				ExpiresAt:  nearExpiry,
			}, nil
		},
		updateExpiryFn: func(_ context.Context, _ string, _ time.Time) {
			// Should never be called.
		},
	}
	rolledCh := make(chan struct{}, 1)
	sessionRepo.updateExpiryFn = func(_ context.Context, _ string, _ time.Time) {
		rolledCh <- struct{}{}
	}
	userRepo := &mockUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			return &model.User{ID: 1, Email: "test@example.com"}, nil
		},
	}

	mw := middleware.Auth(userRepo, sessionRepo, &mockApiKeyRepo{})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "sess-noroll"})
	handler.ServeHTTP(httptest.NewRecorder(), req)

	select {
	case <-rolledCh:
		t.Error("UpdateExpiry should NOT be called for non-remember-me session")
	case <-time.After(200 * time.Millisecond):
		// Good — not rolled.
	}
}

func TestAuthMiddleware_RememberMeSessionFarFromExpiry_NotRolled(t *testing.T) {
	farExpiry := time.Now().Add(25 * 24 * time.Hour) // > 15-day threshold
	rolledCh := make(chan struct{}, 1)

	sessionRepo := &mockSessionRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.Session, error) {
			return &model.Session{
				ID:         "sess-far",
				UserID:     1,
				RememberMe: true,
				ExpiresAt:  farExpiry,
			}, nil
		},
		updateExpiryFn: func(_ context.Context, _ string, _ time.Time) {
			rolledCh <- struct{}{}
		},
	}
	userRepo := &mockUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			return &model.User{ID: 1, Email: "test@example.com"}, nil
		},
	}

	mw := middleware.Auth(userRepo, sessionRepo, &mockApiKeyRepo{})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "sess-far"})
	handler.ServeHTTP(httptest.NewRecorder(), req)

	select {
	case <-rolledCh:
		t.Error("UpdateExpiry should NOT be called when more than 15 days remain")
	case <-time.After(200 * time.Millisecond):
		// Good — not rolled.
	}
}

func TestAuthMiddleware_ExpiredKeyViaSessionCookie_Returns401(t *testing.T) {
	pastTime := time.Now().Add(-1 * time.Hour)
	apiKeyID := int64(5)

	sessionRepo := &mockSessionRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.Session, error) {
			return &model.Session{
				ID:       "sess-123",
				UserID:   1,
				ApiKeyID: &apiKeyID,
			}, nil
		},
	}
	userRepo := &mockUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			return &model.User{ID: 1, Email: "test@example.com"}, nil
		},
	}
	apiKeyRepo := &mockApiKeyRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.ApiKey, error) {
			return &model.ApiKey{
				ID:          5,
				UserID:      1,
				Permissions: model.PermissionReadonly,
				ExpiresAt:   &pastTime,
			}, nil
		},
	}

	mw := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called for expired API key via session")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "sess-123"})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

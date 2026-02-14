package handlers

// White-box tests for OIDCHandler. Being in package handlers (not handlers_test)
// lets us access unexported fields and methods (resolveOIDCUser, errSignupDisabled).

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/config"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"golang.org/x/oauth2"
)

// ── mock types ───────────────────────────────────────────────────────────────

// oidcTestUserRepo is a test double for repository.UserRepo, used only in
// this file's white-box OIDC tests.
type oidcTestUserRepo struct {
	getByOIDCSubjectFn func(ctx context.Context, subject, issuer string) (*model.User, error)
	getByEmailFn       func(ctx context.Context, email string) (*model.User, error)
	setOIDCSubjectFn   func(ctx context.Context, userID int64, subject, issuer string) error
	createOIDCUserFn   func(ctx context.Context, email, name, role, subject, issuer string) (*model.User, error)
	updateFn           func(ctx context.Context, user *model.User) error
}

var _ repository.UserRepo = (*oidcTestUserRepo)(nil)

func (m *oidcTestUserRepo) Create(_ context.Context, _ *model.User) error {
	return errors.New("not implemented")
}
func (m *oidcTestUserRepo) CreateOIDCUser(ctx context.Context, email, name, role, subject, issuer string) (*model.User, error) {
	if m.createOIDCUserFn != nil {
		return m.createOIDCUserFn(ctx, email, name, role, subject, issuer)
	}
	return nil, errors.New("not found")
}
func (m *oidcTestUserRepo) GetByOIDCSubject(ctx context.Context, subject, issuer string) (*model.User, error) {
	if m.getByOIDCSubjectFn != nil {
		return m.getByOIDCSubjectFn(ctx, subject, issuer)
	}
	return nil, errors.New("not found")
}
func (m *oidcTestUserRepo) SetOIDCSubject(ctx context.Context, userID int64, subject, issuer string) error {
	if m.setOIDCSubjectFn != nil {
		return m.setOIDCSubjectFn(ctx, userID, subject, issuer)
	}
	return nil
}
func (m *oidcTestUserRepo) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	if m.getByEmailFn != nil {
		return m.getByEmailFn(ctx, email)
	}
	return nil, errors.New("not found")
}
func (m *oidcTestUserRepo) GetByID(_ context.Context, _ int64) (*model.User, error) {
	return nil, errors.New("not implemented")
}
func (m *oidcTestUserRepo) GetByToken(_ context.Context, _ string) (*model.User, error) {
	return nil, errors.New("not implemented")
}
func (m *oidcTestUserRepo) ListAll(_ context.Context) ([]*model.User, error) { return nil, nil }
func (m *oidcTestUserRepo) Update(ctx context.Context, user *model.User) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, user)
	}
	return nil
}
func (m *oidcTestUserRepo) UpdatePassword(_ context.Context, _ int64, _ string) error {
	return errors.New("not implemented")
}
func (m *oidcTestUserRepo) Delete(_ context.Context, _ int64) error {
	return errors.New("not implemented")
}
func (m *oidcTestUserRepo) GetDevicesForUser(_ context.Context, _ int64) ([]int64, error) {
	return nil, nil
}
func (m *oidcTestUserRepo) AssignDevice(_ context.Context, _, _ int64) error {
	return errors.New("not implemented")
}
func (m *oidcTestUserRepo) UnassignDevice(_ context.Context, _, _ int64) error {
	return errors.New("not implemented")
}
func (m *oidcTestUserRepo) GenerateToken(_ context.Context, _ int64) (string, error) {
	return "", errors.New("not implemented")
}

// oidcTestSessionRepo is a minimal test double for repository.SessionRepo.
type oidcTestSessionRepo struct {
	createWithExpiryFn func(ctx context.Context, userID int64, expiresAt time.Time, rememberMe bool) (*model.Session, error)
}

var _ repository.SessionRepo = (*oidcTestSessionRepo)(nil)

func (m *oidcTestSessionRepo) Create(_ context.Context, _ int64) (*model.Session, error) {
	return nil, errors.New("not implemented")
}
func (m *oidcTestSessionRepo) CreateWithExpiry(ctx context.Context, userID int64, expiresAt time.Time, rememberMe bool) (*model.Session, error) {
	if m.createWithExpiryFn != nil {
		return m.createWithExpiryFn(ctx, userID, expiresAt, rememberMe)
	}
	return &model.Session{ID: "test-session", UserID: userID, ExpiresAt: expiresAt}, nil
}
func (m *oidcTestSessionRepo) CreateWithApiKey(_ context.Context, _ int64, _ int64, _ time.Time, _ bool) (*model.Session, error) {
	return nil, errors.New("not implemented")
}
func (m *oidcTestSessionRepo) CreateSudo(_ context.Context, _, _ int64) (*model.Session, error) {
	return nil, errors.New("not implemented")
}
func (m *oidcTestSessionRepo) GetByID(_ context.Context, _ string) (*model.Session, error) {
	return nil, errors.New("not implemented")
}
func (m *oidcTestSessionRepo) GetByIDPrefix(_ context.Context, _ int64, _ string) (*model.Session, error) {
	return nil, errors.New("not implemented")
}
func (m *oidcTestSessionRepo) Delete(_ context.Context, _ string) error { return nil }
func (m *oidcTestSessionRepo) ListByUser(_ context.Context, _ int64) ([]*model.Session, error) {
	return nil, nil
}

// oidcTestStateRepo is a minimal test double for repository.OIDCStateRepo.
type oidcTestStateRepo struct {
	createFn  func(ctx context.Context, state string) error
	consumeFn func(ctx context.Context, state string) (bool, error)
}

var _ repository.OIDCStateRepo = (*oidcTestStateRepo)(nil)

func (m *oidcTestStateRepo) Create(ctx context.Context, state string) error {
	if m.createFn != nil {
		return m.createFn(ctx, state)
	}
	return nil
}
func (m *oidcTestStateRepo) Consume(ctx context.Context, state string) (bool, error) {
	if m.consumeFn != nil {
		return m.consumeFn(ctx, state)
	}
	return true, nil
}

// ── GetConfig ────────────────────────────────────────────────────────────────

func TestOIDCHandler_GetConfig_Disabled(t *testing.T) {
	h := &OIDCHandler{cfg: config.OIDCConfig{Enabled: false}}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/config", nil)
	rr := httptest.NewRecorder()
	h.GetConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]bool
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["enabled"] {
		t.Error("expected enabled=false")
	}
}

func TestOIDCHandler_GetConfig_Enabled(t *testing.T) {
	h := &OIDCHandler{cfg: config.OIDCConfig{Enabled: true}}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/config", nil)
	rr := httptest.NewRecorder()
	h.GetConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]bool
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp["enabled"] {
		t.Error("expected enabled=true")
	}
}

// ── Login ─────────────────────────────────────────────────────────────────────

func TestOIDCHandler_Login_Disabled(t *testing.T) {
	h := &OIDCHandler{cfg: config.OIDCConfig{Enabled: false}}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/login", nil)
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestOIDCHandler_Login_Enabled(t *testing.T) {
	states := &oidcTestStateRepo{
		createFn: func(_ context.Context, _ string) error { return nil },
	}
	h := &OIDCHandler{
		cfg:    config.OIDCConfig{Enabled: true},
		states: states,
		oauth2Config: oauth2.Config{
			ClientID: "test-client",
			Endpoint: oauth2.Endpoint{
				AuthURL: "https://provider.example.com/auth",
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/login", nil)
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://provider.example.com/auth") {
		t.Errorf("expected redirect to provider auth URL, got %q", loc)
	}
}

func TestOIDCHandler_Login_StateStoreFails(t *testing.T) {
	states := &oidcTestStateRepo{
		createFn: func(_ context.Context, _ string) error { return errors.New("db error") },
	}
	h := &OIDCHandler{
		cfg:    config.OIDCConfig{Enabled: true},
		states: states,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/login", nil)
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// ── Callback ──────────────────────────────────────────────────────────────────

func TestOIDCHandler_Callback_Disabled(t *testing.T) {
	h := &OIDCHandler{cfg: config.OIDCConfig{Enabled: false}}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/callback?state=s&code=c", nil)
	rr := httptest.NewRecorder()
	h.Callback(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "provider_error") {
		t.Errorf("expected redirect with provider_error, got %q", loc)
	}
}

func TestOIDCHandler_Callback_ProviderErrorParam(t *testing.T) {
	h := &OIDCHandler{cfg: config.OIDCConfig{Enabled: true}}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/callback?error=access_denied&error_description=user+denied", nil)
	rr := httptest.NewRecorder()
	h.Callback(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "provider_error") {
		t.Errorf("expected redirect with provider_error, got %q", loc)
	}
}

func TestOIDCHandler_Callback_MissingState(t *testing.T) {
	h := &OIDCHandler{cfg: config.OIDCConfig{Enabled: true}}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/callback?code=mycode", nil)
	rr := httptest.NewRecorder()
	h.Callback(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "provider_error") {
		t.Errorf("expected redirect with provider_error, got %q", loc)
	}
}

func TestOIDCHandler_Callback_MissingCode(t *testing.T) {
	h := &OIDCHandler{cfg: config.OIDCConfig{Enabled: true}}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/callback?state=mystate", nil)
	rr := httptest.NewRecorder()
	h.Callback(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "provider_error") {
		t.Errorf("expected redirect with provider_error, got %q", loc)
	}
}

func TestOIDCHandler_Callback_StateConsumeError(t *testing.T) {
	states := &oidcTestStateRepo{
		consumeFn: func(_ context.Context, _ string) (bool, error) {
			return false, errors.New("db error")
		},
	}
	h := &OIDCHandler{
		cfg:    config.OIDCConfig{Enabled: true},
		states: states,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/callback?state=s&code=c", nil)
	rr := httptest.NewRecorder()
	h.Callback(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "provider_error") {
		t.Errorf("expected redirect with provider_error, got %q", loc)
	}
}

func TestOIDCHandler_Callback_StateConsumeReturnsFalse(t *testing.T) {
	states := &oidcTestStateRepo{
		consumeFn: func(_ context.Context, _ string) (bool, error) { return false, nil },
	}
	h := &OIDCHandler{
		cfg:    config.OIDCConfig{Enabled: true},
		states: states,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/callback?state=expired&code=c", nil)
	rr := httptest.NewRecorder()
	h.Callback(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "provider_error") {
		t.Errorf("expected redirect with provider_error, got %q", loc)
	}
}

func TestOIDCHandler_Callback_OAuth2ExchangeFails(t *testing.T) {
	// Mock token endpoint that returns 400 / invalid_grant.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer tokenSrv.Close()

	states := &oidcTestStateRepo{
		consumeFn: func(_ context.Context, _ string) (bool, error) { return true, nil },
	}
	h := &OIDCHandler{
		cfg:    config.OIDCConfig{Enabled: true},
		states: states,
		oauth2Config: oauth2.Config{
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			Endpoint:     oauth2.Endpoint{TokenURL: tokenSrv.URL},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/callback?state=validstate&code=validcode", nil)
	rr := httptest.NewRecorder()
	h.Callback(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "provider_error") {
		t.Errorf("expected redirect with provider_error, got %q", loc)
	}
}

func TestOIDCHandler_Callback_NoIDToken(t *testing.T) {
	// Mock token endpoint that returns a valid access token but no id_token.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at-1","token_type":"bearer","expires_in":3600}`))
	}))
	defer tokenSrv.Close()

	states := &oidcTestStateRepo{
		consumeFn: func(_ context.Context, _ string) (bool, error) { return true, nil },
	}
	h := &OIDCHandler{
		cfg:    config.OIDCConfig{Enabled: true},
		states: states,
		oauth2Config: oauth2.Config{
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			Endpoint:     oauth2.Endpoint{TokenURL: tokenSrv.URL},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/callback?state=validstate&code=validcode", nil)
	rr := httptest.NewRecorder()
	h.Callback(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "provider_error") {
		t.Errorf("expected redirect with provider_error (no id_token), got %q", loc)
	}
}

// ── resolveOIDCUser ───────────────────────────────────────────────────────────

func TestOIDCHandler_resolveOIDCUser_SubjectFound(t *testing.T) {
	existing := &model.User{ID: 1, Email: "user@example.com", Role: model.RoleUser}
	users := &oidcTestUserRepo{
		getByOIDCSubjectFn: func(_ context.Context, _, _ string) (*model.User, error) {
			return existing, nil
		},
	}
	h := &OIDCHandler{
		cfg:   config.OIDCConfig{Issuer: "https://issuer.example.com"},
		users: users,
	}
	user, err := h.resolveOIDCUser(context.Background(), "sub-1", "user@example.com", "User")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ID != existing.ID {
		t.Errorf("expected user ID %d, got %d", existing.ID, user.ID)
	}
}

func TestOIDCHandler_resolveOIDCUser_EmailFallback(t *testing.T) {
	existing := &model.User{ID: 2, Email: "fallback@example.com", Role: model.RoleUser}
	linked := false
	users := &oidcTestUserRepo{
		// Primary subject lookup fails.
		getByOIDCSubjectFn: func(_ context.Context, _, _ string) (*model.User, error) {
			return nil, errors.New("not found")
		},
		// Email fallback succeeds.
		getByEmailFn: func(_ context.Context, _ string) (*model.User, error) {
			return existing, nil
		},
		// SetOIDCSubject should be called to link the subject.
		setOIDCSubjectFn: func(_ context.Context, _ int64, _, _ string) error {
			linked = true
			return nil
		},
	}
	h := &OIDCHandler{
		cfg:   config.OIDCConfig{Issuer: "https://issuer.example.com"},
		users: users,
	}
	user, err := h.resolveOIDCUser(context.Background(), "sub-2", "fallback@example.com", "Fallback")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ID != existing.ID {
		t.Errorf("expected user ID %d, got %d", existing.ID, user.ID)
	}
	if !linked {
		t.Error("expected SetOIDCSubject to be called to link the subject")
	}
}

func TestOIDCHandler_resolveOIDCUser_SignupDisabled(t *testing.T) {
	// Both lookups fail; signup is disabled.
	h := &OIDCHandler{
		cfg: config.OIDCConfig{
			Issuer:        "https://issuer.example.com",
			SignupEnabled: false,
		},
		users: &oidcTestUserRepo{},
	}
	_, err := h.resolveOIDCUser(context.Background(), "sub-3", "new@example.com", "New")
	if !errors.Is(err, errSignupDisabled) {
		t.Errorf("expected errSignupDisabled, got %v", err)
	}
}

func TestOIDCHandler_resolveOIDCUser_NewUser(t *testing.T) {
	newUser := &model.User{ID: 3, Email: "new@example.com", Role: model.RoleUser}
	users := &oidcTestUserRepo{
		createOIDCUserFn: func(_ context.Context, _, _, _, _, _ string) (*model.User, error) {
			return newUser, nil
		},
	}
	h := &OIDCHandler{
		cfg: config.OIDCConfig{
			Issuer:        "https://issuer.example.com",
			SignupEnabled: true,
		},
		users: users,
	}
	user, err := h.resolveOIDCUser(context.Background(), "sub-4", "new@example.com", "New")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ID != newUser.ID {
		t.Errorf("expected user ID %d, got %d", newUser.ID, user.ID)
	}
}

func TestOIDCHandler_resolveOIDCUser_EmptyName_FallsBackToEmail(t *testing.T) {
	var capturedName string
	users := &oidcTestUserRepo{
		createOIDCUserFn: func(_ context.Context, _, name, _, _, _ string) (*model.User, error) {
			capturedName = name
			return &model.User{ID: 4, Email: "noname@example.com", Role: model.RoleUser}, nil
		},
	}
	h := &OIDCHandler{
		cfg: config.OIDCConfig{
			Issuer:        "https://issuer.example.com",
			SignupEnabled: true,
		},
		users: users,
	}
	_, err := h.resolveOIDCUser(context.Background(), "sub-5", "noname@example.com", "" /* no name */)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedName != "noname@example.com" {
		t.Errorf("expected display name to fall back to email, got %q", capturedName)
	}
}

func TestOIDCHandler_resolveOIDCUser_CreateFails(t *testing.T) {
	users := &oidcTestUserRepo{
		createOIDCUserFn: func(_ context.Context, _, _, _, _, _ string) (*model.User, error) {
			return nil, errors.New("db error")
		},
	}
	h := &OIDCHandler{
		cfg: config.OIDCConfig{
			Issuer:        "https://issuer.example.com",
			SignupEnabled: true,
		},
		users: users,
	}
	_, err := h.resolveOIDCUser(context.Background(), "sub-6", "fail@example.com", "Fail")
	if err == nil {
		t.Error("expected error when CreateOIDCUser fails")
	}
}

func TestOIDCHandler_resolveOIDCUser_SetLinkFails_StillReturnsUser(t *testing.T) {
	// If SetOIDCSubject fails, the handler logs a warning but still returns the user.
	existing := &model.User{ID: 5, Email: "linkfail@example.com", Role: model.RoleUser}
	users := &oidcTestUserRepo{
		getByOIDCSubjectFn: func(_ context.Context, _, _ string) (*model.User, error) {
			return nil, errors.New("not found")
		},
		getByEmailFn: func(_ context.Context, _ string) (*model.User, error) {
			return existing, nil
		},
		setOIDCSubjectFn: func(_ context.Context, _ int64, _, _ string) error {
			return errors.New("db error during link")
		},
	}
	h := &OIDCHandler{
		cfg:   config.OIDCConfig{Issuer: "https://issuer.example.com"},
		users: users,
	}
	user, err := h.resolveOIDCUser(context.Background(), "sub-7", "linkfail@example.com", "LinkFail")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ID != existing.ID {
		t.Errorf("expected user ID %d, got %d", existing.ID, user.ID)
	}
}

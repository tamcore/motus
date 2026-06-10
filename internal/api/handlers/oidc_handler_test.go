package handlers

// White-box tests for the live ogen OIDC implementation in oidc_oas.go.
// Being in package handlers (not handlers_test) lets us access unexported
// methods (resolveOIDCUserFromCtx, oidcIsAdminByFilter, claimBool) and
// errSignupDisabled.

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/config"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// TestOidcLogin_Disabled_Returns404 verifies that calling OidcLogin when OIDC
// is not enabled produces an error that NewError maps to 404, not 500.
func TestOidcLogin_Disabled_Returns404(t *testing.T) {
	h := NewHandler(HandlerConfig{OIDCConfig: config.OIDCConfig{Enabled: false}})
	err := h.OidcLogin(context.Background())
	if err == nil {
		t.Fatal("expected an error when OIDC is disabled")
	}
	resp := h.NewError(context.Background(), err)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected NewError to produce 404, got %d (error: %v)", resp.StatusCode, err)
	}
}

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
func (m *oidcTestSessionRepo) UpdateLastSeen(_ context.Context, _, _, _ string) error { return nil }
func (m *oidcTestSessionRepo) UpdateExpiry(_ context.Context, _ string, _ time.Time) error {
	return nil
}
func (m *oidcTestSessionRepo) DeleteAllByUser(_ context.Context, _ int64, _ string) error {
	return nil
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

// ── GetOIDCConfig ────────────────────────────────────────────────────────────

func TestGetOIDCConfig_Disabled(t *testing.T) {
	h := NewHandler(HandlerConfig{OIDCConfig: config.OIDCConfig{Enabled: false}})
	cfg, err := h.GetOIDCConfig(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Enabled {
		t.Error("expected Enabled=false")
	}
	if cfg.Issuer.Set {
		t.Error("expected issuer to be unset when disabled")
	}
}

func TestGetOIDCConfig_Enabled(t *testing.T) {
	h := NewHandler(HandlerConfig{OIDCConfig: config.OIDCConfig{
		Enabled: true,
		Issuer:  "https://issuer.example.com",
	}})
	cfg, err := h.GetOIDCConfig(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Enabled {
		t.Error("expected Enabled=true")
	}
	if got, _ := cfg.Issuer.Get(); got != "https://issuer.example.com" {
		t.Errorf("expected issuer to be set, got %q", got)
	}
}

// ── OidcCallback (live ogen handler) ─────────────────────────────────────────

func TestOidcCallback_Disabled(t *testing.T) {
	h := NewHandler(HandlerConfig{OIDCConfig: config.OIDCConfig{Enabled: false}})
	res, err := h.OidcCallback(context.Background(), oas.OidcCallbackParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e, ok := res.(*oas.Error); !ok || e.Error == "" {
		t.Fatalf("expected *oas.Error for disabled OIDC, got %T", res)
	}
}

func TestOidcCallback_MissingStateOrCode(t *testing.T) {
	h := NewHandler(HandlerConfig{OIDCConfig: config.OIDCConfig{Enabled: true}})

	res, err := h.OidcCallback(context.Background(), oas.OidcCallbackParams{
		Code: oas.NewOptString("some-code"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.Error); !ok {
		t.Fatalf("expected *oas.Error for missing state, got %T", res)
	}

	res, err = h.OidcCallback(context.Background(), oas.OidcCallbackParams{
		State: oas.NewOptString("some-state"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.Error); !ok {
		t.Fatalf("expected *oas.Error for missing code, got %T", res)
	}
}

func TestOidcCallback_StateConsumeError(t *testing.T) {
	h := NewHandler(HandlerConfig{
		OIDCConfig: config.OIDCConfig{Enabled: true},
		OIDCStateRepo: &oidcTestStateRepo{
			consumeFn: func(_ context.Context, _ string) (bool, error) {
				return false, errors.New("redis down")
			},
		},
	})
	res, err := h.OidcCallback(context.Background(), oas.OidcCallbackParams{
		State: oas.NewOptString("st"), Code: oas.NewOptString("co"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.Error); !ok {
		t.Fatalf("expected *oas.Error on state consume failure, got %T", res)
	}
}

func TestOidcCallback_StateConsumeReturnsFalse(t *testing.T) {
	h := NewHandler(HandlerConfig{
		OIDCConfig: config.OIDCConfig{Enabled: true},
		OIDCStateRepo: &oidcTestStateRepo{
			consumeFn: func(_ context.Context, _ string) (bool, error) {
				return false, nil
			},
		},
	})
	res, err := h.OidcCallback(context.Background(), oas.OidcCallbackParams{
		State: oas.NewOptString("expired"), Code: oas.NewOptString("co"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.Error); !ok {
		t.Fatalf("expected *oas.Error on expired/unknown state, got %T", res)
	}
}

// ── resolveOIDCUserFromCtx ───────────────────────────────────────────────────

func newOIDCTestHandler(users repository.UserRepo, cfg config.OIDCConfig) *Handler {
	if cfg.Issuer == "" {
		cfg.Issuer = "https://issuer.example.com"
	}
	return NewHandler(HandlerConfig{Users: users, OIDCConfig: cfg})
}

func TestResolveOIDCUser_SubjectFound(t *testing.T) {
	existing := &model.User{ID: 1, Email: "user@example.com", Role: model.RoleUser}
	users := &oidcTestUserRepo{
		getByOIDCSubjectFn: func(_ context.Context, _, _ string) (*model.User, error) {
			return existing, nil
		},
	}
	h := newOIDCTestHandler(users, config.OIDCConfig{})
	user, err := h.resolveOIDCUserFromCtx(context.Background(), "sub-1", "user@example.com", "User", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ID != existing.ID {
		t.Errorf("expected user ID %d, got %d", existing.ID, user.ID)
	}
}

func TestResolveOIDCUser_EmailFallback(t *testing.T) {
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
	h := newOIDCTestHandler(users, config.OIDCConfig{})
	user, err := h.resolveOIDCUserFromCtx(context.Background(), "sub-2", "fallback@example.com", "Fallback", true)
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

func TestResolveOIDCUser_EmailFallback_UnverifiedEmail_NotLinked(t *testing.T) {
	// An unverified email must never link an OIDC subject to an existing
	// account: an IdP that does not verify emails could otherwise be used
	// to take over a local account with a matching address.
	existing := &model.User{ID: 2, Email: "victim@example.com", Role: model.RoleUser}
	linked := false
	users := &oidcTestUserRepo{
		getByOIDCSubjectFn: func(_ context.Context, _, _ string) (*model.User, error) {
			return nil, errors.New("not found")
		},
		getByEmailFn: func(_ context.Context, _ string) (*model.User, error) {
			return existing, nil
		},
		setOIDCSubjectFn: func(_ context.Context, _ int64, _, _ string) error {
			linked = true
			return nil
		},
	}
	h := newOIDCTestHandler(users, config.OIDCConfig{SignupEnabled: false})
	_, err := h.resolveOIDCUserFromCtx(context.Background(), "sub-attacker", "victim@example.com", "Attacker", false)
	if !errors.Is(err, errSignupDisabled) {
		t.Errorf("expected errSignupDisabled for unverified email, got %v", err)
	}
	if linked {
		t.Error("SetOIDCSubject must not be called for an unverified email")
	}
}

func TestResolveOIDCUser_EmailFallback_TrustUnverifiedEmail_ConfigOverride(t *testing.T) {
	existing := &model.User{ID: 2, Email: "legacy@example.com", Role: model.RoleUser}
	linked := false
	users := &oidcTestUserRepo{
		getByOIDCSubjectFn: func(_ context.Context, _, _ string) (*model.User, error) {
			return nil, errors.New("not found")
		},
		getByEmailFn: func(_ context.Context, _ string) (*model.User, error) {
			return existing, nil
		},
		setOIDCSubjectFn: func(_ context.Context, _ int64, _, _ string) error {
			linked = true
			return nil
		},
	}
	h := newOIDCTestHandler(users, config.OIDCConfig{TrustUnverifiedEmail: true})
	user, err := h.resolveOIDCUserFromCtx(context.Background(), "sub-legacy", "legacy@example.com", "Legacy", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ID != existing.ID {
		t.Errorf("expected user ID %d, got %d", existing.ID, user.ID)
	}
	if !linked {
		t.Error("expected SetOIDCSubject to be called when TrustUnverifiedEmail is set")
	}
}

func TestResolveOIDCUser_SignupDisabled(t *testing.T) {
	// Both lookups fail; signup is disabled.
	h := newOIDCTestHandler(&oidcTestUserRepo{}, config.OIDCConfig{SignupEnabled: false})
	_, err := h.resolveOIDCUserFromCtx(context.Background(), "sub-3", "new@example.com", "New", true)
	if !errors.Is(err, errSignupDisabled) {
		t.Errorf("expected errSignupDisabled, got %v", err)
	}
}

func TestResolveOIDCUser_NewUser(t *testing.T) {
	newUser := &model.User{ID: 3, Email: "new@example.com", Role: model.RoleUser}
	users := &oidcTestUserRepo{
		createOIDCUserFn: func(_ context.Context, _, _, _, _, _ string) (*model.User, error) {
			return newUser, nil
		},
	}
	h := newOIDCTestHandler(users, config.OIDCConfig{SignupEnabled: true})
	user, err := h.resolveOIDCUserFromCtx(context.Background(), "sub-4", "new@example.com", "New", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ID != newUser.ID {
		t.Errorf("expected user ID %d, got %d", newUser.ID, user.ID)
	}
}

func TestResolveOIDCUser_EmptyName_FallsBackToEmail(t *testing.T) {
	var capturedName string
	users := &oidcTestUserRepo{
		createOIDCUserFn: func(_ context.Context, _, name, _, _, _ string) (*model.User, error) {
			capturedName = name
			return &model.User{ID: 4, Email: "noname@example.com", Role: model.RoleUser}, nil
		},
	}
	h := newOIDCTestHandler(users, config.OIDCConfig{SignupEnabled: true})
	_, err := h.resolveOIDCUserFromCtx(context.Background(), "sub-5", "noname@example.com", "" /* no name */, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedName != "noname@example.com" {
		t.Errorf("expected display name to fall back to email, got %q", capturedName)
	}
}

func TestResolveOIDCUser_CreateFails(t *testing.T) {
	users := &oidcTestUserRepo{
		createOIDCUserFn: func(_ context.Context, _, _, _, _, _ string) (*model.User, error) {
			return nil, errors.New("db error")
		},
	}
	h := newOIDCTestHandler(users, config.OIDCConfig{SignupEnabled: true})
	_, err := h.resolveOIDCUserFromCtx(context.Background(), "sub-6", "fail@example.com", "Fail", true)
	if err == nil {
		t.Error("expected error when CreateOIDCUser fails")
	}
}

func TestResolveOIDCUser_SetLinkFails_StillReturnsUser(t *testing.T) {
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
	h := newOIDCTestHandler(users, config.OIDCConfig{})
	user, err := h.resolveOIDCUserFromCtx(context.Background(), "sub-7", "linkfail@example.com", "LinkFail", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ID != existing.ID {
		t.Errorf("expected user ID %d, got %d", existing.ID, user.ID)
	}
}

// ── claimBool ────────────────────────────────────────────────────────────────

func TestClaimBool(t *testing.T) {
	tests := []struct {
		name   string
		claims map[string]interface{}
		want   bool
	}{
		{"bool true", map[string]interface{}{"email_verified": true}, true},
		{"bool false", map[string]interface{}{"email_verified": false}, false},
		{"string true", map[string]interface{}{"email_verified": "true"}, true},
		{"string false", map[string]interface{}{"email_verified": "false"}, false},
		{"missing", map[string]interface{}{}, false},
		{"nil claims", nil, false},
		{"unrelated type", map[string]interface{}{"email_verified": 1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := claimBool(tt.claims, "email_verified"); got != tt.want {
				t.Errorf("claimBool(%v) = %v, want %v", tt.claims, got, tt.want)
			}
		})
	}
}

// ── oidcIsAdminByFilter ──────────────────────────────────────────────────────

// TestOidcIsAdminByFilter checks every combination of email regex and
// claim-based admin filter on the live ogen handler.
func TestOidcIsAdminByFilter(t *testing.T) {
	tests := []struct {
		name      string
		cfg       config.OIDCConfig
		email     string
		allClaims map[string]interface{}
		wantAdmin bool
	}{
		{
			name:      "no filters configured",
			email:     "user@example.com",
			allClaims: map[string]interface{}{},
			wantAdmin: false,
		},
		{
			name:      "email regex matches",
			cfg:       config.OIDCConfig{AdminEmailRegex: `@example\.com$`},
			email:     "admin@example.com",
			allClaims: map[string]interface{}{},
			wantAdmin: true,
		},
		{
			name:      "email regex does not match",
			cfg:       config.OIDCConfig{AdminEmailRegex: `@example\.com$`},
			email:     "admin@other.com",
			allClaims: map[string]interface{}{},
			wantAdmin: false,
		},
		{
			name:  "claim string value matches",
			email: "user@other.com",
			cfg: config.OIDCConfig{
				AdminClaim:      "role",
				AdminClaimValue: "admin",
			},
			allClaims: map[string]interface{}{"role": "admin"},
			wantAdmin: true,
		},
		{
			name:  "claim string value does not match",
			email: "user@other.com",
			cfg: config.OIDCConfig{
				AdminClaim:      "role",
				AdminClaimValue: "admin",
			},
			allClaims: map[string]interface{}{"role": "viewer"},
			wantAdmin: false,
		},
		{
			name:  "claim array contains value",
			email: "user@other.com",
			cfg: config.OIDCConfig{
				AdminClaim:      "groups",
				AdminClaimValue: "motus-admin",
			},
			allClaims: map[string]interface{}{
				"groups": []interface{}{"viewers", "motus-admin", "editors"},
			},
			wantAdmin: true,
		},
		{
			name:  "claim array does not contain value",
			email: "user@other.com",
			cfg: config.OIDCConfig{
				AdminClaim:      "groups",
				AdminClaimValue: "motus-admin",
			},
			allClaims: map[string]interface{}{
				"groups": []interface{}{"viewers", "editors"},
			},
			wantAdmin: false,
		},
		{
			name:  "claim missing from token",
			email: "user@other.com",
			cfg: config.OIDCConfig{
				AdminClaim:      "groups",
				AdminClaimValue: "motus-admin",
			},
			allClaims: map[string]interface{}{},
			wantAdmin: false,
		},
		{
			name:  "email regex matches - claim also set but does not match",
			email: "admin@example.com",
			cfg: config.OIDCConfig{
				AdminEmailRegex: `@example\.com$`,
				AdminClaim:      "groups",
				AdminClaimValue: "motus-admin",
			},
			allClaims: map[string]interface{}{"groups": []interface{}{"viewers"}},
			wantAdmin: true, // email regex is sufficient
		},
		{
			name:  "neither filter matches",
			email: "admin@other.com",
			cfg: config.OIDCConfig{
				AdminEmailRegex: `@example\.com$`,
				AdminClaim:      "groups",
				AdminClaimValue: "motus-admin",
			},
			allClaims: map[string]interface{}{"groups": []interface{}{"viewers"}},
			wantAdmin: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(HandlerConfig{OIDCConfig: tt.cfg})
			got := h.oidcIsAdminByFilter(tt.email, tt.allClaims)
			if got != tt.wantAdmin {
				t.Errorf("oidcIsAdminByFilter(%q, %v) = %v, want %v", tt.email, tt.allClaims, got, tt.wantAdmin)
			}
		})
	}
}

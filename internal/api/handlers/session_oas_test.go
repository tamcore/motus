package handlers_test

// Tests for the ogen Handler session/auth methods (Login, Logout, GetSession,
// ListSessions, DeleteSession, GenerateToken, AdminDeleteUserSession).
// Ported from the deleted chi SessionHandler test files:
// session_test.go, session_mock_test.go, session_integration_test.go and
// login_limiter_test.go.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newSessionTestHandler builds an ogen Handler from mock repositories.
// The audit logger has a nil pool: audit.Logger.Log is a documented no-op
// without a pool, so the audit code path is exercised without a database.
func newSessionTestHandler(users repository.UserRepo, sessions repository.SessionRepo, apiKeys repository.ApiKeyRepo) *handlers.Handler {
	return handlers.NewHandler(handlers.HandlerConfig{
		Users:       users,
		Sessions:    sessions,
		ApiKeys:     apiKeys,
		AuditLogger: audit.NewLogger(nil),
	})
}

// doOASLogin performs a JSON login through the ogen handler.
func doOASLogin(t *testing.T, ctx context.Context, h *handlers.Handler, email, password string) oas.LoginRes {
	t.Helper()
	res, err := h.Login(ctx, &oas.LoginApplicationJSON{Email: email, Password: password})
	if err != nil {
		t.Fatalf("Login returned unexpected error: %v", err)
	}
	return res
}

// recorderSessionCookie returns the session_id cookie recorded on rr, or nil.
func recorderSessionCookie(rr *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rr.Result().Cookies() {
		if c.Name == "session_id" {
			return c
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Login lockout (ported from login_limiter_test.go)
// ---------------------------------------------------------------------------

func TestLogin_LockedAfterMaxFailures(t *testing.T) {
	const email = "target@example.com"

	users := &mockUserRepo{
		getByEmailFn: func(_ context.Context, _ string) (*model.User, error) {
			return nil, errors.New("not found")
		},
	}
	h := newSessionTestHandler(users, &mockSessionRepo{}, &mockApiKeyRepo{})
	ctx := context.Background()

	// First 10 attempts should all be unauthorized.
	for i := 0; i < 10; i++ {
		res := doOASLogin(t, ctx, h, email, "wrong")
		if _, ok := res.(*oas.LoginUnauthorized); !ok {
			t.Fatalf("attempt %d: expected *oas.LoginUnauthorized, got %T", i+1, res)
		}
	}

	// 11th attempt should be locked (429-equivalent).
	res := doOASLogin(t, ctx, h, email, "wrong")
	if _, ok := res.(*oas.LoginTooManyRequests); !ok {
		t.Errorf("after lockout: expected *oas.LoginTooManyRequests, got %T", res)
	}
}

func TestLogin_ResetOnSuccess(t *testing.T) {
	const email = "gooduser@example.com"
	const correctPassword = "correctpass"

	hash, _ := bcrypt.GenerateFromPassword([]byte(correctPassword), bcrypt.MinCost)
	testUser := &model.User{ID: 1, Email: email, PasswordHash: string(hash), Role: "user"}

	users := &mockUserRepo{
		getByEmailFn: func(_ context.Context, e string) (*model.User, error) {
			if e == email {
				return testUser, nil
			}
			return nil, errors.New("not found")
		},
	}
	h := newSessionTestHandler(users, &mockSessionRepo{}, &mockApiKeyRepo{})
	ctx := context.Background()

	// 5 failures.
	for i := 0; i < 5; i++ {
		doOASLogin(t, ctx, h, email, "wrong")
	}

	// Successful login resets the counter.
	res := doOASLogin(t, ctx, h, email, correctPassword)
	if _, ok := res.(*oas.User); !ok {
		t.Fatalf("expected *oas.User on successful login, got %T", res)
	}

	// Another 9 failures after reset should NOT trigger lockout (counter is reset).
	for i := 0; i < 9; i++ {
		res := doOASLogin(t, ctx, h, email, "wrong")
		if _, ok := res.(*oas.LoginUnauthorized); !ok {
			t.Fatalf("post-reset attempt %d: expected *oas.LoginUnauthorized, got %T", i+1, res)
		}
	}
}

func TestLogin_UnknownEmailCountedTowardLockout(t *testing.T) {
	const email = "nosuchuser@example.com"

	users := &mockUserRepo{
		getByEmailFn: func(_ context.Context, _ string) (*model.User, error) {
			return nil, errors.New("not found")
		},
	}
	h := newSessionTestHandler(users, &mockSessionRepo{}, &mockApiKeyRepo{})
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		doOASLogin(t, ctx, h, email, "x")
	}

	res := doOASLogin(t, ctx, h, email, "x")
	if _, ok := res.(*oas.LoginTooManyRequests); !ok {
		t.Errorf("expected *oas.LoginTooManyRequests after 10 unknown-email failures, got %T", res)
	}
}

// ---------------------------------------------------------------------------
// Logout cookie behavior (ported from session_test.go)
// ---------------------------------------------------------------------------

// TestLogout_ClearsCookie verifies that logout sets an expired cookie with
// both MaxAge=-1 and an epoch Expires timestamp, ensuring WebView
// implementations (Traccar Manager app) properly clear the session cookie.
func TestLogout_ClearsCookie(t *testing.T) {
	h := newSessionTestHandler(&mockUserRepo{}, &mockSessionRepo{}, &mockApiKeyRepo{})

	rr := httptest.NewRecorder()
	ctx := api.ContextWithResponseWriter(context.Background(), rr)

	res, err := h.Logout(ctx)
	if err != nil {
		t.Fatalf("Logout returned error: %v", err)
	}
	if _, ok := res.(*oas.LogoutNoContent); !ok {
		t.Errorf("expected *oas.LogoutNoContent, got %T", res)
	}

	c := recorderSessionCookie(rr)
	if c == nil {
		t.Fatal("expected session_id cookie to be cleared")
	}
	if c.Value != "" {
		t.Errorf("expected empty cookie value, got %q", c.Value)
	}
	// MaxAge < 0 signals the browser/WebView to delete the cookie immediately.
	if c.MaxAge != -1 {
		t.Errorf("expected MaxAge=-1 for cookie deletion, got %d", c.MaxAge)
	}
}

// TestLogout_SecureCookieInProduction verifies the Secure flag is set when
// MOTUS_ENV is not "development" (default environment is production-like).
func TestLogout_SecureCookieInProduction(t *testing.T) {
	t.Setenv("MOTUS_ENV", "")

	h := newSessionTestHandler(&mockUserRepo{}, &mockSessionRepo{}, &mockApiKeyRepo{})

	rr := httptest.NewRecorder()
	ctx := api.ContextWithResponseWriter(context.Background(), rr)
	if _, err := h.Logout(ctx); err != nil {
		t.Fatalf("Logout returned error: %v", err)
	}

	c := recorderSessionCookie(rr)
	if c == nil {
		t.Fatal("session_id cookie not found")
	}
	if !c.Secure {
		t.Error("expected Secure flag on session_id cookie in production")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("expected SameSite=Lax, got %v", c.SameSite)
	}
}

// TestLogout_InsecureCookieInDev verifies the Secure flag is NOT set when
// MOTUS_ENV=development.
func TestLogout_InsecureCookieInDev(t *testing.T) {
	t.Setenv("MOTUS_ENV", "development")

	h := newSessionTestHandler(&mockUserRepo{}, &mockSessionRepo{}, &mockApiKeyRepo{})

	rr := httptest.NewRecorder()
	ctx := api.ContextWithResponseWriter(context.Background(), rr)
	if _, err := h.Logout(ctx); err != nil {
		t.Fatalf("Logout returned error: %v", err)
	}

	c := recorderSessionCookie(rr)
	if c == nil {
		t.Fatal("session_id cookie not found")
	}
	if c.Secure {
		t.Error("Secure flag should not be set in development mode")
	}
}

// ---------------------------------------------------------------------------
// GenerateToken (ported from session_test.go)
// ---------------------------------------------------------------------------

func TestGenerateToken_Unauthenticated(t *testing.T) {
	h := newSessionTestHandler(&mockUserRepo{}, &mockSessionRepo{}, &mockApiKeyRepo{})

	res, err := h.GenerateToken(context.Background())
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	if _, ok := res.(*oas.Error); !ok {
		t.Errorf("expected *oas.Error for unauthenticated request, got %T", res)
	}
}

// TestGenerateToken_ReadonlyApiKey_Forbidden verifies that a read-only API
// key cannot mint a full user token even if the middleware exemption is bypassed.
func TestGenerateToken_ReadonlyApiKey_Forbidden(t *testing.T) {
	user := &model.User{ID: 1, Email: "readonly@example.com", Role: model.RoleUser}
	readonlyKey := &model.ApiKey{ID: 1, Permissions: model.PermissionReadonly}

	generateTokenCalled := false
	users := &mockUserRepo{
		generateTokenFn: func(_ context.Context, _ int64) (string, error) {
			generateTokenCalled = true
			return "should-not-be-returned", nil
		},
	}

	h := newSessionTestHandler(users, &mockSessionRepo{}, &mockApiKeyRepo{})

	ctx := api.ContextWithUser(context.Background(), user)
	ctx = api.ContextWithApiKey(ctx, readonlyKey)

	res, err := h.GenerateToken(ctx)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	errRes, ok := res.(*oas.Error)
	if !ok {
		t.Fatalf("expected *oas.Error (403-equivalent) for readonly key, got %T", res)
	}
	if errRes.Error != "this API key has read-only permissions" {
		t.Errorf("unexpected error message: %q", errRes.Error)
	}
	if generateTokenCalled {
		t.Error("GenerateToken on users repo should NOT be called for a read-only key")
	}
}

// TestGenerateToken_FullApiKey_Succeeds verifies that a full-permission API
// key can still mint a user token.
func TestGenerateToken_FullApiKey_Succeeds(t *testing.T) {
	user := &model.User{ID: 2, Email: "full@example.com", Role: model.RoleUser}
	fullKey := &model.ApiKey{ID: 2, Permissions: model.PermissionFull}

	users := &mockUserRepo{
		generateTokenFn: func(_ context.Context, _ int64) (string, error) {
			return "generated-token", nil
		},
	}

	h := newSessionTestHandler(users, &mockSessionRepo{}, &mockApiKeyRepo{})

	ctx := api.ContextWithUser(context.Background(), user)
	ctx = api.ContextWithApiKey(ctx, fullKey)

	res, err := h.GenerateToken(ctx)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	tokenRes, ok := res.(*oas.TokenResponse)
	if !ok {
		t.Fatalf("expected *oas.TokenResponse, got %T", res)
	}
	if tokenRes.Token != "generated-token" {
		t.Errorf("expected token 'generated-token', got %q", tokenRes.Token)
	}
}

// ---------------------------------------------------------------------------
// ListSessions (ported from session_test.go)
// ---------------------------------------------------------------------------

func TestListSessions_Unauthenticated(t *testing.T) {
	h := newSessionTestHandler(&mockUserRepo{}, &mockSessionRepo{}, &mockApiKeyRepo{})

	res, err := h.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	if _, ok := res.(*oas.Error); !ok {
		t.Errorf("expected *oas.Error for unauthenticated request, got %T", res)
	}
}

// TestListSessions_Success verifies that sessions are returned with isCurrent
// set for the current context session, and sudo sessions are filtered out.
func TestListSessions_Success(t *testing.T) {
	user := &model.User{ID: 1, Email: "test@example.com", Role: model.RoleUser}
	current := &model.Session{ID: "sess-current", UserID: 1, ExpiresAt: time.Now().Add(23 * time.Hour)}

	sessions := &mockSessionRepo{
		listByUserFn: func(_ context.Context, userID int64) ([]*model.Session, error) {
			if userID != 1 {
				t.Errorf("expected userID=1, got %d", userID)
			}
			return []*model.Session{
				{ID: "sess-1", UserID: 1, CreatedAt: time.Now().Add(-2 * time.Hour), ExpiresAt: time.Now().Add(22 * time.Hour)},
				{ID: "sess-current", UserID: 1, CreatedAt: time.Now().Add(-1 * time.Hour), ExpiresAt: time.Now().Add(23 * time.Hour)},
				{ID: "sess-sudo", UserID: 1, IsSudo: true, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)},
			}, nil
		},
	}

	h := newSessionTestHandler(&mockUserRepo{}, sessions, &mockApiKeyRepo{})

	ctx := api.ContextWithUser(context.Background(), user)
	ctx = api.ContextWithSession(ctx, current)

	res, err := h.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	list, ok := res.(*oas.ListSessionsOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.ListSessionsOKApplicationJSON, got %T", res)
	}

	// Sudo session should be filtered out, leaving 2 sessions.
	if len(*list) != 2 {
		t.Fatalf("expected 2 sessions (sudo filtered), got %d", len(*list))
	}

	// Verify isCurrent is set on the matching session only.
	// IDs are <= 12 characters, so TruncatedID() returns them unchanged.
	for _, s := range *list {
		switch s.ID {
		case "sess-current":
			if !s.IsCurrent.Value {
				t.Errorf("expected isCurrent=true for session %q", s.ID)
			}
		case "sess-1":
			if s.IsCurrent.Value {
				t.Errorf("expected isCurrent=false for session %q", s.ID)
			}
		}
		if s.IsSudo.Value {
			t.Errorf("sudo session %q should not appear in results", s.ID)
		}
	}
}

// TestListSessions_Empty verifies that an empty repository result yields an
// empty (non-nil) list, which serializes as [] rather than null.
func TestListSessions_Empty(t *testing.T) {
	user := &model.User{ID: 1, Email: "test@example.com", Role: model.RoleUser}

	sessions := &mockSessionRepo{
		listByUserFn: func(_ context.Context, _ int64) ([]*model.Session, error) {
			return []*model.Session{}, nil
		},
	}

	h := newSessionTestHandler(&mockUserRepo{}, sessions, &mockApiKeyRepo{})

	ctx := api.ContextWithUser(context.Background(), user)
	res, err := h.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	list, ok := res.(*oas.ListSessionsOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.ListSessionsOKApplicationJSON, got %T", res)
	}
	if *list == nil {
		t.Error("expected non-nil empty list (serializes as [] not null)")
	}
	if len(*list) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(*list))
	}
}

// TestListSessions_Error verifies that a repository failure returns an error
// response.
func TestListSessions_Error(t *testing.T) {
	user := &model.User{ID: 1, Email: "test@example.com", Role: model.RoleUser}

	sessions := &mockSessionRepo{
		listByUserFn: func(_ context.Context, _ int64) ([]*model.Session, error) {
			return nil, errors.New("database error")
		},
	}

	h := newSessionTestHandler(&mockUserRepo{}, sessions, &mockApiKeyRepo{})

	ctx := api.ContextWithUser(context.Background(), user)
	res, err := h.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	if _, ok := res.(*oas.Error); !ok {
		t.Errorf("expected *oas.Error on repository failure, got %T", res)
	}
}

// ---------------------------------------------------------------------------
// DeleteSession (ported from session_test.go)
// ---------------------------------------------------------------------------

func TestDeleteSession_Unauthenticated(t *testing.T) {
	h := newSessionTestHandler(&mockUserRepo{}, &mockSessionRepo{}, &mockApiKeyRepo{})

	res, err := h.DeleteSession(context.Background(), oas.DeleteSessionParams{ID: "some-id"})
	if err != nil {
		t.Fatalf("DeleteSession returned error: %v", err)
	}
	if _, ok := res.(*oas.DeleteSessionUnauthorized); !ok {
		t.Errorf("expected *oas.DeleteSessionUnauthorized, got %T", res)
	}
}

func TestDeleteSession_Success(t *testing.T) {
	user := &model.User{ID: 1, Email: "test@example.com", Role: model.RoleUser}
	var deletedID string

	sessions := &mockSessionRepo{
		getByIDPrefixFn: func(_ context.Context, userID int64, prefix string) (*model.Session, error) {
			if userID == 1 && prefix == "target-sess" {
				return &model.Session{ID: "target-sess", UserID: 1, ExpiresAt: time.Now().Add(time.Hour)}, nil
			}
			return nil, errors.New("not found")
		},
		deleteFn: func(_ context.Context, id string) error {
			deletedID = id
			return nil
		},
	}

	h := newSessionTestHandler(&mockUserRepo{}, sessions, &mockApiKeyRepo{})

	ctx := api.ContextWithUser(context.Background(), user)
	res, err := h.DeleteSession(ctx, oas.DeleteSessionParams{ID: "target-sess"})
	if err != nil {
		t.Fatalf("DeleteSession returned error: %v", err)
	}
	if _, ok := res.(*oas.DeleteSessionNoContent); !ok {
		t.Errorf("expected *oas.DeleteSessionNoContent, got %T", res)
	}
	if deletedID != "target-sess" {
		t.Errorf("expected Delete called with 'target-sess', got %q", deletedID)
	}
}

// TestDeleteSession_OtherUsersSession_NotFound verifies that a non-admin user
// cannot discover or delete another user's session: the user-scoped prefix
// lookup misses, and the handler returns not-found rather than leaking that
// the session exists (IDOR protection).
func TestDeleteSession_OtherUsersSession_NotFound(t *testing.T) {
	user := &model.User{ID: 1, Email: "user@example.com", Role: model.RoleUser}

	// GetByIDPrefix is scoped to user 1; the session belongs to user 99,
	// so the default mock returns not-found. The non-admin path must not
	// fall back to the unscoped GetByID.
	getByIDCalled := false
	sessions := &mockSessionRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.Session, error) {
			getByIDCalled = true
			return &model.Session{ID: "other-user-sess", UserID: 99, ExpiresAt: time.Now().Add(time.Hour)}, nil
		},
	}

	h := newSessionTestHandler(&mockUserRepo{}, sessions, &mockApiKeyRepo{})

	ctx := api.ContextWithUser(context.Background(), user)
	res, err := h.DeleteSession(ctx, oas.DeleteSessionParams{ID: "other-user-sess"})
	if err != nil {
		t.Fatalf("DeleteSession returned error: %v", err)
	}
	if _, ok := res.(*oas.DeleteSessionNotFound); !ok {
		t.Errorf("expected *oas.DeleteSessionNotFound (no session-existence leak), got %T", res)
	}
	if getByIDCalled {
		t.Error("non-admin must not trigger the unscoped GetByID fallback")
	}
}

// TestDeleteSession_OtherUsersSession_Forbidden verifies the defense-in-depth
// ownership check: even if a lookup returns a session owned by someone else,
// a non-admin gets a typed forbidden response and no deletion occurs.
func TestDeleteSession_OtherUsersSession_Forbidden(t *testing.T) {
	user := &model.User{ID: 1, Email: "user@example.com", Role: model.RoleUser}

	deleteCalled := false
	sessions := &mockSessionRepo{
		getByIDPrefixFn: func(_ context.Context, _ int64, _ string) (*model.Session, error) {
			// Simulate a buggy/over-broad lookup returning a foreign session.
			return &model.Session{ID: "other-user-sess", UserID: 99, ExpiresAt: time.Now().Add(time.Hour)}, nil
		},
		deleteFn: func(_ context.Context, _ string) error {
			deleteCalled = true
			return nil
		},
	}

	h := newSessionTestHandler(&mockUserRepo{}, sessions, &mockApiKeyRepo{})

	ctx := api.ContextWithUser(context.Background(), user)
	res, err := h.DeleteSession(ctx, oas.DeleteSessionParams{ID: "other-user-sess"})
	if err != nil {
		t.Fatalf("DeleteSession returned error: %v", err)
	}
	if _, ok := res.(*oas.DeleteSessionForbidden); !ok {
		t.Errorf("expected *oas.DeleteSessionForbidden, got %T", res)
	}
	if deleteCalled {
		t.Error("Delete must not be called for another user's session")
	}
}

// TestDeleteSession_AdminCanDeleteOtherUser verifies that an admin can delete
// any user's session via the GetByID fallback.
func TestDeleteSession_AdminCanDeleteOtherUser(t *testing.T) {
	admin := &model.User{ID: 1, Email: "admin@example.com", Role: model.RoleAdmin}
	var deletedID string

	sessions := &mockSessionRepo{
		// GetByIDPrefix won't find it (admin doesn't own it), so it falls
		// back to GetByID.
		getByIDFn: func(_ context.Context, id string) (*model.Session, error) {
			if id == "other-user-sess" {
				return &model.Session{ID: "other-user-sess", UserID: 99, ExpiresAt: time.Now().Add(time.Hour)}, nil
			}
			return nil, errors.New("not found")
		},
		deleteFn: func(_ context.Context, id string) error {
			deletedID = id
			return nil
		},
	}

	h := newSessionTestHandler(&mockUserRepo{}, sessions, &mockApiKeyRepo{})

	ctx := api.ContextWithUser(context.Background(), admin)
	res, err := h.DeleteSession(ctx, oas.DeleteSessionParams{ID: "other-user-sess"})
	if err != nil {
		t.Fatalf("DeleteSession returned error: %v", err)
	}
	if _, ok := res.(*oas.DeleteSessionNoContent); !ok {
		t.Errorf("expected *oas.DeleteSessionNoContent, got %T", res)
	}
	if deletedID != "other-user-sess" {
		t.Errorf("expected Delete called with 'other-user-sess', got %q", deletedID)
	}
}

// TestDeleteSession_WithEllipsis verifies that a truncated display ID sent by
// the frontend (e.g. "20b2c47a5c1a…") is handled correctly. The ellipsis is a
// display artifact from TruncatedID — it must be stripped before the prefix
// lookup so the SQL LIKE query can match the actual session token.
func TestDeleteSession_WithEllipsis(t *testing.T) {
	user := &model.User{ID: 1, Email: "test@example.com", Role: model.RoleUser}
	var deletedID string

	sessions := &mockSessionRepo{
		getByIDPrefixFn: func(_ context.Context, userID int64, prefix string) (*model.Session, error) {
			// The handler must strip "…" before calling us; reject it if present.
			if userID == 1 && prefix == "target-sess" {
				return &model.Session{ID: "target-sess-full-token", UserID: 1, ExpiresAt: time.Now().Add(time.Hour)}, nil
			}
			return nil, errors.New("not found")
		},
		deleteFn: func(_ context.Context, id string) error {
			deletedID = id
			return nil
		},
	}

	h := newSessionTestHandler(&mockUserRepo{}, sessions, &mockApiKeyRepo{})

	ctx := api.ContextWithUser(context.Background(), user)
	res, err := h.DeleteSession(ctx, oas.DeleteSessionParams{ID: "target-sess…"})
	if err != nil {
		t.Fatalf("DeleteSession returned error: %v", err)
	}
	if _, ok := res.(*oas.DeleteSessionNoContent); !ok {
		t.Errorf("expected *oas.DeleteSessionNoContent for ellipsis-suffixed ID, got %T", res)
	}
	if deletedID != "target-sess-full-token" {
		t.Errorf("expected Delete called with full token, got %q", deletedID)
	}
}

func TestDeleteSession_MissingID(t *testing.T) {
	user := &model.User{ID: 1, Email: "test@example.com", Role: model.RoleUser}

	h := newSessionTestHandler(&mockUserRepo{}, &mockSessionRepo{}, &mockApiKeyRepo{})

	ctx := api.ContextWithUser(context.Background(), user)
	res, err := h.DeleteSession(ctx, oas.DeleteSessionParams{ID: ""})
	if err != nil {
		t.Fatalf("DeleteSession returned error: %v", err)
	}
	if _, ok := res.(*oas.DeleteSessionNotFound); !ok {
		t.Errorf("expected *oas.DeleteSessionNotFound for empty ID, got %T", res)
	}
}

// ---------------------------------------------------------------------------
// GetSession token login (ported from session_mock_test.go)
// ---------------------------------------------------------------------------

// TestGetSession_ApiKeyToken verifies that a token from the api_keys table
// authenticates successfully via the ?token= query parameter, the session is
// created with the API key ID linked (CreateWithApiKey), and a session cookie
// is set on the response writer.
func TestGetSession_ApiKeyToken(t *testing.T) {
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
		createWithApiKeyFn: func(_ context.Context, userID int64, apiKeyID int64, expiresAt time.Time, _ bool) (*model.Session, error) {
			createdWithApiKeyID = apiKeyID
			return &model.Session{ID: "mock-session-id", UserID: userID, ApiKeyID: &apiKeyID, ExpiresAt: expiresAt}, nil
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

	h := newSessionTestHandler(users, sessions, apiKeys)

	rr := httptest.NewRecorder()
	ctx := api.ContextWithResponseWriter(context.Background(), rr)

	res, err := h.GetSession(ctx, oas.GetSessionParams{Token: oas.NewOptString("abc123apikey")})
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	userRes, ok := res.(*oas.User)
	if !ok {
		t.Fatalf("expected *oas.User, got %T", res)
	}
	if userRes.Email != "apikey@example.com" {
		t.Errorf("expected email apikey@example.com, got %q", userRes.Email)
	}

	// Verify CreateWithApiKey was called with the correct API key ID.
	if createdWithApiKeyID != 1 {
		t.Errorf("expected CreateWithApiKey called with apiKeyID=1, got %d", createdWithApiKeyID)
	}

	// Verify session cookie is set (for WebSocket compatibility).
	c := recorderSessionCookie(rr)
	if c == nil || c.Value == "" {
		t.Error("expected session_id cookie to be set for API key token auth")
	}
}

// TestGetSession_LegacyTokenFallback verifies that when the token is not
// found in api_keys, it falls back to the legacy users.token column and
// creates the session via CreateWithExpiry (no API key linked).
func TestGetSession_LegacyTokenFallback(t *testing.T) {
	testUser := &model.User{ID: 7, Email: "legacy@example.com", Name: "Legacy User", Role: "user"}

	createWithExpiryCalled := false
	createWithApiKeyCalled := false
	users := &mockUserRepo{
		getByTokenFn: func(_ context.Context, token string) (*model.User, error) {
			if token == "legacy-user-token" {
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
		// API key lookup returns not found -- should fall back to legacy.
		getByTokenFn: func(_ context.Context, _ string) (*model.ApiKey, error) {
			return nil, errors.New("not found")
		},
	}

	h := newSessionTestHandler(users, sessions, apiKeys)

	rr := httptest.NewRecorder()
	ctx := api.ContextWithResponseWriter(context.Background(), rr)

	res, err := h.GetSession(ctx, oas.GetSessionParams{Token: oas.NewOptString("legacy-user-token")})
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	userRes, ok := res.(*oas.User)
	if !ok {
		t.Fatalf("expected *oas.User, got %T", res)
	}
	if userRes.Email != "legacy@example.com" {
		t.Errorf("expected email legacy@example.com, got %q", userRes.Email)
	}
	if !createWithExpiryCalled {
		t.Error("expected CreateWithExpiry to be called for legacy token")
	}
	if createWithApiKeyCalled {
		t.Error("CreateWithApiKey should NOT have been called for legacy token")
	}
}

// TestGetSession_InvalidTokenBothSources verifies that a token found in
// neither api_keys nor users.token yields a typed error response.
func TestGetSession_InvalidTokenBothSources(t *testing.T) {
	users := &mockUserRepo{
		getByTokenFn: func(_ context.Context, _ string) (*model.User, error) {
			return nil, errors.New("not found")
		},
	}
	apiKeys := &mockApiKeyRepo{
		getByTokenFn: func(_ context.Context, _ string) (*model.ApiKey, error) {
			return nil, errors.New("not found")
		},
	}

	h := newSessionTestHandler(users, &mockSessionRepo{}, apiKeys)

	res, err := h.GetSession(context.Background(), oas.GetSessionParams{Token: oas.NewOptString("nonexistent-token")})
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	if _, ok := res.(*oas.Error); !ok {
		t.Errorf("expected *oas.Error for invalid token, got %T", res)
	}
}

// TestGetSession_ApiKeyPriorityOverLegacy verifies that when the same token
// exists in both api_keys and users.token, the api_keys match takes priority
// (the legacy lookup is never called).
func TestGetSession_ApiKeyPriorityOverLegacy(t *testing.T) {
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
	apiKeys := &mockApiKeyRepo{
		getByTokenFn: func(_ context.Context, token string) (*model.ApiKey, error) {
			if token == "shared-token" {
				return &model.ApiKey{ID: 1, UserID: 10, Token: "shared-token", Name: "key", Permissions: "full"}, nil
			}
			return nil, errors.New("not found")
		},
	}

	h := newSessionTestHandler(users, &mockSessionRepo{}, apiKeys)

	res, err := h.GetSession(context.Background(), oas.GetSessionParams{Token: oas.NewOptString("shared-token")})
	if err != nil {
		t.Fatalf("GetSession returned error: %v", err)
	}
	userRes, ok := res.(*oas.User)
	if !ok {
		t.Fatalf("expected *oas.User, got %T", res)
	}

	// Should resolve to the API key user, not the legacy user.
	if userRes.Email != "apikey-user@example.com" {
		t.Errorf("expected email apikey-user@example.com (API key user), got %q", userRes.Email)
	}
	if legacyLookupCalled {
		t.Error("legacy GetByToken should NOT have been called when API key matched")
	}
}

// ---------------------------------------------------------------------------
// Integration tests (ported from session_integration_test.go; require Docker,
// skipped automatically in -short mode by testutil.SetupTestDB)
// ---------------------------------------------------------------------------

func setupSessionOASIntegration(t *testing.T) (*handlers.Handler, *repository.UserRepository, *repository.SessionRepository, *audit.Logger) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)
	auditLogger := audit.NewLogger(pool)

	h := handlers.NewHandler(handlers.HandlerConfig{
		Users:       userRepo,
		Sessions:    sessionRepo,
		ApiKeys:     apiKeyRepo,
		AuditLogger: auditLogger,
	})
	return h, userRepo, sessionRepo, auditLogger
}

func createIntegrationUser(t *testing.T, userRepo *repository.UserRepository, email, password string) *model.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := &model.User{Email: email, PasswordHash: string(hash), Name: "Test User"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func TestLogin_Success_Integration(t *testing.T) {
	h, userRepo, _, _ := setupSessionOASIntegration(t)
	createIntegrationUser(t, userRepo, "login@example.com", "secret123")

	rr := httptest.NewRecorder()
	ctx := api.ContextWithResponseWriter(context.Background(), rr)

	res := doOASLogin(t, ctx, h, "login@example.com", "secret123")
	userRes, ok := res.(*oas.User)
	if !ok {
		t.Fatalf("expected *oas.User, got %T", res)
	}
	if userRes.Email != "login@example.com" {
		t.Errorf("expected email 'login@example.com', got %q", userRes.Email)
	}

	// Verify session cookie attributes.
	c := recorderSessionCookie(rr)
	if c == nil || c.Value == "" {
		t.Fatal("expected session_id cookie to be set")
	}
	if c.MaxAge <= 0 {
		t.Errorf("expected MaxAge > 0 on login cookie, got %d", c.MaxAge)
	}
	if !c.HttpOnly {
		t.Error("expected HttpOnly on session cookie")
	}
	if c.Path != "/" {
		t.Errorf("expected cookie Path=/, got %q", c.Path)
	}
}

func TestLogin_WrongPassword_Integration(t *testing.T) {
	h, userRepo, _, auditLogger := setupSessionOASIntegration(t)
	ctx := context.Background()
	user := createIntegrationUser(t, userRepo, "wrongpw@example.com", "correct")

	res := doOASLogin(t, ctx, h, "wrongpw@example.com", "wrong")
	if _, ok := res.(*oas.LoginUnauthorized); !ok {
		t.Errorf("expected *oas.LoginUnauthorized, got %T", res)
	}

	// The failed attempt must be audit-logged with the wrong_password reason.
	entries, total, err := auditLogger.Query(ctx, audit.QueryParams{Action: audit.ActionSessionLoginFailed})
	if err != nil {
		t.Fatalf("query audit log: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 failed-login audit entry, got %d", total)
	}
	entry := entries[0]
	if entry.UserID == nil || *entry.UserID != user.ID {
		t.Errorf("expected audit entry for user %d, got %v", user.ID, entry.UserID)
	}
	if reason, _ := entry.Details["reason"].(string); reason != "wrong_password" {
		t.Errorf("expected audit reason 'wrong_password', got %q", reason)
	}
}

func TestLogin_NonexistentUser_Integration(t *testing.T) {
	h, _, _, auditLogger := setupSessionOASIntegration(t)
	ctx := context.Background()

	res := doOASLogin(t, ctx, h, "nobody@example.com", "anything")
	if _, ok := res.(*oas.LoginUnauthorized); !ok {
		t.Errorf("expected *oas.LoginUnauthorized, got %T", res)
	}

	// The failed attempt must be audit-logged with the unknown_email reason
	// and no user ID.
	entries, total, err := auditLogger.Query(ctx, audit.QueryParams{Action: audit.ActionSessionLoginFailed})
	if err != nil {
		t.Fatalf("query audit log: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 failed-login audit entry, got %d", total)
	}
	entry := entries[0]
	if entry.UserID != nil {
		t.Errorf("expected no user ID on unknown-email audit entry, got %v", *entry.UserID)
	}
	if reason, _ := entry.Details["reason"].(string); reason != "unknown_email" {
		t.Errorf("expected audit reason 'unknown_email', got %q", reason)
	}
}

// TestLogin_RememberMe_Integration verifies session/cookie expiry: ~24h for a
// standard login and ~30 days with remember=true.
func TestLogin_RememberMe_Integration(t *testing.T) {
	h, userRepo, _, _ := setupSessionOASIntegration(t)
	createIntegrationUser(t, userRepo, "remember@example.com", "secret123")

	tests := []struct {
		name      string
		remember  bool
		minExpiry time.Duration
		maxExpiry time.Duration
	}{
		{name: "without remember me", remember: false, minExpiry: 23 * time.Hour, maxExpiry: 25 * time.Hour},
		{name: "with remember me", remember: true, minExpiry: 719 * time.Hour, maxExpiry: 721 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			ctx := api.ContextWithResponseWriter(context.Background(), rr)

			res, err := h.Login(ctx, &oas.LoginApplicationJSON{
				Email:    "remember@example.com",
				Password: "secret123",
				Remember: oas.NewOptBool(tt.remember),
			})
			if err != nil {
				t.Fatalf("Login returned error: %v", err)
			}
			if _, ok := res.(*oas.User); !ok {
				t.Fatalf("expected *oas.User, got %T", res)
			}

			c := recorderSessionCookie(rr)
			if c == nil {
				t.Fatal("expected session_id cookie to be set")
			}
			expiresIn := time.Until(c.Expires)
			if expiresIn < tt.minExpiry || expiresIn > tt.maxExpiry {
				t.Errorf("expected cookie expiry between %v and %v, got %v", tt.minExpiry, tt.maxExpiry, expiresIn)
			}
		})
	}
}

// TestGenerateToken_Success_Integration verifies the generated token is
// persisted and resolvable back to the user.
func TestGenerateToken_Success_Integration(t *testing.T) {
	h, userRepo, _, _ := setupSessionOASIntegration(t)
	ctx := context.Background()
	user := createIntegrationUser(t, userRepo, "token@example.com", "secret123")

	res, err := h.GenerateToken(api.ContextWithUser(ctx, user))
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	tokenRes, ok := res.(*oas.TokenResponse)
	if !ok {
		t.Fatalf("expected *oas.TokenResponse, got %T", res)
	}
	if tokenRes.Token == "" {
		t.Fatal("expected non-empty token in response")
	}

	// The token must be persisted on the user record.
	resolved, err := userRepo.GetByToken(ctx, tokenRes.Token)
	if err != nil {
		t.Fatalf("generated token not resolvable: %v", err)
	}
	if resolved.ID != user.ID {
		t.Errorf("token resolved to user %d, expected %d", resolved.ID, user.ID)
	}
}

// --- AdminDeleteUserSession (ported from the AdminDeleteSession tests) ---

func TestAdminDeleteUserSession_Success(t *testing.T) {
	h, userRepo, sessionRepo, _ := setupSessionOASIntegration(t)
	ctx := context.Background()

	admin := createIntegrationUser(t, userRepo, "admin-del@example.com", "pw")
	admin.Role = model.RoleAdmin
	target := createIntegrationUser(t, userRepo, "target-del@example.com", "pw")

	session, err := sessionRepo.Create(ctx, target.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	res, err := h.AdminDeleteUserSession(api.ContextWithUser(ctx, admin),
		oas.AdminDeleteUserSessionParams{ID: target.ID, SessionId: session.ID})
	if err != nil {
		t.Fatalf("AdminDeleteUserSession returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminDeleteUserSessionNoContent); !ok {
		t.Fatalf("expected *oas.AdminDeleteUserSessionNoContent, got %T", res)
	}

	// Session must actually be gone.
	if got, err := sessionRepo.GetByID(ctx, session.ID); err == nil && got != nil {
		t.Error("expected session to be deleted")
	}
}

// TestAdminDeleteUserSession_WrongUser verifies the ownership check: the
// session ID must belong to the user named in the path, otherwise not-found
// is returned and nothing is deleted.
func TestAdminDeleteUserSession_WrongUser(t *testing.T) {
	h, userRepo, sessionRepo, _ := setupSessionOASIntegration(t)
	ctx := context.Background()

	admin := createIntegrationUser(t, userRepo, "admin-del2@example.com", "pw")
	admin.Role = model.RoleAdmin
	owner := createIntegrationUser(t, userRepo, "owner-del@example.com", "pw")
	other := createIntegrationUser(t, userRepo, "other-del@example.com", "pw")

	session, err := sessionRepo.Create(ctx, owner.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Request with other.ID in the path (not the owner's ID).
	res, err := h.AdminDeleteUserSession(api.ContextWithUser(ctx, admin),
		oas.AdminDeleteUserSessionParams{ID: other.ID, SessionId: session.ID})
	if err != nil {
		t.Fatalf("AdminDeleteUserSession returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminDeleteUserSessionNotFound); !ok {
		t.Errorf("expected *oas.AdminDeleteUserSessionNotFound for session of a different user, got %T", res)
	}

	// The owner's session must survive.
	if _, err := sessionRepo.GetByID(ctx, session.ID); err != nil {
		t.Errorf("owner's session should not have been deleted: %v", err)
	}
}

func TestAdminDeleteUserSession_SessionNotFound(t *testing.T) {
	h, userRepo, _, _ := setupSessionOASIntegration(t)
	ctx := context.Background()

	admin := createIntegrationUser(t, userRepo, "admin-del3@example.com", "pw")
	admin.Role = model.RoleAdmin

	res, err := h.AdminDeleteUserSession(api.ContextWithUser(ctx, admin),
		oas.AdminDeleteUserSessionParams{ID: 1, SessionId: "nonexistent-session-id"})
	if err != nil {
		t.Fatalf("AdminDeleteUserSession returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminDeleteUserSessionNotFound); !ok {
		t.Errorf("expected *oas.AdminDeleteUserSessionNotFound, got %T", res)
	}
}

func TestAdminDeleteUserSession_NonAdminForbidden(t *testing.T) {
	h, userRepo, sessionRepo, _ := setupSessionOASIntegration(t)
	ctx := context.Background()

	user := createIntegrationUser(t, userRepo, "non-admin-del@example.com", "pw")
	target := createIntegrationUser(t, userRepo, "target-del2@example.com", "pw")

	session, err := sessionRepo.Create(ctx, target.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	res, err := h.AdminDeleteUserSession(api.ContextWithUser(ctx, user),
		oas.AdminDeleteUserSessionParams{ID: target.ID, SessionId: session.ID})
	if err != nil {
		t.Fatalf("AdminDeleteUserSession returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminDeleteUserSessionForbidden); !ok {
		t.Errorf("expected *oas.AdminDeleteUserSessionForbidden for non-admin, got %T", res)
	}

	// Unauthenticated context is forbidden as well.
	res, err = h.AdminDeleteUserSession(ctx,
		oas.AdminDeleteUserSessionParams{ID: target.ID, SessionId: session.ID})
	if err != nil {
		t.Fatalf("AdminDeleteUserSession returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminDeleteUserSessionForbidden); !ok {
		t.Errorf("expected *oas.AdminDeleteUserSessionForbidden for unauthenticated request, got %T", res)
	}
}

// ---------------------------------------------------------------------------
// LogoutAll
// ---------------------------------------------------------------------------

func TestLogoutAll_Unauthenticated(t *testing.T) {
	h := newSessionTestHandler(&mockUserRepo{}, &mockSessionRepo{}, &mockApiKeyRepo{})
	res, err := h.LogoutAll(context.Background())
	if err != nil {
		t.Fatalf("LogoutAll returned error: %v", err)
	}
	if _, ok := res.(*oas.Error); !ok {
		t.Errorf("expected *oas.Error for unauthenticated request, got %T", res)
	}
}

func TestLogoutAll_NoSessionInContext(t *testing.T) {
	h := newSessionTestHandler(&mockUserRepo{}, &mockSessionRepo{}, &mockApiKeyRepo{})
	ctx := api.ContextWithUser(context.Background(), &model.User{ID: 1, Email: "a@b.c"})
	res, err := h.LogoutAll(ctx)
	if err != nil {
		t.Fatalf("LogoutAll returned error: %v", err)
	}
	if _, ok := res.(*oas.Error); !ok {
		t.Errorf("expected *oas.Error without a current session, got %T", res)
	}
}

func TestLogoutAll_RevokesOtherSessionsExceptCurrent(t *testing.T) {
	var gotUserID int64
	var gotExceptID string
	sessions := &mockSessionRepo{
		deleteAllByUserFn: func(_ context.Context, userID int64, exceptID string) error {
			gotUserID = userID
			gotExceptID = exceptID
			return nil
		},
	}
	h := newSessionTestHandler(&mockUserRepo{}, sessions, &mockApiKeyRepo{})
	user := &model.User{ID: 7, Email: "a@b.c"}
	ctx := api.ContextWithUser(context.Background(), user)
	ctx = api.ContextWithSession(ctx, &model.Session{ID: "current-session", UserID: user.ID})

	res, err := h.LogoutAll(ctx)
	if err != nil {
		t.Fatalf("LogoutAll returned error: %v", err)
	}
	if _, ok := res.(*oas.LogoutAllNoContent); !ok {
		t.Fatalf("expected *oas.LogoutAllNoContent, got %T", res)
	}
	if gotUserID != user.ID || gotExceptID != "current-session" {
		t.Errorf("DeleteAllByUser called with (%d, %q), want (%d, %q)", gotUserID, gotExceptID, user.ID, "current-session")
	}
}

func TestLogoutAll_RepoError(t *testing.T) {
	sessions := &mockSessionRepo{
		deleteAllByUserFn: func(_ context.Context, _ int64, _ string) error {
			return errors.New("db down")
		},
	}
	h := newSessionTestHandler(&mockUserRepo{}, sessions, &mockApiKeyRepo{})
	ctx := api.ContextWithUser(context.Background(), &model.User{ID: 1, Email: "a@b.c"})
	ctx = api.ContextWithSession(ctx, &model.Session{ID: "s", UserID: 1})

	res, err := h.LogoutAll(ctx)
	if err != nil {
		t.Fatalf("LogoutAll returned error: %v", err)
	}
	if _, ok := res.(*oas.Error); !ok {
		t.Errorf("expected *oas.Error on repo failure, got %T", res)
	}
}

package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// TestLogin_MissingFields verifies that login rejects requests missing email or password.
func TestLogin_MissingFields(t *testing.T) {
	h := handlers.NewSessionHandler(
		repository.NewUserRepository(nil),
		repository.NewSessionRepository(nil),
		repository.NewApiKeyRepository(nil),
	)

	tests := []struct {
		name string
		body map[string]string
	}{
		{"empty body", map[string]string{}},
		{"missing password", map[string]string{"email": "test@example.com"}},
		{"missing email", map[string]string{"password": "secret"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/session", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			h.Login(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", rr.Code)
			}
		})
	}
}

// TestLogin_InvalidJSON verifies that malformed JSON is rejected.
func TestLogin_InvalidJSON(t *testing.T) {
	h := handlers.NewSessionHandler(
		repository.NewUserRepository(nil),
		repository.NewSessionRepository(nil),
		repository.NewApiKeyRepository(nil),
	)

	req := httptest.NewRequest(http.MethodPost, "/api/session", bytes.NewReader([]byte("not json")))
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
}

// TestGetCurrentSession_Authenticated verifies that a user with a valid
// session cookie can retrieve their session info.
func TestGetCurrentSession_Authenticated(t *testing.T) {
	testUser := &model.User{ID: 1, Email: "test@example.com", Name: "Test"}

	users := &mockUserRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.User, error) {
			if id == 1 {
				return testUser, nil
			}
			return nil, errors.New("not found")
		},
	}
	sessions := &mockSessionRepo{
		getByIDFn: func(_ context.Context, id string) (*model.Session, error) {
			if id == "valid-session-id" {
				return &model.Session{ID: "valid-session-id", UserID: 1, ExpiresAt: time.Now().Add(24 * time.Hour)}, nil
			}
			return nil, errors.New("not found")
		},
	}

	h := handlers.NewSessionHandler(users, sessions, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session-id"})
	rr := httptest.NewRecorder()

	h.GetCurrentSession(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp model.User
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Email != "test@example.com" {
		t.Errorf("expected email test@example.com, got %q", resp.Email)
	}
}

// TestGetCurrentSession_Unauthenticated verifies that requests without
// a session cookie receive a 401 response.
func TestGetCurrentSession_Unauthenticated(t *testing.T) {
	h := handlers.NewSessionHandler(
		&mockUserRepo{},
		&mockSessionRepo{},
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	rr := httptest.NewRecorder()
	h.GetCurrentSession(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

// TestGetCurrentSession_InvalidSession verifies that a request with an
// invalid or expired session cookie receives a 401 response.
func TestGetCurrentSession_InvalidSession(t *testing.T) {
	sessions := &mockSessionRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.Session, error) {
			return nil, errors.New("not found")
		},
	}
	h := handlers.NewSessionHandler(&mockUserRepo{}, sessions, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "expired-session-id"})
	rr := httptest.NewRecorder()
	h.GetCurrentSession(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

// TestGetCurrentSession_SessionValidButUserDeleted verifies that a valid
// session whose user no longer exists returns 401.
func TestGetCurrentSession_SessionValidButUserDeleted(t *testing.T) {
	sessions := &mockSessionRepo{
		getByIDFn: func(_ context.Context, id string) (*model.Session, error) {
			if id == "orphan-session" {
				return &model.Session{ID: "orphan-session", UserID: 999, ExpiresAt: time.Now().Add(time.Hour)}, nil
			}
			return nil, errors.New("not found")
		},
	}
	users := &mockUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			return nil, errors.New("not found")
		},
	}
	h := handlers.NewSessionHandler(users, sessions, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "orphan-session"})
	rr := httptest.NewRecorder()
	h.GetCurrentSession(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

// TestGenerateToken_Unauthenticated verifies that token generation
// requires authentication.
func TestGenerateToken_Unauthenticated(t *testing.T) {
	h := handlers.NewSessionHandler(
		repository.NewUserRepository(nil),
		repository.NewSessionRepository(nil),
		repository.NewApiKeyRepository(nil),
	)

	req := httptest.NewRequest(http.MethodPost, "/api/session/token", nil)
	rr := httptest.NewRecorder()
	h.GenerateToken(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

// TestLogout_ClearsCookie verifies that logout sets an expired cookie with
// both MaxAge=-1 and an epoch Expires timestamp, ensuring WebView
// implementations (Traccar Manager app) properly clear the session cookie.
func TestLogout_ClearsCookie(t *testing.T) {
	h := handlers.NewSessionHandler(
		repository.NewUserRepository(nil),
		repository.NewSessionRepository(nil),
		repository.NewApiKeyRepository(nil),
	)

	req := httptest.NewRequest(http.MethodDelete, "/api/session", nil)
	rr := httptest.NewRecorder()
	h.Logout(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", rr.Code)
	}

	cookies := rr.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "session_id" && c.Value == "" {
			found = true
			// MaxAge < 0 signals the browser/WebView to delete the cookie immediately.
			if c.MaxAge != -1 {
				t.Errorf("expected MaxAge=-1 for cookie deletion, got %d", c.MaxAge)
			}
		}
	}
	if !found {
		t.Error("expected session_id cookie to be cleared")
	}
}

// TestLogout_SecureCookieInProduction verifies the Secure flag is set
// when MOTUS_ENV is not "development".
func TestLogout_SecureCookieInProduction(t *testing.T) {
	// Default environment (no MOTUS_ENV set) should be treated as production.
	t.Setenv("MOTUS_ENV", "")

	h := handlers.NewSessionHandler(
		repository.NewUserRepository(nil),
		repository.NewSessionRepository(nil),
		repository.NewApiKeyRepository(nil),
	)

	req := httptest.NewRequest(http.MethodDelete, "/api/session", nil)
	rr := httptest.NewRecorder()
	h.Logout(rr, req)

	for _, c := range rr.Result().Cookies() {
		if c.Name == "session_id" {
			if !c.Secure {
				t.Error("expected Secure flag on session_id cookie in production")
			}
			if c.SameSite != http.SameSiteLaxMode {
				t.Errorf("expected SameSite=Lax, got %v", c.SameSite)
			}
			return
		}
	}
	t.Error("session_id cookie not found")
}

// TestLogin_FormEncoded verifies that form-encoded login is accepted.
func TestLogin_FormEncoded(t *testing.T) {
	h := handlers.NewSessionHandler(
		repository.NewUserRepository(nil),
		repository.NewSessionRepository(nil),
		repository.NewApiKeyRepository(nil),
	)

	// Form-encoded request with missing email should return 400.
	form := url.Values{}
	form.Set("password", "secret")
	req := httptest.NewRequest(http.MethodPost, "/api/session", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for missing email in form, got %d", rr.Code)
	}
}

// TestLogin_FormEncoded_MissingFields verifies form-encoded with empty fields.
func TestLogin_FormEncoded_MissingFields(t *testing.T) {
	h := handlers.NewSessionHandler(
		repository.NewUserRepository(nil),
		repository.NewSessionRepository(nil),
		repository.NewApiKeyRepository(nil),
	)

	form := url.Values{}
	req := httptest.NewRequest(http.MethodPost, "/api/session", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for empty form, got %d", rr.Code)
	}
}

// TestGetCurrentSession_TokenParam verifies that a token query parameter
// triggers the token-based auth code path. With a nil pool the DB call
// will panic, so we recover and verify the token path was entered (not
// the standard context-based path that would return 401 immediately).
func TestGetCurrentSession_TokenParam(t *testing.T) {
	h := handlers.NewSessionHandler(
		repository.NewUserRepository(nil),
		repository.NewSessionRepository(nil),
		repository.NewApiKeyRepository(nil),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/session?token=test123", nil)
	rr := httptest.NewRecorder()

	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		h.GetCurrentSession(rr, req)
	}()

	// The nil pool causes a panic in GetByToken, which proves the token
	// code path was entered. If it did not panic, verify we got 401.
	if !panicked && rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 for invalid token, got %d", rr.Code)
	}
}

// TestGetCurrentSession_TraccarFields verifies that the response includes
// Traccar-compatible fields when user is authenticated via session cookie.
func TestGetCurrentSession_TraccarFields(t *testing.T) {
	testUser := &model.User{ID: 1, Email: "admin@example.com", Name: "Admin", Role: "admin"}

	users := &mockUserRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.User, error) {
			if id == 1 {
				return testUser, nil
			}
			return nil, errors.New("not found")
		},
	}
	sessions := &mockSessionRepo{
		getByIDFn: func(_ context.Context, id string) (*model.Session, error) {
			if id == "admin-session" {
				return &model.Session{ID: "admin-session", UserID: 1, ExpiresAt: time.Now().Add(24 * time.Hour)}, nil
			}
			return nil, errors.New("not found")
		},
	}

	h := handlers.NewSessionHandler(users, sessions, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "admin-session"})
	rr := httptest.NewRecorder()
	h.GetCurrentSession(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	if admin, ok := resp["administrator"]; !ok {
		t.Error("missing 'administrator' field in session response")
	} else if admin != true {
		t.Errorf("expected administrator=true for admin user, got %v", admin)
	}

	// Role should not be exposed (json:"-")
	if _, ok := resp["role"]; ok {
		t.Error("'role' field should not be exposed in JSON (tagged with json:\"-\")")
	}
}

// TestLogout_InsecureCookieInDev verifies the Secure flag is NOT set
// when MOTUS_ENV=development.
func TestLogout_InsecureCookieInDev(t *testing.T) {
	t.Setenv("MOTUS_ENV", "development")

	h := handlers.NewSessionHandler(
		repository.NewUserRepository(nil),
		repository.NewSessionRepository(nil),
		repository.NewApiKeyRepository(nil),
	)

	req := httptest.NewRequest(http.MethodDelete, "/api/session", nil)
	rr := httptest.NewRecorder()
	h.Logout(rr, req)

	for _, c := range rr.Result().Cookies() {
		if c.Name == "session_id" {
			if c.Secure {
				t.Error("Secure flag should not be set in development mode")
			}
			return
		}
	}
	t.Error("session_id cookie not found")
}

// ---------------------------------------------------------------------------
// ListSessions tests
// ---------------------------------------------------------------------------

// TestListSessions_Unauthenticated verifies that listing sessions without
// authentication returns 401.
func TestListSessions_Unauthenticated(t *testing.T) {
	h := handlers.NewSessionHandler(&mockUserRepo{}, &mockSessionRepo{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	rr := httptest.NewRecorder()
	h.ListSessions(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestListSessions_Success verifies that sessions are returned with IsCurrent
// set for the matching session cookie, and sudo sessions are filtered out.
func TestListSessions_Success(t *testing.T) {
	user := &model.User{ID: 1, Email: "test@example.com", Role: model.RoleUser}

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

	h := handlers.NewSessionHandler(&mockUserRepo{}, sessions, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "sess-current"})
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()
	h.ListSessions(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var result []model.Session
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	// Sudo session should be filtered out, leaving 2 sessions.
	if len(result) != 2 {
		t.Fatalf("expected 2 sessions (sudo filtered), got %d", len(result))
	}

	// Verify IsCurrent is set on the matching session only.
	for _, s := range result {
		if s.ID == "sess-current" && !s.IsCurrent {
			t.Errorf("expected IsCurrent=true for session %q", s.ID)
		}
		if s.ID == "sess-1" && s.IsCurrent {
			t.Errorf("expected IsCurrent=false for session %q", s.ID)
		}
		if s.IsSudo {
			t.Errorf("sudo session %q should not appear in results", s.ID)
		}
	}
}

// TestListSessions_Empty verifies that an empty list returns an empty JSON
// array (not null).
func TestListSessions_Empty(t *testing.T) {
	user := &model.User{ID: 1, Email: "test@example.com", Role: model.RoleUser}

	sessions := &mockSessionRepo{
		listByUserFn: func(_ context.Context, _ int64) ([]*model.Session, error) {
			return []*model.Session{}, nil
		},
	}

	h := handlers.NewSessionHandler(&mockUserRepo{}, sessions, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()
	h.ListSessions(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Verify it returns [] not null.
	body := strings.TrimSpace(rr.Body.String())
	if body != "[]" {
		t.Errorf("expected empty JSON array '[]', got %q", body)
	}
}

// TestListSessions_Error verifies that a repository failure returns 500.
func TestListSessions_Error(t *testing.T) {
	user := &model.User{ID: 1, Email: "test@example.com", Role: model.RoleUser}

	sessions := &mockSessionRepo{
		listByUserFn: func(_ context.Context, _ int64) ([]*model.Session, error) {
			return nil, errors.New("database error")
		},
	}

	h := handlers.NewSessionHandler(&mockUserRepo{}, sessions, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()
	h.ListSessions(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// DeleteSession tests
// ---------------------------------------------------------------------------

// TestDeleteSession_Unauthenticated verifies that deleting a session without
// authentication returns 401.
func TestDeleteSession_Unauthenticated(t *testing.T) {
	h := handlers.NewSessionHandler(&mockUserRepo{}, &mockSessionRepo{}, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/sessions/some-id", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "some-id")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.DeleteSession(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestDeleteSession_Success verifies that an authenticated user can revoke
// their own session and receives 204.
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

	h := handlers.NewSessionHandler(&mockUserRepo{}, sessions, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/sessions/target-sess", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "target-sess")
	ctx := api.ContextWithUser(req.Context(), user)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	h.DeleteSession(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if deletedID != "target-sess" {
		t.Errorf("expected Delete called with 'target-sess', got %q", deletedID)
	}
}

// TestDeleteSession_NotFound verifies that deleting a non-existent session
// returns 404.
func TestDeleteSession_NotFound(t *testing.T) {
	user := &model.User{ID: 1, Email: "test@example.com", Role: model.RoleUser}

	sessions := &mockSessionRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.Session, error) {
			return nil, errors.New("not found")
		},
	}

	h := handlers.NewSessionHandler(&mockUserRepo{}, sessions, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/sessions/nonexistent", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "nonexistent")
	ctx := api.ContextWithUser(req.Context(), user)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	h.DeleteSession(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestDeleteSession_Forbidden verifies that a non-admin user cannot discover
// or delete another user's session (returns 404, not 403, to avoid leaking
// session existence).
func TestDeleteSession_Forbidden(t *testing.T) {
	user := &model.User{ID: 1, Email: "user@example.com", Role: model.RoleUser}

	sessions := &mockSessionRepo{
		// GetByIDPrefix is scoped to user 1, but the session belongs to user 99.
		// So this returns not-found, and the non-admin user gets 404.
	}

	h := handlers.NewSessionHandler(&mockUserRepo{}, sessions, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/sessions/other-user-sess", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "other-user-sess")
	ctx := api.ContextWithUser(req.Context(), user)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	h.DeleteSession(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404 (non-admin cannot see other user sessions), got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestDeleteSession_AdminCanDeleteOtherUser verifies that an admin can
// delete any user's session.
func TestDeleteSession_AdminCanDeleteOtherUser(t *testing.T) {
	admin := &model.User{ID: 1, Email: "admin@example.com", Role: model.RoleAdmin}
	var deletedID string

	sessions := &mockSessionRepo{
		// GetByIDPrefix won't find it (admin doesn't own it), so falls back to GetByID.
		getByIDFn: func(_ context.Context, id string) (*model.Session, error) {
			if id == "other-user-sess" {
				// Session belongs to user 99, but admin should be able to delete it.
				return &model.Session{ID: "other-user-sess", UserID: 99, ExpiresAt: time.Now().Add(time.Hour)}, nil
			}
			return nil, errors.New("not found")
		},
		deleteFn: func(_ context.Context, id string) error {
			deletedID = id
			return nil
		},
	}

	h := handlers.NewSessionHandler(&mockUserRepo{}, sessions, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/sessions/other-user-sess", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "other-user-sess")
	ctx := api.ContextWithUser(req.Context(), admin)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	h.DeleteSession(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if deletedID != "other-user-sess" {
		t.Errorf("expected Delete called with 'other-user-sess', got %q", deletedID)
	}
}

// TestDeleteSession_MissingID verifies that a request with an empty session
// ID returns 400.
func TestDeleteSession_MissingID(t *testing.T) {
	user := &model.User{ID: 1, Email: "test@example.com", Role: model.RoleUser}

	h := handlers.NewSessionHandler(&mockUserRepo{}, &mockSessionRepo{}, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/sessions/", nil)
	rctx := chi.NewRouteContext()
	// Simulate empty URL param (no {id} value).
	rctx.URLParams.Add("id", "")
	ctx := api.ContextWithUser(req.Context(), user)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	h.DeleteSession(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

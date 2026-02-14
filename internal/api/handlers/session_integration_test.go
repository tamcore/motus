package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/tamcore/motus/internal/storage/repository/testutil"
	"golang.org/x/crypto/bcrypt"
)

func setupSessionHandler(t *testing.T) (*handlers.SessionHandler, *repository.UserRepository, *repository.SessionRepository) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	h := handlers.NewSessionHandler(userRepo, sessionRepo, apiKeyRepo)
	return h, userRepo, sessionRepo
}

func TestLogin_Success(t *testing.T) {
	h, userRepo, _ := setupSessionHandler(t)
	ctx := context.Background()

	// Create a user with a known password.
	hash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	user := &model.User{Email: "login@example.com", PasswordHash: string(hash), Name: "Login User"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	body, _ := json.Marshal(map[string]string{
		"email":    "login@example.com",
		"password": "secret123",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/session", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Verify session cookie is set.
	cookies := rr.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "session_id" && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Error("expected session_id cookie to be set")
	}

	// Verify response body contains user info.
	var resp model.User
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Email != "login@example.com" {
		t.Errorf("expected email 'login@example.com', got %q", resp.Email)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	h, userRepo, _ := setupSessionHandler(t)
	ctx := context.Background()

	hash, _ := bcrypt.GenerateFromPassword([]byte("correct"), bcrypt.DefaultCost)
	user := &model.User{Email: "wrongpw@example.com", PasswordHash: string(hash), Name: "Wrong PW"}
	_ = userRepo.Create(ctx, user)

	body, _ := json.Marshal(map[string]string{
		"email":    "wrongpw@example.com",
		"password": "wrong",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/session", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

func TestLogin_NonexistentUser(t *testing.T) {
	h, _, _ := setupSessionHandler(t)

	body, _ := json.Marshal(map[string]string{
		"email":    "nobody@example.com",
		"password": "anything",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/session", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

func TestGenerateToken_Success(t *testing.T) {
	h, userRepo, _ := setupSessionHandler(t)
	ctx := context.Background()

	user := &model.User{Email: "token@example.com", PasswordHash: "hash", Name: "Token User"}
	_ = userRepo.Create(ctx, user)

	req := httptest.NewRequest(http.MethodPost, "/api/session/token", nil)
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()

	h.GenerateToken(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp["token"] == "" {
		t.Error("expected non-empty token in response")
	}
}

func TestLogin_RememberMe(t *testing.T) {
	h, userRepo, _ := setupSessionHandler(t)
	ctx := context.Background()

	hash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	user := &model.User{Email: "remember@example.com", PasswordHash: string(hash), Name: "Remember User"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	tests := []struct {
		name      string
		remember  bool
		minExpiry time.Duration
		maxExpiry time.Duration
	}{
		{
			name:      "without remember me",
			remember:  false,
			minExpiry: 23 * time.Hour,
			maxExpiry: 25 * time.Hour,
		},
		{
			name:      "with remember me",
			remember:  true,
			minExpiry: 87599 * time.Hour,
			maxExpiry: 87601 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{
				"email":    "remember@example.com",
				"password": "secret123",
				"remember": tt.remember,
			})

			req := httptest.NewRequest(http.MethodPost, "/api/session", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			h.Login(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d; body: %s", rr.Code, rr.Body.String())
			}

			// Verify cookie expiry is in the expected range.
			cookies := rr.Result().Cookies()
			var sessionCookie *http.Cookie
			for _, c := range cookies {
				if c.Name == "session_id" {
					sessionCookie = c
					break
				}
			}
			if sessionCookie == nil {
				t.Fatal("expected session_id cookie to be set")
			}

			expiresIn := time.Until(sessionCookie.Expires)
			if expiresIn < tt.minExpiry || expiresIn > tt.maxExpiry {
				t.Errorf("expected cookie expiry between %v and %v, got %v",
					tt.minExpiry, tt.maxExpiry, expiresIn)
			}
		})
	}
}

func TestLogin_RememberMe_FormEncoded(t *testing.T) {
	h, userRepo, _ := setupSessionHandler(t)
	ctx := context.Background()

	hash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	user := &model.User{Email: "rememberform@example.com", PasswordHash: string(hash), Name: "Form User"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	form := url.Values{}
	form.Set("email", "rememberform@example.com")
	form.Set("password", "secret123")
	form.Set("remember", "true")

	req := httptest.NewRequest(http.MethodPost, "/api/session", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Verify cookie expiry is ~87600 hours (10 years / effectively indefinite).
	for _, c := range rr.Result().Cookies() {
		if c.Name == "session_id" {
			expiresIn := time.Until(c.Expires)
			if expiresIn < 87599*time.Hour {
				t.Errorf("expected cookie expiry ~87600h for remember=true form, got %v", expiresIn)
			}
			return
		}
	}
	t.Fatal("expected session_id cookie")
}

func TestLogout_WithCookie(t *testing.T) {
	h, userRepo, sessionRepo := setupSessionHandler(t)
	ctx := context.Background()

	user := &model.User{Email: "logout@example.com", PasswordHash: "hash", Name: "Logout User"}
	_ = userRepo.Create(ctx, user)

	session, err := sessionRepo.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/session", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: session.ID})
	rr := httptest.NewRecorder()

	h.Logout(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", rr.Code)
	}

	// Verify session is deleted.
	got, err := sessionRepo.GetByID(ctx, session.ID)
	if err == nil && got != nil {
		t.Error("expected session to be deleted")
	}
}

// --- AdminDeleteSession ---

func TestAdminDeleteSession_Success(t *testing.T) {
	h, userRepo, sessionRepo := setupSessionHandler(t)
	ctx := context.Background()

	admin := &model.User{Email: "admin-del@example.com", PasswordHash: "hash", Name: "Admin", Role: model.RoleAdmin}
	if err := userRepo.Create(ctx, admin); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	target := &model.User{Email: "target-del@example.com", PasswordHash: "hash", Name: "Target"}
	if err := userRepo.Create(ctx, target); err != nil {
		t.Fatalf("create target: %v", err)
	}

	session, err := sessionRepo.Create(ctx, target.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/users/"+fmt.Sprint(target.ID)+"/sessions/"+session.ID, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprint(target.ID))
	rctx.URLParams.Add("sessionId", session.ID)
	ctx2 := context.WithValue(api.ContextWithUser(req.Context(), admin), chi.RouteCtxKey, rctx)
	req = req.WithContext(ctx2)
	rr := httptest.NewRecorder()

	h.AdminDeleteSession(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestAdminDeleteSession_WrongUser(t *testing.T) {
	h, userRepo, sessionRepo := setupSessionHandler(t)
	ctx := context.Background()

	admin := &model.User{Email: "admin-del2@example.com", PasswordHash: "hash", Name: "Admin", Role: model.RoleAdmin}
	if err := userRepo.Create(ctx, admin); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	owner := &model.User{Email: "owner-del@example.com", PasswordHash: "hash", Name: "Owner"}
	if err := userRepo.Create(ctx, owner); err != nil {
		t.Fatalf("create owner: %v", err)
	}

	otherUser := &model.User{Email: "other-del@example.com", PasswordHash: "hash", Name: "Other"}
	if err := userRepo.Create(ctx, otherUser); err != nil {
		t.Fatalf("create other: %v", err)
	}

	session, err := sessionRepo.Create(ctx, owner.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Request with otherUser.ID in the URL (not owner's ID).
	req := httptest.NewRequest(http.MethodDelete, "/api/users/"+fmt.Sprint(otherUser.ID)+"/sessions/"+session.ID, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprint(otherUser.ID))
	rctx.URLParams.Add("sessionId", session.ID)
	ctx2 := context.WithValue(api.ContextWithUser(req.Context(), admin), chi.RouteCtxKey, rctx)
	req = req.WithContext(ctx2)
	rr := httptest.NewRecorder()

	h.AdminDeleteSession(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for session belonging to different user, got %d", rr.Code)
	}
}

func TestAdminDeleteSession_SessionNotFound(t *testing.T) {
	h, userRepo, _ := setupSessionHandler(t)
	ctx := context.Background()

	admin := &model.User{Email: "admin-del3@example.com", PasswordHash: "hash", Name: "Admin", Role: model.RoleAdmin}
	if err := userRepo.Create(ctx, admin); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/users/1/sessions/nonexistent-session-id", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	rctx.URLParams.Add("sessionId", "nonexistent-session-id")
	ctx2 := context.WithValue(api.ContextWithUser(req.Context(), admin), chi.RouteCtxKey, rctx)
	req = req.WithContext(ctx2)
	rr := httptest.NewRecorder()

	h.AdminDeleteSession(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestAdminDeleteSession_NotAuthenticated(t *testing.T) {
	h, _, _ := setupSessionHandler(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/users/1/sessions/abc", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	rctx.URLParams.Add("sessionId", "abc")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.AdminDeleteSession(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAdminDeleteSession_InvalidUserID(t *testing.T) {
	h, userRepo, _ := setupSessionHandler(t)
	ctx := context.Background()

	admin := &model.User{Email: "admin-del4@example.com", PasswordHash: "hash", Name: "Admin", Role: model.RoleAdmin}
	if err := userRepo.Create(ctx, admin); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/users/notanumber/sessions/abc", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "notanumber")
	rctx.URLParams.Add("sessionId", "abc")
	ctx2 := context.WithValue(api.ContextWithUser(req.Context(), admin), chi.RouteCtxKey, rctx)
	req = req.WithContext(ctx2)
	rr := httptest.NewRecorder()

	h.AdminDeleteSession(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

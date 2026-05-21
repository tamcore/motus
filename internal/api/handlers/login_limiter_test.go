package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/tamcore/motus/internal/api/handlers"
	"github.com/tamcore/motus/internal/model"
)

func TestLogin_LockedAfterMaxFailures(t *testing.T) {
	const email = "target@example.com"

	users := &mockUserRepo{
		getByEmailFn: func(_ context.Context, _ string) (*model.User, error) {
			return nil, errors.New("not found")
		},
	}
	h := handlers.NewSessionHandler(users, &mockSessionRepo{}, &mockApiKeyRepo{})

	doLogin := func() int {
		body := strings.NewReader(`{"email":"` + email + `","password":"wrong"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/session", body)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.Login(rr, req)
		return rr.Code
	}

	// First 10 attempts should all be 401.
	for i := 0; i < 10; i++ {
		if code := doLogin(); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, code)
		}
	}

	// 11th attempt should be 429 (locked).
	if code := doLogin(); code != http.StatusTooManyRequests {
		t.Errorf("after lockout: expected 429, got %d", code)
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
	h := handlers.NewSessionHandler(users, &mockSessionRepo{}, &mockApiKeyRepo{})

	doFailedLogin := func() {
		body := strings.NewReader(`{"email":"` + email + `","password":"wrong"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/session", body)
		req.Header.Set("Content-Type", "application/json")
		h.Login(httptest.NewRecorder(), req)
	}

	// 5 failures.
	for i := 0; i < 5; i++ {
		doFailedLogin()
	}

	// Successful login resets the counter.
	body := strings.NewReader(`{"email":"` + email + `","password":"` + correctPassword + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/session", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Login(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 on successful login, got %d", rr.Code)
	}

	// Another 9 failures after reset should NOT trigger lockout (counter is reset).
	for i := 0; i < 9; i++ {
		body := strings.NewReader(`{"email":"` + email + `","password":"wrong"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/session", body)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.Login(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("post-reset attempt %d: expected 401, got %d", i+1, rr.Code)
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
	h := handlers.NewSessionHandler(users, &mockSessionRepo{}, &mockApiKeyRepo{})

	for i := 0; i < 10; i++ {
		body := strings.NewReader(`{"email":"` + email + `","password":"x"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/session", body)
		req.Header.Set("Content-Type", "application/json")
		h.Login(httptest.NewRecorder(), req)
	}

	body := strings.NewReader(`{"email":"` + email + `","password":"x"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/session", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 after 10 unknown-email failures, got %d", rr.Code)
	}
}

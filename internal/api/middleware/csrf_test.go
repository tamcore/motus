package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gorillacsrf "github.com/gorilla/csrf"
	"github.com/tamcore/motus/internal/api/middleware"
)

func testCSRFSecret() []byte {
	// 32 bytes for gorilla/csrf.
	return []byte("01234567890123456789012345678901")
}

func csrfMiddleware() func(http.Handler) http.Handler {
	return middleware.CSRF(middleware.CSRFConfig{
		Secret: testCSRFSecret(),
		Secure: false, // Disable Secure flag for testing (no HTTPS).
	})
}

func TestCSRF_GETRequestAllowed(t *testing.T) {
	mw := csrfMiddleware()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET request: expected status 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestCSRF_POSTWithBearerTokenExempt(t *testing.T) {
	mw := csrfMiddleware()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/devices", strings.NewReader(`{"name":"test"}`))
	req.Header.Set("Authorization", "Bearer some-valid-token")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("POST with Bearer token: expected status 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestCSRF_POSTWithEmptyBearerNotExempt(t *testing.T) {
	mw := csrfMiddleware()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/devices", strings.NewReader(`{"name":"test"}`))
	req.Header.Set("Authorization", "Bearer ")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Empty bearer should NOT be exempt; CSRF check should reject.
	if rr.Code != http.StatusForbidden {
		t.Errorf("POST with empty Bearer: expected status 403, got %d", rr.Code)
	}
}

func TestCSRF_POSTWithoutTokenRejected(t *testing.T) {
	mw := csrfMiddleware()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/devices", strings.NewReader(`{"name":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("POST without CSRF token: expected status 403, got %d", rr.Code)
	}

	// Verify JSON error response.
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(body["error"], "CSRF") {
		t.Errorf("expected CSRF error message, got %q", body["error"])
	}
}

func TestCSRF_POSTWithValidTokenSucceeds(t *testing.T) {
	mw := csrfMiddleware()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Step 1: Make a GET to obtain the CSRF token and cookie.
	// Mark as plaintext HTTP so gorilla/csrf skips the strict
	// TLS-only origin/referer checks.
	getReq := gorillacsrf.PlaintextHTTPRequest(
		httptest.NewRequest(http.MethodGet, "http://localhost/api/devices", nil),
	)
	getRR := httptest.NewRecorder()
	handler.ServeHTTP(getRR, getReq)

	if getRR.Code != http.StatusOK {
		t.Fatalf("GET request failed: %d", getRR.Code)
	}

	// Extract the CSRF token from the response header.
	csrfToken := getRR.Header().Get("X-CSRF-Token")
	if csrfToken == "" {
		t.Fatal("expected X-CSRF-Token header in GET response")
	}

	// Extract the CSRF cookie.
	var csrfCookie *http.Cookie
	for _, c := range getRR.Result().Cookies() {
		if c.Name == "_gorilla_csrf" {
			csrfCookie = c
			break
		}
	}
	if csrfCookie == nil {
		t.Fatal("expected _gorilla_csrf cookie in GET response")
	}

	// Step 2: Make a POST with the CSRF token and cookie.
	postReq := gorillacsrf.PlaintextHTTPRequest(
		httptest.NewRequest(http.MethodPost, "http://localhost/api/devices", strings.NewReader(`{"name":"test"}`)),
	)
	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("X-CSRF-Token", csrfToken)
	postReq.AddCookie(csrfCookie)
	postRR := httptest.NewRecorder()
	handler.ServeHTTP(postRR, postReq)

	if postRR.Code != http.StatusOK {
		t.Errorf("POST with valid CSRF token: expected status 200, got %d; body: %s", postRR.Code, postRR.Body.String())
	}
}

func TestCSRF_POSTBehindTLSProxy(t *testing.T) {
	// Simulates MOTUS_ENV=development (Secure: false) behind a
	// TLS-terminating reverse proxy (nginx ingress). The browser sends
	// Origin: https://... but the pod sees HTTP. X-Forwarded-Proto: https
	// must prevent PlaintextHTTPRequest from being called so the origin
	// scheme comparison uses "https" instead of "http".
	mw := csrfMiddleware() // Secure: false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Step 1: GET to obtain token and cookie.
	getReq := httptest.NewRequest(http.MethodGet, "https://example.com/api/devices", nil)
	getReq.Header.Set("X-Forwarded-Proto", "https")
	getRR := httptest.NewRecorder()
	handler.ServeHTTP(getRR, getReq)

	if getRR.Code != http.StatusOK {
		t.Fatalf("GET failed: %d", getRR.Code)
	}

	csrfToken := getRR.Header().Get("X-CSRF-Token")
	if csrfToken == "" {
		t.Fatal("expected X-CSRF-Token header")
	}

	var csrfCookie *http.Cookie
	for _, c := range getRR.Result().Cookies() {
		if c.Name == "_gorilla_csrf" {
			csrfCookie = c
			break
		}
	}
	if csrfCookie == nil {
		t.Fatal("expected _gorilla_csrf cookie")
	}

	// Step 2: POST with Origin: https://... and X-Forwarded-Proto: https.
	postReq := httptest.NewRequest(http.MethodPost, "https://example.com/api/devices", strings.NewReader(`{"name":"test"}`))
	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("Origin", "https://example.com")
	postReq.Header.Set("X-Forwarded-Proto", "https")
	postReq.Header.Set("X-CSRF-Token", csrfToken)
	postReq.AddCookie(csrfCookie)
	postRR := httptest.NewRecorder()
	handler.ServeHTTP(postRR, postReq)

	if postRR.Code != http.StatusOK {
		t.Errorf("POST behind TLS proxy: expected 200, got %d; body: %s", postRR.Code, postRR.Body.String())
	}
}

func TestCSRF_DELETEWithBearerTokenExempt(t *testing.T) {
	mw := csrfMiddleware()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodDelete, "/api/devices/1", nil)
	req.Header.Set("Authorization", "Bearer valid-api-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("DELETE with Bearer: expected 200, got %d", rr.Code)
	}
}

func TestCSRF_PUTWithoutTokenRejected(t *testing.T) {
	mw := csrfMiddleware()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPut, "/api/devices/1", strings.NewReader(`{"name":"updated"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("PUT without CSRF token: expected 403, got %d", rr.Code)
	}
}

func TestCSRF_HeadRequestAllowed(t *testing.T) {
	mw := csrfMiddleware()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodHead, "/api/server", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("HEAD request: expected 200, got %d", rr.Code)
	}
}

func TestExemptLogin_CallsInnerHandler(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	// ExemptLogin wraps the handler — use it directly without the CSRF middleware
	// to verify it invokes the inner handler normally.
	handler := middleware.ExemptLogin(inner)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("ExemptLogin: inner handler was not called")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("ExemptLogin: expected 200, got %d", rr.Code)
	}
}

package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/api/middleware"
)

func okHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func TestRateLimit_AllowsWithinBurst(t *testing.T) {
	// Allow 5 requests per minute with a burst of 5.
	cfg := middleware.RateLimitConfig{Max: 5, Period: time.Minute}
	mw := middleware.RateLimit(cfg)
	handler := mw(http.HandlerFunc(okHandler))

	// All 5 burst requests should succeed.
	for i := range 5 {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected status 200, got %d", i+1, rr.Code)
		}
	}
}

func TestRateLimit_BlocksAfterBurstExhausted(t *testing.T) {
	// Allow 3 requests per minute with a burst of 3.
	cfg := middleware.RateLimitConfig{Max: 3, Period: time.Minute}
	mw := middleware.RateLimit(cfg)
	handler := mw(http.HandlerFunc(okHandler))

	// Exhaust the burst.
	for i := range 3 {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("burst request %d: expected status 200, got %d", i+1, rr.Code)
		}
	}

	// The 4th request should be rate limited.
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("request after burst: expected status 429, got %d", rr.Code)
	}

	// Verify JSON error response.
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body["error"] != "rate limit exceeded" {
		t.Errorf("expected error 'rate limit exceeded', got %q", body["error"])
	}

	// Verify Retry-After header.
	if rr.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
}

func TestRateLimit_DifferentIPsNotAffected(t *testing.T) {
	// Only 1 request per minute.
	cfg := middleware.RateLimitConfig{Max: 1, Period: time.Minute}
	mw := middleware.RateLimit(cfg)
	handler := mw(http.HandlerFunc(okHandler))

	// First IP uses its burst.
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "10.0.0.2:12345"
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusOK {
		t.Fatalf("first IP: expected status 200, got %d", rr1.Code)
	}

	// Second IP should still have its own burst available.
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "10.0.0.3:12345"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Errorf("different IP: expected status 200, got %d", rr2.Code)
	}
}

func TestRateLimit_ResponseFormat(t *testing.T) {
	cfg := middleware.RateLimitConfig{Max: 1, Period: time.Minute}
	mw := middleware.RateLimit(cfg)
	handler := mw(http.HandlerFunc(okHandler))

	// Use up the burst.
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "10.0.0.50:12345"
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)

	// Trigger 429.
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "10.0.0.50:12345"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr2.Code)
	}

	if ct := rr2.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var body map[string]string
	_ = json.NewDecoder(rr2.Body).Decode(&body)
	if body["error"] != "rate limit exceeded" {
		t.Errorf("expected error 'rate limit exceeded', got %q", body["error"])
	}
}

func TestLoginRateLimit_BlocksAfterFiveRequests(t *testing.T) {
	mw := middleware.RateLimit(middleware.RateLimitConfig{Max: 5, Period: time.Minute})
	handler := mw(http.HandlerFunc(okHandler))

	// LoginRateLimit allows 5 requests per minute (burst of 5).
	for i := range 5 {
		req := httptest.NewRequest(http.MethodPost, "/api/session", nil)
		req.RemoteAddr = "172.16.0.1:12345"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("login request %d: expected 200, got %d", i+1, rr.Code)
		}
	}

	// 6th request should be blocked.
	req := httptest.NewRequest(http.MethodPost, "/api/session", nil)
	req.RemoteAddr = "172.16.0.1:12345"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("6th login request: expected 429, got %d", rr.Code)
	}
}

func TestAPIRateLimit_AllowsManyRequests(t *testing.T) {
	mw := middleware.RateLimit(middleware.RateLimitConfig{Max: 100, Period: time.Minute})
	handler := mw(http.HandlerFunc(okHandler))

	// APIRateLimit allows 100 requests per minute (burst of 100).
	// First 50 should all succeed easily.
	for i := range 50 {
		req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
		req.RemoteAddr = "172.16.0.2:12345"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected status 200, got %d", i+1, rr.Code)
		}
	}
}

func TestRateLimit_HeadersAndKeys(t *testing.T) {
	handler := middleware.RateLimit(middleware.RateLimitConfig{Max: 2, Period: time.Minute})(http.HandlerFunc(okHandler))
	do := func(path, remote, xff string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = remote
		if xff != "" {
			req.Header.Set("X-Forwarded-For", xff)
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}
	want := func(rr *httptest.ResponseRecorder, code int, hdr map[string]string) {
		t.Helper()
		if rr.Code != code {
			t.Errorf("status = %d, want %d", rr.Code, code)
		}
		for k, v := range hdr {
			if got := rr.Header().Values(k); len(got) != 1 || got[0] != v {
				t.Errorf("%s = %q, want %q", k, got, v)
			}
		}
	}

	rr := do("/a", "10.9.0.1", "1.1.1.1, 2.2.2.2")
	want(rr, http.StatusOK, map[string]string{
		"RateLimit-Limit":     "0",
		"RateLimit-Reset":     "1",
		"RateLimit-Remaining": "1",
	})
	for k := range rr.Header() {
		if strings.HasPrefix(k, "X-Rate-Limit") {
			t.Errorf("unexpected header %s: request metadata must not be echoed", k)
		}
	}
	want(do("/a", "10.9.0.1:1", ""), http.StatusOK, map[string]string{"RateLimit-Remaining": "0"})
	want(do("/a", "10.9.0.1:2", ""), http.StatusTooManyRequests, map[string]string{"RateLimit-Remaining": "0", "Retry-After": "60"})
	want(do("/b", "10.9.0.1", ""), http.StatusOK, nil)

	do("/v6", "[2001:db8:1:2:aaaa::1]:5", "")
	do("/v6", "2001:db8:1:2:bbbb::2", "")
	want(do("/v6", "2001:db8:1:2:cccc::3", ""), http.StatusTooManyRequests, nil)

	for range 3 {
		rr := do("/a", "", "")
		want(rr, http.StatusOK, nil)
		if rr.Header().Get("RateLimit-Limit") != "" {
			t.Error("RateLimit-* headers must be absent when the client IP is unknown")
		}
	}
}

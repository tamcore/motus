package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	for i := 0; i < 5; i++ {
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
	for i := 0; i < 3; i++ {
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
	mw := middleware.LoginRateLimit()
	handler := mw(http.HandlerFunc(okHandler))

	// LoginRateLimit allows 5 requests per minute (burst of 5).
	for i := 0; i < 5; i++ {
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
	mw := middleware.APIRateLimit()
	handler := mw(http.HandlerFunc(okHandler))

	// APIRateLimit allows 100 requests per minute (burst of 100).
	// First 50 should all succeed easily.
	for i := 0; i < 50; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
		req.RemoteAddr = "172.16.0.2:12345"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected status 200, got %d", i+1, rr.Code)
		}
	}
}

func TestDefaultLoginRateLimit(t *testing.T) {
	cfg := middleware.DefaultLoginRateLimit()
	if cfg.Max != 5 {
		t.Errorf("expected Max 5, got %f", cfg.Max)
	}
	if cfg.Period != time.Minute {
		t.Errorf("expected Period 1m, got %v", cfg.Period)
	}
}

func TestDefaultAPIRateLimit(t *testing.T) {
	cfg := middleware.DefaultAPIRateLimit()
	if cfg.Max != 100 {
		t.Errorf("expected Max 100, got %f", cfg.Max)
	}
	if cfg.Period != time.Minute {
		t.Errorf("expected Period 1m, got %v", cfg.Period)
	}
}

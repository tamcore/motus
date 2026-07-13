package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tamcore/motus/internal/api/middleware"
)

func TestSecurityHeaders_AllHeadersPresent(t *testing.T) {
	handler := middleware.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	tests := []struct {
		header   string
		expected string
	}{
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "DENY"},
		{"Strict-Transport-Security", "max-age=31536000; includeSubDomains"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
	}

	for _, tc := range tests {
		got := rr.Header().Get(tc.header)
		if got != tc.expected {
			t.Errorf("header %s: expected %q, got %q", tc.header, tc.expected, got)
		}
	}
}

func TestSecurityHeaders_CSPPresent(t *testing.T) {
	handler := middleware.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	csp := rr.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("expected Content-Security-Policy header")
	}

	// Verify key CSP directives are present.
	directives := []string{
		"default-src 'self'",
		"script-src 'self'",
		"frame-ancestors 'none'",
		"connect-src 'self' wss:",
		"worker-src 'self'",
		"manifest-src 'self'",
		"img-src 'self' data: https://*.tile.openstreetmap.org https://*.basemaps.cartocdn.com",
	}
	for _, d := range directives {
		if !strings.Contains(csp, d) {
			t.Errorf("CSP missing directive %q; full CSP: %s", d, csp)
		}
	}
}

func TestSecurityHeaders_PermissionsPolicyPresent(t *testing.T) {
	handler := middleware.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	pp := rr.Header().Get("Permissions-Policy")
	if pp == "" {
		t.Fatal("expected Permissions-Policy header")
	}

	features := []string{"camera=()", "microphone=()", "geolocation=()", "payment=()"}
	for _, f := range features {
		if !strings.Contains(pp, f) {
			t.Errorf("Permissions-Policy missing %q; full: %s", f, pp)
		}
	}
}

func TestSecurityHeaders_DoesNotOverrideHandlerResponse(t *testing.T) {
	handler := middleware.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Status code should be preserved.
	if rr.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", rr.Code)
	}

	// Handler's Content-Type should be preserved.
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	// Security headers should also be present.
	if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("expected X-Content-Type-Options header")
	}
}

func TestSecurityHeaders_UnpkgNotInScriptOrStyleSrc(t *testing.T) {
	handler := middleware.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	csp := rr.Header().Get("Content-Security-Policy")

	scriptSrc := extractCSPDirective(csp, "script-src")
	styleSrc := extractCSPDirective(csp, "style-src")

	if strings.Contains(scriptSrc, "unpkg.com") {
		t.Errorf("script-src must not contain unpkg.com (Leaflet JS is bundled); got: %s", scriptSrc)
	}
	if strings.Contains(styleSrc, "unpkg.com") {
		t.Errorf("style-src must not contain unpkg.com (Leaflet CSS is bundled); got: %s", styleSrc)
	}

	// unpkg.com must NOT appear in img-src: Leaflet marker icons are now bundled locally.
	imgSrc := extractCSPDirective(csp, "img-src")
	if strings.Contains(imgSrc, "unpkg.com") {
		t.Errorf("img-src must not contain unpkg.com (marker icons are bundled); got: %s", imgSrc)
	}
}

// extractCSPDirective returns the value of a named directive from a CSP header string,
// or an empty string if the directive is not present.
func extractCSPDirective(csp, directive string) string {
	for part := range strings.SplitSeq(csp, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, directive+" ") || part == directive {
			return part
		}
	}
	return ""
}

func TestSecurityHeaders_CORPHeader(t *testing.T) {
	handler := middleware.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	corp := rr.Header().Get("Cross-Origin-Resource-Policy")
	if corp != "same-origin" {
		t.Errorf("expected Cross-Origin-Resource-Policy: same-origin, got %q", corp)
	}
}

func TestSecurityHeaders_CSPExplicitFallbackDirectives(t *testing.T) {
	handler := middleware.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	csp := rr.Header().Get("Content-Security-Policy")
	for _, d := range []string{"frame-src 'none'", "form-action 'self'"} {
		if !strings.Contains(csp, d) {
			t.Errorf("CSP missing explicit directive %q; full CSP: %s", d, csp)
		}
	}
}

func TestSecurityHeaders_AppliedToAllMethods(t *testing.T) {
	handler := middleware.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodHead,
	}

	for _, method := range methods {
		req := httptest.NewRequest(method, "/api/test", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("method %s: expected X-Content-Type-Options header", method)
		}
		if rr.Header().Get("X-Frame-Options") != "DENY" {
			t.Errorf("method %s: expected X-Frame-Options header", method)
		}
	}
}

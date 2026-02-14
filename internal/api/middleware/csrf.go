package middleware

import (
	"encoding/json"
	"net/http"
	"strings"

	gorillacsrf "github.com/gorilla/csrf"
)

// CSRFConfig holds configuration for CSRF protection.
type CSRFConfig struct {
	// Secret is the 32-byte CSRF authentication key.
	Secret []byte
	// Secure sets the Secure flag on the CSRF cookie (true for production).
	Secure bool
}

// CSRF returns middleware that applies CSRF protection to cookie-authenticated
// requests. Requests that carry a Bearer token in the Authorization header are
// exempt because they originate from API clients (Home Assistant, Traccar
// Manager) that use token-based auth and are not vulnerable to CSRF.
//
// Safe HTTP methods (GET, HEAD, OPTIONS, TRACE) are always exempt per the
// gorilla/csrf library behavior.
func CSRF(cfg CSRFConfig) func(http.Handler) http.Handler {
	protect := gorillacsrf.Protect(
		cfg.Secret,
		gorillacsrf.Secure(cfg.Secure),
		gorillacsrf.Path("/"),
		// Set MaxAge so the CSRF cookie persists across PWA restarts.
		// Without this, gorilla/csrf creates a session cookie that mobile
		// OSes clear when they kill the PWA process, causing CSRF failures
		// when the user reopens the app.
		gorillacsrf.MaxAge(365*24*3600),
		gorillacsrf.SameSite(gorillacsrf.SameSiteLaxMode),
		gorillacsrf.ErrorHandler(http.HandlerFunc(csrfErrorHandler)),
	)

	return func(next http.Handler) http.Handler {
		// Wrap `next` with a handler that injects the X-CSRF-Token header.
		// This runs after gorilla/csrf has stored the token in the context,
		// so gorillacsrf.Token(r) returns the correct value.
		tokenInjector := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-CSRF-Token", gorillacsrf.Token(r))
			next.ServeHTTP(w, r)
		})

		// Apply gorilla/csrf around the token injector.
		csrfProtected := protect(tokenInjector)

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Bearer token requests are exempt from CSRF; they come from
			// API clients, not browsers, and are not subject to CSRF attacks.
			if isBearerTokenRequest(r) {
				next.ServeHTTP(w, r)
				return
			}

			// When the CSRF cookie is not Secure (i.e. development over
			// plain HTTP), tell gorilla/csrf to use "http" when comparing
			// the Origin header against the request URL — but only when
			// the original client request was actually over HTTP. Behind a
			// TLS-terminating reverse proxy (e.g. nginx ingress), the
			// browser sends Origin: https://... while the pod sees HTTP.
			// Calling PlaintextHTTPRequest in that case causes a scheme
			// mismatch ("origin invalid"). Respect X-Forwarded-Proto to
			// detect this.
			if !cfg.Secure && r.Header.Get("X-Forwarded-Proto") != "https" {
				r = gorillacsrf.PlaintextHTTPRequest(r)
			}

			csrfProtected.ServeHTTP(w, r)
		})
	}
}

// isBearerTokenRequest returns true if the request carries a non-empty
// Authorization: Bearer token.
func isBearerTokenRequest(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	return token != ""
}

// ExemptLogin wraps a handler to exempt it from CSRF protection.
// Use for the login endpoint since users cannot obtain a CSRF token before authenticating.
func ExemptLogin(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = gorillacsrf.UnsafeSkipCheck(r)
		handler.ServeHTTP(w, r)
	})
}

// csrfErrorHandler writes a JSON 403 response for CSRF validation failures.
func csrfErrorHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	reason := gorillacsrf.FailureReason(r)
	msg := "CSRF token validation failed"
	if reason != nil {
		msg = "CSRF token validation failed: " + reason.Error()
	}
	resp, _ := json.Marshal(map[string]string{"error": msg})
	_, _ = w.Write(resp)
}

package middleware

import (
	"net/http"
	"strings"

	"github.com/tamcore/motus/internal/api"
)

// RequireWriteAccess returns middleware that blocks state-changing requests
// (POST, PUT, DELETE, PATCH) when the request was authenticated with a
// read-only API key. GET and HEAD requests are always allowed.
//
// This middleware must be applied after the Auth middleware so that the
// API key (if any) is available in the request context.
//
// The API key in context may come from a Bearer token header or from a
// session cookie linked to an API key (see Auth middleware). Sessions
// created via password login have no API key in context and are
// unrestricted.
//
// Session management routes (/api/session*) are exempt from restrictions
// so users can always logout and manage their own sessions.
func RequireWriteAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Exempt session management routes - users can always logout
		if isSessionRoute(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		apiKey := api.ApiKeyFromContext(r.Context())
		if apiKey != nil && apiKey.IsReadonly() {
			method := r.Method
			if method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions {
				api.RespondError(w, http.StatusForbidden, "this API key has read-only permissions")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// isSessionRoute returns true for session management endpoints that should
// be exempt from readonly restrictions.
func isSessionRoute(path string) bool {
	return path == "/api/session" ||
		path == "/api/session/token" ||
		path == "/api/sessions" ||
		strings.HasPrefix(path, "/api/sessions/")
}

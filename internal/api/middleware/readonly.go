package middleware

import (
	"net/http"
	"strings"

	"github.com/tamcore/motus/internal/api"
)

// RequireWriteAccess blocks state-changing requests made with a read-only API
// key, from a Bearer token or an API-key-linked session. It must run after
// Auth. Session routes stay exempt so users can always log out.
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
		path == "/api/sessions" ||
		strings.HasPrefix(path, "/api/sessions/") ||
		// Passkey ceremonies are self-service auth actions. They must be reachable
		// by demo users (who authenticate via a read-only API key) so they can
		// register and log in with a passkey. Registration is scoped to the
		// authenticated user, and passkey login for a demo account is forced onto
		// a read-only session, so this exemption does not grant write access.
		strings.HasPrefix(path, "/api/session/passkey/")
}

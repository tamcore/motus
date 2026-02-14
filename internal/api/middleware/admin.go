package middleware

import (
	"net/http"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/model"
)

// RequireAdmin returns middleware that restricts access to users with the
// admin role. It must be applied after the Auth middleware so that a user
// is already present in the request context.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := api.UserFromContext(r.Context())
		if user == nil || user.Role != model.RoleAdmin {
			api.RespondError(w, http.StatusForbidden, "admin access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

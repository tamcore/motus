package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/storage/repository"
)

// Auth returns middleware that authenticates requests via bearer token
// (API key), legacy user token, or session cookie.
//
// Bearer tokens are checked first against the api_keys table for multi-key
// support with permissions. If no match is found, the legacy users.token
// column is checked for backward compatibility. Session cookies are the
// fallback for browser clients.
//
// When authentication succeeds via an API key, both the user and the API key
// are stored in the request context. The API key context value is used by
// the RequireWriteAccess middleware to enforce read-only restrictions.
func Auth(users repository.UserRepo, sessions repository.SessionRepo, apiKeys repository.ApiKeyRepo) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Try bearer token first (Home Assistant, Traccar Manager, API clients).
			if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
				token := strings.TrimPrefix(authHeader, "Bearer ")
				if token != "" {
					// Check api_keys table first (new multi-key system).
					apiKey, err := apiKeys.GetByToken(r.Context(), token)
					if err == nil && apiKey != nil {
						// Reject expired API keys.
						if apiKey.IsExpired() {
							api.RespondError(w, http.StatusUnauthorized, "API key has expired")
							return
						}
						user, err := users.GetByID(r.Context(), apiKey.UserID)
						if err == nil && user != nil {
							ctx := api.ContextWithUser(r.Context(), user)
							ctx = api.ContextWithApiKey(ctx, apiKey)
							next.ServeHTTP(w, r.WithContext(ctx))

							// Update last_used_at asynchronously to avoid adding
							// latency to every API request.
							go apiKeys.UpdateLastUsed(context.Background(), apiKey.ID) //nolint:errcheck
							return
						}
					}

					// Fall back to legacy users.token column for backward
					// compatibility with existing integrations.
					user, err := users.GetByToken(r.Context(), token)
					if err == nil && user != nil {
						ctx := api.ContextWithUser(r.Context(), user)
						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}
				}
			}

			// Fall back to session cookie.
			cookie, err := r.Cookie("session_id")
			if err == nil && cookie.Value != "" {
				session, err := sessions.GetByID(r.Context(), cookie.Value)
				if err == nil && session != nil {
					user, err := users.GetByID(r.Context(), session.UserID)
					if err == nil && user != nil {
						ctx := api.ContextWithUser(r.Context(), user)

						// If this session was created from an API key token,
						// restore the key in context so RequireWriteAccess
						// enforces the original permission level.
						if session.ApiKeyID != nil && apiKeys != nil {
							apiKey, err := apiKeys.GetByID(r.Context(), *session.ApiKeyID)
							if err == nil && apiKey != nil {
								// Reject sessions whose originating API key has expired.
								if apiKey.IsExpired() {
									api.RespondError(w, http.StatusUnauthorized, "API key has expired")
									return
								}
								ctx = api.ContextWithApiKey(ctx, apiKey)
							}
							// If api_key_id is set but the key no longer exists
							// (edge case: race during ON DELETE CASCADE),
							// proceed without restrictions.
						}

						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}
				}
			}

			api.RespondError(w, http.StatusUnauthorized, "authentication required")
		})
	}
}

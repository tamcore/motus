package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/storage/repository"
)

// Auth returns middleware that authenticates requests via bearer token
// (API key), legacy user token, X-Auth-Token header, or session cookie.
// Unauthenticated requests receive a 401 response.
//
// Use LoadAuthContext when wrapping an ogen server — ogen's SecurityHandler
// handles per-operation auth enforcement, so the chi-level middleware only
// needs to populate context (not block).
func Auth(users repository.UserRepo, sessions repository.SessionRepo, apiKeys repository.ApiKeyRepo) func(http.Handler) http.Handler {
	return buildAuthMiddleware(users, sessions, apiKeys, true)
}

// LoadAuthContext returns middleware that loads auth context from the request
// when credentials are present but always passes through unauthenticated
// requests unchanged.
//
// Use this as RouterConfig.Auth so WriteAccess can inspect API key permissions
// before the ogen SecurityHandler runs. The SecurityHandler enforces auth
// requirements per-operation (e.g. /api/health needs no auth, /api/devices does).
func LoadAuthContext(users repository.UserRepo, sessions repository.SessionRepo, apiKeys repository.ApiKeyRepo) func(http.Handler) http.Handler {
	return buildAuthMiddleware(users, sessions, apiKeys, false)
}

// buildAuthMiddleware is the shared implementation. requireAuth=true returns 401
// for unauthenticated requests; requireAuth=false passes them through unchanged.
func buildAuthMiddleware(users repository.UserRepo, sessions repository.SessionRepo, apiKeys repository.ApiKeyRepo, requireAuth bool) func(http.Handler) http.Handler {
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

			// X-Auth-Token header: localStorage/IndexedDB-backed fallback for
			// iOS WebKit and Firefox-iOS PWA contexts that evict the session
			// cookie. The header value is a session ID, so the lookup is
			// identical to the cookie path below.
			if hdrToken := r.Header.Get("X-Auth-Token"); hdrToken != "" {
				if session, err := sessions.GetByID(r.Context(), hdrToken); err == nil && session != nil {
					if user, err := users.GetByID(r.Context(), session.UserID); err == nil && user != nil {
						ctx := api.ContextWithUser(r.Context(), user)
						if session.ApiKeyID != nil && apiKeys != nil {
							if apiKey, err := apiKeys.GetByID(r.Context(), *session.ApiKeyID); err == nil && apiKey != nil {
								if apiKey.IsExpired() {
									api.RespondError(w, http.StatusUnauthorized, "API key has expired")
									return
								}
								ctx = api.ContextWithApiKey(ctx, apiKey)
							}
						}
						next.ServeHTTP(w, r.WithContext(ctx))
						go sessions.UpdateLastSeen(context.Background(), session.ID, //nolint:errcheck
							audit.ExtractIP(r), r.Header.Get("User-Agent"))
						if session.RememberMe && time.Until(session.ExpiresAt) < 15*24*time.Hour {
							go sessions.UpdateExpiry(context.Background(), session.ID, //nolint:errcheck
								time.Now().Add(30*24*time.Hour))
						}
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
						go sessions.UpdateLastSeen(context.Background(), session.ID, //nolint:errcheck
							audit.ExtractIP(r), r.Header.Get("User-Agent"))
						if session.RememberMe && time.Until(session.ExpiresAt) < 15*24*time.Hour {
							go sessions.UpdateExpiry(context.Background(), session.ID, //nolint:errcheck
								time.Now().Add(30*24*time.Hour))
						}
						return
					}
				}
			}

			if requireAuth {
				api.RespondError(w, http.StatusUnauthorized, "authentication required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

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

// LoadAuthContext returns middleware that loads auth context from the request
// when credentials are present but always passes through unauthenticated
// requests unchanged.
//
// Use this as RouterConfig.Auth so WriteAccess can inspect API key permissions
// before the ogen SecurityHandler runs. The SecurityHandler enforces auth
// requirements per-operation (e.g. /api/health needs no auth, /api/devices does).
func LoadAuthContext(users repository.UserRepo, sessions repository.SessionRepo, apiKeys repository.ApiKeyRepo) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Try bearer token first (Home Assistant, Traccar Manager, API clients).
			if token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
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
							go apiKeys.UpdateLastUsed(context.WithoutCancel(r.Context()), apiKey.ID) //nolint:errcheck
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
			// cookie. The header value is a session ID, like the cookie.
			if trySession(w, r, next, users, sessions, apiKeys, r.Header.Get("X-Auth-Token")) {
				return
			}
			if cookie, err := r.Cookie("session_id"); err == nil {
				if trySession(w, r, next, users, sessions, apiKeys, cookie.Value) {
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// trySession authenticates and serves the request by session ID. It returns
// false when the session or its user cannot be resolved, so the caller can
// try the next credential source.
func trySession(w http.ResponseWriter, r *http.Request, next http.Handler, users repository.UserRepo, sessions repository.SessionRepo, apiKeys repository.ApiKeyRepo, sessionID string) bool {
	if sessionID == "" {
		return false
	}
	session, err := sessions.GetByID(r.Context(), sessionID)
	if err != nil || session == nil {
		return false
	}
	user, err := users.GetByID(r.Context(), session.UserID)
	if err != nil || user == nil {
		return false
	}
	ctx := api.ContextWithUser(r.Context(), user)

	// A session created from an API key token keeps that key's permission
	// level. A missing key (race during ON DELETE CASCADE) is ignored.
	if session.ApiKeyID != nil && apiKeys != nil {
		if apiKey, err := apiKeys.GetByID(r.Context(), *session.ApiKeyID); err == nil && apiKey != nil {
			if apiKey.IsExpired() {
				api.RespondError(w, http.StatusUnauthorized, "API key has expired")
				return true
			}
			ctx = api.ContextWithApiKey(ctx, apiKey)
		}
	}

	next.ServeHTTP(w, r.WithContext(ctx))
	go sessions.UpdateLastSeen(context.WithoutCancel(r.Context()), session.ID, //nolint:errcheck
		audit.ExtractIP(r), r.Header.Get("User-Agent"))
	if session.RememberMe && time.Until(session.ExpiresAt) < 15*24*time.Hour {
		go sessions.UpdateExpiry(context.WithoutCancel(r.Context()), session.ID, //nolint:errcheck
			time.Now().Add(30*24*time.Hour))
	}
	return true
}

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
			if user, apiKey := api.ResolveToken(r.Context(), users, apiKeys, bearerToken(r)); user != nil {
				if apiKey == nil {
					next.ServeHTTP(w, r.WithContext(api.ContextWithUser(r.Context(), user)))
					return
				}
				if apiKey.IsExpired() {
					api.RespondError(w, http.StatusUnauthorized, "API key has expired")
					return
				}
				next.ServeHTTP(w, r.WithContext(api.ContextWithApiKey(api.ContextWithUser(r.Context(), user), apiKey)))
				// Update last_used_at asynchronously to avoid adding
				// latency to every API request.
				go apiKeys.UpdateLastUsed(context.WithoutCancel(r.Context()), apiKey.ID) //nolint:errcheck
				return
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
	user, session, apiKey := api.ResolveSession(r.Context(), users, sessions, apiKeys, sessionID)
	if user == nil {
		return false
	}
	ctx := api.ContextWithUser(r.Context(), user)
	// A session created from an API key token keeps that key's permission level.
	if apiKey != nil {
		if apiKey.IsExpired() {
			api.RespondError(w, http.StatusUnauthorized, "API key has expired")
			return true
		}
		ctx = api.ContextWithApiKey(ctx, apiKey)
	}

	next.ServeHTTP(w, r.WithContext(ctx))
	go sessions.UpdateLastSeen(context.WithoutCancel(r.Context()), session.ID, //nolint:errcheck
		audit.ExtractIP(r), r.Header.Get("User-Agent"))
	if session.RememberMe && time.Until(session.ExpiresAt) < api.SessionExpiryRememberMe/2 {
		go sessions.UpdateExpiry(context.WithoutCancel(r.Context()), session.ID, //nolint:errcheck
			time.Now().Add(api.SessionExpiryRememberMe))
	}
	return true
}

// bearerToken returns the Authorization: Bearer token, or "" if absent.
func bearerToken(r *http.Request) string {
	if token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return token
	}
	return ""
}

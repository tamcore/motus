package handlers

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"golang.org/x/crypto/bcrypt"
)

const (
	// sessionExpiryDefault is the duration for standard login sessions (24 hours).
	sessionExpiryDefault = 24 * time.Hour

	// sessionExpiryRememberMe is the initial duration for "remember me" and
	// token-based login sessions (30 days). Active sessions are extended via
	// session rolling in the auth middleware before this window closes.
	sessionExpiryRememberMe = 30 * 24 * time.Hour
)

// isSecureEnvironment returns true when running in a production-like
// environment. In development (MOTUS_ENV=development) the Secure flag
// on cookies is omitted so that HTTP works on localhost.
func isSecureEnvironment() bool {
	return os.Getenv("MOTUS_ENV") != "development"
}

// ---------------------------------------------------------------------------
// Ogen *Handler session/auth methods
// ---------------------------------------------------------------------------

// Login authenticates a user with email and password.
// Accepts both JSON and form-encoded bodies (Traccar Manager posts
// x-www-form-urlencoded credentials). Returns the user object on success;
// sets a session_id cookie via the ResponseWriter stored in the context by
// the ogen middleware.
func (h *Handler) Login(ctx context.Context, req oas.LoginReq) (oas.LoginRes, error) {
	var creds oas.LoginRequest
	switch r := req.(type) {
	case *oas.LoginApplicationJSON:
		creds = oas.LoginRequest(*r)
	case *oas.LoginApplicationXWwwFormUrlencoded:
		creds = oas.LoginRequest(*r)
	default:
		return &oas.LoginUnauthorized{Error: "unsupported request body"}, nil
	}

	email := creds.Email
	password := creds.Password
	remember, _ := creds.Remember.Get()

	if email == "" || password == "" {
		return &oas.LoginUnauthorized{Error: "email and password required"}, nil
	}

	if h.loginLimiter.isLocked(email) {
		return &oas.LoginTooManyRequests{Error: "account temporarily locked due to too many failed login attempts"}, nil
	}

	user, err := h.cfg.Users.GetByEmail(ctx, email)
	if err != nil {
		h.loginLimiter.recordFailure(email)
		h.cfg.AuditLogger.Log(ctx, nil, audit.ActionSessionLoginFailed, audit.ResourceSession, nil,
			map[string]interface{}{"email": email, "reason": "unknown_email"}, "", "")
		return &oas.LoginUnauthorized{Error: "invalid credentials"}, nil
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		h.loginLimiter.recordFailure(email)
		h.cfg.AuditLogger.Log(ctx, &user.ID, audit.ActionSessionLoginFailed, audit.ResourceSession, nil,
			map[string]interface{}{"email": email, "reason": "wrong_password"}, "", "")
		return &oas.LoginUnauthorized{Error: "invalid credentials"}, nil
	}

	h.loginLimiter.reset(email)

	var expiresAt time.Time
	if remember {
		expiresAt = time.Now().Add(sessionExpiryRememberMe)
	} else {
		expiresAt = time.Now().Add(sessionExpiryDefault)
	}

	session, err := h.cfg.Sessions.CreateWithExpiry(ctx, user.ID, expiresAt, remember)
	if err != nil {
		return &oas.LoginUnauthorized{Error: "failed to create session"}, nil
	}

	if w := api.ResponseWriterFromContext(ctx); w != nil {
		http.SetCookie(w, &http.Cookie{
			Name:     "session_id",
			Value:    session.ID,
			Path:     "/",
			Expires:  session.ExpiresAt,
			MaxAge:   int(time.Until(session.ExpiresAt).Seconds()),
			HttpOnly: true,
			Secure:   isSecureEnvironment(),
			SameSite: http.SameSiteLaxMode,
		})
	}

	h.cfg.AuditLogger.Log(ctx, &user.ID, audit.ActionSessionLogin, audit.ResourceSession, nil,
		map[string]interface{}{"email": user.Email}, "", "")

	user.PopulateTraccarFields()
	out := userToOAS(user)
	return &out, nil
}

// Logout destroys the current session.
func (h *Handler) Logout(ctx context.Context) (oas.LogoutRes, error) {
	user := api.UserFromContext(ctx)
	if user != nil {
		h.cfg.AuditLogger.Log(ctx, &user.ID, audit.ActionSessionLogout, audit.ResourceSession, nil, nil, "", "")
	}

	session := api.SessionFromContext(ctx)
	if session != nil {
		_ = h.cfg.Sessions.Delete(ctx, session.ID)
	}

	if w := api.ResponseWriterFromContext(ctx); w != nil {
		http.SetCookie(w, &http.Cookie{
			Name:     "session_id",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			Expires:  time.Unix(0, 0),
			HttpOnly: true,
			Secure:   isSecureEnvironment(),
			SameSite: http.SameSiteLaxMode,
		})
	}

	return &oas.LogoutNoContent{}, nil
}

// LogoutAll revokes all sessions for the authenticated user except the current one.
func (h *Handler) LogoutAll(ctx context.Context) (oas.LogoutAllRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.Error{Error: "unauthorized"}, nil
	}

	session := api.SessionFromContext(ctx)
	if session == nil {
		return &oas.Error{Error: "could not determine current session"}, nil
	}

	if err := h.cfg.Sessions.DeleteAllByUser(ctx, user.ID, session.ID); err != nil {
		return &oas.Error{Error: "failed to revoke sessions"}, nil
	}

	h.cfg.AuditLogger.Log(ctx, &user.ID, audit.ActionSessionRevoke, audit.ResourceSession, nil,
		map[string]interface{}{"scope": "all_other_sessions"}, "", "")

	return &oas.LogoutAllNoContent{}, nil
}

// GetSession returns the currently authenticated user from the context.
func (h *Handler) GetSession(ctx context.Context, params oas.GetSessionParams) (oas.GetSessionRes, error) {
	// ?token= performs a token login (pytraccar / QR code / Traccar Manager /
	// Home Assistant): resolve the token, create a session, set the cookie.
	if token, ok := params.Token.Get(); ok && token != "" {
		return h.tokenLogin(ctx, token)
	}

	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.Error{Error: "unauthorized"}, nil
	}
	user.PopulateTraccarFields()
	out := userToOAS(user)
	return &out, nil
}

// tokenLogin authenticates via an API-key token (with legacy users.token
// fallback) and establishes a session cookie.
func (h *Handler) tokenLogin(ctx context.Context, token string) (oas.GetSessionRes, error) {
	user, apiKey := h.resolveLoginToken(ctx, token)
	if user == nil {
		return &oas.Error{Error: "invalid token"}, nil
	}

	// Token-based login uses the same expiry as "remember me" so the
	// session survives browser/app restarts.
	tokenExpiry := time.Now().Add(sessionExpiryRememberMe)

	// Link the session to the API key so the auth middleware can restore
	// the key's permission level on subsequent cookie requests.
	var session *model.Session
	var err error
	if apiKey != nil {
		session, err = h.cfg.Sessions.CreateWithApiKey(ctx, user.ID, apiKey.ID, tokenExpiry, true)
	} else {
		session, err = h.cfg.Sessions.CreateWithExpiry(ctx, user.ID, tokenExpiry, true)
	}
	if err != nil {
		return &oas.Error{Error: "failed to create session"}, nil
	}

	if w := api.ResponseWriterFromContext(ctx); w != nil {
		http.SetCookie(w, &http.Cookie{
			Name:     "session_id",
			Value:    session.ID,
			Path:     "/",
			Expires:  session.ExpiresAt,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   isSecureEnvironment(),
		})
	}

	if h.cfg.AuditLogger != nil {
		details := map[string]interface{}{"method": "token"}
		if apiKey != nil {
			details["apiKeyId"] = apiKey.ID
		}
		h.cfg.AuditLogger.Log(ctx, &user.ID, audit.ActionSessionLogin, audit.ResourceSession, nil, details, "", "")
	}

	user.PopulateTraccarFields()
	out := userToOAS(user)
	return &out, nil
}

// resolveLoginToken resolves a login token to a user: api_keys first
// (modern keys), then the legacy users.token column.
func (h *Handler) resolveLoginToken(ctx context.Context, token string) (*model.User, *model.ApiKey) {
	if h.cfg.ApiKeys != nil {
		apiKey, err := h.cfg.ApiKeys.GetByToken(ctx, token)
		if err == nil && apiKey != nil {
			user, err := h.cfg.Users.GetByID(ctx, apiKey.UserID)
			if err == nil && user != nil {
				return user, apiKey
			}
		}
	}

	user, err := h.cfg.Users.GetByToken(ctx, token)
	if err == nil && user != nil {
		return user, nil
	}

	return nil, nil
}

// ListSessions returns all active (non-sudo) sessions for the authenticated user.
func (h *Handler) ListSessions(ctx context.Context) (oas.ListSessionsRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.Error{Error: "unauthorized"}, nil
	}

	sessions, err := h.cfg.Sessions.ListByUser(ctx, user.ID)
	if err != nil {
		return &oas.Error{Error: "failed to list sessions"}, nil
	}

	currentSession := api.SessionFromContext(ctx)
	var currentID string
	if currentSession != nil {
		currentID = currentSession.ID
	}

	var result oas.ListSessionsOKApplicationJSON
	for _, s := range sessions {
		if s.IsSudo {
			continue
		}
		if s.ID == currentID {
			s.IsCurrent = true
		}
		result = append(result, sessionToOAS(s))
	}
	if result == nil {
		result = oas.ListSessionsOKApplicationJSON{}
	}
	return &result, nil
}

// DeleteSession revokes a session owned by the authenticated user.
// The session ID in params is the truncated display ID returned by ListSessions.
func (h *Handler) DeleteSession(ctx context.Context, params oas.DeleteSessionParams) (oas.DeleteSessionRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.DeleteSessionUnauthorized{Error: "unauthorized"}, nil
	}

	displayID := strings.TrimSuffix(params.ID, "…")
	if displayID == "" {
		return &oas.DeleteSessionNotFound{Error: "session ID is required"}, nil
	}

	target, err := h.cfg.Sessions.GetByIDPrefix(ctx, user.ID, displayID)
	if err != nil || target == nil {
		if user.IsAdmin() {
			target, err = h.cfg.Sessions.GetByID(ctx, displayID)
		}
		if err != nil || target == nil {
			return &oas.DeleteSessionNotFound{Error: "session not found"}, nil
		}
	}

	if target.UserID != user.ID && !user.IsAdmin() {
		return &oas.DeleteSessionForbidden{Error: "cannot revoke another user's session"}, nil
	}

	if err := h.cfg.Sessions.Delete(ctx, target.ID); err != nil {
		return &oas.DeleteSessionNotFound{Error: "failed to revoke session"}, nil
	}

	h.cfg.AuditLogger.Log(ctx, &user.ID, audit.ActionSessionRevoke, audit.ResourceSession, nil,
		map[string]interface{}{"revokedSessionId": target.TruncatedID(), "sessionOwnerUserId": target.UserID}, "", "")

	return &oas.DeleteSessionNoContent{}, nil
}

// GenerateToken creates a new API token for the authenticated user.
func (h *Handler) GenerateToken(ctx context.Context) (oas.GenerateTokenRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.Error{Error: "unauthorized"}, nil
	}

	if k := api.ApiKeyFromContext(ctx); k != nil && k.IsReadonly() {
		return &oas.Error{Error: "this API key has read-only permissions"}, nil
	}

	token, err := h.cfg.Users.GenerateToken(ctx, user.ID)
	if err != nil {
		return &oas.Error{Error: "failed to generate token"}, nil
	}

	return &oas.TokenResponse{Token: token}, nil
}

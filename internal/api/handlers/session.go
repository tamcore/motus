package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
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

// SessionHandler handles authentication endpoints.
type SessionHandler struct {
	users    repository.UserRepo
	sessions repository.SessionRepo
	apiKeys  repository.ApiKeyRepo
	audit    *audit.Logger
	limiter  *loginLimiter
}

// NewSessionHandler creates a new session handler.
func NewSessionHandler(users repository.UserRepo, sessions repository.SessionRepo, apiKeys repository.ApiKeyRepo) *SessionHandler {
	return &SessionHandler{users: users, sessions: sessions, apiKeys: apiKeys, limiter: newLoginLimiter()}
}

// SetAuditLogger configures audit logging for session events.
func (h *SessionHandler) SetAuditLogger(logger *audit.Logger) {
	h.audit = logger
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Remember bool   `json:"remember"`
}

// Login authenticates a user with email and password.
// Supports both JSON and form-encoded (Traccar Manager) requests.
// When remember=true, the session is effectively indefinite (10 years).
// POST /api/session
func (h *SessionHandler) Login(w http.ResponseWriter, r *http.Request) {
	var email, password string
	var remember bool

	contentType := r.Header.Get("Content-Type")

	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		// Limit form body to 1 MB to prevent memory exhaustion before ParseForm reads it.
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		// Parse form data (Traccar Manager compatibility).
		if err := r.ParseForm(); err != nil {
			api.RespondError(w, http.StatusBadRequest, "invalid form data")
			return
		}
		email = r.FormValue("email")
		password = r.FormValue("password")
		remember = r.FormValue("remember") == "true" || r.FormValue("remember") == "1"
	} else {
		// Parse JSON (default).
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.RespondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		email = req.Email
		password = req.Password
		remember = req.Remember
	}

	if email == "" || password == "" {
		api.RespondError(w, http.StatusBadRequest, "email and password required")
		return
	}

	if h.limiter.isLocked(email) {
		api.RespondError(w, http.StatusTooManyRequests, "account temporarily locked due to too many failed login attempts")
		return
	}

	user, err := h.users.GetByEmail(r.Context(), email)
	if err != nil {
		h.limiter.recordFailure(email)
		// Log failed login attempt for unknown email.
		if h.audit != nil {
			h.audit.LogFromRequest(r, nil, audit.ActionSessionLoginFailed, audit.ResourceSession, nil,
				map[string]interface{}{"email": email, "reason": "unknown_email"})
		}
		api.RespondError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		h.limiter.recordFailure(email)
		// Log failed login attempt for wrong password.
		if h.audit != nil {
			h.audit.LogFromRequest(r, &user.ID, audit.ActionSessionLoginFailed, audit.ResourceSession, nil,
				map[string]interface{}{"email": email, "reason": "wrong_password"})
		}
		api.RespondError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	h.limiter.reset(email)

	// Create session with appropriate expiry.
	var expiresAt time.Time
	if remember {
		expiresAt = time.Now().Add(sessionExpiryRememberMe)
	} else {
		expiresAt = time.Now().Add(sessionExpiryDefault)
	}

	session, err := h.sessions.CreateWithExpiry(r.Context(), user.ID, expiresAt, remember)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	_ = h.sessions.UpdateLastSeen(r.Context(), session.ID, audit.ExtractIP(r), r.Header.Get("User-Agent"))

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

	// Audit log the successful login.
	if h.audit != nil {
		h.audit.LogFromRequest(r, &user.ID, audit.ActionSessionLogin, audit.ResourceSession, nil,
			map[string]interface{}{"email": user.Email})
	}

	user.PopulateTraccarFields()

	// For "remember me" logins we additionally return the session ID in the
	// response body. iOS WebKit / Firefox-iOS PWA contexts evict the
	// session cookie across app restarts, so the frontend stores this token
	// in localStorage and replays it via the X-Auth-Token header when the
	// cookie is gone.
	resp := struct {
		*model.User
		AuthToken string `json:"authToken,omitempty"`
	}{User: user}
	if remember {
		resp.AuthToken = session.ID
	}
	api.RespondJSON(w, http.StatusOK, resp)
}

// GetCurrentSession returns the currently authenticated user.
// Supports two authentication modes:
// 1. Standard: Authenticated via middleware (cookie or bearer token)
// 2. Traccar compatibility: ?token= query parameter (for pytraccar)
// GET /api/session
func (h *SessionHandler) GetCurrentSession(w http.ResponseWriter, r *http.Request) {
	// Check for token query parameter first (pytraccar / QR code / Home Assistant).
	tokenParam := r.URL.Query().Get("token")
	if tokenParam != "" {
		user, apiKey := h.resolveToken(r.Context(), tokenParam)
		if user == nil {
			api.RespondError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		// Token-based login uses the same expiry as "remember me"
		// so the session survives browser/app restarts.
		tokenExpiry := time.Now().Add(sessionExpiryRememberMe)

		// Create session linked to the API key so the auth middleware can
		// restore the key's permission level on subsequent cookie requests.
		var session *model.Session
		var err error
		if apiKey != nil {
			session, err = h.sessions.CreateWithApiKey(r.Context(), user.ID, apiKey.ID, tokenExpiry, true)
		} else {
			session, err = h.sessions.CreateWithExpiry(r.Context(), user.ID, tokenExpiry, true)
		}
		if err != nil {
			api.RespondError(w, http.StatusInternalServerError, "failed to create session")
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "session_id",
			Value:    session.ID,
			Path:     "/",
			Expires:  session.ExpiresAt,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   isSecureEnvironment(),
		})

		// Audit log the token-based login.
		if h.audit != nil {
			details := map[string]interface{}{"method": "token"}
			if apiKey != nil {
				details["apiKeyId"] = apiKey.ID
			}
			h.audit.LogFromRequest(r, &user.ID, audit.ActionSessionLogin, audit.ResourceSession, nil, details)
		}

		user.PopulateTraccarFields()
		api.RespondJSON(w, http.StatusOK, user)
		return
	}

	// This route is public (no auth middleware) to support the ?token= path
	// above, so api.UserFromContext will always be nil. Instead, manually
	// validate either the session cookie or the X-Auth-Token header
	// (localStorage/IDB-backed fallback for iOS PWA contexts where the
	// cookie gets evicted).
	var sessionID string
	if cookie, err := r.Cookie("session_id"); err == nil && cookie.Value != "" {
		sessionID = cookie.Value
	} else if hdr := r.Header.Get("X-Auth-Token"); hdr != "" {
		sessionID = hdr
	}
	if sessionID == "" {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	session, err := h.sessions.GetByID(r.Context(), sessionID)
	if err != nil || session == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	user, err := h.users.GetByID(r.Context(), session.UserID)
	if err != nil || user == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	user.PopulateTraccarFields()
	api.RespondJSON(w, http.StatusOK, user)
}

// Logout destroys the current session.
// DELETE /api/session
func (h *SessionHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Audit log the logout before deleting the session.
	if h.audit != nil {
		user := api.UserFromContext(r.Context())
		if user != nil {
			h.audit.LogFromRequest(r, &user.ID, audit.ActionSessionLogout, audit.ResourceSession, nil, nil)
		}
	}

	cookie, err := r.Cookie("session_id")
	if err == nil {
		_ = h.sessions.Delete(r.Context(), cookie.Value)
	}

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

	w.WriteHeader(http.StatusNoContent)
}

// LogoutAll revokes all sessions for the authenticated user except the current one.
// DELETE /api/sessions
func (h *SessionHandler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	if user == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var currentSessionID string
	if cookie, err := r.Cookie("session_id"); err == nil {
		currentSessionID = cookie.Value
	} else if hdr := r.Header.Get("X-Auth-Token"); hdr != "" {
		currentSessionID = hdr
	}
	if currentSessionID == "" {
		api.RespondError(w, http.StatusBadRequest, "could not determine current session")
		return
	}

	if err := h.sessions.DeleteAllByUser(r.Context(), user.ID, currentSessionID); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to revoke sessions")
		return
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &user.ID, audit.ActionSessionRevoke, audit.ResourceSession, nil,
			map[string]interface{}{"scope": "all_other_sessions"})
	}

	w.WriteHeader(http.StatusNoContent)
}

// GenerateToken creates a new API token for the authenticated user.
// POST /api/session/token
func (h *SessionHandler) GenerateToken(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	if user == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	if k := api.ApiKeyFromContext(r.Context()); k != nil && k.IsReadonly() {
		api.RespondError(w, http.StatusForbidden, "this API key has read-only permissions")
		return
	}

	token, err := h.users.GenerateToken(r.Context(), user.ID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	api.RespondJSON(w, http.StatusOK, map[string]string{"token": token})
}

// ListSessions returns all active (non-sudo) sessions for the authenticated user.
// GET /api/sessions
func (h *SessionHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	if user == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	sessions, err := h.sessions.ListByUser(r.Context(), user.ID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}

	// Read current session ID from cookie.
	var currentSessionID string
	if cookie, err := r.Cookie("session_id"); err == nil {
		currentSessionID = cookie.Value
	}

	// Filter out sudo sessions and mark the current session.
	var result []*model.Session
	for _, s := range sessions {
		if s.IsSudo {
			continue
		}
		if s.ID == currentSessionID {
			s.IsCurrent = true
		}
		result = append(result, s)
	}

	if result == nil {
		result = []*model.Session{}
	}

	api.RespondJSON(w, http.StatusOK, result)
}

// DeleteSession revokes a session owned by the authenticated user.
// The session ID in the URL is the truncated display ID returned by ListSessions.
// DELETE /api/sessions/{id}
func (h *SessionHandler) DeleteSession(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	if user == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Strip the trailing ellipsis that MarshalJSON appends for display purposes.
	displayID := strings.TrimSuffix(chi.URLParam(r, "id"), "…")
	if displayID == "" {
		api.RespondError(w, http.StatusBadRequest, "session ID is required")
		return
	}

	// The API exposes truncated session IDs. Look up the full session by
	// prefix, scoped to the authenticated user (or any user for admins).
	target, err := h.sessions.GetByIDPrefix(r.Context(), user.ID, displayID)
	if err != nil || target == nil {
		// Admin: try any user's session.
		if user.IsAdmin() {
			// Fall back to exact match for admin cross-user revocation.
			target, err = h.sessions.GetByID(r.Context(), displayID)
		}
		if err != nil || target == nil {
			api.RespondError(w, http.StatusNotFound, "session not found")
			return
		}
	}

	if target.UserID != user.ID && !user.IsAdmin() {
		api.RespondError(w, http.StatusForbidden, "cannot revoke another user's session")
		return
	}

	if err := h.sessions.Delete(r.Context(), target.ID); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to revoke session")
		return
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &user.ID, audit.ActionSessionRevoke, audit.ResourceSession, nil,
			map[string]interface{}{"revokedSessionId": target.TruncatedID(), "sessionOwnerUserId": target.UserID})
	}

	w.WriteHeader(http.StatusNoContent)
}

// AdminDeleteSession revokes a specific session for a given user (admin only).
// DELETE /api/users/{id}/sessions/{sessionId}
func (h *SessionHandler) AdminDeleteSession(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	if user == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	userIDStr := chi.URLParam(r, "id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid user ID")
		return
	}

	sessionID := chi.URLParam(r, "sessionId")
	if sessionID == "" {
		api.RespondError(w, http.StatusBadRequest, "session ID is required")
		return
	}

	// Verify the session exists and belongs to the target user.
	target, err := h.sessions.GetByID(r.Context(), sessionID)
	if err != nil || target == nil {
		api.RespondError(w, http.StatusNotFound, "session not found")
		return
	}

	if target.UserID != userID {
		api.RespondError(w, http.StatusNotFound, "session not found for this user")
		return
	}

	if err := h.sessions.Delete(r.Context(), sessionID); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to revoke session")
		return
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &user.ID, audit.ActionSessionRevoke, audit.ResourceSession, nil,
			map[string]interface{}{"revokedSessionId": sessionID, "sessionOwnerUserId": userID})
	}

	w.WriteHeader(http.StatusNoContent)
}

// resolveToken looks up a user by token, checking the api_keys table first
// (modern API keys from QR codes and the key management UI) and falling back
// to the legacy users.token column (pytraccar compatibility).
//
// When the token matches an API key, the key is returned alongside the user
// so the caller can link the session to the key's permission level.
func (h *SessionHandler) resolveToken(ctx context.Context, token string) (*model.User, *model.ApiKey) {
	// Try api_keys table first (modern keys).
	if h.apiKeys != nil {
		apiKey, err := h.apiKeys.GetByToken(ctx, token)
		if err == nil && apiKey != nil {
			user, err := h.users.GetByID(ctx, apiKey.UserID)
			if err == nil && user != nil {
				return user, apiKey
			}
		}
	}

	// Fall back to legacy users.token column (no API key to link).
	user, err := h.users.GetByToken(ctx, token)
	if err == nil && user != nil {
		return user, nil
	}

	return nil, nil
}

// ---------------------------------------------------------------------------
// Ogen *Handler session/auth methods
// ---------------------------------------------------------------------------

// Login authenticates a user with email and password.
// Returns the user object on success; sets a session_id cookie via the
// ResponseWriter stored in the context by the ogen middleware.
func (h *Handler) Login(ctx context.Context, req *oas.LoginRequest) (oas.LoginRes, error) {
	email := req.Email
	password := req.Password
	remember, _ := req.Remember.Get()

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
func (h *Handler) GetSession(ctx context.Context) (oas.GetSessionRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.Error{Error: "unauthorized"}, nil
	}
	user.PopulateTraccarFields()
	out := userToOAS(user)
	return &out, nil
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

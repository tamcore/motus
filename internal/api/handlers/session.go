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
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"golang.org/x/crypto/bcrypt"
)

const (
	// sessionExpiryDefault is the duration for standard login sessions (24 hours).
	sessionExpiryDefault = 24 * time.Hour

	// sessionExpiryRememberMe is the duration for "remember me" and token-based
	// login sessions (10 years - effectively indefinite).
	sessionExpiryRememberMe = 10 * 365 * 24 * time.Hour
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
}

// NewSessionHandler creates a new session handler.
func NewSessionHandler(users repository.UserRepo, sessions repository.SessionRepo, apiKeys repository.ApiKeyRepo) *SessionHandler {
	return &SessionHandler{users: users, sessions: sessions, apiKeys: apiKeys}
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

	user, err := h.users.GetByEmail(r.Context(), email)
	if err != nil {
		// Log failed login attempt for unknown email.
		if h.audit != nil {
			h.audit.LogFromRequest(r, nil, audit.ActionSessionLoginFailed, audit.ResourceSession, nil,
				map[string]interface{}{"email": email, "reason": "unknown_email"})
		}
		api.RespondError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		// Log failed login attempt for wrong password.
		if h.audit != nil {
			h.audit.LogFromRequest(r, &user.ID, audit.ActionSessionLoginFailed, audit.ResourceSession, nil,
				map[string]interface{}{"email": email, "reason": "wrong_password"})
		}
		api.RespondError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

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

	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    session.ID,
		Path:     "/",
		Expires:  session.ExpiresAt,
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
	api.RespondJSON(w, http.StatusOK, user)
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
	// validate the session cookie to restore the authenticated user.
	cookie, err := r.Cookie("session_id")
	if err != nil || cookie.Value == "" {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	session, err := h.sessions.GetByID(r.Context(), cookie.Value)
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

// GenerateToken creates a new API token for the authenticated user.
// POST /api/session/token
func (h *SessionHandler) GenerateToken(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	if user == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
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

	displayID := chi.URLParam(r, "id")
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

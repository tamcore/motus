package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/storage/repository"
)

// SudoHandler handles admin impersonation (sudo) endpoints.
type SudoHandler struct {
	users    repository.UserRepo
	sessions repository.SessionRepo
	audit    *audit.Logger
}

// NewSudoHandler creates a new sudo handler.
func NewSudoHandler(users repository.UserRepo, sessions repository.SessionRepo) *SudoHandler {
	return &SudoHandler{users: users, sessions: sessions}
}

// SetAuditLogger configures audit logging for sudo events.
func (h *SudoHandler) SetAuditLogger(logger *audit.Logger) {
	h.audit = logger
}

// StartSudo creates a sudo session allowing the admin to act as the target user.
// POST /api/admin/sudo/{id}
func (h *SudoHandler) StartSudo(w http.ResponseWriter, r *http.Request) {
	currentUser := api.UserFromContext(r.Context())
	if currentUser == nil || !currentUser.IsAdmin() {
		api.RespondError(w, http.StatusForbidden, "admin access required")
		return
	}

	targetUserID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	// Cannot sudo as yourself.
	if targetUserID == currentUser.ID {
		api.RespondError(w, http.StatusBadRequest, "cannot impersonate yourself")
		return
	}

	// Verify target user exists.
	targetUser, err := h.users.GetByID(r.Context(), targetUserID)
	if err != nil {
		api.RespondError(w, http.StatusNotFound, "user not found")
		return
	}

	// Create sudo session.
	session, err := h.sessions.CreateSudo(r.Context(), targetUser.ID, currentUser.ID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to create sudo session")
		return
	}

	// Set the sudo session cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    session.ID,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		Secure:   isSecureEnvironment(),
		SameSite: http.SameSiteLaxMode,
	})

	// Audit log the sudo action.
	if h.audit != nil {
		h.audit.LogFromRequest(r, &currentUser.ID, audit.ActionSessionSudo, audit.ResourceUser, &targetUser.ID,
			map[string]interface{}{
				"targetEmail": targetUser.Email,
				"adminEmail":  currentUser.Email,
			})
	}

	targetUser.PopulateTraccarFields()
	api.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message":        "sudo session started",
		"user":           targetUser,
		"isSudo":         true,
		"originalUserId": currentUser.ID,
		"expiresAt":      session.ExpiresAt,
	})
}

// EndSudo restores the original admin session.
// DELETE /api/admin/sudo
func (h *SudoHandler) EndSudo(w http.ResponseWriter, r *http.Request) {
	currentUser := api.UserFromContext(r.Context())
	if currentUser == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Look up the current session to get the original user ID.
	cookie, err := r.Cookie("session_id")
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "no session found")
		return
	}

	session, err := h.sessions.GetByID(r.Context(), cookie.Value)
	if err != nil || session == nil {
		api.RespondError(w, http.StatusBadRequest, "invalid session")
		return
	}

	if !session.IsSudo || session.OriginalUserID == nil {
		api.RespondError(w, http.StatusBadRequest, "not in a sudo session")
		return
	}

	// Delete the sudo session.
	_ = h.sessions.Delete(r.Context(), session.ID)

	// Restore original admin session.
	originalUser, err := h.users.GetByID(r.Context(), *session.OriginalUserID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to restore original user")
		return
	}

	// Create a new session for the original admin.
	newSession, err := h.sessions.CreateWithExpiry(
		r.Context(),
		originalUser.ID,
		time.Now().Add(24*time.Hour),
		false,
	)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    newSession.ID,
		Path:     "/",
		Expires:  newSession.ExpiresAt,
		HttpOnly: true,
		Secure:   isSecureEnvironment(),
		SameSite: http.SameSiteLaxMode,
	})

	// Audit log the end of sudo.
	if h.audit != nil {
		h.audit.LogFromRequest(r, &originalUser.ID, audit.ActionSessionSudoEnd, audit.ResourceUser, &currentUser.ID,
			map[string]interface{}{
				"adminEmail":  originalUser.Email,
				"targetEmail": currentUser.Email,
			})
	}

	originalUser.PopulateTraccarFields()
	api.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "sudo session ended",
		"user":    originalUser,
	})
}

// GetSudoStatus returns the current sudo session status.
// GET /api/admin/sudo
func (h *SudoHandler) GetSudoStatus(w http.ResponseWriter, r *http.Request) {
	currentUser := api.UserFromContext(r.Context())
	if currentUser == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	cookie, err := r.Cookie("session_id")
	if err != nil {
		api.RespondJSON(w, http.StatusOK, map[string]interface{}{"isSudo": false})
		return
	}

	session, err := h.sessions.GetByID(r.Context(), cookie.Value)
	if err != nil || session == nil || !session.IsSudo {
		api.RespondJSON(w, http.StatusOK, map[string]interface{}{"isSudo": false})
		return
	}

	response := map[string]interface{}{
		"isSudo":    true,
		"expiresAt": session.ExpiresAt,
	}

	if session.OriginalUserID != nil {
		originalUser, err := h.users.GetByID(r.Context(), *session.OriginalUserID)
		if err == nil {
			originalUser.PopulateTraccarFields()
			response["originalUser"] = originalUser
		}
	}

	api.RespondJSON(w, http.StatusOK, response)
}

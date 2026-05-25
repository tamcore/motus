package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
)

// AdminStartSudo creates a sudo session allowing the admin to act as the target user.
// POST /api/admin/sudo/{id}
func (h *Handler) AdminStartSudo(ctx context.Context, params oas.AdminStartSudoParams) (oas.AdminStartSudoRes, error) {
	currentUser, err := requireAdminCtx(ctx)
	if err != nil {
		return &oas.AdminStartSudoForbidden{Error: "admin access required"}, nil
	}

	if params.ID == currentUser.ID {
		return &oas.AdminStartSudoForbidden{Error: "cannot impersonate yourself"}, nil
	}

	targetUser, err := h.cfg.Users.GetByID(ctx, params.ID)
	if err != nil {
		return &oas.AdminStartSudoNotFound{Error: "user not found"}, nil
	}

	session, err := h.cfg.Sessions.CreateSudo(ctx, targetUser.ID, currentUser.ID)
	if err != nil {
		return &oas.AdminStartSudoForbidden{Error: "failed to create sudo session"}, nil
	}

	if w := api.ResponseWriterFromContext(ctx); w != nil {
		http.SetCookie(w, &http.Cookie{
			Name:     "session_id",
			Value:    session.ID,
			Path:     "/",
			Expires:  session.ExpiresAt,
			HttpOnly: true,
			Secure:   isSecureEnvironment(),
			SameSite: http.SameSiteLaxMode,
		})
	}

	if h.cfg.AuditLogger != nil {
		h.cfg.AuditLogger.Log(ctx, &currentUser.ID, audit.ActionSessionSudo, audit.ResourceUser, &targetUser.ID,
			map[string]interface{}{
				"targetEmail": targetUser.Email,
				"adminEmail":  currentUser.Email,
			}, "", "")
	}

	return &oas.AdminStartSudoNoContent{}, nil
}

// EndSudo restores the original admin session.
// DELETE /api/admin/sudo
func (h *Handler) EndSudo(ctx context.Context) (oas.EndSudoRes, error) {
	currentUser := api.UserFromContext(ctx)
	if currentUser == nil {
		return &oas.Error{Error: "not authenticated"}, nil
	}

	session := api.SessionFromContext(ctx)
	if session == nil {
		return &oas.Error{Error: "no session found"}, nil
	}

	if !session.IsSudo || session.OriginalUserID == nil {
		return &oas.Error{Error: "not in a sudo session"}, nil
	}

	_ = h.cfg.Sessions.Delete(ctx, session.ID)

	originalUser, err := h.cfg.Users.GetByID(ctx, *session.OriginalUserID)
	if err != nil {
		return &oas.Error{Error: "failed to restore original user"}, nil
	}

	newSession, err := h.cfg.Sessions.CreateWithExpiry(
		ctx,
		originalUser.ID,
		time.Now().Add(sessionExpiryDefault),
		false,
	)
	if err != nil {
		return &oas.Error{Error: "failed to create session"}, nil
	}

	if w := api.ResponseWriterFromContext(ctx); w != nil {
		http.SetCookie(w, &http.Cookie{
			Name:     "session_id",
			Value:    newSession.ID,
			Path:     "/",
			Expires:  newSession.ExpiresAt,
			HttpOnly: true,
			Secure:   isSecureEnvironment(),
			SameSite: http.SameSiteLaxMode,
		})
	}

	if h.cfg.AuditLogger != nil {
		h.cfg.AuditLogger.Log(ctx, &originalUser.ID, audit.ActionSessionSudoEnd, audit.ResourceUser, &currentUser.ID,
			map[string]interface{}{
				"adminEmail":  originalUser.Email,
				"targetEmail": currentUser.Email,
			}, "", "")
	}

	return &oas.EndSudoNoContent{}, nil
}

// GetSudoStatus returns the current sudo session status.
// GET /api/admin/sudo
func (h *Handler) GetSudoStatus(ctx context.Context) (oas.GetSudoStatusRes, error) {
	if api.UserFromContext(ctx) == nil {
		return &oas.Error{Error: "not authenticated"}, nil
	}

	session := api.SessionFromContext(ctx)
	if session == nil || !session.IsSudo {
		return &oas.SudoStatus{Active: false}, nil
	}

	status := &oas.SudoStatus{
		Active:       true,
		TargetUserId: ptrToOptInt64(nil),
	}

	if session.OriginalUserID != nil {
		status.OriginalUserId = oas.OptNilInt64{Value: *session.OriginalUserID, Set: true}
		status.TargetUserId = oas.OptNilInt64{Value: session.UserID, Set: true}
	}

	return status, nil
}

package handlers

import (
	"context"

	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/demo"
	"github.com/tamcore/motus/internal/validation"
	"golang.org/x/crypto/bcrypt"
)

// UpdateProfile allows the authenticated user to update their own name, email, and password.
// PUT /api/profile
func (h *Handler) UpdateProfile(ctx context.Context, req *oas.UpdateProfileRequest) (oas.UpdateProfileRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.UpdateProfileUnauthorized{Error: "not authenticated"}, nil
	}

	if demo.IsEnabled() && demo.IsDemoAccount(user.Email) {
		return &oas.UpdateProfileBadRequest{Error: "demo accounts cannot be modified"}, nil
	}

	existing, err := h.cfg.Users.GetByID(ctx, user.ID)
	if err != nil {
		return &oas.UpdateProfileBadRequest{Error: "failed to fetch user"}, nil
	}

	changes := map[string]interface{}{}

	if email, ok := req.Email.Get(); ok && email != "" && email != existing.Email {
		if err := validation.ValidateEmail(email); err != nil {
			return &oas.UpdateProfileBadRequest{Error: err.Error()}, nil
		}
		changes["oldEmail"] = existing.Email
		changes["newEmail"] = email
		existing.Email = email
	}
	if name, ok := req.Name.Get(); ok && name != "" && name != existing.Name {
		if err := validation.ValidateName(name); err != nil {
			return &oas.UpdateProfileBadRequest{Error: err.Error()}, nil
		}
		changes["oldName"] = existing.Name
		changes["newName"] = name
		existing.Name = name
	}

	if len(changes) > 0 {
		if err := h.cfg.Users.Update(ctx, existing); err != nil {
			return &oas.UpdateProfileBadRequest{Error: "failed to update profile"}, nil
		}
	}

	if pw, ok := req.Password.Get(); ok && pw != "" {
		currentPw, ok := req.CurrentPassword.Get()
		if !ok || currentPw == "" {
			return &oas.UpdateProfileBadRequest{Error: "current password is required to set a new password"}, nil
		}
		if err := bcrypt.CompareHashAndPassword([]byte(existing.PasswordHash), []byte(currentPw)); err != nil {
			return &oas.UpdateProfileBadRequest{Error: "current password is incorrect"}, nil
		}
		if err := validation.ValidatePassword(pw); err != nil {
			return &oas.UpdateProfileBadRequest{Error: err.Error()}, nil
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
		if err != nil {
			return &oas.UpdateProfileBadRequest{Error: "failed to hash password"}, nil
		}
		if err := h.cfg.Users.UpdatePassword(ctx, existing.ID, string(hash)); err != nil {
			return &oas.UpdateProfileBadRequest{Error: "failed to update password"}, nil
		}
		changes["passwordChanged"] = true

		// Revoke every other session so a stolen session does not survive a
		// password rotation. Keep the current session when known; with
		// API-key auth there is no session in context, so revoke all.
		exceptID := ""
		if session := api.SessionFromContext(ctx); session != nil {
			exceptID = session.ID
		}
		if err := h.cfg.Sessions.DeleteAllByUser(ctx, existing.ID, exceptID); err != nil {
			return &oas.UpdateProfileBadRequest{Error: "failed to revoke sessions"}, nil
		}
		changes["sessionsRevoked"] = true
	}

	if h.cfg.AuditLogger != nil && len(changes) > 0 {
		h.cfg.AuditLogger.Log(ctx, &existing.ID, audit.ActionUserUpdate, audit.ResourceUser, &existing.ID, changes, "", "")
	}

	existing.PopulateTraccarFields()
	result := userToOAS(existing)
	return &result, nil
}

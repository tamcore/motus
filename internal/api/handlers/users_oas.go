package handlers

import (
	"context"

	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/demo"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/validation"
	"golang.org/x/crypto/bcrypt"
)

// AdminListUsers returns all users in the system.
// GET /api/admin/users
func (h *Handler) AdminListUsers(ctx context.Context) (oas.AdminListUsersRes, error) {
	if _, err := requireAdminCtx(ctx); err != nil {
		return &oas.AdminListUsersForbidden{Error: "admin access required"}, nil
	}

	users, err := h.cfg.Users.ListAll(ctx)
	if err != nil {
		return &oas.AdminListUsersForbidden{Error: "failed to list users"}, nil
	}

	result := make(oas.AdminListUsersOKApplicationJSON, 0, len(users))
	for _, u := range users {
		u.PopulateTraccarFields()
		result = append(result, userToOAS(u))
	}
	return &result, nil
}

// AdminCreateUser adds a new user.
// POST /api/admin/users
func (h *Handler) AdminCreateUser(ctx context.Context, req *oas.UserInput) (oas.AdminCreateUserRes, error) {
	admin, err := requireAdminCtx(ctx)
	if err != nil {
		return &oas.AdminCreateUserForbidden{Error: "admin access required"}, nil
	}

	if err := validation.ValidateEmail(req.Email); err != nil {
		return &oas.AdminCreateUserBadRequest{Error: err.Error()}, nil
	}

	if req.Name != "" {
		if err := validation.ValidateName(req.Name); err != nil {
			return &oas.AdminCreateUserBadRequest{Error: err.Error()}, nil
		}
	}

	role := model.RoleUser
	if r, ok := req.Role.Get(); ok {
		role = string(r)
	}
	if !model.IsValidRole(role) {
		return &oas.AdminCreateUserBadRequest{Error: "invalid role: must be admin, user, or readonly"}, nil
	}

	var passwordHash string
	if pw, ok := req.Password.Get(); ok && pw != "" {
		if err := validation.ValidatePassword(pw); err != nil {
			return &oas.AdminCreateUserBadRequest{Error: err.Error()}, nil
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
		if err != nil {
			return &oas.AdminCreateUserForbidden{Error: "failed to hash password"}, nil
		}
		passwordHash = string(hash)
	}

	user := &model.User{
		Email:        req.Email,
		PasswordHash: passwordHash,
		Name:         req.Name,
		Role:         role,
	}
	if err := h.cfg.Users.Create(ctx, user); err != nil {
		return &oas.AdminCreateUserForbidden{Error: "failed to create user"}, nil
	}

	if h.cfg.AuditLogger != nil {
		h.cfg.AuditLogger.Log(ctx, &admin.ID, audit.ActionUserCreate, audit.ResourceUser, &user.ID,
			map[string]interface{}{"email": user.Email, "role": user.Role}, "", "")
	}

	user.PopulateTraccarFields()
	result := userToOAS(user)
	return &result, nil
}

// AdminUpdateUser modifies an existing user.
// PUT /api/admin/users/{id}
func (h *Handler) AdminUpdateUser(ctx context.Context, req *oas.UserInput, params oas.AdminUpdateUserParams) (oas.AdminUpdateUserRes, error) {
	admin, err := requireAdminCtx(ctx)
	if err != nil {
		return &oas.AdminUpdateUserForbidden{Error: "admin access required"}, nil
	}

	existing, err := h.cfg.Users.GetByID(ctx, params.ID)
	if err != nil {
		return &oas.AdminUpdateUserNotFound{Error: "user not found"}, nil
	}

	if demo.IsEnabled() && demo.IsDemoAccount(existing.Email) {
		return &oas.AdminUpdateUserForbidden{Error: "demo accounts cannot be modified"}, nil
	}

	if existing.ID == admin.ID {
		if r, ok := req.Role.Get(); ok && string(r) != model.RoleAdmin {
			return &oas.AdminUpdateUserBadRequest{Error: "cannot change your own role"}, nil
		}
	}

	changes := map[string]interface{}{"email": existing.Email}

	if req.Email != "" {
		if err := validation.ValidateEmail(req.Email); err != nil {
			return &oas.AdminUpdateUserBadRequest{Error: err.Error()}, nil
		}
		if req.Email != existing.Email {
			changes["oldEmail"] = existing.Email
			changes["newEmail"] = req.Email
		}
		existing.Email = req.Email
	}
	if req.Name != "" {
		if err := validation.ValidateName(req.Name); err != nil {
			return &oas.AdminUpdateUserBadRequest{Error: err.Error()}, nil
		}
		if req.Name != existing.Name {
			changes["oldName"] = existing.Name
			changes["newName"] = req.Name
		}
		existing.Name = req.Name
	}
	if r, ok := req.Role.Get(); ok && string(r) != "" {
		role := string(r)
		if !model.IsValidRole(role) {
			return &oas.AdminUpdateUserBadRequest{Error: "invalid role: must be admin, user, or readonly"}, nil
		}
		if role != existing.Role {
			changes["oldRole"] = existing.Role
			changes["newRole"] = role
		}
		existing.Role = role
	}

	if err := h.cfg.Users.Update(ctx, existing); err != nil {
		return &oas.AdminUpdateUserForbidden{Error: "failed to update user"}, nil
	}

	if pw, ok := req.Password.Get(); ok && pw != "" {
		if err := validation.ValidatePassword(pw); err != nil {
			return &oas.AdminUpdateUserBadRequest{Error: err.Error()}, nil
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
		if err != nil {
			return &oas.AdminUpdateUserForbidden{Error: "failed to hash password"}, nil
		}
		if err := h.cfg.Users.UpdatePassword(ctx, existing.ID, string(hash)); err != nil {
			return &oas.AdminUpdateUserForbidden{Error: "failed to update password"}, nil
		}
		changes["passwordChanged"] = true
	}

	if h.cfg.AuditLogger != nil {
		h.cfg.AuditLogger.Log(ctx, &admin.ID, audit.ActionUserUpdate, audit.ResourceUser, &existing.ID, changes, "", "")
	}

	existing.PopulateTraccarFields()
	result := userToOAS(existing)
	return &result, nil
}

// AdminDeleteUser removes a user.
// DELETE /api/admin/users/{id}
func (h *Handler) AdminDeleteUser(ctx context.Context, params oas.AdminDeleteUserParams) (oas.AdminDeleteUserRes, error) {
	admin, err := requireAdminCtx(ctx)
	if err != nil {
		return &oas.AdminDeleteUserForbidden{Error: "admin access required"}, nil
	}

	if params.ID == admin.ID {
		return &oas.AdminDeleteUserForbidden{Error: "cannot delete your own account"}, nil
	}

	if demo.IsEnabled() {
		target, err := h.cfg.Users.GetByID(ctx, params.ID)
		if err == nil && demo.IsDemoAccount(target.Email) {
			return &oas.AdminDeleteUserForbidden{Error: "demo accounts cannot be deleted"}, nil
		}
	}

	if err := h.cfg.Users.Delete(ctx, params.ID); err != nil {
		return &oas.AdminDeleteUserForbidden{Error: "failed to delete user"}, nil
	}

	if h.cfg.AuditLogger != nil {
		h.cfg.AuditLogger.Log(ctx, &admin.ID, audit.ActionUserDelete, audit.ResourceUser, &params.ID,
			map[string]interface{}{}, "", "")
	}

	return &oas.AdminDeleteUserNoContent{}, nil
}

// AdminDeleteUserSession revokes a specific session for a user.
// DELETE /api/admin/users/{id}/sessions/{sessionId}
func (h *Handler) AdminDeleteUserSession(ctx context.Context, params oas.AdminDeleteUserSessionParams) (oas.AdminDeleteUserSessionRes, error) {
	if _, err := requireAdminCtx(ctx); err != nil {
		return &oas.AdminDeleteUserSessionForbidden{Error: "admin access required"}, nil
	}

	session, err := h.cfg.Sessions.GetByIDPrefix(ctx, params.ID, params.SessionId)
	if err != nil || session == nil {
		return &oas.AdminDeleteUserSessionNotFound{Error: "session not found"}, nil
	}

	if err := h.cfg.Sessions.Delete(ctx, session.ID); err != nil {
		return &oas.AdminDeleteUserSessionForbidden{Error: "failed to delete session"}, nil
	}

	return &oas.AdminDeleteUserSessionNoContent{}, nil
}

// AdminListUserKeys returns all API keys for a specific user (admin only).
// GET /api/admin/users/{id}/keys
func (h *Handler) AdminListUserKeys(ctx context.Context, params oas.AdminListUserKeysParams) (oas.AdminListUserKeysRes, error) {
	if _, err := requireAdminCtx(ctx); err != nil {
		return &oas.AdminListUserKeysForbidden{Error: "admin access required"}, nil
	}

	if _, err := h.cfg.Users.GetByID(ctx, params.ID); err != nil {
		return &oas.AdminListUserKeysNotFound{Error: "user not found"}, nil
	}

	keys, err := h.cfg.ApiKeys.ListByUser(ctx, params.ID)
	if err != nil {
		return &oas.AdminListUserKeysForbidden{Error: "failed to list API keys"}, nil
	}

	result := make(oas.AdminListUserKeysOKApplicationJSON, 0, len(keys))
	for _, k := range keys {
		result = append(result, apiKeyToOAS(k, false))
	}
	return &result, nil
}

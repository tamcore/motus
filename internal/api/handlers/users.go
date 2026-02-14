package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/demo"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/validation"
	"golang.org/x/crypto/bcrypt"
)

// UserHandler handles admin user-management endpoints.
type UserHandler struct {
	users            repository.UserRepo
	devices          repository.DeviceRepo
	cacheInvalidator DeviceCacheInvalidator
	audit            *audit.Logger
	uniqueIDPrefix   string
}

// NewUserHandler creates a new user handler.
func NewUserHandler(users repository.UserRepo, devices repository.DeviceRepo, uniqueIDPrefix string) *UserHandler {
	return &UserHandler{users: users, devices: devices, uniqueIDPrefix: uniqueIDPrefix}
}

// SetAuditLogger configures audit logging for user events.
func (h *UserHandler) SetAuditLogger(logger *audit.Logger) {
	h.audit = logger
}

// requireAdmin checks that the authenticated user has the admin role.
// It writes a 403 response and returns false when the check fails.
func requireAdmin(w http.ResponseWriter, r *http.Request) (*model.User, bool) {
	user := api.UserFromContext(r.Context())
	if user == nil || !user.IsAdmin() {
		api.RespondError(w, http.StatusForbidden, "admin access required")
		return nil, false
	}
	return user, true
}

// List returns all users in the system.
// GET /api/users
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAdmin(w, r); !ok {
		return
	}

	users, err := h.users.ListAll(r.Context())
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	if users == nil {
		users = []*model.User{}
	}
	for _, u := range users {
		u.PopulateTraccarFields()
	}
	api.RespondJSON(w, http.StatusOK, users)
}

type createUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
	Role     string `json:"role"`
}

// Create adds a new user.
// POST /api/users
func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	admin, ok := requireAdmin(w, r)
	if !ok {
		return
	}

	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := validation.ValidateEmail(req.Email); err != nil {
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validation.ValidatePassword(req.Password); err != nil {
		api.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name != "" {
		if err := validation.ValidateName(req.Name); err != nil {
			api.RespondError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if req.Role == "" {
		req.Role = model.RoleUser
	}
	if !model.IsValidRole(req.Role) {
		api.RespondError(w, http.StatusBadRequest, "invalid role: must be admin, user, or readonly")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	user := &model.User{
		Email:        req.Email,
		PasswordHash: string(hash),
		Name:         req.Name,
		Role:         req.Role,
	}
	if err := h.users.Create(r.Context(), user); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &admin.ID, audit.ActionUserCreate, audit.ResourceUser, &user.ID,
			map[string]interface{}{"email": user.Email, "role": user.Role})
	}

	user.PopulateTraccarFields()
	api.RespondJSON(w, http.StatusCreated, user)
}

type updateUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password,omitempty"`
	Name     string `json:"name"`
	Role     string `json:"role"`
}

// Update modifies an existing user.
// PUT /api/users/{id}
func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	admin, ok := requireAdmin(w, r)
	if !ok {
		return
	}

	userID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	existing, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		api.RespondError(w, http.StatusNotFound, "user not found")
		return
	}

	// Prevent modifications to demo accounts in demo mode.
	if demo.IsEnabled() && demo.IsDemoAccount(existing.Email) {
		api.RespondError(w, http.StatusForbidden, "demo accounts cannot be modified")
		return
	}

	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Prevent admin from demoting themselves.
	if existing.ID == admin.ID && req.Role != "" && req.Role != model.RoleAdmin {
		api.RespondError(w, http.StatusBadRequest, "cannot change your own role")
		return
	}

	// Track changes for audit log.
	changes := map[string]interface{}{"email": existing.Email}

	if req.Email != "" {
		if err := validation.ValidateEmail(req.Email); err != nil {
			api.RespondError(w, http.StatusBadRequest, err.Error())
			return
		}
		if req.Email != existing.Email {
			changes["oldEmail"] = existing.Email
			changes["newEmail"] = req.Email
		}
		existing.Email = req.Email
	}
	if req.Name != "" {
		if err := validation.ValidateName(req.Name); err != nil {
			api.RespondError(w, http.StatusBadRequest, err.Error())
			return
		}
		if req.Name != existing.Name {
			changes["oldName"] = existing.Name
			changes["newName"] = req.Name
		}
		existing.Name = req.Name
	}
	if req.Role != "" {
		if !model.IsValidRole(req.Role) {
			api.RespondError(w, http.StatusBadRequest, "invalid role: must be admin, user, or readonly")
			return
		}
		if req.Role != existing.Role {
			changes["oldRole"] = existing.Role
			changes["newRole"] = req.Role
		}
		existing.Role = req.Role
	}

	if err := h.users.Update(r.Context(), existing); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to update user")
		return
	}

	// Update password separately if provided.
	if req.Password != "" {
		if err := validation.ValidatePassword(req.Password); err != nil {
			api.RespondError(w, http.StatusBadRequest, err.Error())
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			api.RespondError(w, http.StatusInternalServerError, "failed to hash password")
			return
		}
		if err := h.users.UpdatePassword(r.Context(), existing.ID, string(hash)); err != nil {
			api.RespondError(w, http.StatusInternalServerError, "failed to update password")
			return
		}
		changes["passwordChanged"] = true
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &admin.ID, audit.ActionUserUpdate, audit.ResourceUser, &existing.ID, changes)
	}

	existing.PopulateTraccarFields()
	api.RespondJSON(w, http.StatusOK, existing)
}

// Delete removes a user.
// DELETE /api/users/{id}
func (h *UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	admin, ok := requireAdmin(w, r)
	if !ok {
		return
	}

	userID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	// Prevent admin from deleting themselves.
	if userID == admin.ID {
		api.RespondError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}

	// Prevent deletion of demo accounts in demo mode.
	var targetEmail string
	if demo.IsEnabled() {
		target, err := h.users.GetByID(r.Context(), userID)
		if err == nil && demo.IsDemoAccount(target.Email) {
			api.RespondError(w, http.StatusForbidden, "demo accounts cannot be deleted")
			return
		}
		if err == nil {
			targetEmail = target.Email
		}
	}

	if err := h.users.Delete(r.Context(), userID); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}

	if h.audit != nil {
		details := map[string]interface{}{}
		if targetEmail != "" {
			details["email"] = targetEmail
		}
		h.audit.LogFromRequest(r, &admin.ID, audit.ActionUserDelete, audit.ResourceUser, &userID, details)
	}

	w.WriteHeader(http.StatusNoContent)
}

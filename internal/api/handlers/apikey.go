package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// ApiKeyHandler handles API key management endpoints.
type ApiKeyHandler struct {
	apiKeys repository.ApiKeyRepo
	audit   *audit.Logger
}

// NewApiKeyHandler creates a new API key handler.
func NewApiKeyHandler(apiKeys repository.ApiKeyRepo) *ApiKeyHandler {
	return &ApiKeyHandler{apiKeys: apiKeys}
}

// SetAuditLogger configures audit logging for API key events.
func (h *ApiKeyHandler) SetAuditLogger(logger *audit.Logger) {
	h.audit = logger
}

type createApiKeyRequest struct {
	Name           string  `json:"name"`
	Permissions    string  `json:"permissions"`
	ExpiresInHours *int    `json:"expiresInHours,omitempty"`
	ExpiresAt      *string `json:"expiresAt,omitempty"`
}

// redactToken returns a masked version of the token for list responses.
// Only the first 8 characters are shown, the rest replaced with "...".
func redactToken(token string) string {
	if len(token) <= 8 {
		return token
	}
	return token[:8] + "..."
}

// Create creates a new API key for the authenticated user.
// POST /api/keys
//
// Accepts optional expiration via either:
//   - expiresInHours: integer number of hours from now (e.g. 24, 168, 720)
//   - expiresAt: ISO 8601 timestamp string for a custom expiration date
//
// If neither is provided, the key never expires.
func (h *ApiKeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	if user == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req createApiKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		api.RespondError(w, http.StatusBadRequest, "name is required")
		return
	}

	if req.Permissions == "" {
		req.Permissions = model.PermissionFull
	}
	if !model.IsValidPermission(req.Permissions) {
		api.RespondError(w, http.StatusBadRequest, "permissions must be 'full' or 'readonly'")
		return
	}

	// Resolve expiration time.
	var expiresAt *time.Time
	if req.ExpiresInHours != nil && req.ExpiresAt != nil {
		api.RespondError(w, http.StatusBadRequest, "specify either expiresInHours or expiresAt, not both")
		return
	}
	if req.ExpiresInHours != nil {
		hours := *req.ExpiresInHours
		if hours <= 0 {
			api.RespondError(w, http.StatusBadRequest, "expiresInHours must be positive")
			return
		}
		t := time.Now().Add(time.Duration(hours) * time.Hour)
		expiresAt = &t
	}
	if req.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			api.RespondError(w, http.StatusBadRequest, "expiresAt must be a valid RFC 3339 timestamp")
			return
		}
		if t.Before(time.Now()) {
			api.RespondError(w, http.StatusBadRequest, "expiresAt must be in the future")
			return
		}
		expiresAt = &t
	}

	key := &model.ApiKey{
		UserID:      user.ID,
		Name:        req.Name,
		Permissions: req.Permissions,
		ExpiresAt:   expiresAt,
	}

	if err := h.apiKeys.Create(r.Context(), key); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to create API key")
		return
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &user.ID, audit.ActionApiKeyCreate, audit.ResourceApiKey, &key.ID,
			map[string]interface{}{"name": key.Name, "permissions": key.Permissions})
	}

	// Return the full token only on creation. Subsequent list calls
	// will return redacted tokens.
	api.RespondJSON(w, http.StatusCreated, key)
}

// List returns all API keys for the authenticated user.
// Tokens are redacted for security.
// GET /api/keys
func (h *ApiKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	if user == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	keys, err := h.apiKeys.ListByUser(r.Context(), user.ID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to list API keys")
		return
	}

	// Redact tokens in list response.
	for _, k := range keys {
		k.Token = redactToken(k.Token)
	}

	api.RespondJSON(w, http.StatusOK, keys)
}

// Delete removes an API key by ID. Users can only delete their own keys
// unless they are admins.
// DELETE /api/keys/{id}
func (h *ApiKeyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := api.UserFromContext(r.Context())
	if user == nil {
		api.RespondError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid key ID")
		return
	}

	// Verify ownership.
	key, err := h.apiKeys.GetByID(r.Context(), id)
	if err != nil {
		api.RespondError(w, http.StatusNotFound, "API key not found")
		return
	}

	// Allow deletion if the key belongs to the user or user is admin.
	if key.UserID != user.ID && !user.IsAdmin() {
		api.RespondError(w, http.StatusForbidden, "cannot delete another user's API key")
		return
	}

	if err := h.apiKeys.Delete(r.Context(), id); err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to delete API key")
		return
	}

	if h.audit != nil {
		h.audit.LogFromRequest(r, &user.ID, audit.ActionApiKeyDelete, audit.ResourceApiKey, &id,
			map[string]interface{}{"name": key.Name, "keyOwnerUserId": key.UserID})
	}

	w.WriteHeader(http.StatusNoContent)
}

// AdminListUserKeys returns all API keys for a specific user (admin only).
// GET /api/users/{id}/keys
func (h *ApiKeyHandler) AdminListUserKeys(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	userID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		api.RespondError(w, http.StatusBadRequest, "invalid user ID")
		return
	}

	keys, err := h.apiKeys.ListByUser(r.Context(), userID)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to list API keys")
		return
	}

	// Redact tokens in list response.
	for _, k := range keys {
		k.Token = redactToken(k.Token)
	}

	api.RespondJSON(w, http.StatusOK, keys)
}

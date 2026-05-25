package handlers

import (
	"context"
	"time"

	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
)

// ListApiKeys returns all API keys for the authenticated user.
// GET /api/keys
func (h *Handler) ListApiKeys(ctx context.Context) (oas.ListApiKeysRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.Error{Error: "not authenticated"}, nil
	}

	keys, err := h.cfg.ApiKeys.ListByUser(ctx, user.ID)
	if err != nil {
		return &oas.Error{Error: "failed to list API keys"}, nil
	}

	result := make(oas.ListApiKeysOKApplicationJSON, 0, len(keys))
	for _, k := range keys {
		result = append(result, apiKeyToOAS(k, false))
	}
	return &result, nil
}

// CreateApiKey creates a new API key for the authenticated user.
// POST /api/keys
func (h *Handler) CreateApiKey(ctx context.Context, req *oas.ApiKeyInput) (oas.CreateApiKeyRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.CreateApiKeyUnauthorized{Error: "not authenticated"}, nil
	}

	if req.Name == "" {
		return &oas.CreateApiKeyBadRequest{Error: "name is required"}, nil
	}

	permissions := model.PermissionFull
	if p, ok := req.Permissions.Get(); ok {
		permissions = string(p)
	}
	if !model.IsValidPermission(permissions) {
		return &oas.CreateApiKeyBadRequest{Error: "permissions must be 'full' or 'readonly'"}, nil
	}

	var expiresAt *time.Time
	if ea, ok := req.ExpiresAt.Get(); ok {
		t := time.Time(ea)
		if t.Before(time.Now()) {
			return &oas.CreateApiKeyBadRequest{Error: "expiresAt must be in the future"}, nil
		}
		expiresAt = &t
	}

	key := &model.ApiKey{
		UserID:      user.ID,
		Name:        req.Name,
		Permissions: permissions,
		ExpiresAt:   expiresAt,
	}

	if err := h.cfg.ApiKeys.Create(ctx, key); err != nil {
		return &oas.CreateApiKeyBadRequest{Error: "failed to create API key"}, nil
	}

	if h.cfg.AuditLogger != nil {
		h.cfg.AuditLogger.Log(ctx, &user.ID, audit.ActionApiKeyCreate, audit.ResourceApiKey, &key.ID,
			map[string]interface{}{"name": key.Name, "permissions": key.Permissions}, "", "")
	}

	result := apiKeyToOAS(key, true)
	return &result, nil
}

// DeleteApiKey removes an API key by ID.
// DELETE /api/keys/{id}
func (h *Handler) DeleteApiKey(ctx context.Context, params oas.DeleteApiKeyParams) (oas.DeleteApiKeyRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.DeleteApiKeyUnauthorized{Error: "not authenticated"}, nil
	}

	key, err := h.cfg.ApiKeys.GetByID(ctx, params.ID)
	if err != nil {
		return &oas.DeleteApiKeyNotFound{Error: "API key not found"}, nil
	}

	if key.UserID != user.ID && !user.IsAdmin() {
		return &oas.DeleteApiKeyForbidden{Error: "cannot delete another user's API key"}, nil
	}

	if err := h.cfg.ApiKeys.Delete(ctx, params.ID); err != nil {
		return &oas.DeleteApiKeyForbidden{Error: "failed to delete API key"}, nil
	}

	if h.cfg.AuditLogger != nil {
		h.cfg.AuditLogger.Log(ctx, &user.ID, audit.ActionApiKeyDelete, audit.ResourceApiKey, &params.ID,
			map[string]interface{}{"name": key.Name, "keyOwnerUserId": key.UserID}, "", "")
	}

	return &oas.DeleteApiKeyNoContent{}, nil
}

package handlers

import (
	"context"
	"net/http"
	"strconv"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/audit"
)

// AuditQuerier is the minimal interface for querying audit log entries.
// *audit.Logger satisfies this interface.
type AuditQuerier interface {
	Query(ctx context.Context, params audit.QueryParams) ([]audit.Entry, int64, error)
}

// AuditHandler handles admin audit log endpoints.
type AuditHandler struct {
	querier AuditQuerier
}

// NewAuditHandler creates a new audit handler.
func NewAuditHandler(q AuditQuerier) *AuditHandler {
	return &AuditHandler{querier: q}
}

// GetAuditLog returns paginated audit log entries.
// GET /api/admin/audit?action=...&userId=...&resourceType=...&limit=...&offset=...
func (h *AuditHandler) GetAuditLog(w http.ResponseWriter, r *http.Request) {
	admin := api.UserFromContext(r.Context())
	if admin == nil || !admin.IsAdmin() {
		api.RespondError(w, http.StatusForbidden, "admin access required")
		return
	}

	params := audit.QueryParams{
		Action:       r.URL.Query().Get("action"),
		ResourceType: r.URL.Query().Get("resourceType"),
	}

	if userIDStr := r.URL.Query().Get("userId"); userIDStr != "" {
		uid, err := strconv.ParseInt(userIDStr, 10, 64)
		if err == nil {
			params.UserID = &uid
		}
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			params.Limit = limit
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil {
			params.Offset = offset
		}
	}

	entries, total, err := h.querier.Query(r.Context(), params)
	if err != nil {
		api.RespondError(w, http.StatusInternalServerError, "failed to query audit log")
		return
	}

	if entries == nil {
		entries = []audit.Entry{}
	}

	api.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"entries": entries,
		"total":   total,
		"limit":   params.Limit,
		"offset":  params.Offset,
	})
}

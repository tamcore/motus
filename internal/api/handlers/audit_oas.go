package handlers

import (
	"context"

	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
)

// AdminGetAuditLog returns paginated audit log entries.
// GET /api/admin/audit
func (h *Handler) AdminGetAuditLog(ctx context.Context, params oas.AdminGetAuditLogParams) (oas.AdminGetAuditLogRes, error) {
	if _, err := requireAdminCtx(ctx); err != nil {
		return &oas.AdminGetAuditLogForbidden{Error: "admin access required"}, nil
	}

	qp := audit.QueryParams{
		Action:       params.Action.Or(""),
		ResourceType: params.ResourceType.Or(""),
		Limit:        params.Limit.Or(0),
		Offset:       params.Offset.Or(0),
	}
	if userID, ok := params.UserId.Get(); ok {
		qp.UserID = &userID
	}

	entries, total, err := h.cfg.AuditLogger.Query(ctx, qp)
	if err != nil {
		return &oas.AdminGetAuditLogForbidden{Error: "failed to query audit log"}, nil
	}

	return &oas.AuditPage{
		Entries: mapSlice[[]oas.AuditEntry](entries, auditEntryToOAS),
		Total:   total,
	}, nil
}

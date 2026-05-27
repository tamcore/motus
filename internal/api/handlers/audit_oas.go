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

	qp := audit.QueryParams{}

	if action, ok := params.Action.Get(); ok {
		qp.Action = action
	}
	if resourceType, ok := params.ResourceType.Get(); ok {
		qp.ResourceType = resourceType
	}
	if userID, ok := params.UserId.Get(); ok {
		qp.UserID = &userID
	}
	if limit, ok := params.Limit.Get(); ok {
		qp.Limit = limit
	}
	if offset, ok := params.Offset.Get(); ok {
		qp.Offset = offset
	}

	entries, total, err := h.cfg.AuditLogger.Query(ctx, qp)
	if err != nil {
		return &oas.AdminGetAuditLogForbidden{Error: "failed to query audit log"}, nil
	}

	oasEntries := make([]oas.AuditEntry, 0, len(entries))
	for _, e := range entries {
		oasEntries = append(oasEntries, auditEntryToOAS(e))
	}

	return &oas.AuditPage{
		Entries: oasEntries,
		Total:   total,
	}, nil
}

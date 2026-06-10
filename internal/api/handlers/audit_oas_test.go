package handlers_test

// Tests for the ogen Handler audit method (AdminGetAuditLog), ported from the
// deleted chi AuditHandler tests in audit_test.go. The chi handler accepted an
// AuditQuerier interface, so success/empty/limit/filter paths were mockable;
// the ogen Handler depends on the concrete *audit.Logger, so those paths run
// as integration tests against the shared test database (skipped in -short
// mode by testutil.SetupTestDB). The query-error path is covered as a unit
// test via an invalid action filter, which fails validation inside
// audit.Logger.Query before the (nil) pool is touched.

import (
	"context"
	"testing"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

// newAuditTestHandler builds an ogen Handler around the given audit logger.
func newAuditTestHandler(logger *audit.Logger) *handlers.Handler {
	return handlers.NewHandler(handlers.HandlerConfig{
		AuditLogger: logger,
	})
}

// auditAdminCtx returns a context carrying an authenticated admin user.
func auditAdminCtx(id int64) context.Context {
	admin := &model.User{ID: id, Role: model.RoleAdmin}
	return api.ContextWithUser(context.Background(), admin)
}

// ---------------------------------------------------------------------------
// Unit tests (nil-pool logger; never reach the database)
// ---------------------------------------------------------------------------

func TestAdminGetAuditLog_RequiresAdmin(t *testing.T) {
	h := newAuditTestHandler(audit.NewLogger(nil))

	// No user in context.
	res, err := h.AdminGetAuditLog(context.Background(), oas.AdminGetAuditLogParams{})
	if err != nil {
		t.Fatalf("AdminGetAuditLog returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminGetAuditLogForbidden); !ok {
		t.Errorf("expected *oas.AdminGetAuditLogForbidden, got %T", res)
	}
}

func TestAdminGetAuditLog_NonAdminForbidden(t *testing.T) {
	h := newAuditTestHandler(audit.NewLogger(nil))

	user := &model.User{ID: 1, Role: model.RoleUser}
	ctx := api.ContextWithUser(context.Background(), user)

	res, err := h.AdminGetAuditLog(ctx, oas.AdminGetAuditLogParams{})
	if err != nil {
		t.Fatalf("AdminGetAuditLog returned error: %v", err)
	}
	forbidden, ok := res.(*oas.AdminGetAuditLogForbidden)
	if !ok {
		t.Fatalf("expected *oas.AdminGetAuditLogForbidden for non-admin, got %T", res)
	}
	if forbidden.Error != "admin access required" {
		t.Errorf("unexpected error message: %q", forbidden.Error)
	}
}

// TestAdminGetAuditLog_QueryError forces audit.Logger.Query to fail by passing
// an action filter that violates the audit filter pattern. Validation happens
// before any database access, so a nil-pool logger is sufficient.
func TestAdminGetAuditLog_QueryError(t *testing.T) {
	h := newAuditTestHandler(audit.NewLogger(nil))

	res, err := h.AdminGetAuditLog(auditAdminCtx(1), oas.AdminGetAuditLogParams{
		Action: oas.NewOptString("Bad Filter!"),
	})
	if err != nil {
		t.Fatalf("AdminGetAuditLog returned error: %v", err)
	}
	forbidden, ok := res.(*oas.AdminGetAuditLogForbidden)
	if !ok {
		t.Fatalf("expected *oas.AdminGetAuditLogForbidden on query error, got %T", res)
	}
	if forbidden.Error != "failed to query audit log" {
		t.Errorf("unexpected error message: %q", forbidden.Error)
	}
}

// ---------------------------------------------------------------------------
// Integration tests (require Docker; skipped in -short mode)
// ---------------------------------------------------------------------------

// setupAuditIntegration returns a Handler backed by a real audit logger plus
// a user created in the database (audit_log.user_id has an FK to users).
func setupAuditIntegration(t *testing.T) (*handlers.Handler, *audit.Logger, *model.User) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	logger := audit.NewLogger(pool)
	h := newAuditTestHandler(logger)

	userRepo := repository.NewUserRepository(pool)
	user := &model.User{Email: "audit-admin@example.com", PasswordHash: "hash", Name: "Audit Admin", Role: model.RoleAdmin}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return h, logger, user
}

// logEntries writes n audit entries for the given user via the real logger.
func logEntries(t *testing.T, logger *audit.Logger, userID int64, actions []string) {
	t.Helper()
	ctx := context.Background()
	for _, action := range actions {
		logger.Log(ctx, &userID, action, "", nil, nil, "", "")
	}
}

func TestAdminGetAuditLog_Integration_SuccessWithEntries(t *testing.T) {
	h, logger, user := setupAuditIntegration(t)
	logEntries(t, logger, user.ID, []string{
		audit.ActionSessionLogin,
		audit.ActionDeviceCreate,
		audit.ActionUserUpdate,
	})

	res, err := h.AdminGetAuditLog(auditAdminCtx(user.ID), oas.AdminGetAuditLogParams{})
	if err != nil {
		t.Fatalf("AdminGetAuditLog returned error: %v", err)
	}
	page, ok := res.(*oas.AuditPage)
	if !ok {
		t.Fatalf("expected *oas.AuditPage, got %T", res)
	}
	if page.Total != 3 {
		t.Errorf("expected total=3, got %d", page.Total)
	}
	if len(page.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(page.Entries))
	}
	seen := make(map[string]bool, len(page.Entries))
	for _, e := range page.Entries {
		seen[e.Action] = true
		if e.UserId != user.ID {
			t.Errorf("expected userId=%d, got %d", user.ID, e.UserId)
		}
	}
	for _, action := range []string{audit.ActionSessionLogin, audit.ActionDeviceCreate, audit.ActionUserUpdate} {
		if !seen[action] {
			t.Errorf("expected entry with action %q", action)
		}
	}
}

func TestAdminGetAuditLog_Integration_EmptyEntries(t *testing.T) {
	h, _, user := setupAuditIntegration(t)

	res, err := h.AdminGetAuditLog(auditAdminCtx(user.ID), oas.AdminGetAuditLogParams{})
	if err != nil {
		t.Fatalf("AdminGetAuditLog returned error: %v", err)
	}
	page, ok := res.(*oas.AuditPage)
	if !ok {
		t.Fatalf("expected *oas.AuditPage, got %T", res)
	}
	if page.Total != 0 {
		t.Errorf("expected total=0, got %d", page.Total)
	}
	if page.Entries == nil {
		t.Error("expected non-nil entries slice for empty result")
	}
	if len(page.Entries) != 0 {
		t.Errorf("expected empty entries, got %d", len(page.Entries))
	}
}

func TestAdminGetAuditLog_Integration_LimitOffset(t *testing.T) {
	h, logger, user := setupAuditIntegration(t)
	logEntries(t, logger, user.ID, []string{
		audit.ActionSessionLogin,
		audit.ActionDeviceCreate,
		audit.ActionUserUpdate,
	})

	res, err := h.AdminGetAuditLog(auditAdminCtx(user.ID), oas.AdminGetAuditLogParams{
		Limit:  oas.NewOptInt(1),
		Offset: oas.NewOptInt(1),
	})
	if err != nil {
		t.Fatalf("AdminGetAuditLog returned error: %v", err)
	}
	page, ok := res.(*oas.AuditPage)
	if !ok {
		t.Fatalf("expected *oas.AuditPage, got %T", res)
	}
	if page.Total != 3 {
		t.Errorf("expected total=3 (pagination keeps full count), got %d", page.Total)
	}
	if len(page.Entries) != 1 {
		t.Errorf("expected 1 entry with limit=1, got %d", len(page.Entries))
	}
}

func TestAdminGetAuditLog_Integration_UserIDFilter(t *testing.T) {
	h, logger, admin := setupAuditIntegration(t)

	pool := testutil.SetupTestDB(t)
	userRepo := repository.NewUserRepository(pool)
	other := &model.User{Email: "audit-other@example.com", PasswordHash: "hash", Name: "Other"}
	if err := userRepo.Create(context.Background(), other); err != nil {
		t.Fatalf("create other user: %v", err)
	}

	logEntries(t, logger, admin.ID, []string{audit.ActionSessionLogin, audit.ActionUserUpdate})
	logEntries(t, logger, other.ID, []string{audit.ActionDeviceCreate})

	res, err := h.AdminGetAuditLog(auditAdminCtx(admin.ID), oas.AdminGetAuditLogParams{
		UserId: oas.NewOptInt64(other.ID),
	})
	if err != nil {
		t.Fatalf("AdminGetAuditLog returned error: %v", err)
	}
	page, ok := res.(*oas.AuditPage)
	if !ok {
		t.Fatalf("expected *oas.AuditPage, got %T", res)
	}
	if page.Total != 1 {
		t.Errorf("expected total=1 for userId filter, got %d", page.Total)
	}
	if len(page.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(page.Entries))
	}
	if page.Entries[0].UserId != other.ID {
		t.Errorf("expected entry for user %d, got %d", other.ID, page.Entries[0].UserId)
	}
	if page.Entries[0].Action != audit.ActionDeviceCreate {
		t.Errorf("expected action %q, got %q", audit.ActionDeviceCreate, page.Entries[0].Action)
	}
}

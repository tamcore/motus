package handlers_test

// Tests for the ogen Handler sudo methods (AdminStartSudo, EndSudo,
// GetSudoStatus). Ported from the deleted chi SudoHandler tests in
// sudo_test.go. The chi-specific variants (invalid path-param parsing,
// cookie-based session lookup) are intentionally dropped: ogen owns path
// param decoding, and the session now arrives via api.ContextWithSession.

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// newSudoTestHandler builds an ogen Handler from mock repositories with a
// nil-pool audit logger (Log is a documented no-op without a pool).
func newSudoTestHandler(users repository.UserRepo, sessions repository.SessionRepo) *handlers.Handler {
	return handlers.NewHandler(handlers.HandlerConfig{
		Users:       users,
		Sessions:    sessions,
		AuditLogger: audit.NewLogger(nil),
	})
}

// ---------------------------------------------------------------------------
// AdminStartSudo
// ---------------------------------------------------------------------------

func TestAdminStartSudo_Unauthenticated(t *testing.T) {
	h := newSudoTestHandler(&mockUserRepo{}, &mockSessionRepo{})

	res, err := h.AdminStartSudo(context.Background(), oas.AdminStartSudoParams{ID: 2})
	if err != nil {
		t.Fatalf("AdminStartSudo returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminStartSudoForbidden); !ok {
		t.Errorf("expected *oas.AdminStartSudoForbidden, got %T", res)
	}
}

func TestAdminStartSudo_NonAdminForbidden(t *testing.T) {
	h := newSudoTestHandler(&mockUserRepo{}, &mockSessionRepo{})

	user := &model.User{ID: 1, Role: model.RoleUser}
	ctx := api.ContextWithUser(context.Background(), user)

	res, err := h.AdminStartSudo(ctx, oas.AdminStartSudoParams{ID: 2})
	if err != nil {
		t.Fatalf("AdminStartSudo returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminStartSudoForbidden); !ok {
		t.Errorf("expected *oas.AdminStartSudoForbidden for non-admin, got %T", res)
	}
}

func TestAdminStartSudo_CannotSudoSelf(t *testing.T) {
	h := newSudoTestHandler(&mockUserRepo{}, &mockSessionRepo{})

	admin := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), admin)

	res, err := h.AdminStartSudo(ctx, oas.AdminStartSudoParams{ID: 1})
	if err != nil {
		t.Fatalf("AdminStartSudo returned error: %v", err)
	}
	forbidden, ok := res.(*oas.AdminStartSudoForbidden)
	if !ok {
		t.Fatalf("expected *oas.AdminStartSudoForbidden, got %T", res)
	}
	if forbidden.Error != "cannot impersonate yourself" {
		t.Errorf("unexpected error message: %q", forbidden.Error)
	}
}

func TestAdminStartSudo_Success(t *testing.T) {
	targetUser := &model.User{ID: 2, Email: "target@example.com", Role: model.RoleUser, Name: "Target"}
	users := &mockUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			return targetUser, nil
		},
	}
	adminID := int64(1)
	sessions := &mockSessionRepo{
		createSudoFn: func(_ context.Context, targetID, origID int64) (*model.Session, error) {
			if targetID != 2 || origID != 1 {
				t.Errorf("expected CreateSudo(target=2, orig=1), got (%d, %d)", targetID, origID)
			}
			return &model.Session{
				ID:             "sudo-session",
				UserID:         targetID,
				IsSudo:         true,
				OriginalUserID: &adminID,
				ExpiresAt:      time.Now().Add(time.Hour),
			}, nil
		},
	}
	h := newSudoTestHandler(users, sessions)

	admin := &model.User{ID: 1, Role: model.RoleAdmin, Email: "admin@example.com"}
	rr := httptest.NewRecorder()
	ctx := api.ContextWithUser(context.Background(), admin)
	ctx = api.ContextWithResponseWriter(ctx, rr)

	res, err := h.AdminStartSudo(ctx, oas.AdminStartSudoParams{ID: 2})
	if err != nil {
		t.Fatalf("AdminStartSudo returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminStartSudoNoContent); !ok {
		t.Fatalf("expected *oas.AdminStartSudoNoContent, got %T", res)
	}

	// The sudo session cookie must be set on the response writer.
	c := recorderSessionCookie(rr)
	if c == nil {
		t.Fatal("expected session_id cookie to be set")
	}
	if c.Value != "sudo-session" {
		t.Errorf("expected cookie value 'sudo-session', got %q", c.Value)
	}
	if !c.HttpOnly {
		t.Error("expected HttpOnly on sudo session cookie")
	}
}

func TestAdminStartSudo_TargetUserNotFound(t *testing.T) {
	users := &mockUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			return nil, errors.New("not found")
		},
	}
	h := newSudoTestHandler(users, &mockSessionRepo{})

	admin := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), admin)

	res, err := h.AdminStartSudo(ctx, oas.AdminStartSudoParams{ID: 999})
	if err != nil {
		t.Fatalf("AdminStartSudo returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminStartSudoNotFound); !ok {
		t.Errorf("expected *oas.AdminStartSudoNotFound, got %T", res)
	}
}

func TestAdminStartSudo_CreateSudoFails(t *testing.T) {
	targetUser := &model.User{ID: 2, Email: "target@example.com", Role: model.RoleUser}
	users := &mockUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			return targetUser, nil
		},
	}
	sessions := &mockSessionRepo{
		createSudoFn: func(_ context.Context, _, _ int64) (*model.Session, error) {
			return nil, errors.New("db error")
		},
	}
	h := newSudoTestHandler(users, sessions)

	admin := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), admin)

	res, err := h.AdminStartSudo(ctx, oas.AdminStartSudoParams{ID: 2})
	if err != nil {
		t.Fatalf("AdminStartSudo returned error: %v", err)
	}
	forbidden, ok := res.(*oas.AdminStartSudoForbidden)
	if !ok {
		t.Fatalf("expected *oas.AdminStartSudoForbidden on CreateSudo failure, got %T", res)
	}
	if forbidden.Error != "failed to create sudo session" {
		t.Errorf("unexpected error message: %q", forbidden.Error)
	}
}

// ---------------------------------------------------------------------------
// EndSudo
// ---------------------------------------------------------------------------

func TestEndSudo_Unauthenticated(t *testing.T) {
	h := newSudoTestHandler(&mockUserRepo{}, &mockSessionRepo{})

	res, err := h.EndSudo(context.Background())
	if err != nil {
		t.Fatalf("EndSudo returned error: %v", err)
	}
	errRes, ok := res.(*oas.Error)
	if !ok {
		t.Fatalf("expected *oas.Error, got %T", res)
	}
	if errRes.Error != "not authenticated" {
		t.Errorf("unexpected error message: %q", errRes.Error)
	}
}

// TestEndSudo_NoSession replaces the old cookie-absence test: without
// api.ContextWithSession the handler must report a missing session.
func TestEndSudo_NoSession(t *testing.T) {
	h := newSudoTestHandler(&mockUserRepo{}, &mockSessionRepo{})

	user := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), user)

	res, err := h.EndSudo(ctx)
	if err != nil {
		t.Fatalf("EndSudo returned error: %v", err)
	}
	errRes, ok := res.(*oas.Error)
	if !ok {
		t.Fatalf("expected *oas.Error, got %T", res)
	}
	if errRes.Error != "no session found" {
		t.Errorf("unexpected error message: %q", errRes.Error)
	}
}

func TestEndSudo_NotInSudoSession(t *testing.T) {
	h := newSudoTestHandler(&mockUserRepo{}, &mockSessionRepo{})

	user := &model.User{ID: 2, Role: model.RoleUser}
	ctx := api.ContextWithUser(context.Background(), user)
	ctx = api.ContextWithSession(ctx, &model.Session{ID: "regular-sess", IsSudo: false})

	res, err := h.EndSudo(ctx)
	if err != nil {
		t.Fatalf("EndSudo returned error: %v", err)
	}
	errRes, ok := res.(*oas.Error)
	if !ok {
		t.Fatalf("expected *oas.Error, got %T", res)
	}
	if errRes.Error != "not in a sudo session" {
		t.Errorf("unexpected error message: %q", errRes.Error)
	}
}

// TestEndSudo_Success verifies that ending a sudo session deletes the sudo
// session, restores the original admin user, and sets a fresh session cookie.
func TestEndSudo_Success(t *testing.T) {
	adminID := int64(1)
	targetUser := &model.User{ID: 2, Email: "target@example.com", Role: model.RoleUser}
	adminUser := &model.User{ID: 1, Email: "admin@example.com", Role: model.RoleAdmin}

	users := &mockUserRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.User, error) {
			if id == adminID {
				return adminUser, nil
			}
			return nil, errors.New("not found")
		},
	}
	var deletedSessionID string
	sessions := &mockSessionRepo{
		deleteFn: func(_ context.Context, id string) error {
			deletedSessionID = id
			return nil
		},
		createWithExpiryFn: func(_ context.Context, userID int64, exp time.Time, rm bool) (*model.Session, error) {
			if userID != adminID {
				t.Errorf("expected new session for original user %d, got %d", adminID, userID)
			}
			return &model.Session{ID: "restored-session", UserID: userID, RememberMe: rm, ExpiresAt: exp}, nil
		},
	}
	h := newSudoTestHandler(users, sessions)

	rr := httptest.NewRecorder()
	ctx := api.ContextWithUser(context.Background(), targetUser)
	ctx = api.ContextWithSession(ctx, &model.Session{
		ID:             "sudo-sess",
		UserID:         targetUser.ID,
		IsSudo:         true,
		OriginalUserID: &adminID,
		ExpiresAt:      time.Now().Add(time.Hour),
	})
	ctx = api.ContextWithResponseWriter(ctx, rr)

	res, err := h.EndSudo(ctx)
	if err != nil {
		t.Fatalf("EndSudo returned error: %v", err)
	}
	if _, ok := res.(*oas.EndSudoNoContent); !ok {
		t.Fatalf("expected *oas.EndSudoNoContent, got %T", res)
	}

	// The sudo session must be deleted and swapped for a fresh one.
	if deletedSessionID != "sudo-sess" {
		t.Errorf("expected sudo session 'sudo-sess' to be deleted, got %q", deletedSessionID)
	}
	c := recorderSessionCookie(rr)
	if c == nil {
		t.Fatal("expected new session_id cookie after EndSudo")
	}
	if c.Value != "restored-session" {
		t.Errorf("expected cookie value 'restored-session', got %q", c.Value)
	}
}

func TestEndSudo_RestoreUserFails(t *testing.T) {
	adminID := int64(1)
	users := &mockUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			return nil, errors.New("user not found")
		},
	}
	h := newSudoTestHandler(users, &mockSessionRepo{})

	user := &model.User{ID: 2, Role: model.RoleUser}
	ctx := api.ContextWithUser(context.Background(), user)
	ctx = api.ContextWithSession(ctx, &model.Session{
		ID:             "sudo-sess",
		UserID:         2,
		IsSudo:         true,
		OriginalUserID: &adminID,
	})

	res, err := h.EndSudo(ctx)
	if err != nil {
		t.Fatalf("EndSudo returned error: %v", err)
	}
	errRes, ok := res.(*oas.Error)
	if !ok {
		t.Fatalf("expected *oas.Error, got %T", res)
	}
	if errRes.Error != "failed to restore original user" {
		t.Errorf("unexpected error message: %q", errRes.Error)
	}
}

func TestEndSudo_CreateSessionFails(t *testing.T) {
	adminID := int64(1)
	adminUser := &model.User{ID: 1, Email: "admin@example.com", Role: model.RoleAdmin}

	users := &mockUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			return adminUser, nil
		},
	}
	sessions := &mockSessionRepo{
		createWithExpiryFn: func(_ context.Context, _ int64, _ time.Time, _ bool) (*model.Session, error) {
			return nil, errors.New("db error")
		},
	}
	h := newSudoTestHandler(users, sessions)

	user := &model.User{ID: 2, Role: model.RoleUser}
	ctx := api.ContextWithUser(context.Background(), user)
	ctx = api.ContextWithSession(ctx, &model.Session{
		ID:             "sudo-sess",
		UserID:         2,
		IsSudo:         true,
		OriginalUserID: &adminID,
	})

	res, err := h.EndSudo(ctx)
	if err != nil {
		t.Fatalf("EndSudo returned error: %v", err)
	}
	errRes, ok := res.(*oas.Error)
	if !ok {
		t.Fatalf("expected *oas.Error, got %T", res)
	}
	if errRes.Error != "failed to create session" {
		t.Errorf("unexpected error message: %q", errRes.Error)
	}
}

// ---------------------------------------------------------------------------
// GetSudoStatus
// ---------------------------------------------------------------------------

func TestGetSudoStatus_Unauthenticated(t *testing.T) {
	h := newSudoTestHandler(&mockUserRepo{}, &mockSessionRepo{})

	res, err := h.GetSudoStatus(context.Background())
	if err != nil {
		t.Fatalf("GetSudoStatus returned error: %v", err)
	}
	if _, ok := res.(*oas.Error); !ok {
		t.Errorf("expected *oas.Error for unauthenticated request, got %T", res)
	}
}

// TestGetSudoStatus_NoSession replaces the old cookie-absence test: without a
// session in context the status must report inactive.
func TestGetSudoStatus_NoSession(t *testing.T) {
	h := newSudoTestHandler(&mockUserRepo{}, &mockSessionRepo{})

	user := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), user)

	res, err := h.GetSudoStatus(ctx)
	if err != nil {
		t.Fatalf("GetSudoStatus returned error: %v", err)
	}
	status, ok := res.(*oas.SudoStatus)
	if !ok {
		t.Fatalf("expected *oas.SudoStatus, got %T", res)
	}
	if status.Active {
		t.Error("expected active=false without a session")
	}
}

func TestGetSudoStatus_ActiveSudoSession(t *testing.T) {
	adminID := int64(1)
	h := newSudoTestHandler(&mockUserRepo{}, &mockSessionRepo{})

	user := &model.User{ID: 2, Role: model.RoleUser}
	ctx := api.ContextWithUser(context.Background(), user)
	ctx = api.ContextWithSession(ctx, &model.Session{
		ID:             "sudo-sess",
		UserID:         2,
		IsSudo:         true,
		OriginalUserID: &adminID,
		ExpiresAt:      time.Now().Add(time.Hour),
	})

	res, err := h.GetSudoStatus(ctx)
	if err != nil {
		t.Fatalf("GetSudoStatus returned error: %v", err)
	}
	status, ok := res.(*oas.SudoStatus)
	if !ok {
		t.Fatalf("expected *oas.SudoStatus, got %T", res)
	}
	if !status.Active {
		t.Error("expected active=true for sudo session")
	}
	if !status.OriginalUserId.Set || status.OriginalUserId.Value != adminID {
		t.Errorf("expected originalUserId=%d, got %+v", adminID, status.OriginalUserId)
	}
	if !status.TargetUserId.Set || status.TargetUserId.Value != 2 {
		t.Errorf("expected targetUserId=2, got %+v", status.TargetUserId)
	}
}

func TestGetSudoStatus_NonSudoSession(t *testing.T) {
	h := newSudoTestHandler(&mockUserRepo{}, &mockSessionRepo{})

	user := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), user)
	ctx = api.ContextWithSession(ctx, &model.Session{ID: "regular-sess", IsSudo: false})

	res, err := h.GetSudoStatus(ctx)
	if err != nil {
		t.Fatalf("GetSudoStatus returned error: %v", err)
	}
	status, ok := res.(*oas.SudoStatus)
	if !ok {
		t.Fatalf("expected *oas.SudoStatus, got %T", res)
	}
	if status.Active {
		t.Error("expected active=false for non-sudo session")
	}
}

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// sudoAuditLogger is a shared nil-pool audit logger for sudo handler tests.
var sudoAuditLogger = audit.NewLogger(nil)

// sudoTestUserRepo is a test double for repository.UserRepo used in sudo tests.
type sudoTestUserRepo struct {
	getByIDFn func(ctx context.Context, id int64) (*model.User, error)
}

var _ repository.UserRepo = (*sudoTestUserRepo)(nil)

func (m *sudoTestUserRepo) Create(_ context.Context, _ *model.User) error {
	return errors.New("not impl")
}
func (m *sudoTestUserRepo) CreateOIDCUser(_ context.Context, _, _, _, _, _ string) (*model.User, error) {
	return nil, errors.New("not impl")
}
func (m *sudoTestUserRepo) GetByEmail(_ context.Context, _ string) (*model.User, error) {
	return nil, errors.New("not impl")
}
func (m *sudoTestUserRepo) GetByID(ctx context.Context, id int64) (*model.User, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, errors.New("not found")
}
func (m *sudoTestUserRepo) GetByToken(_ context.Context, _ string) (*model.User, error) {
	return nil, errors.New("not impl")
}
func (m *sudoTestUserRepo) GetByOIDCSubject(_ context.Context, _, _ string) (*model.User, error) {
	return nil, errors.New("not impl")
}
func (m *sudoTestUserRepo) SetOIDCSubject(_ context.Context, _ int64, _, _ string) error { return nil }
func (m *sudoTestUserRepo) ListAll(_ context.Context) ([]*model.User, error)             { return nil, nil }
func (m *sudoTestUserRepo) Update(_ context.Context, _ *model.User) error                { return nil }
func (m *sudoTestUserRepo) UpdatePassword(_ context.Context, _ int64, _ string) error {
	return errors.New("not impl")
}
func (m *sudoTestUserRepo) Delete(_ context.Context, _ int64) error { return errors.New("not impl") }
func (m *sudoTestUserRepo) GetDevicesForUser(_ context.Context, _ int64) ([]int64, error) {
	return nil, nil
}
func (m *sudoTestUserRepo) AssignDevice(_ context.Context, _, _ int64) error   { return nil }
func (m *sudoTestUserRepo) UnassignDevice(_ context.Context, _, _ int64) error { return nil }
func (m *sudoTestUserRepo) GenerateToken(_ context.Context, _ int64) (string, error) {
	return "", errors.New("not impl")
}

// sudoTestSessionRepo is a test double for repository.SessionRepo used in sudo tests.
type sudoTestSessionRepo struct {
	getByIDFn          func(ctx context.Context, id string) (*model.Session, error)
	createSudoFn       func(ctx context.Context, targetID, origID int64) (*model.Session, error)
	deleteFn           func(ctx context.Context, id string) error
	createWithExpiryFn func(ctx context.Context, userID int64, exp time.Time, rm bool) (*model.Session, error)
}

var _ repository.SessionRepo = (*sudoTestSessionRepo)(nil)

func (m *sudoTestSessionRepo) Create(_ context.Context, _ int64) (*model.Session, error) {
	return nil, errors.New("not impl")
}
func (m *sudoTestSessionRepo) CreateWithExpiry(ctx context.Context, userID int64, exp time.Time, rm bool) (*model.Session, error) {
	if m.createWithExpiryFn != nil {
		return m.createWithExpiryFn(ctx, userID, exp, rm)
	}
	return &model.Session{ID: "new-session", UserID: userID, ExpiresAt: exp}, nil
}
func (m *sudoTestSessionRepo) CreateWithApiKey(_ context.Context, _ int64, _ int64, _ time.Time, _ bool) (*model.Session, error) {
	return nil, errors.New("not impl")
}
func (m *sudoTestSessionRepo) CreateSudo(ctx context.Context, targetID, origID int64) (*model.Session, error) {
	if m.createSudoFn != nil {
		return m.createSudoFn(ctx, targetID, origID)
	}
	origIDCopy := origID
	return &model.Session{ID: "sudo-session", UserID: targetID, IsSudo: true, OriginalUserID: &origIDCopy, ExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (m *sudoTestSessionRepo) GetByID(ctx context.Context, id string) (*model.Session, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, errors.New("not found")
}
func (m *sudoTestSessionRepo) GetByIDPrefix(_ context.Context, _ int64, _ string) (*model.Session, error) {
	return nil, errors.New("not found")
}
func (m *sudoTestSessionRepo) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}
func (m *sudoTestSessionRepo) ListByUser(_ context.Context, _ int64) ([]*model.Session, error) {
	return nil, nil
}

func TestStartSudo_RequiresAdmin(t *testing.T) {
	h := NewSudoHandler(nil, nil)

	// No user in context.
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sudo/2", nil)
	rec := httptest.NewRecorder()

	h.StartSudo(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestStartSudo_NonAdminForbidden(t *testing.T) {
	h := NewSudoHandler(nil, nil)

	user := &model.User{ID: 1, Role: model.RoleUser}
	ctx := api.ContextWithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sudo/2", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.StartSudo(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestStartSudo_CannotSudoSelf(t *testing.T) {
	h := NewSudoHandler(nil, nil)

	admin := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), admin)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/sudo/1", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.StartSudo(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}

	var body map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] != "cannot impersonate yourself" {
		t.Errorf("unexpected error: %s", body["error"])
	}
}

func TestStartSudo_InvalidUserID(t *testing.T) {
	h := NewSudoHandler(nil, nil)

	admin := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), admin)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "abc")
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/sudo/abc", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.StartSudo(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestEndSudo_NotAuthenticated(t *testing.T) {
	h := NewSudoHandler(nil, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/sudo", nil)
	rec := httptest.NewRecorder()

	h.EndSudo(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestEndSudo_NoCookie(t *testing.T) {
	h := NewSudoHandler(nil, nil)

	user := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/sudo", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.EndSudo(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestGetSudoStatus_NotAuthenticated(t *testing.T) {
	h := NewSudoHandler(nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/sudo", nil)
	rec := httptest.NewRecorder()

	h.GetSudoStatus(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestGetSudoStatus_NoCookie(t *testing.T) {
	h := NewSudoHandler(nil, nil)

	user := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/sudo", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.GetSudoStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var body map[string]interface{}
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["isSudo"] != false {
		t.Errorf("expected isSudo=false, got %v", body["isSudo"])
	}
}

// ── StartSudo success paths ───────────────────────────────────────────────────

func TestStartSudo_Success(t *testing.T) {
	targetUser := &model.User{ID: 2, Email: "target@example.com", Role: model.RoleUser, Name: "Target"}
	users := &sudoTestUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			return targetUser, nil
		},
	}
	sessions := &sudoTestSessionRepo{}
	h := NewSudoHandler(users, sessions)

	admin := &model.User{ID: 1, Role: model.RoleAdmin, Email: "admin@example.com"}
	ctx := api.ContextWithUser(context.Background(), admin)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "2")
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/sudo/2", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.StartSudo(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify Set-Cookie header is present.
	cookies := rec.Result().Cookies()
	foundSession := false
	for _, c := range cookies {
		if c.Name == "session_id" {
			foundSession = true
			break
		}
	}
	if !foundSession {
		t.Error("expected session_id cookie to be set")
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["isSudo"] != true {
		t.Errorf("expected isSudo=true, got %v", body["isSudo"])
	}
}

func TestStartSudo_TargetUserNotFound(t *testing.T) {
	users := &sudoTestUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			return nil, errors.New("not found")
		},
	}
	h := NewSudoHandler(users, nil)

	admin := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), admin)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "999")
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/sudo/999", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.StartSudo(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestStartSudo_CreateSudoFails(t *testing.T) {
	targetUser := &model.User{ID: 2, Email: "target@example.com", Role: model.RoleUser}
	users := &sudoTestUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			return targetUser, nil
		},
	}
	sessions := &sudoTestSessionRepo{
		createSudoFn: func(_ context.Context, _, _ int64) (*model.Session, error) {
			return nil, errors.New("db error")
		},
	}
	h := NewSudoHandler(users, sessions)

	admin := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), admin)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "2")
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/sudo/2", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.StartSudo(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// ── EndSudo success paths ─────────────────────────────────────────────────────

func TestEndSudo_Success(t *testing.T) {
	adminID := int64(1)
	targetUser := &model.User{ID: 2, Email: "target@example.com", Role: model.RoleUser}
	adminUser := &model.User{ID: 1, Email: "admin@example.com", Role: model.RoleAdmin}

	users := &sudoTestUserRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.User, error) {
			if id == adminID {
				return adminUser, nil
			}
			return nil, errors.New("not found")
		},
	}
	sessions := &sudoTestSessionRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.Session, error) {
			return &model.Session{
				ID:             "sudo-sess",
				UserID:         targetUser.ID,
				IsSudo:         true,
				OriginalUserID: &adminID,
				ExpiresAt:      time.Now().Add(time.Hour),
			}, nil
		},
	}
	h := NewSudoHandler(users, sessions)

	ctx := api.ContextWithUser(context.Background(), targetUser)
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/sudo", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "sudo-sess"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.EndSudo(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify a new session_id cookie is set.
	cookies := rec.Result().Cookies()
	foundSession := false
	for _, c := range cookies {
		if c.Name == "session_id" {
			foundSession = true
			break
		}
	}
	if !foundSession {
		t.Error("expected new session_id cookie after EndSudo")
	}
}

func TestEndSudo_NotInSudoSession(t *testing.T) {
	sessions := &sudoTestSessionRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.Session, error) {
			return &model.Session{ID: "regular-sess", IsSudo: false}, nil
		},
	}
	h := NewSudoHandler(nil, sessions)

	user := &model.User{ID: 2, Role: model.RoleUser}
	ctx := api.ContextWithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/sudo", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "regular-sess"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.EndSudo(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}

	var body map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] != "not in a sudo session" {
		t.Errorf("unexpected error message: %s", body["error"])
	}
}

// ── GetSudoStatus success paths ───────────────────────────────────────────────

func TestGetSudoStatus_ActiveSudoSession(t *testing.T) {
	adminID := int64(1)
	adminUser := &model.User{ID: 1, Email: "admin@example.com", Role: model.RoleAdmin, Name: "Admin"}

	users := &sudoTestUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			return adminUser, nil
		},
	}
	sessions := &sudoTestSessionRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.Session, error) {
			return &model.Session{
				ID:             "sudo-sess",
				UserID:         2,
				IsSudo:         true,
				OriginalUserID: &adminID,
				ExpiresAt:      time.Now().Add(time.Hour),
			}, nil
		},
	}
	h := NewSudoHandler(users, sessions)

	user := &model.User{ID: 2, Role: model.RoleUser}
	ctx := api.ContextWithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/sudo", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "sudo-sess"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.GetSudoStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["isSudo"] != true {
		t.Errorf("expected isSudo=true, got %v", body["isSudo"])
	}
	if body["originalUser"] == nil {
		t.Error("expected originalUser to be set")
	}
}

func TestGetSudoStatus_NonSudoSession(t *testing.T) {
	sessions := &sudoTestSessionRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.Session, error) {
			return &model.Session{ID: "regular-sess", IsSudo: false}, nil
		},
	}
	h := NewSudoHandler(nil, sessions)

	user := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/sudo", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "regular-sess"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.GetSudoStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]interface{}
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["isSudo"] != false {
		t.Errorf("expected isSudo=false, got %v", body["isSudo"])
	}
}

func TestEndSudo_InvalidSession(t *testing.T) {
	sessions := &sudoTestSessionRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.Session, error) {
			return nil, errors.New("session not found")
		},
	}
	h := NewSudoHandler(nil, sessions)

	user := &model.User{ID: 2, Role: model.RoleUser}
	ctx := api.ContextWithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/sudo", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "bad-sess"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.EndSudo(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestEndSudo_RestoreUserFails(t *testing.T) {
	adminID := int64(1)
	sessions := &sudoTestSessionRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.Session, error) {
			return &model.Session{
				ID:             "sudo-sess",
				UserID:         2,
				IsSudo:         true,
				OriginalUserID: &adminID,
			}, nil
		},
	}
	users := &sudoTestUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			return nil, errors.New("user not found")
		},
	}
	h := NewSudoHandler(users, sessions)

	user := &model.User{ID: 2, Role: model.RoleUser}
	ctx := api.ContextWithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/sudo", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "sudo-sess"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.EndSudo(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestEndSudo_CreateSessionFails(t *testing.T) {
	adminID := int64(1)
	adminUser := &model.User{ID: 1, Email: "admin@example.com", Role: model.RoleAdmin}

	sessions := &sudoTestSessionRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.Session, error) {
			return &model.Session{
				ID:             "sudo-sess",
				UserID:         2,
				IsSudo:         true,
				OriginalUserID: &adminID,
			}, nil
		},
		createWithExpiryFn: func(_ context.Context, _ int64, _ time.Time, _ bool) (*model.Session, error) {
			return nil, errors.New("db error")
		},
	}
	users := &sudoTestUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			return adminUser, nil
		},
	}
	h := NewSudoHandler(users, sessions)

	user := &model.User{ID: 2, Role: model.RoleUser}
	ctx := api.ContextWithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/sudo", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "sudo-sess"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.EndSudo(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestSudoHandler_SetAuditLogger(t *testing.T) {
	h := NewSudoHandler(nil, nil)
	if h.audit != nil {
		t.Error("expected audit to be nil initially")
	}
	// SetAuditLogger stores a *audit.Logger.
	logger := sudoAuditLogger
	h.SetAuditLogger(logger)
	if h.audit != logger {
		t.Error("expected audit logger to be set")
	}
}

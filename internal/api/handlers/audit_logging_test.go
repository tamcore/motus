package handlers_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// --- Mock repos that do NOT already exist in other *_mock_test.go files ---
// mockDeviceRepo, mockApiKeyRepo, mockUserRepo, mockSessionRepo are
// defined in device_mock_test.go, apikey_mock_test.go, session_mock_test.go
// respectively and are reused here.

// auditMockNotificationRepo is a test double for repository.NotificationRepo.
type auditMockNotificationRepo struct {
	getByUserFn      func(ctx context.Context, userID int64) ([]*model.NotificationRule, error)
	getByEventTypeFn func(ctx context.Context, userID int64, eventType string) ([]*model.NotificationRule, error)
	getByIDFn        func(ctx context.Context, id int64) (*model.NotificationRule, error)
	createFn         func(ctx context.Context, rule *model.NotificationRule) error
	updateFn         func(ctx context.Context, rule *model.NotificationRule) error
	deleteFn         func(ctx context.Context, id int64) error
	logDeliveryFn    func(ctx context.Context, log *model.NotificationLog) error
	getLogsByRuleFn  func(ctx context.Context, ruleID int64, limit int) ([]*model.NotificationLog, error)
}

var _ repository.NotificationRepo = (*auditMockNotificationRepo)(nil)

func (m *auditMockNotificationRepo) GetByUser(ctx context.Context, userID int64) ([]*model.NotificationRule, error) {
	if m.getByUserFn != nil {
		return m.getByUserFn(ctx, userID)
	}
	return nil, nil
}
func (m *auditMockNotificationRepo) GetByEventType(ctx context.Context, userID int64, eventType string) ([]*model.NotificationRule, error) {
	if m.getByEventTypeFn != nil {
		return m.getByEventTypeFn(ctx, userID, eventType)
	}
	return nil, nil
}
func (m *auditMockNotificationRepo) GetByID(ctx context.Context, id int64) (*model.NotificationRule, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, errors.New("not found")
}
func (m *auditMockNotificationRepo) Create(ctx context.Context, rule *model.NotificationRule) error {
	if m.createFn != nil {
		return m.createFn(ctx, rule)
	}
	rule.ID = 1
	return nil
}
func (m *auditMockNotificationRepo) Update(ctx context.Context, rule *model.NotificationRule) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, rule)
	}
	return nil
}
func (m *auditMockNotificationRepo) Delete(ctx context.Context, id int64) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}
func (m *auditMockNotificationRepo) LogDelivery(ctx context.Context, log *model.NotificationLog) error {
	if m.logDeliveryFn != nil {
		return m.logDeliveryFn(ctx, log)
	}
	return nil
}
func (m *auditMockNotificationRepo) GetLogsByRule(ctx context.Context, ruleID int64, limit int) ([]*model.NotificationLog, error) {
	if m.getLogsByRuleFn != nil {
		return m.getLogsByRuleFn(ctx, ruleID, limit)
	}
	return nil, nil
}

func (m *auditMockNotificationRepo) GetAll(_ context.Context) ([]*model.NotificationRule, error) {
	return nil, nil
}

// auditMockDeviceShareRepo is a test double for repository.DeviceShareRepo.
type auditMockDeviceShareRepo struct {
	createFn       func(ctx context.Context, share *model.DeviceShare) error
	getByIDFn      func(ctx context.Context, id int64) (*model.DeviceShare, error)
	getByTokenFn   func(ctx context.Context, token string) (*model.DeviceShare, error)
	listByDeviceFn func(ctx context.Context, deviceID int64) ([]*model.DeviceShare, error)
	deleteFn       func(ctx context.Context, id int64) error
}

var _ repository.DeviceShareRepo = (*auditMockDeviceShareRepo)(nil)

func (m *auditMockDeviceShareRepo) Create(ctx context.Context, share *model.DeviceShare) error {
	if m.createFn != nil {
		return m.createFn(ctx, share)
	}
	share.ID = 1
	share.Token = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	return nil
}
func (m *auditMockDeviceShareRepo) GetByID(ctx context.Context, id int64) (*model.DeviceShare, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, errors.New("not found")
}
func (m *auditMockDeviceShareRepo) GetByToken(ctx context.Context, token string) (*model.DeviceShare, error) {
	if m.getByTokenFn != nil {
		return m.getByTokenFn(ctx, token)
	}
	return nil, errors.New("not found")
}
func (m *auditMockDeviceShareRepo) ListByDevice(ctx context.Context, deviceID int64) ([]*model.DeviceShare, error) {
	if m.listByDeviceFn != nil {
		return m.listByDeviceFn(ctx, deviceID)
	}
	return nil, nil
}
func (m *auditMockDeviceShareRepo) Delete(ctx context.Context, id int64) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

// auditMockPositionRepo is a minimal test double for repository.PositionRepo.
type auditMockPositionRepo struct {
	getLatestByDeviceFn func(ctx context.Context, deviceID int64) (*model.Position, error)
}

var _ repository.PositionRepo = (*auditMockPositionRepo)(nil)

func (m *auditMockPositionRepo) Create(_ context.Context, _ *model.Position) error { return nil }
func (m *auditMockPositionRepo) GetByID(_ context.Context, _ int64) (*model.Position, error) {
	return nil, errors.New("not found")
}
func (m *auditMockPositionRepo) GetLatestByDevice(ctx context.Context, deviceID int64) (*model.Position, error) {
	if m.getLatestByDeviceFn != nil {
		return m.getLatestByDeviceFn(ctx, deviceID)
	}
	return nil, errors.New("not found")
}
func (m *auditMockPositionRepo) GetLatestByUser(_ context.Context, _ int64) ([]*model.Position, error) {
	return nil, nil
}
func (m *auditMockPositionRepo) GetByDeviceAndTimeRange(_ context.Context, _ int64, _, _ time.Time, _ int) ([]*model.Position, error) {
	return nil, nil
}
func (m *auditMockPositionRepo) GetPreviousByDevice(_ context.Context, _ int64, _ time.Time) (*model.Position, error) {
	return nil, errors.New("not found")
}
func (m *auditMockPositionRepo) GetByIDs(_ context.Context, _ []int64) ([]*model.Position, error) {
	return nil, nil
}
func (m *auditMockPositionRepo) UpdateAddress(_ context.Context, _ int64, _ string) error {
	return nil
}
func (m *auditMockPositionRepo) UpdateGeofenceIDs(_ context.Context, _ int64, _ []int64) error {
	return nil
}
func (m *auditMockPositionRepo) GetLatestAll(_ context.Context) ([]*model.Position, error) {
	return nil, nil
}
func (m *auditMockPositionRepo) GetLastMovingPosition(_ context.Context, _ int64, _ float64) (*model.Position, error) {
	return nil, nil
}

// auditMockGeofenceRepo is a minimal test double for repository.GeofenceRepo.
type auditMockGeofenceRepo struct {
	getByUserFn     func(ctx context.Context, userID int64) ([]*model.Geofence, error)
	getByIDFn       func(ctx context.Context, id int64) (*model.Geofence, error)
	createFn        func(ctx context.Context, g *model.Geofence) error
	updateFn        func(ctx context.Context, g *model.Geofence) error
	deleteFn        func(ctx context.Context, id int64) error
	userHasAccessFn func(ctx context.Context, user *model.User, geofenceID int64) bool
	associateUserFn func(ctx context.Context, userID, geofenceID int64) error
}

var _ repository.GeofenceRepo = (*auditMockGeofenceRepo)(nil)

func (m *auditMockGeofenceRepo) GetByUser(ctx context.Context, userID int64) ([]*model.Geofence, error) {
	if m.getByUserFn != nil {
		return m.getByUserFn(ctx, userID)
	}
	return nil, nil
}
func (m *auditMockGeofenceRepo) GetByID(ctx context.Context, id int64) (*model.Geofence, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, errors.New("not found")
}
func (m *auditMockGeofenceRepo) GetAll(_ context.Context) ([]*model.Geofence, error) {
	return nil, nil
}
func (m *auditMockGeofenceRepo) GetAllWithOwners(_ context.Context) ([]*model.Geofence, error) {
	return nil, nil
}
func (m *auditMockGeofenceRepo) Create(ctx context.Context, g *model.Geofence) error {
	if m.createFn != nil {
		return m.createFn(ctx, g)
	}
	g.ID = 1
	return nil
}
func (m *auditMockGeofenceRepo) Update(ctx context.Context, g *model.Geofence) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, g)
	}
	return nil
}
func (m *auditMockGeofenceRepo) Delete(ctx context.Context, id int64) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}
func (m *auditMockGeofenceRepo) AssociateUser(ctx context.Context, userID, geofenceID int64) error {
	if m.associateUserFn != nil {
		return m.associateUserFn(ctx, userID, geofenceID)
	}
	return nil
}
func (m *auditMockGeofenceRepo) UserHasAccess(ctx context.Context, user *model.User, geofenceID int64) bool {
	if m.userHasAccessFn != nil {
		return m.userHasAccessFn(ctx, user, geofenceID)
	}
	return false
}
func (m *auditMockGeofenceRepo) CheckContainment(_ context.Context, _ int64, _, _ float64) ([]int64, error) {
	return nil, nil
}

// auditMockCalendarRepo is a minimal test double for repository.CalendarRepo.
type auditMockCalendarRepo struct {
	getByUserFn     func(ctx context.Context, userID int64) ([]*model.Calendar, error)
	getByIDFn       func(ctx context.Context, id int64) (*model.Calendar, error)
	createFn        func(ctx context.Context, c *model.Calendar) error
	updateFn        func(ctx context.Context, c *model.Calendar) error
	deleteFn        func(ctx context.Context, id int64) error
	userHasAccessFn func(ctx context.Context, user *model.User, calendarID int64) bool
	associateUserFn func(ctx context.Context, userID, calendarID int64) error
}

var _ repository.CalendarRepo = (*auditMockCalendarRepo)(nil)

func (m *auditMockCalendarRepo) GetByUser(ctx context.Context, userID int64) ([]*model.Calendar, error) {
	if m.getByUserFn != nil {
		return m.getByUserFn(ctx, userID)
	}
	return nil, nil
}
func (m *auditMockCalendarRepo) GetByID(ctx context.Context, id int64) (*model.Calendar, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, errors.New("not found")
}
func (m *auditMockCalendarRepo) Create(ctx context.Context, c *model.Calendar) error {
	if m.createFn != nil {
		return m.createFn(ctx, c)
	}
	c.ID = 1
	return nil
}
func (m *auditMockCalendarRepo) Update(ctx context.Context, c *model.Calendar) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, c)
	}
	return nil
}
func (m *auditMockCalendarRepo) Delete(ctx context.Context, id int64) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}
func (m *auditMockCalendarRepo) UserHasAccess(ctx context.Context, user *model.User, calendarID int64) bool {
	if m.userHasAccessFn != nil {
		return m.userHasAccessFn(ctx, user, calendarID)
	}
	return false
}
func (m *auditMockCalendarRepo) AssociateUser(ctx context.Context, userID, calendarID int64) error {
	if m.associateUserFn != nil {
		return m.associateUserFn(ctx, userID, calendarID)
	}
	return nil
}
func (m *auditMockCalendarRepo) GetAll(_ context.Context) ([]*model.Calendar, error) {
	return nil, nil
}

// --- SetAuditLogger tests ---

// TestDeviceHandler_SetAuditLogger verifies the setter method works.
func TestDeviceHandler_SetAuditLogger(t *testing.T) {
	h := handlers.NewDeviceHandler(&mockDeviceRepo{}, "")

	// Should not panic with nil.
	h.SetAuditLogger(nil)

	// Should accept a real logger (even with nil pool).
	h.SetAuditLogger(audit.NewLogger(nil))
}

// TestGeofenceHandler_SetAuditLogger verifies the setter method works.
func TestGeofenceHandler_SetAuditLogger(t *testing.T) {
	h := handlers.NewGeofenceHandler(&auditMockGeofenceRepo{})

	h.SetAuditLogger(nil)
	h.SetAuditLogger(audit.NewLogger(nil))
}

// TestCalendarHandler_SetAuditLogger verifies the setter method works.
func TestCalendarHandler_SetAuditLogger(t *testing.T) {
	h := handlers.NewCalendarHandler(&auditMockCalendarRepo{})

	h.SetAuditLogger(nil)
	h.SetAuditLogger(audit.NewLogger(nil))
}

// TestNotificationHandler_SetAuditLogger verifies the setter method works.
func TestNotificationHandler_SetAuditLogger(t *testing.T) {
	h := handlers.NewNotificationHandler(&auditMockNotificationRepo{}, nil)

	h.SetAuditLogger(nil)
	h.SetAuditLogger(audit.NewLogger(nil))
}

// TestApiKeyHandler_SetAuditLogger verifies the setter method works.
func TestApiKeyHandler_SetAuditLogger(t *testing.T) {
	h := handlers.NewApiKeyHandler(&mockApiKeyRepo{})

	h.SetAuditLogger(nil)
	h.SetAuditLogger(audit.NewLogger(nil))
}

// TestShareHandler_SetAuditLogger verifies the setter method works.
func TestShareHandler_SetAuditLogger(t *testing.T) {
	h := handlers.NewShareHandler(&auditMockDeviceShareRepo{}, &mockDeviceRepo{}, &auditMockPositionRepo{}, "")

	h.SetAuditLogger(nil)
	h.SetAuditLogger(audit.NewLogger(nil))
}

// TestUserHandler_SetAuditLogger verifies the setter method works.
func TestUserHandler_SetAuditLogger(t *testing.T) {
	h := handlers.NewUserHandler(&mockUserRepo{}, &mockDeviceRepo{}, "")

	h.SetAuditLogger(nil)
	h.SetAuditLogger(audit.NewLogger(nil))
}

// --- Device CRUD with audit logger ---

// TestDeviceHandler_Create_WithNilAudit verifies Create works with nil audit logger.
func TestDeviceHandler_Create_WithNilAudit(t *testing.T) {
	mock := &mockDeviceRepo{
		createFn: func(_ context.Context, d *model.Device, _ int64) error {
			d.ID = 1
			return nil
		},
	}
	h := handlers.NewDeviceHandler(mock, "")
	// Do NOT set audit logger -- it stays nil.

	user := &model.User{ID: 1, Email: "test@example.com"}
	body := `{"uniqueId":"test-001","name":"Test Device"}`
	req := httptest.NewRequest(http.MethodPost, "/api/devices", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestDeviceHandler_Create_WithAuditLogger verifies Create runs with audit logger set
// (audit logger has nil pool, so actual write is a no-op but code path is exercised).
func TestDeviceHandler_Create_WithAuditLogger(t *testing.T) {
	mock := &mockDeviceRepo{
		createFn: func(_ context.Context, d *model.Device, _ int64) error {
			d.ID = 42
			return nil
		},
	}
	h := handlers.NewDeviceHandler(mock, "")
	h.SetAuditLogger(audit.NewLogger(nil))

	user := &model.User{ID: 5, Email: "creator@example.com"}
	body := `{"uniqueId":"new-dev","name":"New Device","protocol":"h02"}`
	req := httptest.NewRequest(http.MethodPost, "/api/devices", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestDeviceHandler_Update_WithAuditLogger exercises the update audit code path.
func TestDeviceHandler_Update_WithAuditLogger(t *testing.T) {
	mock := &mockDeviceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return true },
		getByIDFn: func(_ context.Context, id int64) (*model.Device, error) {
			return &model.Device{ID: id, UniqueID: "dev-1", Name: "Old Name", Protocol: "h02"}, nil
		},
		updateFn: func(_ context.Context, _ *model.Device) error { return nil },
	}
	h := handlers.NewDeviceHandler(mock, "")
	h.SetAuditLogger(audit.NewLogger(nil))

	user := &model.User{ID: 1, Email: "test@example.com"}
	body := `{"name":"New Name","protocol":"watch"}`
	req := httptest.NewRequest(http.MethodPut, "/api/devices/10", bytes.NewReader([]byte(body)))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "10")
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestDeviceHandler_Delete_WithAuditLogger exercises the delete audit code path.
func TestDeviceHandler_Delete_WithAuditLogger(t *testing.T) {
	mock := &mockDeviceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return true },
		deleteFn:        func(_ context.Context, _ int64) error { return nil },
	}
	h := handlers.NewDeviceHandler(mock, "")
	h.SetAuditLogger(audit.NewLogger(nil))

	user := &model.User{ID: 1, Email: "test@example.com"}
	req := httptest.NewRequest(http.MethodDelete, "/api/devices/10", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "10")
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

// --- API key CRUD with audit logger ---

// TestApiKeyHandler_Create_WithAuditLogger exercises the API key create audit path.
func TestApiKeyHandler_Create_WithAuditLogger(t *testing.T) {
	mock := &mockApiKeyRepo{
		createFn: func(_ context.Context, key *model.ApiKey) error {
			key.ID = 1
			key.Token = "test-token-0123456789abcdef0123456789abcdef"
			return nil
		},
	}
	h := handlers.NewApiKeyHandler(mock)
	h.SetAuditLogger(audit.NewLogger(nil))

	user := &model.User{ID: 1, Email: "test@example.com"}
	body := `{"name":"My Key","permissions":"full"}`
	req := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestApiKeyHandler_Delete_WithAuditLogger exercises the API key delete audit path.
func TestApiKeyHandler_Delete_WithAuditLogger(t *testing.T) {
	mock := &mockApiKeyRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.ApiKey, error) {
			return &model.ApiKey{ID: id, UserID: 1, Name: "My Key"}, nil
		},
		deleteFn: func(_ context.Context, _ int64) error { return nil },
	}
	h := handlers.NewApiKeyHandler(mock)
	h.SetAuditLogger(audit.NewLogger(nil))

	user := &model.User{ID: 1, Email: "test@example.com", Role: model.RoleUser}
	req := httptest.NewRequest(http.MethodDelete, "/api/keys/5", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "5")
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// --- Session failed login with audit logger ---

// TestSessionHandler_LoginFailed_UnknownEmail exercises the failed login audit path.
func TestSessionHandler_LoginFailed_UnknownEmail(t *testing.T) {
	users := &mockUserRepo{
		getByEmailFn: func(_ context.Context, _ string) (*model.User, error) {
			return nil, errors.New("not found")
		},
	}
	h := handlers.NewSessionHandler(users, &mockSessionRepo{}, &mockApiKeyRepo{})
	h.SetAuditLogger(audit.NewLogger(nil))

	body := `{"email":"unknown@example.com","password":"wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/api/session", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.1:12345"
	rr := httptest.NewRecorder()

	h.Login(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// TestSessionHandler_LoginFailed_WrongPassword exercises the wrong password audit path.
func TestSessionHandler_LoginFailed_WrongPassword(t *testing.T) {
	// bcrypt hash for "correctpassword"
	users := &mockUserRepo{
		getByEmailFn: func(_ context.Context, _ string) (*model.User, error) {
			return &model.User{
				ID:           1,
				Email:        "test@example.com",
				PasswordHash: "$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ012", // dummy hash
			}, nil
		},
	}
	h := handlers.NewSessionHandler(users, &mockSessionRepo{}, &mockApiKeyRepo{})
	h.SetAuditLogger(audit.NewLogger(nil))

	body := `{"email":"test@example.com","password":"wrongpassword"}`
	req := httptest.NewRequest(http.MethodPost, "/api/session", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.1:54321"
	rr := httptest.NewRecorder()

	h.Login(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// --- User CRUD with audit logger ---

// TestUserHandler_Create_WithAuditLogger exercises the user create audit path.
func TestUserHandler_Create_WithAuditLogger(t *testing.T) {
	users := &mockUserRepo{
		createFn: func(_ context.Context, u *model.User) error {
			u.ID = 10
			return nil
		},
	}
	h := handlers.NewUserHandler(users, &mockDeviceRepo{}, "")
	h.SetAuditLogger(audit.NewLogger(nil))

	admin := &model.User{ID: 1, Email: "admin@example.com", Role: model.RoleAdmin}
	body := `{"email":"newuser@example.com","password":"StrongPass123!","name":"New User","role":"user"}`
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(api.ContextWithUser(req.Context(), admin))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestUserHandler_Update_WithAuditLogger exercises the user update audit path with change tracking.
func TestUserHandler_Update_WithAuditLogger(t *testing.T) {
	users := &mockUserRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.User, error) {
			return &model.User{ID: id, Email: "old@example.com", Name: "Old Name", Role: model.RoleUser}, nil
		},
		updateFn: func(_ context.Context, _ *model.User) error { return nil },
	}
	h := handlers.NewUserHandler(users, &mockDeviceRepo{}, "")
	h.SetAuditLogger(audit.NewLogger(nil))

	admin := &model.User{ID: 1, Email: "admin@example.com", Role: model.RoleAdmin}
	body := `{"email":"new@example.com","name":"New Name","role":"admin"}`
	req := httptest.NewRequest(http.MethodPut, "/api/users/5", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "5")
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), admin), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestUserHandler_Delete_WithAuditLogger exercises the user delete audit path.
func TestUserHandler_Delete_WithAuditLogger(t *testing.T) {
	users := &mockUserRepo{
		deleteFn: func(_ context.Context, _ int64) error { return nil },
	}
	h := handlers.NewUserHandler(users, &mockDeviceRepo{}, "")
	h.SetAuditLogger(audit.NewLogger(nil))

	admin := &model.User{ID: 1, Email: "admin@example.com", Role: model.RoleAdmin}
	req := httptest.NewRequest(http.MethodDelete, "/api/users/5", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "5")
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), admin), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// --- Device assign/unassign with audit logger ---

// TestDeviceAssign_WithAuditLogger exercises the device assign audit path.
func TestDeviceAssign_WithAuditLogger(t *testing.T) {
	users := &mockUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			return &model.User{ID: 5, Email: "user@example.com"}, nil
		},
		assignDeviceFn: func(_ context.Context, _, _ int64) error { return nil },
	}
	devices := &mockDeviceRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.Device, error) {
			return &model.Device{ID: id, UniqueID: "dev-1", Name: "Test"}, nil
		},
	}
	h := handlers.NewUserHandler(users, devices, "")
	h.SetAuditLogger(audit.NewLogger(nil))

	admin := &model.User{ID: 1, Email: "admin@example.com", Role: model.RoleAdmin}
	req := httptest.NewRequest(http.MethodPost, "/api/users/5/devices/10", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "5")
	rctx.URLParams.Add("deviceId", "10")
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), admin), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.AssignDevice(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestDeviceUnassign_WithAuditLogger exercises the device unassign audit path.
func TestDeviceUnassign_WithAuditLogger(t *testing.T) {
	users := &mockUserRepo{
		unassignDeviceFn: func(_ context.Context, _, _ int64) error { return nil },
	}
	h := handlers.NewUserHandler(users, &mockDeviceRepo{}, "")
	h.SetAuditLogger(audit.NewLogger(nil))

	admin := &model.User{ID: 1, Email: "admin@example.com", Role: model.RoleAdmin}
	req := httptest.NewRequest(http.MethodDelete, "/api/users/5/devices/10", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "5")
	rctx.URLParams.Add("deviceId", "10")
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), admin), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.UnassignDevice(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// --- Audit constants test ---

// TestAuditConstants_Comprehensive verifies all new audit constants are non-empty.
func TestAuditConstants_Comprehensive(t *testing.T) {
	actions := []string{
		audit.ActionSessionLogin, audit.ActionSessionLoginFailed, audit.ActionSessionLogout,
		audit.ActionUserCreate, audit.ActionUserUpdate, audit.ActionUserDelete,
		audit.ActionDeviceCreate, audit.ActionDeviceUpdate, audit.ActionDeviceDelete,
		audit.ActionDeviceOnline, audit.ActionDeviceOffline,
		audit.ActionDeviceAssign, audit.ActionDeviceUnassign,
		audit.ActionGeofenceCreate, audit.ActionGeofenceUpdate, audit.ActionGeofenceDelete,
		audit.ActionCalendarCreate, audit.ActionCalendarUpdate, audit.ActionCalendarDelete,
		audit.ActionNotifCreate, audit.ActionNotifUpdate, audit.ActionNotifDelete,
		audit.ActionNotifSent, audit.ActionNotifFailed,
		audit.ActionApiKeyCreate, audit.ActionApiKeyDelete,
		audit.ActionShareCreate, audit.ActionShareDelete,
		audit.ActionSessionSudo, audit.ActionSessionSudoEnd,
	}
	for _, a := range actions {
		if a == "" {
			t.Error("found empty action constant")
		}
	}

	resources := []string{
		audit.ResourceUser, audit.ResourceDevice, audit.ResourceGeofence,
		audit.ResourceCalendar, audit.ResourceNotification, audit.ResourceSession,
		audit.ResourceApiKey, audit.ResourceShare,
	}
	for _, r := range resources {
		if r == "" {
			t.Error("found empty resource type constant")
		}
	}
}

package handlers_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/api/handlers"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// --- Mock repos that do NOT already exist in mocks_test.go ---
// mockDeviceRepo, mockApiKeyRepo, mockUserRepo, mockSessionRepo are defined
// in mocks_test.go (shared across test files) and are reused here.

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
func (m *auditMockPositionRepo) GetByUserAndTimeRange(_ context.Context, _ int64, _, _ time.Time, _ int) ([]*model.Position, error) {
	return nil, nil
}
func (m *auditMockPositionRepo) GetAllByTimeRange(_ context.Context, _, _ time.Time, _ int) ([]*model.Position, error) {
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
func (m *auditMockPositionRepo) StreamByDeviceAndTimeRange(_ context.Context, _ int64, _, _ time.Time, _ int, _ func(*model.Position) error) error {
	return nil
}
func (m *auditMockPositionRepo) StreamByUserAndTimeRange(_ context.Context, _ int64, _, _ time.Time, _ int, _ func(*model.Position) error) error {
	return nil
}
func (m *auditMockPositionRepo) StreamAllByTimeRange(_ context.Context, _, _ time.Time, _ int, _ func(*model.Position) error) error {
	return nil
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
func (m *auditMockGeofenceRepo) CheckContainmentForDevice(_ context.Context, _ int64, _, _ float64) ([]int64, error) {
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

// NOTE: the SetAuditLogger and CRUD audit tests for the deleted chi Device/
// Calendar/Geofence/Notification/Share/User handlers were removed with those
// handlers. The ogen Handler equivalents exercise their audit paths via the
// AuditLogger configured in device_oas_test.go, calendar_oas_test.go,
// geofence_oas_test.go, notification_oas_test.go, share_oas_test.go, and
// users_oas_test.go (the latter asserts persisted entries for
// AdminAssignDevice/AdminUnassignDevice).

// --- Session failed login with audit logger (ogen Handler) ---

// TestLogin_AuditFailedLogin_UnknownEmail exercises the failed login audit
// path. The audit logger has a nil pool, so the write is a no-op but the
// unconditional AuditLogger.Log call in Login is exercised. The persisted
// audit entries are asserted in TestLogin_NonexistentUser_Integration
// (session_oas_test.go).
func TestLogin_AuditFailedLogin_UnknownEmail(t *testing.T) {
	users := &mockUserRepo{
		getByEmailFn: func(_ context.Context, _ string) (*model.User, error) {
			return nil, errors.New("not found")
		},
	}
	h := handlers.NewHandler(handlers.HandlerConfig{
		Users:       users,
		Sessions:    &mockSessionRepo{},
		ApiKeys:     &mockApiKeyRepo{},
		AuditLogger: audit.NewLogger(nil),
	})

	res, err := h.Login(context.Background(), &oas.LoginApplicationJSON{
		Email:    "unknown@example.com",
		Password: "wrong",
	})
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
	if _, ok := res.(*oas.LoginUnauthorized); !ok {
		t.Errorf("expected *oas.LoginUnauthorized, got %T", res)
	}
}

// TestLogin_AuditFailedLogin_WrongPassword exercises the wrong password audit
// path. The persisted audit entries are asserted in
// TestLogin_WrongPassword_Integration (session_oas_test.go).
func TestLogin_AuditFailedLogin_WrongPassword(t *testing.T) {
	users := &mockUserRepo{
		getByEmailFn: func(_ context.Context, _ string) (*model.User, error) {
			return &model.User{
				ID:           1,
				Email:        "test@example.com",
				PasswordHash: "$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ012", // dummy hash
			}, nil
		},
	}
	h := handlers.NewHandler(handlers.HandlerConfig{
		Users:       users,
		Sessions:    &mockSessionRepo{},
		ApiKeys:     &mockApiKeyRepo{},
		AuditLogger: audit.NewLogger(nil),
	})

	res, err := h.Login(context.Background(), &oas.LoginApplicationJSON{
		Email:    "test@example.com",
		Password: "wrongpassword",
	})
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
	if _, ok := res.(*oas.LoginUnauthorized); !ok {
		t.Errorf("expected *oas.LoginUnauthorized, got %T", res)
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

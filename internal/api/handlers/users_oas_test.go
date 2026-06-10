package handlers_test

// Tests for the ogen Handler admin user-management methods (AdminListUsers,
// AdminCreateUser, AdminUpdateUser, AdminDeleteUser, AdminListDevices,
// AdminListUserDevices, AdminAssignDevice, AdminUnassignDevice).
//
// Ported from the deleted chi UserHandler test files: users_test.go, the
// user half of validation_test.go, and the user/device-assignment audit
// tests from audit_logging_test.go (TestUserHandler_*_WithAuditLogger,
// TestDeviceAssign_WithAuditLogger, TestDeviceUnassign_WithAuditLogger).
//
// Dropped tests (no live equivalent):
//   - invalid/missing path-ID transport tests: ogen owns path-param parsing.
//   - invalid-JSON body tests: ogen owns request decoding.
//   - "missing password rejected" on create: the live API intentionally
//     allows passwordless users (OIDC-only accounts); only set-but-invalid
//     passwords are rejected.
//
// Behavioural deltas vs the chi handler, asserted as the live contract:
//   - delete-self returns 403 (chi returned 400).
//   - AdminAssignDevice surfaces nonexistent user/device as 404 via the
//     repository FK error instead of pre-checking existence.
//
// The hub cache-invalidation tests cover the bug fix where
// AdminAssignDevice/AdminUnassignDevice previously never evicted the
// WebSocket hub's per-device access cache (the dead chi handler did, via
// DeviceCacheInvalidator).

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
	"github.com/tamcore/motus/internal/websocket"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newUsersTestHandler builds an ogen Handler over mock repositories. The
// nil-pool audit logger exercises the audit code paths as documented no-ops.
func newUsersTestHandler(users repository.UserRepo, devices repository.DeviceRepo, hub *websocket.Hub) *handlers.Handler {
	return handlers.NewHandler(handlers.HandlerConfig{
		Users:       users,
		Devices:     devices,
		Sessions:    &mockSessionRepo{},
		ApiKeys:     &mockApiKeyRepo{},
		Hub:         hub,
		AuditLogger: audit.NewLogger(nil),
	})
}

func usersTestAdminCtx() context.Context {
	return api.ContextWithUser(context.Background(),
		&model.User{ID: 1, Email: "admin@example.com", Role: model.RoleAdmin})
}

func usersTestRegularCtx() context.Context {
	return api.ContextWithUser(context.Background(),
		&model.User{ID: 999, Email: "user@example.com", Role: model.RoleUser})
}

// ---------------------------------------------------------------------------
// Hub device-access cache invalidation (bug fix, TDD)
// ---------------------------------------------------------------------------

// countingAccessChecker counts how often the hub asks the database for the
// access list of a device. A second lookup without invalidation must be
// served from the hub's cache (count stays flat); after InvalidateDevice
// the next broadcast must hit the checker again.
type countingAccessChecker struct {
	mu    sync.Mutex
	calls int
}

func (c *countingAccessChecker) GetUserIDs(_ context.Context, _ int64) ([]int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	return []int64{1}, nil
}

func (c *countingAccessChecker) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// seedHubDeviceCache primes the hub's per-device access cache and verifies
// that a subsequent broadcast is served from the cache.
func seedHubDeviceCache(t *testing.T, hub *websocket.Hub, checker *countingAccessChecker, deviceID int64) {
	t.Helper()
	hub.BroadcastDeviceStatus(&model.Device{ID: deviceID})
	if got := checker.count(); got != 1 {
		t.Fatalf("expected 1 access lookup after first broadcast, got %d", got)
	}
	hub.BroadcastDeviceStatus(&model.Device{ID: deviceID})
	if got := checker.count(); got != 1 {
		t.Fatalf("expected cached access list on second broadcast (1 lookup), got %d", got)
	}
}

func TestAdminAssignDevice_InvalidatesHubDeviceCache(t *testing.T) {
	const deviceID int64 = 42
	checker := &countingAccessChecker{}
	hub := websocket.NewHub(nil, checker, nil)
	seedHubDeviceCache(t, hub, checker, deviceID)

	users := &mockUserRepo{
		assignDeviceFn: func(_ context.Context, _, _ int64) error { return nil },
	}
	h := newUsersTestHandler(users, &mockDeviceRepo{}, hub)

	res, err := h.AdminAssignDevice(usersTestAdminCtx(), oas.AdminAssignDeviceParams{ID: 7, DeviceId: deviceID})
	if err != nil {
		t.Fatalf("AdminAssignDevice returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminAssignDeviceNoContent); !ok {
		t.Fatalf("expected *oas.AdminAssignDeviceNoContent, got %T", res)
	}

	// The assignment must have evicted the cached access list: the next
	// broadcast has to query the access checker again.
	hub.BroadcastDeviceStatus(&model.Device{ID: deviceID})
	if got := checker.count(); got != 2 {
		t.Errorf("expected access cache invalidated after AdminAssignDevice (2 lookups), got %d", got)
	}
}

func TestAdminUnassignDevice_InvalidatesHubDeviceCache(t *testing.T) {
	const deviceID int64 = 43
	checker := &countingAccessChecker{}
	hub := websocket.NewHub(nil, checker, nil)
	seedHubDeviceCache(t, hub, checker, deviceID)

	users := &mockUserRepo{
		unassignDeviceFn: func(_ context.Context, _, _ int64) error { return nil },
	}
	h := newUsersTestHandler(users, &mockDeviceRepo{}, hub)

	res, err := h.AdminUnassignDevice(usersTestAdminCtx(), oas.AdminUnassignDeviceParams{ID: 7, DeviceId: deviceID})
	if err != nil {
		t.Fatalf("AdminUnassignDevice returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminUnassignDeviceNoContent); !ok {
		t.Fatalf("expected *oas.AdminUnassignDeviceNoContent, got %T", res)
	}

	hub.BroadcastDeviceStatus(&model.Device{ID: deviceID})
	if got := checker.count(); got != 2 {
		t.Errorf("expected access cache invalidated after AdminUnassignDevice (2 lookups), got %d", got)
	}
}

// TestAdminAssignDevice_NilHubSafe ensures the invalidation is nil-guarded:
// handlers constructed without a hub (tests, tools) must not panic.
func TestAdminAssignDevice_NilHubSafe(t *testing.T) {
	users := &mockUserRepo{
		assignDeviceFn:   func(_ context.Context, _, _ int64) error { return nil },
		unassignDeviceFn: func(_ context.Context, _, _ int64) error { return nil },
	}
	h := newUsersTestHandler(users, &mockDeviceRepo{}, nil)

	if _, err := h.AdminAssignDevice(usersTestAdminCtx(), oas.AdminAssignDeviceParams{ID: 1, DeviceId: 2}); err != nil {
		t.Fatalf("AdminAssignDevice with nil hub returned error: %v", err)
	}
	if _, err := h.AdminUnassignDevice(usersTestAdminCtx(), oas.AdminUnassignDeviceParams{ID: 1, DeviceId: 2}); err != nil {
		t.Fatalf("AdminUnassignDevice with nil hub returned error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// AdminListUsers (ported from TestUserHandler_List_AdminOnly)
// ---------------------------------------------------------------------------

func TestAdminListUsers_AdminOnly(t *testing.T) {
	users := &mockUserRepo{
		listAllFn: func(_ context.Context) ([]*model.User, error) {
			return []*model.User{
				{ID: 1, Email: "admin@test.com", Role: model.RoleAdmin},
				{ID: 2, Email: "user@test.com", Role: model.RoleUser},
			}, nil
		},
	}
	h := newUsersTestHandler(users, &mockDeviceRepo{}, nil)

	t.Run("admin can list", func(t *testing.T) {
		res, err := h.AdminListUsers(usersTestAdminCtx())
		if err != nil {
			t.Fatalf("AdminListUsers returned error: %v", err)
		}
		list, ok := res.(*oas.AdminListUsersOKApplicationJSON)
		if !ok {
			t.Fatalf("expected *oas.AdminListUsersOKApplicationJSON, got %T", res)
		}
		if len(*list) != 2 {
			t.Fatalf("expected 2 users, got %d", len(*list))
		}
		// PopulateTraccarFields must derive the administrator flag from role.
		if !(*list)[0].Administrator {
			t.Error("expected administrator=true for admin role")
		}
		if (*list)[1].Administrator {
			t.Error("expected administrator=false for user role")
		}
	})

	t.Run("non-admin forbidden", func(t *testing.T) {
		res, err := h.AdminListUsers(usersTestRegularCtx())
		if err != nil {
			t.Fatalf("AdminListUsers returned error: %v", err)
		}
		if _, ok := res.(*oas.AdminListUsersForbidden); !ok {
			t.Errorf("expected *oas.AdminListUsersForbidden, got %T", res)
		}
	})

	t.Run("no user forbidden", func(t *testing.T) {
		res, err := h.AdminListUsers(context.Background())
		if err != nil {
			t.Fatalf("AdminListUsers returned error: %v", err)
		}
		if _, ok := res.(*oas.AdminListUsersForbidden); !ok {
			t.Errorf("expected *oas.AdminListUsersForbidden, got %T", res)
		}
	})
}

// ---------------------------------------------------------------------------
// AdminCreateUser (ported from TestUserHandler_Create and the user half of
// validation_test.go: TestUserCreate_InvalidEmail/InvalidPassword/InvalidName/
// ValidInput)
// ---------------------------------------------------------------------------

func newCreateUserMock() (*mockUserRepo, *[]*model.User) {
	var created []*model.User
	users := &mockUserRepo{
		createFn: func(_ context.Context, u *model.User) error {
			u.ID = int64(len(created) + 10)
			created = append(created, u)
			return nil
		},
	}
	return users, &created
}

func TestAdminCreateUser_Success(t *testing.T) {
	users, created := newCreateUserMock()
	h := newUsersTestHandler(users, &mockDeviceRepo{}, nil)

	res, err := h.AdminCreateUser(usersTestAdminCtx(), &oas.UserInput{
		Email:    "new@test.com",
		Name:     "New User",
		Password: oas.NewOptString("secret123"),
		Role:     oas.NewOptUserInputRole(oas.UserInputRoleUser),
	})
	if err != nil {
		t.Fatalf("AdminCreateUser returned error: %v", err)
	}
	user, ok := res.(*oas.User)
	if !ok {
		t.Fatalf("expected *oas.User, got %T", res)
	}
	if user.Email != "new@test.com" {
		t.Errorf("expected email 'new@test.com', got %q", user.Email)
	}
	if user.Administrator || user.Readonly {
		t.Errorf("expected administrator=false and readonly=false for user role, got admin=%v readonly=%v",
			user.Administrator, user.Readonly)
	}
	if user.ID == 0 {
		t.Error("expected non-zero user ID")
	}
	if len(*created) != 1 {
		t.Fatalf("expected 1 user persisted, got %d", len(*created))
	}
	if (*created)[0].PasswordHash == "" {
		t.Error("expected password hash to be set")
	}
}

func TestAdminCreateUser_DefaultRole(t *testing.T) {
	users, created := newCreateUserMock()
	h := newUsersTestHandler(users, &mockDeviceRepo{}, nil)

	res, err := h.AdminCreateUser(usersTestAdminCtx(), &oas.UserInput{
		Email:    "default@test.com",
		Name:     "Default Role",
		Password: oas.NewOptString("secret123"),
	})
	if err != nil {
		t.Fatalf("AdminCreateUser returned error: %v", err)
	}
	user, ok := res.(*oas.User)
	if !ok {
		t.Fatalf("expected *oas.User, got %T", res)
	}
	if user.Administrator {
		t.Error("expected administrator=false for default role")
	}
	if len(*created) != 1 || (*created)[0].Role != model.RoleUser {
		t.Errorf("expected default role %q persisted", model.RoleUser)
	}
}

func TestAdminCreateUser_InvalidEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
	}{
		{"missing email", ""},
		{"no at sign", "userexample.com"},
		{"no domain", "user@"},
		{"no tld", "user@example"},
		{"spaces", "user @example.com"},
		{"angle brackets", "user<script>@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newUsersTestHandler(&mockUserRepo{}, &mockDeviceRepo{}, nil)
			res, err := h.AdminCreateUser(usersTestAdminCtx(), &oas.UserInput{
				Email:    tt.email,
				Name:     "Test",
				Password: oas.NewOptString("validpassword123"),
			})
			if err != nil {
				t.Fatalf("AdminCreateUser returned error: %v", err)
			}
			if _, ok := res.(*oas.AdminCreateUserBadRequest); !ok {
				t.Errorf("expected *oas.AdminCreateUserBadRequest for email %q, got %T", tt.email, res)
			}
		})
	}
}

func TestAdminCreateUser_InvalidPassword(t *testing.T) {
	h := newUsersTestHandler(&mockUserRepo{}, &mockDeviceRepo{}, nil)

	res, err := h.AdminCreateUser(usersTestAdminCtx(), &oas.UserInput{
		Email:    "valid@example.com",
		Name:     "Test",
		Password: oas.NewOptString("1234567"), // too short
	})
	if err != nil {
		t.Fatalf("AdminCreateUser returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminCreateUserBadRequest); !ok {
		t.Errorf("expected *oas.AdminCreateUserBadRequest for short password, got %T", res)
	}
}

func TestAdminCreateUser_InvalidName(t *testing.T) {
	h := newUsersTestHandler(&mockUserRepo{}, &mockDeviceRepo{}, nil)

	res, err := h.AdminCreateUser(usersTestAdminCtx(), &oas.UserInput{
		Email:    "nametest@example.com",
		Name:     "name<script>alert(1)</script>",
		Password: oas.NewOptString("validpassword123"),
	})
	if err != nil {
		t.Fatalf("AdminCreateUser returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminCreateUserBadRequest); !ok {
		t.Errorf("expected *oas.AdminCreateUserBadRequest for XSS name, got %T", res)
	}
}

func TestAdminCreateUser_InvalidRole(t *testing.T) {
	h := newUsersTestHandler(&mockUserRepo{}, &mockDeviceRepo{}, nil)

	res, err := h.AdminCreateUser(usersTestAdminCtx(), &oas.UserInput{
		Email:    "bad@test.com",
		Name:     "Bad Role",
		Password: oas.NewOptString("secret123"),
		Role:     oas.OptUserInputRole{Value: "superadmin", Set: true},
	})
	if err != nil {
		t.Fatalf("AdminCreateUser returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminCreateUserBadRequest); !ok {
		t.Errorf("expected *oas.AdminCreateUserBadRequest for invalid role, got %T", res)
	}
}

func TestAdminCreateUser_NonAdminForbidden(t *testing.T) {
	h := newUsersTestHandler(&mockUserRepo{}, &mockDeviceRepo{}, nil)

	res, err := h.AdminCreateUser(usersTestRegularCtx(), &oas.UserInput{
		Email:    "x@test.com",
		Name:     "X",
		Password: oas.NewOptString("secret123"),
	})
	if err != nil {
		t.Fatalf("AdminCreateUser returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminCreateUserForbidden); !ok {
		t.Errorf("expected *oas.AdminCreateUserForbidden, got %T", res)
	}
}

// ---------------------------------------------------------------------------
// AdminUpdateUser (ported from TestUserHandler_Update and
// TestUserUpdate_InvalidEmail)
// ---------------------------------------------------------------------------

// newUpdateUserMock returns a user repo whose GetByID serves a regular target
// user (ID 5) and accepts updates.
func newUpdateUserMock() *mockUserRepo {
	return &mockUserRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.User, error) {
			if id == 5 {
				return &model.User{ID: 5, Email: "target@test.com", Name: "Target", Role: model.RoleUser}, nil
			}
			return nil, errors.New("not found")
		},
		updateFn: func(_ context.Context, _ *model.User) error { return nil },
	}
}

func TestAdminUpdateUser_NameAndRole(t *testing.T) {
	h := newUsersTestHandler(newUpdateUserMock(), &mockDeviceRepo{}, nil)

	res, err := h.AdminUpdateUser(usersTestAdminCtx(), &oas.UserInput{
		Name: "Updated Name",
		Role: oas.NewOptUserInputRole(oas.UserInputRoleReadonly),
	}, oas.AdminUpdateUserParams{ID: 5})
	if err != nil {
		t.Fatalf("AdminUpdateUser returned error: %v", err)
	}
	user, ok := res.(*oas.User)
	if !ok {
		t.Fatalf("expected *oas.User, got %T", res)
	}
	if user.Name != "Updated Name" {
		t.Errorf("expected name 'Updated Name', got %q", user.Name)
	}
	if !user.Readonly || user.Administrator {
		t.Errorf("expected readonly=true and administrator=false, got readonly=%v admin=%v",
			user.Readonly, user.Administrator)
	}
}

func TestAdminUpdateUser_Password_RevokesSessions(t *testing.T) {
	var passwordUpdated bool
	var sessionsRevokedFor int64
	users := newUpdateUserMock()
	users.updatePasswordFn = func(_ context.Context, _ int64, hash string) error {
		passwordUpdated = hash != ""
		return nil
	}
	sessions := &mockSessionRepo{
		deleteAllByUserFn: func(_ context.Context, userID int64, _ string) error {
			sessionsRevokedFor = userID
			return nil
		},
	}
	h := handlers.NewHandler(handlers.HandlerConfig{
		Users:       users,
		Sessions:    sessions,
		AuditLogger: audit.NewLogger(nil),
	})

	res, err := h.AdminUpdateUser(usersTestAdminCtx(), &oas.UserInput{
		Email:    "target@test.com",
		Password: oas.NewOptString("newpassword123"),
	}, oas.AdminUpdateUserParams{ID: 5})
	if err != nil {
		t.Fatalf("AdminUpdateUser returned error: %v", err)
	}
	if _, ok := res.(*oas.User); !ok {
		t.Fatalf("expected *oas.User, got %T", res)
	}
	if !passwordUpdated {
		t.Error("expected UpdatePassword to be called with a non-empty hash")
	}
	// An admin password reset must log out whoever holds the old credentials.
	if sessionsRevokedFor != 5 {
		t.Errorf("expected sessions revoked for user 5, got %d", sessionsRevokedFor)
	}
}

func TestAdminUpdateUser_CannotDemoteSelf(t *testing.T) {
	users := &mockUserRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.User, error) {
			// The admin from usersTestAdminCtx (ID 1) updating themselves.
			return &model.User{ID: 1, Email: "admin@example.com", Role: model.RoleAdmin}, nil
		},
	}
	h := newUsersTestHandler(users, &mockDeviceRepo{}, nil)

	res, err := h.AdminUpdateUser(usersTestAdminCtx(), &oas.UserInput{
		Role: oas.NewOptUserInputRole(oas.UserInputRoleUser),
	}, oas.AdminUpdateUserParams{ID: 1})
	if err != nil {
		t.Fatalf("AdminUpdateUser returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminUpdateUserBadRequest); !ok {
		t.Errorf("expected *oas.AdminUpdateUserBadRequest for self-demotion, got %T", res)
	}
}

func TestAdminUpdateUser_NotFound(t *testing.T) {
	h := newUsersTestHandler(newUpdateUserMock(), &mockDeviceRepo{}, nil)

	res, err := h.AdminUpdateUser(usersTestAdminCtx(), &oas.UserInput{Name: "X"},
		oas.AdminUpdateUserParams{ID: 99999})
	if err != nil {
		t.Fatalf("AdminUpdateUser returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminUpdateUserNotFound); !ok {
		t.Errorf("expected *oas.AdminUpdateUserNotFound, got %T", res)
	}
}

func TestAdminUpdateUser_InvalidRole(t *testing.T) {
	h := newUsersTestHandler(newUpdateUserMock(), &mockDeviceRepo{}, nil)

	res, err := h.AdminUpdateUser(usersTestAdminCtx(), &oas.UserInput{
		Role: oas.OptUserInputRole{Value: "superadmin", Set: true},
	}, oas.AdminUpdateUserParams{ID: 5})
	if err != nil {
		t.Fatalf("AdminUpdateUser returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminUpdateUserBadRequest); !ok {
		t.Errorf("expected *oas.AdminUpdateUserBadRequest for invalid role, got %T", res)
	}
}

func TestAdminUpdateUser_InvalidEmail(t *testing.T) {
	h := newUsersTestHandler(newUpdateUserMock(), &mockDeviceRepo{}, nil)

	res, err := h.AdminUpdateUser(usersTestAdminCtx(), &oas.UserInput{
		Email: "invalid-email",
	}, oas.AdminUpdateUserParams{ID: 5})
	if err != nil {
		t.Fatalf("AdminUpdateUser returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminUpdateUserBadRequest); !ok {
		t.Errorf("expected *oas.AdminUpdateUserBadRequest for invalid email, got %T", res)
	}
}

// ---------------------------------------------------------------------------
// AdminDeleteUser (ported from TestUserHandler_Delete)
// ---------------------------------------------------------------------------

func TestAdminDeleteUser_Success(t *testing.T) {
	var deletedID int64
	users := &mockUserRepo{
		deleteFn: func(_ context.Context, id int64) error {
			deletedID = id
			return nil
		},
	}
	h := newUsersTestHandler(users, &mockDeviceRepo{}, nil)

	res, err := h.AdminDeleteUser(usersTestAdminCtx(), oas.AdminDeleteUserParams{ID: 5})
	if err != nil {
		t.Fatalf("AdminDeleteUser returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminDeleteUserNoContent); !ok {
		t.Fatalf("expected *oas.AdminDeleteUserNoContent, got %T", res)
	}
	if deletedID != 5 {
		t.Errorf("expected delete called with ID=5, got %d", deletedID)
	}
}

func TestAdminDeleteUser_CannotDeleteSelf(t *testing.T) {
	h := newUsersTestHandler(&mockUserRepo{}, &mockDeviceRepo{}, nil)

	// Admin from usersTestAdminCtx has ID 1.
	res, err := h.AdminDeleteUser(usersTestAdminCtx(), oas.AdminDeleteUserParams{ID: 1})
	if err != nil {
		t.Fatalf("AdminDeleteUser returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminDeleteUserForbidden); !ok {
		t.Errorf("expected *oas.AdminDeleteUserForbidden for self-deletion, got %T", res)
	}
}

func TestAdminDeleteUser_NonAdminForbidden(t *testing.T) {
	h := newUsersTestHandler(&mockUserRepo{}, &mockDeviceRepo{}, nil)

	res, err := h.AdminDeleteUser(usersTestRegularCtx(), oas.AdminDeleteUserParams{ID: 5})
	if err != nil {
		t.Fatalf("AdminDeleteUser returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminDeleteUserForbidden); !ok {
		t.Errorf("expected *oas.AdminDeleteUserForbidden, got %T", res)
	}
}

// ---------------------------------------------------------------------------
// AdminListDevices / AdminListUserDevices (ported from
// TestUserHandler_AdminListAllDevices_* and the ListDevices subtests of
// TestUserHandler_DeviceAssignment)
// ---------------------------------------------------------------------------

func TestAdminListDevices_Success(t *testing.T) {
	devices := &mockDeviceRepo{
		getAllFn: func(_ context.Context) ([]model.Device, error) {
			return []model.Device{
				{ID: 1, UniqueID: "admin-dev-0", Name: "Device 0", Status: "online"},
				{ID: 2, UniqueID: "admin-dev-1", Name: "Device 1", Status: "online"},
			}, nil
		},
	}
	h := newUsersTestHandler(&mockUserRepo{}, devices, nil)

	res, err := h.AdminListDevices(usersTestAdminCtx())
	if err != nil {
		t.Fatalf("AdminListDevices returned error: %v", err)
	}
	list, ok := res.(*oas.AdminListDevicesOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.AdminListDevicesOKApplicationJSON, got %T", res)
	}
	if len(*list) != 2 {
		t.Errorf("expected 2 devices, got %d", len(*list))
	}
}

func TestAdminListDevices_NonAdminForbidden(t *testing.T) {
	h := newUsersTestHandler(&mockUserRepo{}, &mockDeviceRepo{}, nil)

	res, err := h.AdminListDevices(usersTestRegularCtx())
	if err != nil {
		t.Fatalf("AdminListDevices returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminListDevicesForbidden); !ok {
		t.Errorf("expected *oas.AdminListDevicesForbidden, got %T", res)
	}
}

func TestAdminListUserDevices_Success(t *testing.T) {
	devices := &mockDeviceRepo{
		getByUserFn: func(_ context.Context, userID int64) ([]*model.Device, error) {
			if userID != 5 {
				t.Errorf("expected lookup for user 5, got %d", userID)
			}
			return []*model.Device{{ID: 10, UniqueID: "assign-001", Name: "Test Device"}}, nil
		},
	}
	h := newUsersTestHandler(&mockUserRepo{}, devices, nil)

	res, err := h.AdminListUserDevices(usersTestAdminCtx(), oas.AdminListUserDevicesParams{ID: 5})
	if err != nil {
		t.Fatalf("AdminListUserDevices returned error: %v", err)
	}
	list, ok := res.(*oas.AdminListUserDevicesOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.AdminListUserDevicesOKApplicationJSON, got %T", res)
	}
	if len(*list) != 1 {
		t.Errorf("expected 1 device, got %d", len(*list))
	}
}

func TestAdminListUserDevices_Empty(t *testing.T) {
	h := newUsersTestHandler(&mockUserRepo{}, &mockDeviceRepo{}, nil)

	res, err := h.AdminListUserDevices(usersTestAdminCtx(), oas.AdminListUserDevicesParams{ID: 5})
	if err != nil {
		t.Fatalf("AdminListUserDevices returned error: %v", err)
	}
	list, ok := res.(*oas.AdminListUserDevicesOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.AdminListUserDevicesOKApplicationJSON, got %T", res)
	}
	if len(*list) != 0 {
		t.Errorf("expected 0 devices, got %d", len(*list))
	}
}

func TestAdminListUserDevices_NonAdminForbidden(t *testing.T) {
	h := newUsersTestHandler(&mockUserRepo{}, &mockDeviceRepo{}, nil)

	res, err := h.AdminListUserDevices(usersTestRegularCtx(), oas.AdminListUserDevicesParams{ID: 5})
	if err != nil {
		t.Fatalf("AdminListUserDevices returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminListUserDevicesForbidden); !ok {
		t.Errorf("expected *oas.AdminListUserDevicesForbidden, got %T", res)
	}
}

// ---------------------------------------------------------------------------
// AdminAssignDevice / AdminUnassignDevice error paths
// ---------------------------------------------------------------------------

func TestAdminAssignDevice_RepoErrorIsNotFound(t *testing.T) {
	users := &mockUserRepo{
		assignDeviceFn: func(_ context.Context, _, _ int64) error {
			return errors.New("foreign key violation")
		},
	}
	h := newUsersTestHandler(users, &mockDeviceRepo{}, nil)

	res, err := h.AdminAssignDevice(usersTestAdminCtx(), oas.AdminAssignDeviceParams{ID: 99999, DeviceId: 1})
	if err != nil {
		t.Fatalf("AdminAssignDevice returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminAssignDeviceNotFound); !ok {
		t.Errorf("expected *oas.AdminAssignDeviceNotFound, got %T", res)
	}
}

func TestAdminAssignDevice_NonAdminForbidden(t *testing.T) {
	h := newUsersTestHandler(&mockUserRepo{}, &mockDeviceRepo{}, nil)

	res, err := h.AdminAssignDevice(usersTestRegularCtx(), oas.AdminAssignDeviceParams{ID: 5, DeviceId: 10})
	if err != nil {
		t.Fatalf("AdminAssignDevice returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminAssignDeviceForbidden); !ok {
		t.Errorf("expected *oas.AdminAssignDeviceForbidden, got %T", res)
	}
}

func TestAdminUnassignDevice_NonAdminForbidden(t *testing.T) {
	h := newUsersTestHandler(&mockUserRepo{}, &mockDeviceRepo{}, nil)

	res, err := h.AdminUnassignDevice(usersTestRegularCtx(), oas.AdminUnassignDeviceParams{ID: 5, DeviceId: 10})
	if err != nil {
		t.Fatalf("AdminUnassignDevice returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminUnassignDeviceForbidden); !ok {
		t.Errorf("expected *oas.AdminUnassignDeviceForbidden, got %T", res)
	}
}

// ---------------------------------------------------------------------------
// Integration tests (require Docker, skipped in -short mode by
// testutil.SetupTestDB). Ported from TestUserHandler_DeviceAssignment plus
// the audit paths of TestDeviceAssign_WithAuditLogger /
// TestDeviceUnassign_WithAuditLogger, upgraded to assert persisted entries.
// ---------------------------------------------------------------------------

type usersOASIntegrationEnv struct {
	handler     *handlers.Handler
	userRepo    *repository.UserRepository
	deviceRepo  *repository.DeviceRepository
	auditLogger *audit.Logger
	admin       *model.User
}

func setupUsersOASIntegration(t *testing.T) *usersOASIntegrationEnv {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	auditLogger := audit.NewLogger(pool)

	admin := &model.User{
		Email:        "admin@test.com",
		PasswordHash: "$2a$10$fakehash",
		Name:         "Admin",
		Role:         model.RoleAdmin,
	}
	if err := userRepo.Create(context.Background(), admin); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	h := handlers.NewHandler(handlers.HandlerConfig{
		Users:       userRepo,
		Devices:     deviceRepo,
		Sessions:    sessionRepo,
		AuditLogger: auditLogger,
	})
	return &usersOASIntegrationEnv{
		handler:     h,
		userRepo:    userRepo,
		deviceRepo:  deviceRepo,
		auditLogger: auditLogger,
		admin:       admin,
	}
}

func (e *usersOASIntegrationEnv) adminCtx() context.Context {
	return api.ContextWithUser(context.Background(), e.admin)
}

func TestAdminAssignDevice_AuditLogged_Integration(t *testing.T) {
	env := setupUsersOASIntegration(t)
	ctx := context.Background()

	target := &model.User{Email: "devuser@test.com", PasswordHash: "hash", Name: "Dev User", Role: model.RoleUser}
	if err := env.userRepo.Create(ctx, target); err != nil {
		t.Fatalf("create target: %v", err)
	}
	device := &model.Device{UniqueID: "assign-001", Name: "Test Device", Status: "unknown"}
	if err := env.deviceRepo.Create(ctx, device, env.admin.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	res, err := env.handler.AdminAssignDevice(env.adminCtx(),
		oas.AdminAssignDeviceParams{ID: target.ID, DeviceId: device.ID})
	if err != nil {
		t.Fatalf("AdminAssignDevice returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminAssignDeviceNoContent); !ok {
		t.Fatalf("expected *oas.AdminAssignDeviceNoContent, got %T", res)
	}

	// The device must now be assigned to the target user.
	assigned, err := env.deviceRepo.GetByUser(ctx, target.ID)
	if err != nil {
		t.Fatalf("GetByUser: %v", err)
	}
	if len(assigned) != 1 || assigned[0].ID != device.ID {
		t.Errorf("expected device %d assigned to user %d, got %v", device.ID, target.ID, assigned)
	}

	// The assignment must be audit-logged with the acting admin, the device
	// as the resource, and the target user in the details.
	entries, total, err := env.auditLogger.Query(ctx, audit.QueryParams{Action: audit.ActionDeviceAssign})
	if err != nil {
		t.Fatalf("query audit log: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 device.assign audit entry, got %d", total)
	}
	entry := entries[0]
	if entry.UserID == nil || *entry.UserID != env.admin.ID {
		t.Errorf("expected audit entry for admin %d, got %v", env.admin.ID, entry.UserID)
	}
	if entry.ResourceID == nil || *entry.ResourceID != device.ID {
		t.Errorf("expected audit resource ID %d, got %v", device.ID, entry.ResourceID)
	}
	if got, _ := entry.Details["userId"].(float64); int64(got) != target.ID {
		t.Errorf("expected audit details userId=%d, got %v", target.ID, entry.Details["userId"])
	}

	// Assigning the same device again is idempotent.
	res, err = env.handler.AdminAssignDevice(env.adminCtx(),
		oas.AdminAssignDeviceParams{ID: target.ID, DeviceId: device.ID})
	if err != nil {
		t.Fatalf("AdminAssignDevice (repeat) returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminAssignDeviceNoContent); !ok {
		t.Errorf("expected idempotent re-assign to return NoContent, got %T", res)
	}
}

func TestAdminUnassignDevice_AuditLogged_Integration(t *testing.T) {
	env := setupUsersOASIntegration(t)
	ctx := context.Background()

	target := &model.User{Email: "unassign@test.com", PasswordHash: "hash", Name: "Unassign User", Role: model.RoleUser}
	if err := env.userRepo.Create(ctx, target); err != nil {
		t.Fatalf("create target: %v", err)
	}
	device := &model.Device{UniqueID: "unassign-001", Name: "Test Device", Status: "unknown"}
	if err := env.deviceRepo.Create(ctx, device, env.admin.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}
	if err := env.userRepo.AssignDevice(ctx, target.ID, device.ID); err != nil {
		t.Fatalf("assign device: %v", err)
	}

	res, err := env.handler.AdminUnassignDevice(env.adminCtx(),
		oas.AdminUnassignDeviceParams{ID: target.ID, DeviceId: device.ID})
	if err != nil {
		t.Fatalf("AdminUnassignDevice returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminUnassignDeviceNoContent); !ok {
		t.Fatalf("expected *oas.AdminUnassignDeviceNoContent, got %T", res)
	}

	// The device must no longer be assigned to the target user.
	assigned, err := env.deviceRepo.GetByUser(ctx, target.ID)
	if err != nil {
		t.Fatalf("GetByUser: %v", err)
	}
	if len(assigned) != 0 {
		t.Errorf("expected no devices assigned after unassign, got %d", len(assigned))
	}

	entries, total, err := env.auditLogger.Query(ctx, audit.QueryParams{Action: audit.ActionDeviceUnassign})
	if err != nil {
		t.Fatalf("query audit log: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 device.unassign audit entry, got %d", total)
	}
	entry := entries[0]
	if entry.UserID == nil || *entry.UserID != env.admin.ID {
		t.Errorf("expected audit entry for admin %d, got %v", env.admin.ID, entry.UserID)
	}
	if entry.ResourceID == nil || *entry.ResourceID != device.ID {
		t.Errorf("expected audit resource ID %d, got %v", device.ID, entry.ResourceID)
	}
	if got, _ := entry.Details["userId"].(float64); int64(got) != target.ID {
		t.Errorf("expected audit details userId=%d, got %v", target.ID, entry.Details["userId"])
	}
}

func TestAdminAssignDevice_NonexistentUser_Integration(t *testing.T) {
	env := setupUsersOASIntegration(t)
	ctx := context.Background()

	device := &model.Device{UniqueID: "orphan-001", Name: "Orphan Device", Status: "unknown"}
	if err := env.deviceRepo.Create(ctx, device, env.admin.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	res, err := env.handler.AdminAssignDevice(env.adminCtx(),
		oas.AdminAssignDeviceParams{ID: 99999, DeviceId: device.ID})
	if err != nil {
		t.Fatalf("AdminAssignDevice returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminAssignDeviceNotFound); !ok {
		t.Errorf("expected *oas.AdminAssignDeviceNotFound for nonexistent user, got %T", res)
	}
}

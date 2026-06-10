package handlers_test

// Shared in-memory mock repositories used by multiple test files in this
// package. Each mock returns the configured Fn when set and a safe default
// otherwise.

import (
	"context"
	"errors"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// mockUserRepo is a mock implementation of repository.UserRepo for
// unit testing without a database.
type mockUserRepo struct {
	createFn           func(ctx context.Context, user *model.User) error
	createOIDCUserFn   func(ctx context.Context, email, name, role, subject, issuer string) (*model.User, error)
	getByEmailFn       func(ctx context.Context, email string) (*model.User, error)
	getByIDFn          func(ctx context.Context, id int64) (*model.User, error)
	getByTokenFn       func(ctx context.Context, token string) (*model.User, error)
	getByOIDCSubjectFn func(ctx context.Context, subject, issuer string) (*model.User, error)
	setOIDCSubjectFn   func(ctx context.Context, userID int64, subject, issuer string) error
	listAllFn          func(ctx context.Context) ([]*model.User, error)
	updateFn           func(ctx context.Context, user *model.User) error
	updatePasswordFn   func(ctx context.Context, userID int64, hash string) error
	deleteFn           func(ctx context.Context, id int64) error
	getDevicesForUser  func(ctx context.Context, userID int64) ([]int64, error)
	assignDeviceFn     func(ctx context.Context, userID, deviceID int64) error
	unassignDeviceFn   func(ctx context.Context, userID, deviceID int64) error
	generateTokenFn    func(ctx context.Context, userID int64) (string, error)
}

var _ repository.UserRepo = (*mockUserRepo)(nil)

func (m *mockUserRepo) Create(ctx context.Context, user *model.User) error {
	if m.createFn != nil {
		return m.createFn(ctx, user)
	}
	return errors.New("not implemented")
}
func (m *mockUserRepo) CreateOIDCUser(ctx context.Context, email, name, role, subject, issuer string) (*model.User, error) {
	if m.createOIDCUserFn != nil {
		return m.createOIDCUserFn(ctx, email, name, role, subject, issuer)
	}
	return nil, errors.New("not implemented")
}
func (m *mockUserRepo) GetByOIDCSubject(ctx context.Context, subject, issuer string) (*model.User, error) {
	if m.getByOIDCSubjectFn != nil {
		return m.getByOIDCSubjectFn(ctx, subject, issuer)
	}
	return nil, errors.New("not found")
}
func (m *mockUserRepo) SetOIDCSubject(ctx context.Context, userID int64, subject, issuer string) error {
	if m.setOIDCSubjectFn != nil {
		return m.setOIDCSubjectFn(ctx, userID, subject, issuer)
	}
	return nil
}
func (m *mockUserRepo) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	if m.getByEmailFn != nil {
		return m.getByEmailFn(ctx, email)
	}
	return nil, errors.New("not found")
}
func (m *mockUserRepo) GetByID(ctx context.Context, id int64) (*model.User, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, errors.New("not found")
}
func (m *mockUserRepo) GetByToken(ctx context.Context, token string) (*model.User, error) {
	if m.getByTokenFn != nil {
		return m.getByTokenFn(ctx, token)
	}
	return nil, errors.New("not found")
}
func (m *mockUserRepo) ListAll(ctx context.Context) ([]*model.User, error) {
	if m.listAllFn != nil {
		return m.listAllFn(ctx)
	}
	return nil, nil
}
func (m *mockUserRepo) Update(ctx context.Context, user *model.User) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, user)
	}
	return errors.New("not implemented")
}
func (m *mockUserRepo) UpdatePassword(ctx context.Context, userID int64, hash string) error {
	if m.updatePasswordFn != nil {
		return m.updatePasswordFn(ctx, userID, hash)
	}
	return errors.New("not implemented")
}
func (m *mockUserRepo) Delete(ctx context.Context, id int64) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return errors.New("not implemented")
}
func (m *mockUserRepo) GetDevicesForUser(ctx context.Context, userID int64) ([]int64, error) {
	if m.getDevicesForUser != nil {
		return m.getDevicesForUser(ctx, userID)
	}
	return nil, nil
}
func (m *mockUserRepo) AssignDevice(ctx context.Context, userID, deviceID int64) error {
	if m.assignDeviceFn != nil {
		return m.assignDeviceFn(ctx, userID, deviceID)
	}
	return errors.New("not implemented")
}
func (m *mockUserRepo) UnassignDevice(ctx context.Context, userID, deviceID int64) error {
	if m.unassignDeviceFn != nil {
		return m.unassignDeviceFn(ctx, userID, deviceID)
	}
	return errors.New("not implemented")
}
func (m *mockUserRepo) GenerateToken(ctx context.Context, userID int64) (string, error) {
	if m.generateTokenFn != nil {
		return m.generateTokenFn(ctx, userID)
	}
	return "", errors.New("not implemented")
}

// mockSessionRepo is a mock implementation of repository.SessionRepo.
type mockSessionRepo struct {
	createFn           func(ctx context.Context, userID int64) (*model.Session, error)
	createWithExpiryFn func(ctx context.Context, userID int64, expiresAt time.Time, rememberMe bool) (*model.Session, error)
	createWithApiKeyFn func(ctx context.Context, userID int64, apiKeyID int64, expiresAt time.Time, rememberMe bool) (*model.Session, error)
	createSudoFn       func(ctx context.Context, targetUserID, originalUserID int64) (*model.Session, error)
	getByIDFn          func(ctx context.Context, id string) (*model.Session, error)
	getByIDPrefixFn    func(ctx context.Context, userID int64, prefix string) (*model.Session, error)
	deleteFn           func(ctx context.Context, id string) error
	listByUserFn       func(ctx context.Context, userID int64) ([]*model.Session, error)
	deleteAllByUserFn  func(ctx context.Context, userID int64, exceptID string) error
}

var _ repository.SessionRepo = (*mockSessionRepo)(nil)

func (m *mockSessionRepo) Create(ctx context.Context, userID int64) (*model.Session, error) {
	if m.createFn != nil {
		return m.createFn(ctx, userID)
	}
	return &model.Session{ID: "mock-session-id", UserID: userID, ExpiresAt: time.Now().Add(24 * time.Hour)}, nil
}
func (m *mockSessionRepo) CreateWithExpiry(ctx context.Context, userID int64, expiresAt time.Time, rememberMe bool) (*model.Session, error) {
	if m.createWithExpiryFn != nil {
		return m.createWithExpiryFn(ctx, userID, expiresAt, rememberMe)
	}
	return &model.Session{ID: "mock-session-id", UserID: userID, ExpiresAt: expiresAt}, nil
}
func (m *mockSessionRepo) CreateWithApiKey(ctx context.Context, userID int64, apiKeyID int64, expiresAt time.Time, rememberMe bool) (*model.Session, error) {
	if m.createWithApiKeyFn != nil {
		return m.createWithApiKeyFn(ctx, userID, apiKeyID, expiresAt, rememberMe)
	}
	return &model.Session{ID: "mock-session-id", UserID: userID, ApiKeyID: &apiKeyID, RememberMe: rememberMe, ExpiresAt: expiresAt}, nil
}
func (m *mockSessionRepo) CreateSudo(ctx context.Context, targetUserID, originalUserID int64) (*model.Session, error) {
	if m.createSudoFn != nil {
		return m.createSudoFn(ctx, targetUserID, originalUserID)
	}
	return nil, errors.New("not implemented")
}
func (m *mockSessionRepo) GetByID(ctx context.Context, id string) (*model.Session, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, errors.New("not found")
}
func (m *mockSessionRepo) GetByIDPrefix(ctx context.Context, userID int64, prefix string) (*model.Session, error) {
	if m.getByIDPrefixFn != nil {
		return m.getByIDPrefixFn(ctx, userID, prefix)
	}
	return nil, errors.New("not found")
}
func (m *mockSessionRepo) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}
func (m *mockSessionRepo) UpdateLastSeen(_ context.Context, _, _, _ string) error { return nil }
func (m *mockSessionRepo) UpdateExpiry(_ context.Context, _ string, _ time.Time) error {
	return nil
}
func (m *mockSessionRepo) DeleteAllByUser(ctx context.Context, userID int64, exceptID string) error {
	if m.deleteAllByUserFn != nil {
		return m.deleteAllByUserFn(ctx, userID, exceptID)
	}
	return nil
}
func (m *mockSessionRepo) ListByUser(ctx context.Context, userID int64) ([]*model.Session, error) {
	if m.listByUserFn != nil {
		return m.listByUserFn(ctx, userID)
	}
	return nil, nil
}

// mockApiKeyRepo is a mock implementation of repository.ApiKeyRepo for
// unit testing handlers without a database.
type mockApiKeyRepo struct {
	createFn         func(ctx context.Context, key *model.ApiKey) error
	getByTokenFn     func(ctx context.Context, token string) (*model.ApiKey, error)
	getByIDFn        func(ctx context.Context, id int64) (*model.ApiKey, error)
	listByUserFn     func(ctx context.Context, userID int64) ([]*model.ApiKey, error)
	deleteFn         func(ctx context.Context, id int64) error
	updateLastUsedFn func(ctx context.Context, id int64) error
}

// Compile-time assertion that mockApiKeyRepo satisfies repository.ApiKeyRepo.
var _ repository.ApiKeyRepo = (*mockApiKeyRepo)(nil)

func (m *mockApiKeyRepo) Create(ctx context.Context, key *model.ApiKey) error {
	if m.createFn != nil {
		return m.createFn(ctx, key)
	}
	key.ID = 1
	key.Token = "generated-test-token-0123456789abcdef0123456789abcdef"
	return nil
}

func (m *mockApiKeyRepo) GetByToken(ctx context.Context, token string) (*model.ApiKey, error) {
	if m.getByTokenFn != nil {
		return m.getByTokenFn(ctx, token)
	}
	return nil, errors.New("not found")
}

func (m *mockApiKeyRepo) GetByID(ctx context.Context, id int64) (*model.ApiKey, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, errors.New("not found")
}

func (m *mockApiKeyRepo) ListByUser(ctx context.Context, userID int64) ([]*model.ApiKey, error) {
	if m.listByUserFn != nil {
		return m.listByUserFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockApiKeyRepo) Delete(ctx context.Context, id int64) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockApiKeyRepo) UpdateLastUsed(ctx context.Context, id int64) error {
	if m.updateLastUsedFn != nil {
		return m.updateLastUsedFn(ctx, id)
	}
	return nil
}

// mockDeviceRepo is a mock implementation of repository.DeviceRepo for
// unit testing handlers without a database.
type mockDeviceRepo struct {
	// Configurable return values.
	userHasAccessFn func(ctx context.Context, user *model.User, deviceID int64) bool
	getByIDFn       func(ctx context.Context, id int64) (*model.Device, error)
	getByUniqueIDFn func(ctx context.Context, uniqueID string) (*model.Device, error)
	getByUserFn     func(ctx context.Context, userID int64) ([]*model.Device, error)
	getAllFn        func(ctx context.Context) ([]model.Device, error)
	getUserIDsFn    func(ctx context.Context, deviceID int64) ([]int64, error)
	createFn        func(ctx context.Context, d *model.Device, userID int64) error
	updateFn        func(ctx context.Context, d *model.Device) error
	deleteFn        func(ctx context.Context, id int64) error
}

// Compile-time assertion that mockDeviceRepo satisfies repository.DeviceRepo.
var _ repository.DeviceRepo = (*mockDeviceRepo)(nil)

func (m *mockDeviceRepo) UserHasAccess(ctx context.Context, user *model.User, deviceID int64) bool {
	if m.userHasAccessFn != nil {
		return m.userHasAccessFn(ctx, user, deviceID)
	}
	return false
}

func (m *mockDeviceRepo) GetByID(ctx context.Context, id int64) (*model.Device, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, errors.New("not found")
}

func (m *mockDeviceRepo) GetByUniqueID(ctx context.Context, uniqueID string) (*model.Device, error) {
	if m.getByUniqueIDFn != nil {
		return m.getByUniqueIDFn(ctx, uniqueID)
	}
	return nil, errors.New("not found")
}

func (m *mockDeviceRepo) GetByUser(ctx context.Context, userID int64) ([]*model.Device, error) {
	if m.getByUserFn != nil {
		return m.getByUserFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockDeviceRepo) GetAll(ctx context.Context) ([]model.Device, error) {
	if m.getAllFn != nil {
		return m.getAllFn(ctx)
	}
	return nil, nil
}

func (m *mockDeviceRepo) GetUserIDs(ctx context.Context, deviceID int64) ([]int64, error) {
	if m.getUserIDsFn != nil {
		return m.getUserIDsFn(ctx, deviceID)
	}
	return nil, nil
}

func (m *mockDeviceRepo) Create(ctx context.Context, d *model.Device, userID int64) error {
	if m.createFn != nil {
		return m.createFn(ctx, d, userID)
	}
	d.ID = 42 // Assign a fake ID.
	return nil
}

func (m *mockDeviceRepo) Update(ctx context.Context, d *model.Device) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, d)
	}
	return nil
}

func (m *mockDeviceRepo) Delete(ctx context.Context, id int64) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}
func (m *mockDeviceRepo) GetTimedOut(_ context.Context, _ time.Time) ([]model.Device, error) {
	return nil, nil
}

func (m *mockDeviceRepo) GetAllWithOwners(ctx context.Context) ([]model.Device, error) {
	if m.getAllFn != nil {
		return m.getAllFn(ctx)
	}
	return nil, nil
}

func (m *mockDeviceRepo) UpdateIgnitionState(_ context.Context, _ int64, _ bool, _ time.Time) error {
	return nil
}

func (m *mockDeviceRepo) UpdateProtocol(_ context.Context, _ int64, _ string) error {
	return nil
}

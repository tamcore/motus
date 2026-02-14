package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

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

// --- Mock-based unit tests ---
// These tests run without a database and are significantly faster than
// the integration tests in device_test.go.

func TestDeviceHandler_Mock_List_ReturnsDevices(t *testing.T) {
	mock := &mockDeviceRepo{
		getByUserFn: func(_ context.Context, userID int64) ([]*model.Device, error) {
			if userID != 1 {
				t.Errorf("expected userID=1, got %d", userID)
			}
			return []*model.Device{
				{ID: 10, UniqueID: "dev-a", Name: "Device A", Status: "online"},
				{ID: 20, UniqueID: "dev-b", Name: "Device B", Status: "offline"},
			}, nil
		},
	}
	h := handlers.NewDeviceHandler(mock, "")
	user := &model.User{ID: 1, Email: "test@example.com", Name: "Test"}

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var devices []*model.Device
	if err := json.NewDecoder(rr.Body).Decode(&devices); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
	if devices[0].Name != "Device A" {
		t.Errorf("expected first device name 'Device A', got %q", devices[0].Name)
	}
}

func TestDeviceHandler_Mock_List_DBError(t *testing.T) {
	mock := &mockDeviceRepo{
		getByUserFn: func(_ context.Context, _ int64) ([]*model.Device, error) {
			return nil, errors.New("connection refused")
		},
	}
	h := handlers.NewDeviceHandler(mock, "")
	user := &model.User{ID: 1, Email: "test@example.com"}

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestDeviceHandler_Mock_Get_Success(t *testing.T) {
	mock := &mockDeviceRepo{
		userHasAccessFn: func(_ context.Context, user *model.User, deviceID int64) bool {
			return user.ID == 1 && deviceID == 10
		},
		getByIDFn: func(_ context.Context, id int64) (*model.Device, error) {
			if id == 10 {
				return &model.Device{ID: 10, UniqueID: "dev-10", Name: "My Device", Status: "online"}, nil
			}
			return nil, errors.New("not found")
		},
	}
	h := handlers.NewDeviceHandler(mock, "")
	user := &model.User{ID: 1, Email: "test@example.com"}

	req := httptest.NewRequest(http.MethodGet, "/api/devices/10", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "10")
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var device model.Device
	if err := json.NewDecoder(rr.Body).Decode(&device); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if device.Name != "My Device" {
		t.Errorf("expected name 'My Device', got %q", device.Name)
	}
}

func TestDeviceHandler_Mock_Get_Forbidden(t *testing.T) {
	mock := &mockDeviceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool {
			return false // Always deny access.
		},
	}
	h := handlers.NewDeviceHandler(mock, "")
	user := &model.User{ID: 99, Email: "stranger@example.com"}

	req := httptest.NewRequest(http.MethodGet, "/api/devices/10", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "10")
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.Get(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestDeviceHandler_Mock_Get_InvalidID(t *testing.T) {
	h := handlers.NewDeviceHandler(&mockDeviceRepo{}, "")
	user := &model.User{ID: 1, Email: "test@example.com"}

	req := httptest.NewRequest(http.MethodGet, "/api/devices/abc", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "abc")
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.Get(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestDeviceHandler_Mock_Create_Success(t *testing.T) {
	var createdDevice *model.Device
	var createdForUser int64

	mock := &mockDeviceRepo{
		createFn: func(_ context.Context, d *model.Device, userID int64) error {
			createdDevice = d
			createdForUser = userID
			d.ID = 99 // Simulate DB assigning an ID.
			return nil
		},
	}
	h := handlers.NewDeviceHandler(mock, "")
	user := &model.User{ID: 7, Email: "creator@example.com"}

	body := `{"uniqueId":"new-001","name":"Brand New Device","protocol":"h02"}`
	req := httptest.NewRequest(http.MethodPost, "/api/devices", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Verify the repo received the right arguments.
	if createdDevice == nil {
		t.Fatal("expected createFn to be called")
	}
	if createdDevice.UniqueID != "new-001" {
		t.Errorf("expected uniqueId 'new-001', got %q", createdDevice.UniqueID)
	}
	if createdForUser != 7 {
		t.Errorf("expected userID=7, got %d", createdForUser)
	}

	// Verify the response contains the assigned ID.
	var respDevice model.Device
	_ = json.NewDecoder(rr.Body).Decode(&respDevice)
	if respDevice.ID != 99 {
		t.Errorf("expected response ID=99, got %d", respDevice.ID)
	}
}

func TestDeviceHandler_Mock_Create_ValidationErrors(t *testing.T) {
	h := handlers.NewDeviceHandler(&mockDeviceRepo{}, "")
	user := &model.User{ID: 1, Email: "test@example.com"}

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"invalid JSON", "not json", http.StatusBadRequest},
		{"empty uniqueId", `{"uniqueId":"","name":"Test"}`, http.StatusBadRequest},
		{"empty name", `{"uniqueId":"test-001","name":""}`, http.StatusBadRequest},
		{"missing fields", `{}`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/devices", bytes.NewReader([]byte(tt.body)))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(api.ContextWithUser(req.Context(), user))
			rr := httptest.NewRecorder()

			h.Create(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected %d, got %d; body: %s", tt.wantStatus, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestDeviceHandler_Mock_Create_DBError(t *testing.T) {
	mock := &mockDeviceRepo{
		createFn: func(_ context.Context, _ *model.Device, _ int64) error {
			return errors.New("unique constraint violation")
		},
	}
	h := handlers.NewDeviceHandler(mock, "")
	user := &model.User{ID: 1, Email: "test@example.com"}

	body := `{"uniqueId":"dup-001","name":"Duplicate"}`
	req := httptest.NewRequest(http.MethodPost, "/api/devices", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestDeviceHandler_Mock_Update_Success(t *testing.T) {
	existingDevice := &model.Device{ID: 10, UniqueID: "upd-001", Name: "Before", Protocol: "h02", Status: "online"}

	mock := &mockDeviceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return true },
		getByIDFn: func(_ context.Context, id int64) (*model.Device, error) {
			if id == 10 {
				// Return a copy to avoid mutation of the original across calls.
				d := *existingDevice
				return &d, nil
			}
			return nil, errors.New("not found")
		},
		updateFn: func(_ context.Context, d *model.Device) error {
			if d.Name != "After Update" {
				t.Errorf("expected updated name 'After Update', got %q", d.Name)
			}
			return nil
		},
	}
	h := handlers.NewDeviceHandler(mock, "")
	user := &model.User{ID: 1, Email: "test@example.com"}

	body := `{"name":"After Update"}`
	req := httptest.NewRequest(http.MethodPut, "/api/devices/10", bytes.NewReader([]byte(body)))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "10")
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestDeviceHandler_Mock_Delete_Success(t *testing.T) {
	var deletedID int64
	mock := &mockDeviceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return true },
		deleteFn: func(_ context.Context, id int64) error {
			deletedID = id
			return nil
		},
	}
	h := handlers.NewDeviceHandler(mock, "")
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
	if deletedID != 10 {
		t.Errorf("expected delete called with ID=10, got %d", deletedID)
	}
}

func TestDeviceHandler_Mock_Delete_Forbidden(t *testing.T) {
	mock := &mockDeviceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return false },
	}
	h := handlers.NewDeviceHandler(mock, "")
	user := &model.User{ID: 99, Email: "stranger@example.com"}

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/devices/%d", 10), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "10")
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.Delete(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

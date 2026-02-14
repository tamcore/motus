package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func setupDeviceHandler(t *testing.T) (*handlers.DeviceHandler, *repository.DeviceRepository, *model.User) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)

	user := &model.User{Email: "devhandler@example.com", PasswordHash: "$2a$10$fakehash", Name: "Dev Handler"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	h := handlers.NewDeviceHandler(deviceRepo, "")
	return h, deviceRepo, user
}

func withUser(r *http.Request, user *model.User) *http.Request {
	ctx := api.ContextWithUser(r.Context(), user)
	return r.WithContext(ctx)
}

func withChiParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestDeviceHandler_List_Empty(t *testing.T) {
	h, _, user := setupDeviceHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var devices []model.Device
	_ = json.NewDecoder(rr.Body).Decode(&devices)
	if len(devices) != 0 {
		t.Errorf("expected empty array, got %d devices", len(devices))
	}
}

func TestDeviceHandler_Create_Success(t *testing.T) {
	h, _, user := setupDeviceHandler(t)

	body := `{"uniqueId":"handler-test-001","name":"Handler Test Device","protocol":"h02"}`
	req := httptest.NewRequest(http.MethodPost, "/api/devices", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var device model.Device
	_ = json.NewDecoder(rr.Body).Decode(&device)
	if device.UniqueID != "handler-test-001" {
		t.Errorf("expected uniqueId 'handler-test-001', got %q", device.UniqueID)
	}
	if device.Name != "Handler Test Device" {
		t.Errorf("expected name 'Handler Test Device', got %q", device.Name)
	}
}

func TestDeviceHandler_Create_MissingFields(t *testing.T) {
	h, _, user := setupDeviceHandler(t)

	tests := []struct {
		name string
		body string
	}{
		{"missing uniqueId", `{"name":"Test"}`},
		{"missing name", `{"uniqueId":"test"}`},
		{"empty body", `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/devices", bytes.NewReader([]byte(tt.body)))
			req = withUser(req, user)
			rr := httptest.NewRecorder()

			h.Create(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", rr.Code)
			}
		})
	}
}

func TestDeviceHandler_Create_InvalidJSON(t *testing.T) {
	h, _, user := setupDeviceHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/devices", bytes.NewReader([]byte("not json")))
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestDeviceHandler_Get_Success(t *testing.T) {
	h, deviceRepo, user := setupDeviceHandler(t)
	ctx := context.Background()

	device := &model.Device{UniqueID: "get-handler-001", Name: "Get Test", Status: "unknown"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	req := httptest.NewRequest(http.MethodGet, "/api/devices/1", nil)
	req = withUser(req, user)
	req = withChiParam(req, "id", "1")
	// Re-set user context since withChiParam may replace it.
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	// We need to use the actual device ID, not a hardcoded "1".
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", device.ID))
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))

	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestDeviceHandler_Get_InvalidID(t *testing.T) {
	h, _, user := setupDeviceHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/devices/abc", nil)
	req = withUser(req, user)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "abc")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestDeviceHandler_Get_Forbidden(t *testing.T) {
	h, deviceRepo, user := setupDeviceHandler(t)
	ctx := context.Background()

	// Create device owned by user.
	device := &model.Device{UniqueID: "forbid-001", Name: "Forbidden", Status: "unknown"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// Use a different user to attempt access.
	otherUser := &model.User{ID: user.ID + 999, Email: "other@example.com", Name: "Other"}

	req := httptest.NewRequest(http.MethodGet, "/api/devices/1", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", device.ID))
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), otherUser), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestDeviceHandler_Update_Success(t *testing.T) {
	h, deviceRepo, user := setupDeviceHandler(t)
	ctx := context.Background()

	device := &model.Device{UniqueID: "update-handler-001", Name: "Before", Status: "unknown"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	body := `{"name":"After Update"}`
	req := httptest.NewRequest(http.MethodPut, "/api/devices/1", bytes.NewReader([]byte(body)))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", device.ID))
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var updated model.Device
	_ = json.NewDecoder(rr.Body).Decode(&updated)
	if updated.Name != "After Update" {
		t.Errorf("expected name 'After Update', got %q", updated.Name)
	}
}

func TestDeviceHandler_Delete_Success(t *testing.T) {
	h, deviceRepo, user := setupDeviceHandler(t)
	ctx := context.Background()

	device := &model.Device{UniqueID: "delete-handler-001", Name: "Delete Me", Status: "unknown"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	req := httptest.NewRequest(http.MethodDelete, "/api/devices/1", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", device.ID))
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

func TestDeviceHandler_List_WithDevices(t *testing.T) {
	h, deviceRepo, user := setupDeviceHandler(t)
	ctx := context.Background()

	d1 := &model.Device{UniqueID: "list-1", Name: "Device A", Status: "unknown"}
	d2 := &model.Device{UniqueID: "list-2", Name: "Device B", Status: "online"}
	_ = deviceRepo.Create(ctx, d1, user.ID)
	_ = deviceRepo.Create(ctx, d2, user.ID)

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var devices []*model.Device
	_ = json.NewDecoder(rr.Body).Decode(&devices)
	if len(devices) != 2 {
		t.Errorf("expected 2 devices, got %d", len(devices))
	}
}

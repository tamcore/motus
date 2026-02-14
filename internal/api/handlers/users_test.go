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

func setupUserHandler(t *testing.T) (*handlers.UserHandler, *repository.UserRepository, *repository.DeviceRepository, *model.User) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)

	admin := &model.User{
		Email:        "admin@test.com",
		PasswordHash: "$2a$10$fakehash",
		Name:         "Admin",
		Role:         model.RoleAdmin,
	}
	if err := userRepo.Create(context.Background(), admin); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	h := handlers.NewUserHandler(userRepo, deviceRepo, "")
	return h, userRepo, deviceRepo, admin
}

func withUserCtx(r *http.Request, user *model.User) *http.Request {
	return r.WithContext(api.ContextWithUser(r.Context(), user))
}

func withUserAndChi(r *http.Request, user *model.User, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	ctx := api.ContextWithUser(r.Context(), user)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	return r.WithContext(ctx)
}

// --- List ---

func TestUserHandler_List_AdminOnly(t *testing.T) {
	h, userRepo, _, admin := setupUserHandler(t)

	// Create a non-admin user.
	regular := &model.User{Email: "user@test.com", PasswordHash: "hash", Name: "User", Role: model.RoleUser}
	_ = userRepo.Create(context.Background(), regular)

	t.Run("admin can list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
		req = withUserCtx(req, admin)
		rr := httptest.NewRecorder()
		h.List(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
		}
		var users []*model.User
		_ = json.NewDecoder(rr.Body).Decode(&users)
		if len(users) != 2 {
			t.Errorf("expected 2 users, got %d", len(users))
		}
	})

	t.Run("non-admin forbidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
		req = withUserCtx(req, regular)
		rr := httptest.NewRecorder()
		h.List(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", rr.Code)
		}
	})

	t.Run("no user forbidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
		rr := httptest.NewRecorder()
		h.List(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", rr.Code)
		}
	})
}

// --- Create ---

func TestUserHandler_Create(t *testing.T) {
	h, _, _, admin := setupUserHandler(t)

	t.Run("success", func(t *testing.T) {
		body := `{"email":"new@test.com","password":"secret123","name":"New User","role":"user"}`
		req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader([]byte(body)))
		req = withUserCtx(req, admin)
		rr := httptest.NewRecorder()
		h.Create(rr, req)

		if rr.Code != http.StatusCreated {
			t.Errorf("expected 201, got %d; body: %s", rr.Code, rr.Body.String())
		}
		var resp map[string]interface{}
		_ = json.NewDecoder(rr.Body).Decode(&resp)
		if resp["email"] != "new@test.com" {
			t.Errorf("expected email 'new@test.com', got %v", resp["email"])
		}
		// Role is not exposed in JSON (json:"-"); check Traccar fields instead.
		if resp["administrator"] != false {
			t.Errorf("expected administrator=false for user role, got %v", resp["administrator"])
		}
		if resp["readonly"] != false {
			t.Errorf("expected readonly=false for user role, got %v", resp["readonly"])
		}
		if resp["id"] == nil || resp["id"].(float64) == 0 {
			t.Error("expected non-zero user ID")
		}
	})

	t.Run("default role", func(t *testing.T) {
		body := `{"email":"default@test.com","password":"secret123","name":"Default Role"}`
		req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader([]byte(body)))
		req = withUserCtx(req, admin)
		rr := httptest.NewRecorder()
		h.Create(rr, req)

		if rr.Code != http.StatusCreated {
			t.Errorf("expected 201, got %d; body: %s", rr.Code, rr.Body.String())
		}
		var resp map[string]interface{}
		_ = json.NewDecoder(rr.Body).Decode(&resp)
		// Default role is "user" -> administrator=false, readonly=false.
		if resp["administrator"] != false {
			t.Errorf("expected administrator=false for default role, got %v", resp["administrator"])
		}
	})

	t.Run("missing email", func(t *testing.T) {
		body := `{"password":"secret123","name":"No Email"}`
		req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader([]byte(body)))
		req = withUserCtx(req, admin)
		rr := httptest.NewRecorder()
		h.Create(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})

	t.Run("missing password", func(t *testing.T) {
		body := `{"email":"nopass@test.com","name":"No Password"}`
		req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader([]byte(body)))
		req = withUserCtx(req, admin)
		rr := httptest.NewRecorder()
		h.Create(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})

	t.Run("invalid role", func(t *testing.T) {
		body := `{"email":"bad@test.com","password":"secret","name":"Bad Role","role":"superadmin"}`
		req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader([]byte(body)))
		req = withUserCtx(req, admin)
		rr := httptest.NewRecorder()
		h.Create(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader([]byte("not json")))
		req = withUserCtx(req, admin)
		rr := httptest.NewRecorder()
		h.Create(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})

	t.Run("non-admin forbidden", func(t *testing.T) {
		regular := &model.User{ID: 999, Email: "user@test.com", Role: model.RoleUser}
		body := `{"email":"x@test.com","password":"secret","name":"X"}`
		req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader([]byte(body)))
		req = withUserCtx(req, regular)
		rr := httptest.NewRecorder()
		h.Create(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", rr.Code)
		}
	})
}

// --- Update ---

func TestUserHandler_Update(t *testing.T) {
	h, userRepo, _, admin := setupUserHandler(t)

	// Create target user.
	target := &model.User{Email: "target@test.com", PasswordHash: "hash", Name: "Target", Role: model.RoleUser}
	_ = userRepo.Create(context.Background(), target)

	t.Run("update name and role", func(t *testing.T) {
		body := `{"name":"Updated Name","role":"readonly"}`
		req := httptest.NewRequest(http.MethodPut, "/api/users/"+fmt.Sprint(target.ID), bytes.NewReader([]byte(body)))
		req = withUserAndChi(req, admin, map[string]string{"id": fmt.Sprint(target.ID)})
		rr := httptest.NewRecorder()
		h.Update(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
		}
		var resp map[string]interface{}
		_ = json.NewDecoder(rr.Body).Decode(&resp)
		if resp["name"] != "Updated Name" {
			t.Errorf("expected name 'Updated Name', got %v", resp["name"])
		}
		// Role is not exposed in JSON (json:"-"); check Traccar fields instead.
		if resp["readonly"] != true {
			t.Errorf("expected readonly=true for readonly role, got %v", resp["readonly"])
		}
		if resp["administrator"] != false {
			t.Errorf("expected administrator=false for readonly role, got %v", resp["administrator"])
		}
	})

	t.Run("update password", func(t *testing.T) {
		body := `{"password":"newpassword123"}`
		req := httptest.NewRequest(http.MethodPut, "/api/users/"+fmt.Sprint(target.ID), bytes.NewReader([]byte(body)))
		req = withUserAndChi(req, admin, map[string]string{"id": fmt.Sprint(target.ID)})
		rr := httptest.NewRecorder()
		h.Update(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("cannot demote self", func(t *testing.T) {
		body := `{"role":"user"}`
		req := httptest.NewRequest(http.MethodPut, "/api/users/"+fmt.Sprint(admin.ID), bytes.NewReader([]byte(body)))
		req = withUserAndChi(req, admin, map[string]string{"id": fmt.Sprint(admin.ID)})
		rr := httptest.NewRecorder()
		h.Update(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d; body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("invalid user id", func(t *testing.T) {
		body := `{"name":"X"}`
		req := httptest.NewRequest(http.MethodPut, "/api/users/abc", bytes.NewReader([]byte(body)))
		req = withUserAndChi(req, admin, map[string]string{"id": "abc"})
		rr := httptest.NewRecorder()
		h.Update(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})

	t.Run("user not found", func(t *testing.T) {
		body := `{"name":"X"}`
		req := httptest.NewRequest(http.MethodPut, "/api/users/99999", bytes.NewReader([]byte(body)))
		req = withUserAndChi(req, admin, map[string]string{"id": "99999"})
		rr := httptest.NewRecorder()
		h.Update(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", rr.Code)
		}
	})

	t.Run("invalid role", func(t *testing.T) {
		body := `{"role":"superadmin"}`
		req := httptest.NewRequest(http.MethodPut, "/api/users/"+fmt.Sprint(target.ID), bytes.NewReader([]byte(body)))
		req = withUserAndChi(req, admin, map[string]string{"id": fmt.Sprint(target.ID)})
		rr := httptest.NewRecorder()
		h.Update(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})
}

// --- Delete ---

func TestUserHandler_Delete(t *testing.T) {
	h, userRepo, _, admin := setupUserHandler(t)

	t.Run("delete user", func(t *testing.T) {
		target := &model.User{Email: "delete@test.com", PasswordHash: "hash", Name: "Delete Me", Role: model.RoleUser}
		_ = userRepo.Create(context.Background(), target)

		req := httptest.NewRequest(http.MethodDelete, "/api/users/"+fmt.Sprint(target.ID), nil)
		req = withUserAndChi(req, admin, map[string]string{"id": fmt.Sprint(target.ID)})
		rr := httptest.NewRecorder()
		h.Delete(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Errorf("expected 204, got %d; body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("cannot delete self", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/users/"+fmt.Sprint(admin.ID), nil)
		req = withUserAndChi(req, admin, map[string]string{"id": fmt.Sprint(admin.ID)})
		rr := httptest.NewRecorder()
		h.Delete(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d; body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("invalid id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/users/abc", nil)
		req = withUserAndChi(req, admin, map[string]string{"id": "abc"})
		rr := httptest.NewRecorder()
		h.Delete(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})
}

// --- Device Assignment ---

func TestUserHandler_DeviceAssignment(t *testing.T) {
	h, userRepo, deviceRepo, admin := setupUserHandler(t)

	// Create a target user and device.
	target := &model.User{Email: "devuser@test.com", PasswordHash: "hash", Name: "Dev User", Role: model.RoleUser}
	_ = userRepo.Create(context.Background(), target)

	device := &model.Device{UniqueID: "assign-001", Name: "Test Device", Status: "unknown"}
	_ = deviceRepo.Create(context.Background(), device, admin.ID)

	t.Run("list devices empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/users/"+fmt.Sprint(target.ID)+"/devices", nil)
		req = withUserAndChi(req, admin, map[string]string{"id": fmt.Sprint(target.ID)})
		rr := httptest.NewRecorder()
		h.ListDevices(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
		}
		var devices []*model.Device
		_ = json.NewDecoder(rr.Body).Decode(&devices)
		if len(devices) != 0 {
			t.Errorf("expected 0 devices, got %d", len(devices))
		}
	})

	t.Run("assign device", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/users/"+fmt.Sprint(target.ID)+"/devices/"+fmt.Sprint(device.ID), nil)
		req = withUserAndChi(req, admin, map[string]string{
			"id":       fmt.Sprint(target.ID),
			"deviceId": fmt.Sprint(device.ID),
		})
		rr := httptest.NewRecorder()
		h.AssignDevice(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Errorf("expected 204, got %d; body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("list devices after assign", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/users/"+fmt.Sprint(target.ID)+"/devices", nil)
		req = withUserAndChi(req, admin, map[string]string{"id": fmt.Sprint(target.ID)})
		rr := httptest.NewRecorder()
		h.ListDevices(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		var devices []*model.Device
		_ = json.NewDecoder(rr.Body).Decode(&devices)
		if len(devices) != 1 {
			t.Errorf("expected 1 device, got %d", len(devices))
		}
	})

	t.Run("assign duplicate is idempotent", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/users/"+fmt.Sprint(target.ID)+"/devices/"+fmt.Sprint(device.ID), nil)
		req = withUserAndChi(req, admin, map[string]string{
			"id":       fmt.Sprint(target.ID),
			"deviceId": fmt.Sprint(device.ID),
		})
		rr := httptest.NewRecorder()
		h.AssignDevice(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Errorf("expected 204, got %d", rr.Code)
		}
	})

	t.Run("unassign device", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/users/"+fmt.Sprint(target.ID)+"/devices/"+fmt.Sprint(device.ID), nil)
		req = withUserAndChi(req, admin, map[string]string{
			"id":       fmt.Sprint(target.ID),
			"deviceId": fmt.Sprint(device.ID),
		})
		rr := httptest.NewRecorder()
		h.UnassignDevice(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Errorf("expected 204, got %d; body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("assign nonexistent user", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/users/99999/devices/"+fmt.Sprint(device.ID), nil)
		req = withUserAndChi(req, admin, map[string]string{
			"id":       "99999",
			"deviceId": fmt.Sprint(device.ID),
		})
		rr := httptest.NewRecorder()
		h.AssignDevice(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", rr.Code)
		}
	})

	t.Run("assign nonexistent device", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/users/"+fmt.Sprint(target.ID)+"/devices/99999", nil)
		req = withUserAndChi(req, admin, map[string]string{
			"id":       fmt.Sprint(target.ID),
			"deviceId": "99999",
		})
		rr := httptest.NewRecorder()
		h.AssignDevice(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", rr.Code)
		}
	})

	t.Run("list devices non-admin forbidden", func(t *testing.T) {
		regular := &model.User{ID: 999, Email: "norm@test.com", Role: model.RoleUser}
		req := httptest.NewRequest(http.MethodGet, "/api/users/"+fmt.Sprint(target.ID)+"/devices", nil)
		req = withUserAndChi(req, regular, map[string]string{"id": fmt.Sprint(target.ID)})
		rr := httptest.NewRecorder()
		h.ListDevices(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", rr.Code)
		}
	})

	t.Run("list devices user not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/users/99999/devices", nil)
		req = withUserAndChi(req, admin, map[string]string{"id": "99999"})
		rr := httptest.NewRecorder()
		h.ListDevices(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", rr.Code)
		}
	})

	t.Run("invalid user id in assign", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/users/abc/devices/1", nil)
		req = withUserAndChi(req, admin, map[string]string{"id": "abc", "deviceId": "1"})
		rr := httptest.NewRecorder()
		h.AssignDevice(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})

	t.Run("invalid device id in assign", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/users/1/devices/abc", nil)
		req = withUserAndChi(req, admin, map[string]string{"id": "1", "deviceId": "abc"})
		rr := httptest.NewRecorder()
		h.AssignDevice(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})

	t.Run("invalid user id in unassign", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/users/abc/devices/1", nil)
		req = withUserAndChi(req, admin, map[string]string{"id": "abc", "deviceId": "1"})
		rr := httptest.NewRecorder()
		h.UnassignDevice(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})

	t.Run("invalid device id in unassign", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/users/1/devices/abc", nil)
		req = withUserAndChi(req, admin, map[string]string{"id": "1", "deviceId": "abc"})
		rr := httptest.NewRecorder()
		h.UnassignDevice(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})
}

// --- AdminListAllDevices ---

func TestUserHandler_AdminListAllDevices_Success(t *testing.T) {
	h, _, deviceRepo, admin := setupUserHandler(t)

	// Create 2 devices.
	for i := 0; i < 2; i++ {
		d := &model.Device{UniqueID: fmt.Sprintf("admin-dev-%d", i), Name: fmt.Sprintf("Device %d", i), Status: "online"}
		_ = deviceRepo.Create(context.Background(), d, admin.ID)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/devices", nil)
	req = withUserCtx(req, admin)
	rr := httptest.NewRecorder()

	h.AdminListAllDevices(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var devices []model.Device
	_ = json.NewDecoder(rr.Body).Decode(&devices)
	if len(devices) < 2 {
		t.Errorf("expected at least 2 devices, got %d", len(devices))
	}
}

func TestUserHandler_AdminListAllDevices_NonAdmin(t *testing.T) {
	h, _, _, _ := setupUserHandler(t)

	regular := &model.User{ID: 999, Email: "user@test.com", Role: model.RoleUser}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/devices", nil)
	req = withUserCtx(req, regular)
	rr := httptest.NewRecorder()

	h.AdminListAllDevices(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

// --- SetCacheInvalidator ---

type fakeDeviceInvalidator struct {
	invalidatedIDs []int64
}

func (f *fakeDeviceInvalidator) InvalidateDevice(deviceID int64) {
	f.invalidatedIDs = append(f.invalidatedIDs, deviceID)
}

func TestUserHandler_SetCacheInvalidator(t *testing.T) {
	h, userRepo, deviceRepo, admin := setupUserHandler(t)

	invalidator := &fakeDeviceInvalidator{}
	h.SetCacheInvalidator(invalidator)

	// Assign a device to verify the invalidator is called.
	target := &model.User{Email: "cache-inv@test.com", PasswordHash: "hash", Name: "Cache Inv", Role: model.RoleUser}
	_ = userRepo.Create(context.Background(), target)

	device := &model.Device{UniqueID: "cache-inv-dev", Name: "Cache Device", Status: "online"}
	_ = deviceRepo.Create(context.Background(), device, admin.ID)

	req := httptest.NewRequest(http.MethodPost, "/api/users/"+fmt.Sprint(target.ID)+"/devices/"+fmt.Sprint(device.ID), nil)
	req = withUserAndChi(req, admin, map[string]string{
		"id":       fmt.Sprint(target.ID),
		"deviceId": fmt.Sprint(device.ID),
	})
	rr := httptest.NewRecorder()
	h.AssignDevice(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}

	if len(invalidator.invalidatedIDs) == 0 {
		t.Error("expected InvalidateDevice to be called after AssignDevice")
	}
	if invalidator.invalidatedIDs[0] != device.ID {
		t.Errorf("expected InvalidateDevice called with device.ID=%d, got %d", device.ID, invalidator.invalidatedIDs[0])
	}
}

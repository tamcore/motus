package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	"github.com/tamcore/motus/internal/api/middleware"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

// TestRouter_ReadonlyApiKey_BlocksDelete is an end-to-end test that verifies
// readonly API keys cannot delete devices. This test uses the full router
// with all middleware (Auth + RequireWriteAccess) to catch any integration
// issues that unit tests might miss.
func TestRouter_ReadonlyApiKey_BlocksDelete(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	ctx := context.Background()
	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	// Create test user.
	user := &model.User{
		Email:        "readonly-delete-test@example.com",
		PasswordHash: "hash",
		Name:         "Readonly Delete Test",
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Create a device owned by the user.
	device := &model.Device{
		UniqueID: "readonly-device-001",
		Name:     "Test Device",
		Status:   "unknown",
	}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	// Create a READONLY API key for the user.
	readonlyKey := &model.ApiKey{
		UserID:      user.ID,
		Name:        "Readonly Key",
		Permissions: model.PermissionReadonly,
	}
	if err := apiKeyRepo.Create(ctx, readonlyKey); err != nil {
		t.Fatalf("create readonly api key: %v", err)
	}

	// Create a FULL access API key for comparison.
	fullKey := &model.ApiKey{
		UserID:      user.ID,
		Name:        "Full Key",
		Permissions: model.PermissionFull,
	}
	if err := apiKeyRepo.Create(ctx, fullKey); err != nil {
		t.Fatalf("create full api key: %v", err)
	}

	// Setup handlers.
	deviceHandler := handlers.NewDeviceHandler(deviceRepo, "")

	// Setup middleware.
	authMW := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	writeAccessMW := middleware.RequireWriteAccess

	// Create router config with write access enforcement.
	routerCfg := api.RouterConfig{
		WriteAccess: writeAccessMW,
	}

	// Build router with full middleware chain.
	h := api.Handlers{
		ListDevices:  deviceHandler.List,
		GetDevice:    deviceHandler.Get,
		CreateDevice: deviceHandler.Create,
		UpdateDevice: deviceHandler.Update,
		DeleteDevice: deviceHandler.Delete,
	}
	adminMW := middleware.RequireAdmin
	router := api.NewRouter(h, authMW, adminMW, nil, routerCfg)

	t.Run("readonly_key_can_GET_devices", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
		req.Header.Set("Authorization", "Bearer "+readonlyKey.Token)
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("GET /api/devices with readonly key: expected 200, got %d; body: %s",
				rr.Code, rr.Body.String())
		}
	})

	t.Run("readonly_key_CANNOT_POST_device", func(t *testing.T) {
		body := `{"uniqueId":"readonly-create-001","name":"Should Fail","protocol":"h02"}`
		req := httptest.NewRequest(http.MethodPost, "/api/devices", bytes.NewReader([]byte(body)))
		req.Header.Set("Authorization", "Bearer "+readonlyKey.Token)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("POST /api/devices with readonly key: expected 403, got %d; body: %s",
				rr.Code, rr.Body.String())
		}

		// Verify the error message mentions read-only permissions.
		var errResp map[string]string
		if err := json.NewDecoder(rr.Body).Decode(&errResp); err == nil {
			if errResp["error"] != "this API key has read-only permissions" {
				t.Errorf("expected specific error message, got: %q", errResp["error"])
			}
		}
	})

	t.Run("readonly_key_CANNOT_DELETE_device", func(t *testing.T) {
		// This is the critical test case that reproduces the reported bug.
		req := httptest.NewRequest(http.MethodDelete, "/api/devices/"+string(rune(device.ID)+'0'), nil)
		req.Header.Set("Authorization", "Bearer "+readonlyKey.Token)
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("DELETE /api/devices with readonly key: expected 403, got %d; body: %s",
				rr.Code, rr.Body.String())
		}

		// Verify the device still exists.
		existingDevice, err := deviceRepo.GetByID(ctx, device.ID)
		if err != nil {
			t.Errorf("device should still exist after blocked delete, got error: %v", err)
		}
		if existingDevice == nil {
			t.Error("device was deleted despite readonly key!")
		}
	})

	t.Run("readonly_key_CANNOT_PUT_device", func(t *testing.T) {
		body := `{"name":"Should Not Update"}`
		req := httptest.NewRequest(http.MethodPut, "/api/devices/"+string(rune(device.ID)+'0'), bytes.NewReader([]byte(body)))
		req.Header.Set("Authorization", "Bearer "+readonlyKey.Token)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("PUT /api/devices with readonly key: expected 403, got %d; body: %s",
				rr.Code, rr.Body.String())
		}
	})

	t.Run("full_key_CAN_DELETE_device", func(t *testing.T) {
		// Create another device to delete with full key.
		device2 := &model.Device{
			UniqueID: "full-delete-001",
			Name:     "Delete With Full Key",
			Status:   "unknown",
		}
		if err := deviceRepo.Create(ctx, device2, user.ID); err != nil {
			t.Fatalf("create device2: %v", err)
		}

		req := httptest.NewRequest(http.MethodDelete, "/api/devices/"+string(rune(device2.ID)+'0'), nil)
		req.Header.Set("Authorization", "Bearer "+fullKey.Token)
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Errorf("DELETE /api/devices with full key: expected 204, got %d; body: %s",
				rr.Code, rr.Body.String())
		}

		// Verify the device was deleted.
		_, err := deviceRepo.GetByID(ctx, device2.ID)
		if err == nil {
			t.Error("device should have been deleted with full access key")
		}
	})

	t.Run("session_auth_CAN_DELETE_device", func(t *testing.T) {
		// Verify that session-based auth (no API key) can still delete devices.
		device3 := &model.Device{
			UniqueID: "session-delete-001",
			Name:     "Delete With Session",
			Status:   "unknown",
		}
		if err := deviceRepo.Create(ctx, device3, user.ID); err != nil {
			t.Fatalf("create device3: %v", err)
		}

		session, err := sessionRepo.Create(ctx, user.ID)
		if err != nil {
			t.Fatalf("create session: %v", err)
		}

		req := httptest.NewRequest(http.MethodDelete, "/api/devices/"+string(rune(device3.ID)+'0'), nil)
		req.AddCookie(&http.Cookie{Name: "session_id", Value: session.ID})
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Errorf("DELETE /api/devices with session: expected 204, got %d; body: %s",
				rr.Code, rr.Body.String())
		}
	})
}

// TestRouter_ReadonlyApiKey_BlocksAllWriteEndpoints tests that readonly keys
// are blocked from ALL state-changing endpoints, not just devices.
func TestRouter_ReadonlyApiKey_BlocksAllWriteEndpoints(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	ctx := context.Background()
	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)
	geofenceRepo := repository.NewGeofenceRepository(pool)

	// Create test user.
	user := &model.User{
		Email:        "readonly-all-endpoints@example.com",
		PasswordHash: "hash",
		Name:         "Readonly All Endpoints",
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Create readonly API key.
	readonlyKey := &model.ApiKey{
		UserID:      user.ID,
		Name:        "Readonly Key",
		Permissions: model.PermissionReadonly,
	}
	if err := apiKeyRepo.Create(ctx, readonlyKey); err != nil {
		t.Fatalf("create readonly api key: %v", err)
	}

	// Setup handlers.
	deviceHandler := handlers.NewDeviceHandler(deviceRepo, "")
	geofenceHandler := handlers.NewGeofenceHandler(geofenceRepo)

	// Setup middleware.
	authMW := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	writeAccessMW := middleware.RequireWriteAccess

	// Create router.
	routerCfg := api.RouterConfig{
		WriteAccess: writeAccessMW,
	}
	h := api.Handlers{
		CreateDevice:   deviceHandler.Create,
		UpdateDevice:   deviceHandler.Update,
		DeleteDevice:   deviceHandler.Delete,
		CreateGeofence: geofenceHandler.Create,
		UpdateGeofence: geofenceHandler.Update,
		DeleteGeofence: geofenceHandler.Delete,
	}
	adminMW := middleware.RequireAdmin
	router := api.NewRouter(h, authMW, adminMW, nil, routerCfg)

	// Test cases for various write endpoints.
	testCases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"POST /api/devices", http.MethodPost, "/api/devices", `{"uniqueId":"test","name":"test"}`},
		{"PUT /api/devices/1", http.MethodPut, "/api/devices/1", `{"name":"updated"}`},
		{"DELETE /api/devices/1", http.MethodDelete, "/api/devices/1", ""},
		{"POST /api/geofences", http.MethodPost, "/api/geofences", `{"name":"test","area":"CIRCLE"}`},
		{"PUT /api/geofences/1", http.MethodPut, "/api/geofences/1", `{"name":"updated"}`},
		{"DELETE /api/geofences/1", http.MethodDelete, "/api/geofences/1", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var body io.Reader
			if tc.body != "" {
				body = bytes.NewReader([]byte(tc.body))
			}

			req := httptest.NewRequest(tc.method, tc.path, body)
			req.Header.Set("Authorization", "Bearer "+readonlyKey.Token)
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			// All write operations should be blocked.
			if rr.Code != http.StatusForbidden {
				t.Errorf("%s %s with readonly key: expected 403, got %d; body: %s",
					tc.method, tc.path, rr.Code, rr.Body.String())
			}
		})
	}
}

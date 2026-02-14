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

// setupValidationTest creates handler instances with a real database for
// testing input validation at the handler level.
func setupValidationTest(t *testing.T) (*handlers.UserHandler, *handlers.DeviceHandler, *model.User) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)

	admin := &model.User{
		Email:        "validation-admin@example.com",
		PasswordHash: "$2a$10$fakehash",
		Name:         "Validation Admin",
		Role:         model.RoleAdmin,
	}
	if err := userRepo.Create(context.Background(), admin); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	userHandler := handlers.NewUserHandler(userRepo, deviceRepo, "")
	deviceHandler := handlers.NewDeviceHandler(deviceRepo, "")
	return userHandler, deviceHandler, admin
}

func adminContext(r *http.Request, admin *model.User) *http.Request {
	return r.WithContext(api.ContextWithUser(r.Context(), admin))
}

// TestUserCreate_InvalidEmail verifies that the user Create handler
// rejects invalid email formats.
func TestUserCreate_InvalidEmail(t *testing.T) {
	uh, _, admin := setupValidationTest(t)

	tests := []struct {
		name  string
		email string
	}{
		{"no at sign", "userexample.com"},
		{"no domain", "user@"},
		{"no tld", "user@example"},
		{"spaces", "user @example.com"},
		{"angle brackets", "user<script>@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{
				"email":    tt.email,
				"password": "validpassword123",
				"name":     "Test",
			})
			req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(body))
			req = adminContext(req, admin)
			rr := httptest.NewRecorder()

			uh.Create(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for email %q, got %d: %s", tt.email, rr.Code, rr.Body.String())
			}
		})
	}
}

// TestUserCreate_InvalidPassword verifies that the user Create handler
// rejects passwords that are too short.
func TestUserCreate_InvalidPassword(t *testing.T) {
	uh, _, admin := setupValidationTest(t)

	tests := []struct {
		name     string
		password string
	}{
		{"empty", ""},
		{"too short", "1234567"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{
				"email":    "valid@example.com",
				"password": tt.password,
				"name":     "Test",
			})
			req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(body))
			req = adminContext(req, admin)
			rr := httptest.NewRecorder()

			uh.Create(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for password %q, got %d: %s", tt.password, rr.Code, rr.Body.String())
			}
		})
	}
}

// TestUserCreate_InvalidName verifies that the user Create handler
// rejects names with dangerous characters.
func TestUserCreate_InvalidName(t *testing.T) {
	uh, _, admin := setupValidationTest(t)

	body, _ := json.Marshal(map[string]string{
		"email":    "nametest@example.com",
		"password": "validpassword123",
		"name":     "name<script>alert(1)</script>",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(body))
	req = adminContext(req, admin)
	rr := httptest.NewRecorder()

	uh.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for XSS name, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestUserCreate_ValidInput verifies the happy path still works.
func TestUserCreate_ValidInput(t *testing.T) {
	uh, _, admin := setupValidationTest(t)

	body, _ := json.Marshal(map[string]string{
		"email":    "newuser@example.com",
		"password": "validpassword123",
		"name":     "New User",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewReader(body))
	req = adminContext(req, admin)
	rr := httptest.NewRecorder()

	uh.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201 for valid input, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestDeviceCreate_InvalidUniqueID verifies that the device Create handler
// rejects invalid unique IDs.
func TestDeviceCreate_InvalidUniqueID(t *testing.T) {
	_, dh, admin := setupValidationTest(t)

	tests := []struct {
		name     string
		uniqueID string
	}{
		{"empty", ""},
		{"spaces", "device 001"},
		{"special chars", "device@001"},
		{"angle brackets", "device<001>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{
				"uniqueId": tt.uniqueID,
				"name":     "Valid Name",
			})
			req := httptest.NewRequest(http.MethodPost, "/api/devices", bytes.NewReader(body))
			req = req.WithContext(api.ContextWithUser(req.Context(), admin))
			rr := httptest.NewRecorder()

			dh.Create(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for uniqueId %q, got %d: %s", tt.uniqueID, rr.Code, rr.Body.String())
			}
		})
	}
}

// TestDeviceCreate_InvalidName verifies that the device Create handler
// rejects invalid device names.
func TestDeviceCreate_InvalidName(t *testing.T) {
	_, dh, admin := setupValidationTest(t)

	body, _ := json.Marshal(map[string]string{
		"uniqueId": "valid-device-001",
		"name":     "name<script>",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/devices", bytes.NewReader(body))
	req = req.WithContext(api.ContextWithUser(req.Context(), admin))
	rr := httptest.NewRecorder()

	dh.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid name, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestDeviceCreate_ValidInput verifies the happy path still works.
func TestDeviceCreate_ValidInput(t *testing.T) {
	_, dh, admin := setupValidationTest(t)

	body, _ := json.Marshal(map[string]string{
		"uniqueId": "valid-device-002",
		"name":     "Valid Device",
		"protocol": "h02",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/devices", bytes.NewReader(body))
	req = req.WithContext(api.ContextWithUser(req.Context(), admin))
	rr := httptest.NewRecorder()

	dh.Create(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for valid input, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestDeviceUpdate_InvalidName verifies that the device Update handler
// rejects invalid names.
func TestDeviceUpdate_InvalidName(t *testing.T) {
	_, dh, admin := setupValidationTest(t)

	pool := testutil.SetupTestDB(t)
	deviceRepo := repository.NewDeviceRepository(pool)

	device := &model.Device{UniqueID: "update-val-001", Name: "Original", Status: "unknown"}
	if err := deviceRepo.Create(context.Background(), device, admin.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	body, _ := json.Marshal(map[string]string{
		"name": "name<script>alert(1)</script>",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/devices/1", bytes.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", device.ID))
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), admin), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	dh.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid name, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestUserUpdate_InvalidEmail verifies that the user Update handler
// rejects invalid email when updating.
func TestUserUpdate_InvalidEmail(t *testing.T) {
	uh, _, admin := setupValidationTest(t)

	pool := testutil.SetupTestDB(t)
	userRepo := repository.NewUserRepository(pool)

	target := &model.User{
		Email:        "target@example.com",
		PasswordHash: "$2a$10$fakehash",
		Name:         "Target User",
		Role:         model.RoleUser,
	}
	if err := userRepo.Create(context.Background(), target); err != nil {
		t.Fatalf("create target: %v", err)
	}

	body, _ := json.Marshal(map[string]string{
		"email": "invalid-email",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/users/1", bytes.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", target.ID))
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), admin), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	uh.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid email, got %d: %s", rr.Code, rr.Body.String())
	}
}

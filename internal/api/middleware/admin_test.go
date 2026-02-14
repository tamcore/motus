package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/middleware"
	"github.com/tamcore/motus/internal/model"
)

func TestRequireAdmin_NoUser(t *testing.T) {
	handler := middleware.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}

	var body map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["error"] != "admin access required" {
		t.Errorf("expected 'admin access required', got %q", body["error"])
	}
}

func TestRequireAdmin_NonAdminUser(t *testing.T) {
	handler := middleware.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	user := &model.User{ID: 1, Email: "user@example.com", Role: model.RoleUser}
	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	ctx := api.ContextWithUser(req.Context(), user)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestRequireAdmin_ReadonlyUser(t *testing.T) {
	handler := middleware.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	user := &model.User{ID: 1, Email: "ro@example.com", Role: model.RoleReadonly}
	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	ctx := api.ContextWithUser(req.Context(), user)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestRequireAdmin_AdminUser(t *testing.T) {
	innerCalled := false
	handler := middleware.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	user := &model.User{ID: 1, Email: "admin@example.com", Role: model.RoleAdmin}
	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	ctx := api.ContextWithUser(req.Context(), user)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !innerCalled {
		t.Error("expected inner handler to be called for admin user")
	}
}

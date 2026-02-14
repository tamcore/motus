package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/middleware"
	"github.com/tamcore/motus/internal/model"
)

func TestRequireWriteAccess_NoApiKey_AllowsAll(t *testing.T) {
	// Requests without an API key in context (e.g. session auth) should
	// not be restricted.
	handler := middleware.RequireWriteAccess(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/devices", nil)
			// No API key in context.
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("%s: expected 200, got %d", method, rr.Code)
			}
		})
	}
}

func TestRequireWriteAccess_FullKey_AllowsAll(t *testing.T) {
	handler := middleware.RequireWriteAccess(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	fullKey := &model.ApiKey{ID: 1, Permissions: model.PermissionFull}

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/devices", nil)
			ctx := api.ContextWithApiKey(req.Context(), fullKey)
			req = req.WithContext(ctx)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("%s: expected 200, got %d", method, rr.Code)
			}
		})
	}
}

func TestRequireWriteAccess_ReadonlyKey_AllowsGET(t *testing.T) {
	handler := middleware.RequireWriteAccess(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	readonlyKey := &model.ApiKey{ID: 1, Permissions: model.PermissionReadonly}

	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		t.Run(method+"_allowed", func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/devices", nil)
			ctx := api.ContextWithApiKey(req.Context(), readonlyKey)
			req = req.WithContext(ctx)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("%s: expected 200, got %d", method, rr.Code)
			}
		})
	}
}

func TestRequireWriteAccess_ReadonlyKey_BlocksWrites(t *testing.T) {
	handler := middleware.RequireWriteAccess(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	readonlyKey := &model.ApiKey{ID: 1, Permissions: model.PermissionReadonly}

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		t.Run(method+"_blocked", func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/devices", nil)
			ctx := api.ContextWithApiKey(req.Context(), readonlyKey)
			req = req.WithContext(ctx)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusForbidden {
				t.Errorf("%s: expected 403, got %d", method, rr.Code)
			}
		})
	}
}

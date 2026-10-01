package api_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/middleware"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/websocket"
)

type anonSec struct{}

func (anonSec) HandleBearerAuth(ctx context.Context, _ oas.OperationName, _ oas.BearerAuth) (context.Context, error) {
	return ctx, nil
}
func (anonSec) HandleCookieAuth(ctx context.Context, _ oas.OperationName, _ oas.CookieAuth) (context.Context, error) {
	return ctx, nil
}
func (anonSec) HandleXAuthToken(ctx context.Context, _ oas.OperationName, _ oas.XAuthToken) (context.Context, error) {
	return ctx, nil
}

func TestRouter_GetSessionCarriesCSRFToken(t *testing.T) {
	hub := websocket.NewHub(nil, nil, func(*http.Request) int64 { return 0 })
	router := api.NewRouter(oas.UnimplementedHandler{}, anonSec{}, hub, api.RouterConfig{
		CSRFProtect: middleware.CSRF(middleware.CSRFConfig{Secret: bytes.Repeat([]byte("k"), 32)}),
	})

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/session", nil))
	if rr.Header().Get("X-CSRF-Token") == "" {
		t.Errorf("missing X-CSRF-Token header (status %d)", rr.Code)
	}
}

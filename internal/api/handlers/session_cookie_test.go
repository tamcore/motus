package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/api"
)

func TestSetSessionCookie(t *testing.T) {
	rr := httptest.NewRecorder()
	ctx := api.ContextWithResponseWriter(t.Context(), rr)

	(&Handler{}).setSessionCookie(ctx, "abc", time.Now().Add(time.Hour))

	cookies := rr.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies, want 1", len(cookies))
	}
	c := cookies[0]
	if c.Name != "session_id" || c.Value != "abc" || c.Path != "/" || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("unexpected cookie: %+v", c)
	}
	if c.MaxAge < 3590 || c.MaxAge > 3600 {
		t.Errorf("MaxAge = %d, want ~3600", c.MaxAge)
	}
}

func TestSetSessionCookie_Clear(t *testing.T) {
	rr := httptest.NewRecorder()
	ctx := api.ContextWithResponseWriter(t.Context(), rr)

	(&Handler{}).setSessionCookie(ctx, "", time.Unix(0, 0))

	c := rr.Result().Cookies()[0]
	if c.Value != "" || c.MaxAge >= 0 {
		t.Errorf("expected cleared cookie, got %+v", c)
	}
}

func TestSetSessionCookie_NoWriter(t *testing.T) {
	(&Handler{}).setSessionCookie(t.Context(), "abc", time.Now())
}

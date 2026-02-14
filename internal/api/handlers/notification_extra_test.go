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
	"github.com/tamcore/motus/internal/services"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func setupNotificationHandlerWithService(t *testing.T) (*handlers.NotificationHandler, *repository.NotificationRepository, *model.User) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	notifRepo := repository.NewNotificationRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	geoRepo := repository.NewGeofenceRepository(pool)
	posRepo := repository.NewPositionRepository(pool)

	user := &model.User{Email: "notifextra@example.com", PasswordHash: "$2a$10$hash", Name: "Notif Extra"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	notifSvc := services.NewNotificationService(notifRepo, deviceRepo, geoRepo, posRepo)
	h := handlers.NewNotificationHandler(notifRepo, notifSvc)
	return h, notifRepo, user
}

func TestNotificationHandler_Test_Success(t *testing.T) {
	h, notifRepo, user := setupNotificationHandlerWithService(t)
	ctx := context.Background()

	// Start a test webhook server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rule := &model.NotificationRule{
		UserID:     user.ID,
		Name:       "Test Rule",
		EventTypes: []string{"geofenceEnter"},
		Channel:    "webhook",
		Config:     map[string]interface{}{"webhookUrl": srv.URL},
		Template:   `{"text":"test notification"}`,
		Enabled:    true,
	}
	_ = notifRepo.Create(ctx, rule)

	req := httptest.NewRequest(http.MethodPost, "/api/notifications/1/test", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", rule.ID))
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "sent" {
		t.Errorf("expected status 'sent', got %q", resp["status"])
	}
}

func TestNotificationHandler_Test_Unauthenticated(t *testing.T) {
	h, _, _ := setupNotificationHandlerWithService(t)

	req := httptest.NewRequest(http.MethodPost, "/api/notifications/1/test", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestNotificationHandler_Test_Forbidden(t *testing.T) {
	h, notifRepo, user := setupNotificationHandlerWithService(t)
	ctx := context.Background()

	rule := &model.NotificationRule{
		UserID:     user.ID,
		Name:       "Forbidden Test Rule",
		EventTypes: []string{"geofenceEnter"},
		Channel:    "webhook",
		Config:     map[string]interface{}{"webhookUrl": "https://example.com/hook"},
		Template:   `{"text":"test"}`,
		Enabled:    true,
	}
	_ = notifRepo.Create(ctx, rule)

	// Use a different user.
	otherUser := &model.User{ID: user.ID + 999, Email: "other@example.com", Name: "Other"}

	req := httptest.NewRequest(http.MethodPost, "/api/notifications/1/test", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", rule.ID))
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), otherUser), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestNotificationHandler_Test_NotFound(t *testing.T) {
	h, _, user := setupNotificationHandlerWithService(t)

	req := httptest.NewRequest(http.MethodPost, "/api/notifications/99999/test", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "99999")
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestNotificationHandler_Test_InvalidID(t *testing.T) {
	h, _, user := setupNotificationHandlerWithService(t)

	req := httptest.NewRequest(http.MethodPost, "/api/notifications/abc/test", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "abc")
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestNotificationHandler_Test_WebhookFails(t *testing.T) {
	h, notifRepo, user := setupNotificationHandlerWithService(t)
	ctx := context.Background()

	// Webhook server that returns 500.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	rule := &model.NotificationRule{
		UserID:     user.ID,
		Name:       "Failing Test Rule",
		EventTypes: []string{"geofenceExit"},
		Channel:    "webhook",
		Config:     map[string]interface{}{"webhookUrl": srv.URL},
		Template:   `{"text":"test"}`,
		Enabled:    true,
	}
	_ = notifRepo.Create(ctx, rule)

	req := httptest.NewRequest(http.MethodPost, "/api/notifications/1/test", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", rule.ID))
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Test(rr, req)

	// Test endpoint returns 200 with failure details in the body.
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "failed" {
		t.Errorf("expected status 'failed', got %q", resp["status"])
	}
}

func TestNotificationHandler_Update_InvalidJSON(t *testing.T) {
	h, notifRepo, user := setupNotificationHandlerWithService(t)
	ctx := context.Background()

	rule := &model.NotificationRule{
		UserID: user.ID, Name: "JSON Test", EventTypes: []string{"deviceOnline"},
		Channel: "webhook", Config: map[string]interface{}{}, Template: "t", Enabled: true,
	}
	_ = notifRepo.Create(ctx, rule)

	req := httptest.NewRequest(http.MethodPut, "/api/notifications/1", bytes.NewReader([]byte("not json")))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", rule.ID))
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestNotificationHandler_Update_MissingName(t *testing.T) {
	h, notifRepo, user := setupNotificationHandlerWithService(t)
	ctx := context.Background()

	rule := &model.NotificationRule{
		UserID: user.ID, Name: "Name Test", EventTypes: []string{"deviceOnline"},
		Channel: "webhook", Config: map[string]interface{}{}, Template: "t", Enabled: true,
	}
	_ = notifRepo.Create(ctx, rule)

	body := `{"eventTypes":["deviceOnline"],"channel":"webhook","template":"t"}`
	req := httptest.NewRequest(http.MethodPut, "/api/notifications/1", bytes.NewReader([]byte(body)))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", rule.ID))
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestNotificationHandler_Update_InvalidChannel(t *testing.T) {
	h, notifRepo, user := setupNotificationHandlerWithService(t)
	ctx := context.Background()

	rule := &model.NotificationRule{
		UserID: user.ID, Name: "Channel Test", EventTypes: []string{"deviceOnline"},
		Channel: "webhook", Config: map[string]interface{}{}, Template: "t", Enabled: true,
	}
	_ = notifRepo.Create(ctx, rule)

	body := `{"name":"Test","eventTypes":["deviceOnline"],"channel":"email","template":"t"}`
	req := httptest.NewRequest(http.MethodPut, "/api/notifications/1", bytes.NewReader([]byte(body)))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", rule.ID))
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestNotificationHandler_Update_Unauthenticated(t *testing.T) {
	h, _, _ := setupNotificationHandlerWithService(t)

	body := `{"name":"Test","eventTypes":["deviceOnline"],"channel":"webhook","template":"t"}`
	req := httptest.NewRequest(http.MethodPut, "/api/notifications/1", bytes.NewReader([]byte(body)))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestNotificationHandler_Update_InvalidID(t *testing.T) {
	h, _, user := setupNotificationHandlerWithService(t)

	body := `{"name":"Test","eventTypes":["deviceOnline"],"channel":"webhook","template":"t"}`
	req := httptest.NewRequest(http.MethodPut, "/api/notifications/abc", bytes.NewReader([]byte(body)))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "abc")
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestNotificationHandler_Create_Unauthenticated(t *testing.T) {
	h, _, _ := setupNotificationHandlerWithService(t)

	body := `{"name":"Test","eventTypes":["geofenceEnter"],"channel":"webhook","template":"t"}`
	req := httptest.NewRequest(http.MethodPost, "/api/notifications", bytes.NewReader([]byte(body)))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestNotificationHandler_Create_InvalidJSON(t *testing.T) {
	h, _, user := setupNotificationHandlerWithService(t)

	req := httptest.NewRequest(http.MethodPost, "/api/notifications", bytes.NewReader([]byte("not json")))
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestNotificationHandler_Create_WebhookSSRFValidation(t *testing.T) {
	h, _, user := setupNotificationHandlerWithService(t)

	body := `{
		"name":"SSRF Test",
		"eventTypes":["geofenceEnter"],
		"channel":"webhook",
		"config":{"webhookUrl":"https://10.0.0.1/internal"},
		"template":"test",
		"enabled":true
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/notifications", bytes.NewReader([]byte(body)))
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for SSRF webhook URL, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestNotificationHandler_Update_WebhookSSRFValidation(t *testing.T) {
	h, notifRepo, user := setupNotificationHandlerWithService(t)
	ctx := context.Background()

	rule := &model.NotificationRule{
		UserID: user.ID, Name: "SSRF Update", EventTypes: []string{"deviceOnline"},
		Channel: "webhook", Config: map[string]interface{}{"webhookUrl": "https://example.com/ok"}, Template: "t", Enabled: true,
	}
	_ = notifRepo.Create(ctx, rule)

	body := `{
		"name":"SSRF Update",
		"eventTypes":["deviceOnline"],
		"channel":"webhook",
		"config":{"webhookUrl":"https://192.168.1.1/internal"},
		"template":"t"
	}`
	req := httptest.NewRequest(http.MethodPut, "/api/notifications/1", bytes.NewReader([]byte(body)))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", rule.ID))
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for SSRF webhook URL, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestNotificationHandler_Delete_InvalidID(t *testing.T) {
	h, _, user := setupNotificationHandlerWithService(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/notifications/abc", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "abc")
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestNotificationHandler_Logs_InvalidID(t *testing.T) {
	h, _, user := setupNotificationHandlerWithService(t)

	req := httptest.NewRequest(http.MethodGet, "/api/notifications/abc/logs", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "abc")
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Logs(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

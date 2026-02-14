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

func setupNotificationHandler(t *testing.T) (*handlers.NotificationHandler, *repository.NotificationRepository, *model.User) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	notifRepo := repository.NewNotificationRepository(pool)

	user := &model.User{Email: "notifhandler@example.com", PasswordHash: "$2a$10$hash", Name: "Notif Handler"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// NotificationHandler takes a *NotificationService but for basic CRUD tests
	// we can pass nil (only Test endpoint uses it).
	h := handlers.NewNotificationHandler(notifRepo, nil)
	return h, notifRepo, user
}

func TestNotificationHandler_List_Empty(t *testing.T) {
	h, _, user := setupNotificationHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/notifications", nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var rules []model.NotificationRule
	_ = json.NewDecoder(rr.Body).Decode(&rules)
	if len(rules) != 0 {
		t.Errorf("expected empty list, got %d", len(rules))
	}
}

func TestNotificationHandler_List_Unauthenticated(t *testing.T) {
	h, _, _ := setupNotificationHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/notifications", nil)
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestNotificationHandler_Create_Success(t *testing.T) {
	h, _, user := setupNotificationHandler(t)

	body := `{
		"name":"Test Notif",
		"eventTypes":["geofenceEnter"],
		"channel":"webhook",
		"config":{"webhookUrl":"https://example.com/hook"},
		"template":"Device {{device.name}} entered",
		"enabled":true
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/notifications", bytes.NewReader([]byte(body)))
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var rule model.NotificationRule
	_ = json.NewDecoder(rr.Body).Decode(&rule)
	if rule.Name != "Test Notif" {
		t.Errorf("expected name 'Test Notif', got %q", rule.Name)
	}
}

func TestNotificationHandler_Create_MissingName(t *testing.T) {
	h, _, user := setupNotificationHandler(t)

	body := `{"eventTypes":["geofenceEnter"],"channel":"webhook","template":"t"}`
	req := httptest.NewRequest(http.MethodPost, "/api/notifications", bytes.NewReader([]byte(body)))
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestNotificationHandler_Create_InvalidEventType(t *testing.T) {
	h, _, user := setupNotificationHandler(t)

	body := `{"name":"Bad Event","eventTypes":["invalid"],"channel":"webhook","template":"t"}`
	req := httptest.NewRequest(http.MethodPost, "/api/notifications", bytes.NewReader([]byte(body)))
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestNotificationHandler_Create_InvalidChannel(t *testing.T) {
	h, _, user := setupNotificationHandler(t)

	body := `{"name":"Bad Channel","eventTypes":["geofenceEnter"],"channel":"email","template":"t"}`
	req := httptest.NewRequest(http.MethodPost, "/api/notifications", bytes.NewReader([]byte(body)))
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestNotificationHandler_Create_NtfyChannelRejected(t *testing.T) {
	h, _, user := setupNotificationHandler(t)

	body := `{"name":"Ntfy Rule","eventTypes":["geofenceEnter"],"channel":"ntfy","config":{"topic":"test"},"template":"t"}`
	req := httptest.NewRequest(http.MethodPost, "/api/notifications", bytes.NewReader([]byte(body)))
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for ntfy channel, got %d", rr.Code)
	}
}

func TestNotificationHandler_Create_MissingTemplate(t *testing.T) {
	h, _, user := setupNotificationHandler(t)

	body := `{"name":"No Template","eventTypes":["geofenceEnter"],"channel":"webhook"}`
	req := httptest.NewRequest(http.MethodPost, "/api/notifications", bytes.NewReader([]byte(body)))
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestNotificationHandler_Update_Success(t *testing.T) {
	h, notifRepo, user := setupNotificationHandler(t)
	ctx := context.Background()

	rule := &model.NotificationRule{
		UserID: user.ID, Name: "Before", EventTypes: []string{"deviceOnline"},
		Channel: "webhook", Config: map[string]interface{}{}, Template: "before", Enabled: true,
	}
	_ = notifRepo.Create(ctx, rule)

	body := `{
		"name":"After",
		"eventTypes":["deviceOffline"],
		"channel":"webhook",
		"config":{"webhookUrl":"https://example.com/hook"},
		"template":"after",
		"enabled":false
	}`
	req := httptest.NewRequest(http.MethodPut, "/api/notifications/1", bytes.NewReader([]byte(body)))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", rule.ID))
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestNotificationHandler_Update_InvalidEventType(t *testing.T) {
	h, notifRepo, user := setupNotificationHandler(t)
	ctx := context.Background()

	rule := &model.NotificationRule{
		UserID: user.ID, Name: "Rule", EventTypes: []string{"deviceOnline"},
		Channel: "webhook", Config: map[string]interface{}{}, Template: "t", Enabled: true,
	}
	_ = notifRepo.Create(ctx, rule)

	body := `{"name":"Rule","eventTypes":["badType"],"channel":"webhook","template":"t"}`
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

func TestNotificationHandler_Delete_Success(t *testing.T) {
	h, notifRepo, user := setupNotificationHandler(t)
	ctx := context.Background()

	rule := &model.NotificationRule{
		UserID: user.ID, Name: "Delete Me", EventTypes: []string{"geofenceExit"},
		Channel: "webhook", Config: map[string]interface{}{}, Template: "t", Enabled: true,
	}
	_ = notifRepo.Create(ctx, rule)

	req := httptest.NewRequest(http.MethodDelete, "/api/notifications/1", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", rule.ID))
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

func TestNotificationHandler_Delete_Unauthenticated(t *testing.T) {
	h, _, _ := setupNotificationHandler(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/notifications/1", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestNotificationHandler_Logs_Unauthenticated(t *testing.T) {
	h, _, _ := setupNotificationHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/notifications/1/logs", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Logs(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestNotificationHandler_Logs_Success(t *testing.T) {
	h, notifRepo, user := setupNotificationHandler(t)
	ctx := context.Background()

	rule := &model.NotificationRule{
		UserID: user.ID, Name: "Logs Rule", EventTypes: []string{"deviceOnline"},
		Channel: "webhook", Config: map[string]interface{}{}, Template: "t", Enabled: true,
	}
	_ = notifRepo.Create(ctx, rule)

	req := httptest.NewRequest(http.MethodGet, "/api/notifications/1/logs", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", rule.ID))
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Logs(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var logs []model.NotificationLog
	_ = json.NewDecoder(rr.Body).Decode(&logs)
	if len(logs) != 0 {
		t.Errorf("expected empty logs, got %d", len(logs))
	}
}

func TestNotificationHandler_Logs_Forbidden(t *testing.T) {
	h, notifRepo, user := setupNotificationHandler(t)
	ctx := context.Background()

	rule := &model.NotificationRule{
		UserID: user.ID, Name: "Other Rule", EventTypes: []string{"geofenceEnter"},
		Channel: "webhook", Config: map[string]interface{}{}, Template: "t", Enabled: true,
	}
	_ = notifRepo.Create(ctx, rule)

	otherUser := &model.User{ID: user.ID + 999, Email: "other@example.com", Name: "Other"}

	req := httptest.NewRequest(http.MethodGet, "/api/notifications/1/logs", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", rule.ID))
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), otherUser), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Logs(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

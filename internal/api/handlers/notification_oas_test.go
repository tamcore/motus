package handlers_test

// Tests for the ogen Handler notification methods (CreateNotification,
// UpdateNotification, DeleteNotification, NotificationLogs,
// TestNotification). Ported from the deleted chi NotificationHandler tests
// in notification_test.go and notification_extra_test.go.
//
// Dropped tests (no live equivalent):
//   - unauthenticated/invalid-JSON/invalid-ID transport tests: ogen owns
//     request decoding and path-param parsing; auth context handling is
//     asserted once per method family elsewhere in this package.
//
// SSRF note: the high-value webhook URL validation cases (private RFC1918
// targets) are ported verbatim from the old tests; the live handler funnels
// them through oasNotificationConfigToModel -> notification.ValidateWebhookURL.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/services"
	"github.com/tamcore/motus/internal/storage/repository"
)

// newNotificationTestHandler builds an ogen Handler from a mock notification
// repo. The NotificationService is wired over mock repositories; only the
// notification repo matters for the methods under test. The nil-pool audit
// logger exercises the audit code paths as documented no-ops.
func newNotificationTestHandler(notifications repository.NotificationRepo) *handlers.Handler {
	svc := services.NewNotificationService(notifications, &mockDeviceRepo{}, &auditMockGeofenceRepo{}, &auditMockPositionRepo{})
	return handlers.NewHandler(handlers.HandlerConfig{
		Notifications:       notifications,
		NotificationService: svc,
		AuditLogger:         audit.NewLogger(nil),
	})
}

func notificationTestUserCtx(id int64) context.Context {
	return api.ContextWithUser(context.Background(), &model.User{ID: id, Email: "notif@example.com", Role: model.RoleUser})
}

// webhookRuleConfig builds a typed webhook config for the given URL.
func webhookRuleConfig(t *testing.T, rawURL string) oas.NotificationRuleConfig {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse webhook URL %q: %v", rawURL, err)
	}
	return oas.NewNotificationConfigWebhookNotificationRuleConfig(oas.NotificationConfigWebhook{
		Channel:    oas.NotificationConfigWebhookChannelWebhook,
		WebhookUrl: *u,
	})
}

// validNotificationInput returns a fully valid webhook rule input.
// 127.0.0.1 is exempt from the SSRF private-IP check (test/dev convenience),
// keeping the test hermetic (no DNS resolution).
func validNotificationInput(t *testing.T) *oas.NotificationRuleInput {
	t.Helper()
	return &oas.NotificationRuleInput{
		Name:       "Test Notif",
		EventTypes: []string{"geofenceEnter"},
		Channel:    "webhook",
		Config:     webhookRuleConfig(t, "http://127.0.0.1:9/hook"),
		Template:   oas.NewOptString("Device {{device.name}} entered"),
		Enabled:    oas.NewOptBool(true),
	}
}

// ---------------------------------------------------------------------------
// CreateNotification
// ---------------------------------------------------------------------------

func TestCreateNotification_Success(t *testing.T) {
	var created *model.NotificationRule
	mock := &auditMockNotificationRepo{
		createFn: func(_ context.Context, rule *model.NotificationRule) error {
			rule.ID = 3
			created = rule
			return nil
		},
	}
	h := newNotificationTestHandler(mock)

	res, err := h.CreateNotification(notificationTestUserCtx(1), validNotificationInput(t))
	if err != nil {
		t.Fatalf("CreateNotification returned error: %v", err)
	}
	rule, ok := res.(*oas.NotificationRule)
	if !ok {
		t.Fatalf("expected *oas.NotificationRule, got %T", res)
	}
	if rule.Name != "Test Notif" {
		t.Errorf("expected name 'Test Notif', got %q", rule.Name)
	}
	if created == nil || created.UserID != 1 {
		t.Fatal("expected rule created for user 1")
	}
	if got, _ := created.Config["webhookUrl"].(string); got != "http://127.0.0.1:9/hook" {
		t.Errorf("expected webhookUrl persisted, got %q", got)
	}
}

func TestCreateNotification_MissingName(t *testing.T) {
	h := newNotificationTestHandler(&auditMockNotificationRepo{})

	in := validNotificationInput(t)
	in.Name = ""
	res, err := h.CreateNotification(notificationTestUserCtx(1), in)
	if err != nil {
		t.Fatalf("CreateNotification returned error: %v", err)
	}
	badReq, ok := res.(*oas.CreateNotificationBadRequest)
	if !ok {
		t.Fatalf("expected *oas.CreateNotificationBadRequest, got %T", res)
	}
	if badReq.Error != "name is required" {
		t.Errorf("unexpected error message: %q", badReq.Error)
	}
}

func TestCreateNotification_InvalidEventType(t *testing.T) {
	h := newNotificationTestHandler(&auditMockNotificationRepo{})

	in := validNotificationInput(t)
	in.EventTypes = []string{"invalid"}
	res, err := h.CreateNotification(notificationTestUserCtx(1), in)
	if err != nil {
		t.Fatalf("CreateNotification returned error: %v", err)
	}
	badReq, ok := res.(*oas.CreateNotificationBadRequest)
	if !ok {
		t.Fatalf("expected *oas.CreateNotificationBadRequest, got %T", res)
	}
	if badReq.Error != "invalid event type: invalid" {
		t.Errorf("unexpected error message: %q", badReq.Error)
	}
}

func TestCreateNotification_InvalidChannel(t *testing.T) {
	h := newNotificationTestHandler(&auditMockNotificationRepo{})

	in := validNotificationInput(t)
	in.Channel = "email"
	res, err := h.CreateNotification(notificationTestUserCtx(1), in)
	if err != nil {
		t.Fatalf("CreateNotification returned error: %v", err)
	}
	badReq, ok := res.(*oas.CreateNotificationBadRequest)
	if !ok {
		t.Fatalf("expected *oas.CreateNotificationBadRequest, got %T", res)
	}
	if badReq.Error != "invalid channel" {
		t.Errorf("unexpected error message: %q", badReq.Error)
	}
}

func TestCreateNotification_NtfyChannelRejected(t *testing.T) {
	h := newNotificationTestHandler(&auditMockNotificationRepo{})

	in := validNotificationInput(t)
	in.Channel = "ntfy"
	res, err := h.CreateNotification(notificationTestUserCtx(1), in)
	if err != nil {
		t.Fatalf("CreateNotification returned error: %v", err)
	}
	badReq, ok := res.(*oas.CreateNotificationBadRequest)
	if !ok {
		t.Fatalf("expected *oas.CreateNotificationBadRequest for ntfy channel, got %T", res)
	}
	if badReq.Error != "invalid channel" {
		t.Errorf("unexpected error message: %q", badReq.Error)
	}
}

func TestCreateNotification_MissingTemplate(t *testing.T) {
	h := newNotificationTestHandler(&auditMockNotificationRepo{})

	in := validNotificationInput(t)
	in.Template = oas.OptString{}
	res, err := h.CreateNotification(notificationTestUserCtx(1), in)
	if err != nil {
		t.Fatalf("CreateNotification returned error: %v", err)
	}
	badReq, ok := res.(*oas.CreateNotificationBadRequest)
	if !ok {
		t.Fatalf("expected *oas.CreateNotificationBadRequest, got %T", res)
	}
	if badReq.Error != "template is required" {
		t.Errorf("unexpected error message: %q", badReq.Error)
	}
}

// TestCreateNotification_WebhookSSRFValidation ports the exact SSRF case
// from the old chi test: a webhook URL pointing into RFC1918 space
// (https://10.0.0.1/internal) must be rejected.
func TestCreateNotification_WebhookSSRFValidation(t *testing.T) {
	createCalled := false
	mock := &auditMockNotificationRepo{
		createFn: func(_ context.Context, _ *model.NotificationRule) error {
			createCalled = true
			return nil
		},
	}
	h := newNotificationTestHandler(mock)

	in := validNotificationInput(t)
	in.Name = "SSRF Test"
	in.Config = webhookRuleConfig(t, "https://10.0.0.1/internal")
	res, err := h.CreateNotification(notificationTestUserCtx(1), in)
	if err != nil {
		t.Fatalf("CreateNotification returned error: %v", err)
	}
	badReq, ok := res.(*oas.CreateNotificationBadRequest)
	if !ok {
		t.Fatalf("expected *oas.CreateNotificationBadRequest for SSRF webhook URL, got %T", res)
	}
	if !strings.HasPrefix(badReq.Error, "invalid webhook URL:") {
		t.Errorf("unexpected error message: %q", badReq.Error)
	}
	if createCalled {
		t.Error("Create must not be called for an SSRF webhook URL")
	}
}

// ---------------------------------------------------------------------------
// UpdateNotification
// ---------------------------------------------------------------------------

func TestUpdateNotification_Success(t *testing.T) {
	var updated *model.NotificationRule
	mock := &auditMockNotificationRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.NotificationRule, error) {
			return &model.NotificationRule{ID: id, UserID: 1, Name: "Before", EventTypes: []string{"deviceOnline"}, Channel: "webhook", Template: "before", Enabled: true}, nil
		},
		updateFn: func(_ context.Context, rule *model.NotificationRule) error {
			updated = rule
			return nil
		},
	}
	h := newNotificationTestHandler(mock)

	in := validNotificationInput(t)
	in.Name = "After"
	in.EventTypes = []string{"deviceOffline"}
	in.Template = oas.NewOptString("after")
	in.Enabled = oas.NewOptBool(false)
	res, err := h.UpdateNotification(notificationTestUserCtx(1), in, oas.UpdateNotificationParams{ID: 8})
	if err != nil {
		t.Fatalf("UpdateNotification returned error: %v", err)
	}
	rule, ok := res.(*oas.NotificationRule)
	if !ok {
		t.Fatalf("expected *oas.NotificationRule, got %T", res)
	}
	if rule.Name != "After" {
		t.Errorf("expected updated name 'After', got %q", rule.Name)
	}
	if updated == nil || updated.Name != "After" || updated.Enabled {
		t.Error("expected repository Update called with new name and enabled=false")
	}
}

func TestUpdateNotification_InvalidEventType(t *testing.T) {
	h := newNotificationTestHandler(&auditMockNotificationRepo{})

	in := validNotificationInput(t)
	in.EventTypes = []string{"badType"}
	res, err := h.UpdateNotification(notificationTestUserCtx(1), in, oas.UpdateNotificationParams{ID: 8})
	if err != nil {
		t.Fatalf("UpdateNotification returned error: %v", err)
	}
	if _, ok := res.(*oas.UpdateNotificationBadRequest); !ok {
		t.Fatalf("expected *oas.UpdateNotificationBadRequest, got %T", res)
	}
}

func TestUpdateNotification_MissingName(t *testing.T) {
	h := newNotificationTestHandler(&auditMockNotificationRepo{})

	in := validNotificationInput(t)
	in.Name = ""
	res, err := h.UpdateNotification(notificationTestUserCtx(1), in, oas.UpdateNotificationParams{ID: 8})
	if err != nil {
		t.Fatalf("UpdateNotification returned error: %v", err)
	}
	badReq, ok := res.(*oas.UpdateNotificationBadRequest)
	if !ok {
		t.Fatalf("expected *oas.UpdateNotificationBadRequest, got %T", res)
	}
	if badReq.Error != "name is required" {
		t.Errorf("unexpected error message: %q", badReq.Error)
	}
}

func TestUpdateNotification_InvalidChannel(t *testing.T) {
	h := newNotificationTestHandler(&auditMockNotificationRepo{})

	in := validNotificationInput(t)
	in.Channel = "email"
	res, err := h.UpdateNotification(notificationTestUserCtx(1), in, oas.UpdateNotificationParams{ID: 8})
	if err != nil {
		t.Fatalf("UpdateNotification returned error: %v", err)
	}
	badReq, ok := res.(*oas.UpdateNotificationBadRequest)
	if !ok {
		t.Fatalf("expected *oas.UpdateNotificationBadRequest, got %T", res)
	}
	if badReq.Error != "invalid channel" {
		t.Errorf("unexpected error message: %q", badReq.Error)
	}
}

// TestUpdateNotification_WebhookSSRFValidation ports the exact SSRF case
// from the old chi test: updating a rule to a private webhook target
// (https://192.168.1.1/internal) must be rejected.
func TestUpdateNotification_WebhookSSRFValidation(t *testing.T) {
	updateCalled := false
	mock := &auditMockNotificationRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.NotificationRule, error) {
			return &model.NotificationRule{ID: id, UserID: 1, Name: "SSRF Update", EventTypes: []string{"deviceOnline"}, Channel: "webhook", Template: "t", Enabled: true}, nil
		},
		updateFn: func(_ context.Context, _ *model.NotificationRule) error {
			updateCalled = true
			return nil
		},
	}
	h := newNotificationTestHandler(mock)

	in := validNotificationInput(t)
	in.Name = "SSRF Update"
	in.Config = webhookRuleConfig(t, "https://192.168.1.1/internal")
	res, err := h.UpdateNotification(notificationTestUserCtx(1), in, oas.UpdateNotificationParams{ID: 8})
	if err != nil {
		t.Fatalf("UpdateNotification returned error: %v", err)
	}
	badReq, ok := res.(*oas.UpdateNotificationBadRequest)
	if !ok {
		t.Fatalf("expected *oas.UpdateNotificationBadRequest for SSRF webhook URL, got %T", res)
	}
	if !strings.HasPrefix(badReq.Error, "invalid webhook URL:") {
		t.Errorf("unexpected error message: %q", badReq.Error)
	}
	if updateCalled {
		t.Error("Update must not be called for an SSRF webhook URL")
	}
}

// ---------------------------------------------------------------------------
// DeleteNotification
// ---------------------------------------------------------------------------

func TestDeleteNotification_Success(t *testing.T) {
	var deletedID int64
	mock := &auditMockNotificationRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.NotificationRule, error) {
			return &model.NotificationRule{ID: id, UserID: 1, Name: "Delete Me", EventTypes: []string{"geofenceExit"}, Channel: "webhook"}, nil
		},
		deleteFn: func(_ context.Context, id int64) error {
			deletedID = id
			return nil
		},
	}
	h := newNotificationTestHandler(mock)

	res, err := h.DeleteNotification(notificationTestUserCtx(1), oas.DeleteNotificationParams{ID: 4})
	if err != nil {
		t.Fatalf("DeleteNotification returned error: %v", err)
	}
	if _, ok := res.(*oas.DeleteNotificationNoContent); !ok {
		t.Fatalf("expected *oas.DeleteNotificationNoContent, got %T", res)
	}
	if deletedID != 4 {
		t.Errorf("expected delete called with ID=4, got %d", deletedID)
	}
}

func TestDeleteNotification_Forbidden(t *testing.T) {
	deleteCalled := false
	mock := &auditMockNotificationRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.NotificationRule, error) {
			return &model.NotificationRule{ID: id, UserID: 99, Name: "Other Rule", EventTypes: []string{"geofenceEnter"}, Channel: "webhook"}, nil
		},
		deleteFn: func(_ context.Context, _ int64) error {
			deleteCalled = true
			return nil
		},
	}
	h := newNotificationTestHandler(mock)

	res, err := h.DeleteNotification(notificationTestUserCtx(1), oas.DeleteNotificationParams{ID: 4})
	if err != nil {
		t.Fatalf("DeleteNotification returned error: %v", err)
	}
	if _, ok := res.(*oas.DeleteNotificationForbidden); !ok {
		t.Fatalf("expected *oas.DeleteNotificationForbidden, got %T", res)
	}
	if deleteCalled {
		t.Error("Delete must not be called for another user's rule")
	}
}

// ---------------------------------------------------------------------------
// NotificationLogs
// ---------------------------------------------------------------------------

func TestNotificationLogs_Success(t *testing.T) {
	mock := &auditMockNotificationRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.NotificationRule, error) {
			return &model.NotificationRule{ID: id, UserID: 1, Name: "Logs Rule", EventTypes: []string{"deviceOnline"}, Channel: "webhook"}, nil
		},
		getLogsByRuleFn: func(_ context.Context, _ int64, _ int) ([]*model.NotificationLog, error) {
			return nil, nil
		},
	}
	h := newNotificationTestHandler(mock)

	res, err := h.NotificationLogs(notificationTestUserCtx(1), oas.NotificationLogsParams{ID: 4})
	if err != nil {
		t.Fatalf("NotificationLogs returned error: %v", err)
	}
	logs, ok := res.(*oas.NotificationLogsOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.NotificationLogsOKApplicationJSON, got %T", res)
	}
	if len(*logs) != 0 {
		t.Errorf("expected empty logs, got %d", len(*logs))
	}
}

// TestNotificationLogs_Forbidden verifies the IDOR protection: a user must
// not be able to read delivery logs of another user's rule.
func TestNotificationLogs_Forbidden(t *testing.T) {
	mock := &auditMockNotificationRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.NotificationRule, error) {
			return &model.NotificationRule{ID: id, UserID: 99, Name: "Other Rule", EventTypes: []string{"geofenceEnter"}, Channel: "webhook"}, nil
		},
		getLogsByRuleFn: func(_ context.Context, _ int64, _ int) ([]*model.NotificationLog, error) {
			return nil, errors.New("must not be reached")
		},
	}
	h := newNotificationTestHandler(mock)

	res, err := h.NotificationLogs(notificationTestUserCtx(1), oas.NotificationLogsParams{ID: 4})
	if err != nil {
		t.Fatalf("NotificationLogs returned error: %v", err)
	}
	if _, ok := res.(*oas.NotificationLogsForbidden); !ok {
		t.Fatalf("expected *oas.NotificationLogsForbidden, got %T", res)
	}
}

// ---------------------------------------------------------------------------
// TestNotification
// ---------------------------------------------------------------------------

// webhookTestRuleRepo returns a mock repo whose rule posts to the given URL.
func webhookTestRuleRepo(ownerID int64, webhookURL string) *auditMockNotificationRepo {
	return &auditMockNotificationRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.NotificationRule, error) {
			return &model.NotificationRule{
				ID:         id,
				UserID:     ownerID,
				Name:       "Test Rule",
				EventTypes: []string{"geofenceEnter"},
				Channel:    "webhook",
				Config:     map[string]interface{}{"webhookUrl": webhookURL},
				Template:   `{"text":"test notification"}`,
				Enabled:    true,
			}, nil
		},
	}
}

func TestTestNotification_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := newNotificationTestHandler(webhookTestRuleRepo(1, srv.URL))

	res, err := h.TestNotification(notificationTestUserCtx(1), oas.TestNotificationParams{ID: 4})
	if err != nil {
		t.Fatalf("TestNotification returned error: %v", err)
	}
	if _, ok := res.(*oas.TestNotificationNoContent); !ok {
		t.Fatalf("expected *oas.TestNotificationNoContent, got %T", res)
	}
}

func TestTestNotification_Forbidden(t *testing.T) {
	h := newNotificationTestHandler(webhookTestRuleRepo(99, "http://127.0.0.1:9/hook"))

	res, err := h.TestNotification(notificationTestUserCtx(1), oas.TestNotificationParams{ID: 4})
	if err != nil {
		t.Fatalf("TestNotification returned error: %v", err)
	}
	if _, ok := res.(*oas.TestNotificationForbidden); !ok {
		t.Fatalf("expected *oas.TestNotificationForbidden, got %T", res)
	}
}

func TestTestNotification_NotFound(t *testing.T) {
	mock := &auditMockNotificationRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.NotificationRule, error) {
			return nil, errors.New("not found")
		},
	}
	h := newNotificationTestHandler(mock)

	res, err := h.TestNotification(notificationTestUserCtx(1), oas.TestNotificationParams{ID: 99999})
	if err != nil {
		t.Fatalf("TestNotification returned error: %v", err)
	}
	if _, ok := res.(*oas.TestNotificationNotFound); !ok {
		t.Fatalf("expected *oas.TestNotificationNotFound, got %T", res)
	}
}

// TestTestNotification_WebhookFails verifies that a failing webhook target
// (HTTP 500) surfaces as a typed error response instead of success.
func TestTestNotification_WebhookFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	h := newNotificationTestHandler(webhookTestRuleRepo(1, srv.URL))

	res, err := h.TestNotification(notificationTestUserCtx(1), oas.TestNotificationParams{ID: 4})
	if err != nil {
		t.Fatalf("TestNotification returned error: %v", err)
	}
	// The live handler maps SendTestNotification failures to the NotFound
	// error envelope carrying the failure message.
	failure, ok := res.(*oas.TestNotificationNotFound)
	if !ok {
		t.Fatalf("expected *oas.TestNotificationNotFound for failing webhook, got %T", res)
	}
	if failure.Error == "" {
		t.Error("expected non-empty failure message")
	}
}

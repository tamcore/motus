package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func setupNotificationService(t *testing.T) (
	*NotificationService,
	*repository.NotificationRepository,
	*repository.DeviceRepository,
	*repository.GeofenceRepository,
	*repository.PositionRepository,
	*repository.EventRepository,
	*repository.UserRepository,
) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	notifRepo := repository.NewNotificationRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	geoRepo := repository.NewGeofenceRepository(pool)
	posRepo := repository.NewPositionRepository(pool)
	eventRepo := repository.NewEventRepository(pool)
	userRepo := repository.NewUserRepository(pool)

	svc := NewNotificationService(notifRepo, deviceRepo, geoRepo, posRepo)
	return svc, notifRepo, deviceRepo, geoRepo, posRepo, eventRepo, userRepo
}

func TestNotificationService_ProcessEvent_FindsMatchingRules(t *testing.T) {
	svc, notifRepo, deviceRepo, geoRepo, posRepo, eventRepo, userRepo := setupNotificationService(t)
	ctx := context.Background()

	// Setup a webhook server to receive notifications.
	received := make(chan string, 10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		received <- "ok"
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_ = geoRepo // Silence unused.
	_ = posRepo

	// Create user + device.
	user := &model.User{Email: "notifsvc@example.com", PasswordHash: "hash", Name: "Notif Svc"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "notifsvc-dev", Name: "Notif Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// Create a notification rule matching geofenceEnter events.
	rule := &model.NotificationRule{
		UserID:     user.ID,
		Name:       "Enter Webhook",
		EventTypes: []string{"geofenceEnter"},
		Channel:    "webhook",
		Config: map[string]interface{}{
			"webhookUrl": srv.URL,
		},
		Template: `{"text":"Device {{device.name}} entered geofence"}`,
		Enabled:  true,
	}
	_ = notifRepo.Create(ctx, rule)

	// Create an event.
	event := &model.Event{
		DeviceID:  device.ID,
		Type:      "geofenceEnter",
		Timestamp: time.Now().UTC(),
	}
	_ = eventRepo.Create(ctx, event)

	// Process the event -- should find matching rules and send notification.
	if err := svc.ProcessEvent(ctx, event); err != nil {
		t.Fatalf("ProcessEvent failed: %v", err)
	}

	// Wait for the async goroutine to send.
	select {
	case <-received:
		// Success: notification was sent.
	case <-time.After(5 * time.Second):
		t.Error("timed out waiting for notification to be sent")
	}
}

func TestNotificationService_ProcessEvent_SkipsDisabledRules(t *testing.T) {
	svc, notifRepo, deviceRepo, _, _, eventRepo, userRepo := setupNotificationService(t)
	ctx := context.Background()

	user := &model.User{Email: "notifdis@example.com", PasswordHash: "hash", Name: "Notif Dis"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "notifdis-dev", Name: "Disabled Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// Create a DISABLED rule.
	rule := &model.NotificationRule{
		UserID:     user.ID,
		Name:       "Disabled Rule",
		EventTypes: []string{"geofenceEnter"},
		Channel:    "webhook",
		Config:     map[string]interface{}{"webhookUrl": "https://shouldnot.be.called"},
		Template:   "should not send",
		Enabled:    false,
	}
	_ = notifRepo.Create(ctx, rule)

	event := &model.Event{
		DeviceID:  device.ID,
		Type:      "geofenceEnter",
		Timestamp: time.Now().UTC(),
	}
	_ = eventRepo.Create(ctx, event)

	// Should not error and should not send (GetByEventType only returns enabled rules).
	if err := svc.ProcessEvent(ctx, event); err != nil {
		t.Fatalf("ProcessEvent failed: %v", err)
	}
}

func TestNotificationService_ProcessEvent_NoRules(t *testing.T) {
	svc, _, deviceRepo, _, _, eventRepo, userRepo := setupNotificationService(t)
	ctx := context.Background()

	user := &model.User{Email: "notifnone@example.com", PasswordHash: "hash", Name: "No Rules"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "notifnone-dev", Name: "No Rules Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	event := &model.Event{
		DeviceID:  device.ID,
		Type:      "deviceOnline",
		Timestamp: time.Now().UTC(),
	}
	_ = eventRepo.Create(ctx, event)

	// Should not error when no rules match.
	if err := svc.ProcessEvent(ctx, event); err != nil {
		t.Fatalf("ProcessEvent failed: %v", err)
	}
}

func TestNotificationService_SendTestNotification(t *testing.T) {
	svc, _, _, _, _, _, _ := setupNotificationService(t)
	ctx := context.Background()

	// Start a test server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rule := &model.NotificationRule{
		EventTypes: []string{"geofenceEnter"},
		Channel:    "webhook",
		Config:     map[string]interface{}{"webhookUrl": srv.URL},
		Template:   `{"text":"test notification for {{device.name}}"}`,
	}

	statusCode, err := svc.SendTestNotification(ctx, rule)
	if err != nil {
		t.Fatalf("SendTestNotification failed: %v", err)
	}
	if statusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", statusCode)
	}
}

func TestNotificationService_SendTestNotification_WebhookError(t *testing.T) {
	svc, _, _, _, _, _, _ := setupNotificationService(t)
	ctx := context.Background()

	// Server that returns 500.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	rule := &model.NotificationRule{
		EventTypes: []string{"geofenceEnter"},
		Channel:    "webhook",
		Config:     map[string]interface{}{"webhookUrl": srv.URL},
		Template:   `{"text":"test"}`,
	}

	statusCode, err := svc.SendTestNotification(ctx, rule)
	if err == nil {
		t.Error("expected error for 500 response, got nil")
	}
	if statusCode != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", statusCode)
	}
}

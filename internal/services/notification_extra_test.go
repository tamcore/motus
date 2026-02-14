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
	"github.com/tamcore/motus/internal/websocket"
)

// TestNotificationService_SendNotification_WithGeofenceAndPosition tests the
// full sendNotification path including geofence and position enrichment.
func TestNotificationService_SendNotification_WithGeofenceAndPosition(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	notifRepo := repository.NewNotificationRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	geoRepo := repository.NewGeofenceRepository(pool)
	posRepo := repository.NewPositionRepository(pool)
	eventRepo := repository.NewEventRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	// Create user + device.
	user := &model.User{Email: "sendnotif@example.com", PasswordHash: "hash", Name: "Send Notif"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "sendnotif-dev", Name: "Notif Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// Create a geofence.
	geoJSON := `{"type":"Polygon","coordinates":[[[13.35,52.51],[13.35,52.53],[13.40,52.53],[13.40,52.51],[13.35,52.51]]]}`
	g := &model.Geofence{Name: "Test Fence", Geometry: geoJSON}
	_ = geoRepo.Create(ctx, g)

	// Create a position.
	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Timestamp: time.Now().UTC(),
	}
	_ = posRepo.Create(ctx, pos)

	// Create an event referencing geofence and position.
	geoID := g.ID
	posID := pos.ID
	event := &model.Event{
		DeviceID:   device.ID,
		GeofenceID: &geoID,
		PositionID: &posID,
		Type:       "geofenceEnter",
		Timestamp:  time.Now().UTC(),
	}
	_ = eventRepo.Create(ctx, event)

	// Start a test webhook server that validates the payload.
	received := make(chan map[string]interface{}, 10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		received <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Create a notification rule.
	rule := &model.NotificationRule{
		UserID:     user.ID,
		Name:       "Geofence Webhook",
		EventTypes: []string{"geofenceEnter"},
		Channel:    "webhook",
		Config:     map[string]interface{}{"webhookUrl": srv.URL},
		Template:   `{"device":"{{device.name}}","geofence":"{{geofence.name}}","lat":"{{position.latitude}}"}`,
		Enabled:    true,
	}
	_ = notifRepo.Create(ctx, rule)

	svc := NewNotificationService(notifRepo, deviceRepo, geoRepo, posRepo)

	// Process the event.
	if err := svc.ProcessEvent(ctx, event); err != nil {
		t.Fatalf("ProcessEvent failed: %v", err)
	}

	// Wait for the async notification to be sent.
	select {
	case body := <-received:
		// Verify the template was rendered with enriched data.
		if body["device"] != "Notif Device" {
			t.Errorf("expected device name 'Notif Device', got %v", body["device"])
		}
		if body["geofence"] != "Test Fence" {
			t.Errorf("expected geofence name 'Test Fence', got %v", body["geofence"])
		}
	case <-time.After(5 * time.Second):
		t.Error("timed out waiting for notification")
	}

	// Wait a moment for log delivery to complete.
	time.Sleep(500 * time.Millisecond)

	// Check that a notification log was recorded.
	logs, err := notifRepo.GetLogsByRule(ctx, rule.ID, 50)
	if err != nil {
		t.Fatalf("GetLogsByRule failed: %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("expected 1 log entry, got %d", len(logs))
	}
	if len(logs) > 0 && logs[0].Status != "sent" {
		t.Errorf("expected log status 'sent', got %q", logs[0].Status)
	}
}

// TestNotificationService_ProcessEvent_DeviceNotFound tests that ProcessEvent
// handles missing devices gracefully.
func TestNotificationService_ProcessEvent_DeviceNotFound(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	notifRepo := repository.NewNotificationRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	geoRepo := repository.NewGeofenceRepository(pool)
	posRepo := repository.NewPositionRepository(pool)
	ctx := context.Background()

	svc := NewNotificationService(notifRepo, deviceRepo, geoRepo, posRepo)

	event := &model.Event{
		DeviceID:  99999, // Non-existent.
		Type:      "deviceOnline",
		Timestamp: time.Now().UTC(),
	}

	err := svc.ProcessEvent(ctx, event)
	if err == nil {
		t.Error("expected error for non-existent device, got nil")
	}
}

func TestNewNotificationService(t *testing.T) {
	pool := testutil.SetupTestDB(t)

	notifRepo := repository.NewNotificationRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	geoRepo := repository.NewGeofenceRepository(pool)
	posRepo := repository.NewPositionRepository(pool)

	svc := NewNotificationService(notifRepo, deviceRepo, geoRepo, posRepo)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if svc.sender == nil {
		t.Error("expected sender to be initialized")
	}
}

// TestGeofenceEventService_CreateEvent_WithNotificationService tests the
// integration between geofence event creation and notification processing.
func TestGeofenceEventService_CreateEvent_WithNotificationService(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	geoRepo := repository.NewGeofenceRepository(pool)
	eventRepo := repository.NewEventRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	posRepo := repository.NewPositionRepository(pool)
	notifRepo := repository.NewNotificationRepository(pool)
	userRepo := repository.NewUserRepository(pool)

	ctx := context.Background()

	// Create user, device, geofence.
	user := &model.User{Email: "geoevent-notif@example.com", PasswordHash: "hash", Name: "Geo Notif"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "geonotif-dev", Name: "GeoNotif Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	geoJSON := `{"type":"Polygon","coordinates":[[[13.35,52.51],[13.35,52.53],[13.40,52.53],[13.40,52.51],[13.35,52.51]]]}`
	g := &model.Geofence{Name: "Notif Fence", Geometry: geoJSON}
	_ = geoRepo.Create(ctx, g)
	_ = geoRepo.AssociateUser(ctx, user.ID, g.ID)

	// Create webhook server.
	received := make(chan bool, 10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Create notification rule.
	rule := &model.NotificationRule{
		UserID:     user.ID,
		Name:       "Enter Webhook",
		EventTypes: []string{"geofenceEnter"},
		Channel:    "webhook",
		Config:     map[string]interface{}{"webhookUrl": srv.URL},
		Template:   `{"text":"entered"}`,
		Enabled:    true,
	}
	_ = notifRepo.Create(ctx, rule)

	// Create the services.
	notifSvc := NewNotificationService(notifRepo, deviceRepo, geoRepo, posRepo)
	hub := newTestHub()
	geoSvc := NewGeofenceEventService(geoRepo, eventRepo, deviceRepo, posRepo, hub, notifSvc)

	// Position inside the geofence.
	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Timestamp: time.Now().UTC(),
	}
	_ = posRepo.Create(ctx, pos)

	// Check geofences -- should create enter event and trigger notification.
	if err := geoSvc.CheckGeofences(ctx, pos); err != nil {
		t.Fatalf("CheckGeofences failed: %v", err)
	}

	// Verify enter event was created.
	events, _ := eventRepo.GetByDevice(ctx, device.ID, 100)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	// Wait for notification.
	select {
	case <-received:
		// Success.
	case <-time.After(5 * time.Second):
		t.Error("timed out waiting for notification from geofence event")
	}
}

func newTestHub() *websocket.Hub {
	return websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })
}

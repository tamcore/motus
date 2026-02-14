package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func TestNotificationRepository_Create(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	notifRepo := repository.NewNotificationRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "notif@example.com", PasswordHash: "hash", Name: "Notif User"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("Create user failed: %v", err)
	}

	rule := &model.NotificationRule{
		UserID:     user.ID,
		Name:       "Test Rule",
		EventTypes: []string{"geofenceEnter"},
		Channel:    "webhook",
		Config: map[string]interface{}{
			"webhookUrl": "https://example.com/hook",
		},
		Template: "Device {{device.name}} entered {{geofence.name}}",
		Enabled:  true,
	}

	if err := notifRepo.Create(ctx, rule); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if rule.ID == 0 {
		t.Error("expected rule ID to be set")
	}
	if rule.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestNotificationRepository_GetByID(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	notifRepo := repository.NewNotificationRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "notifget@example.com", PasswordHash: "hash", Name: "Notif Get"}
	_ = userRepo.Create(ctx, user)

	rule := &model.NotificationRule{
		UserID:     user.ID,
		Name:       "GetByID Rule",
		EventTypes: []string{"deviceOffline"},
		Channel:    "webhook",
		Config:     map[string]interface{}{"webhookUrl": "https://example.com/hook"},
		Template:   "Device {{device.name}} went offline",
		Enabled:    true,
	}
	_ = notifRepo.Create(ctx, rule)

	found, err := notifRepo.GetByID(ctx, rule.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found.Name != "GetByID Rule" {
		t.Errorf("expected name 'GetByID Rule', got %q", found.Name)
	}
	if found.Config["webhookUrl"] != "https://example.com/hook" {
		t.Errorf("expected config webhookUrl 'https://example.com/hook', got %v", found.Config["webhookUrl"])
	}
}

func TestNotificationRepository_GetByID_NotFound(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	notifRepo := repository.NewNotificationRepository(pool)
	ctx := context.Background()

	_, err := notifRepo.GetByID(ctx, 99999)
	if err == nil {
		t.Error("expected error for nonexistent rule")
	}
}

func TestNotificationRepository_GetByUser(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	notifRepo := repository.NewNotificationRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "notifusr@example.com", PasswordHash: "hash", Name: "Notif Usr"}
	_ = userRepo.Create(ctx, user)

	r1 := &model.NotificationRule{
		UserID: user.ID, Name: "Alpha Rule", EventTypes: []string{"geofenceEnter"},
		Channel: "webhook", Config: map[string]interface{}{}, Template: "t1", Enabled: true,
	}
	r2 := &model.NotificationRule{
		UserID: user.ID, Name: "Beta Rule", EventTypes: []string{"geofenceExit"},
		Channel: "webhook", Config: map[string]interface{}{}, Template: "t2", Enabled: false,
	}
	_ = notifRepo.Create(ctx, r1)
	_ = notifRepo.Create(ctx, r2)

	rules, err := notifRepo.GetByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByUser failed: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	// Ordered by name.
	if rules[0].Name != "Alpha Rule" {
		t.Errorf("expected first rule 'Alpha Rule', got %q", rules[0].Name)
	}
}

func TestNotificationRepository_GetByEventType(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	notifRepo := repository.NewNotificationRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "notifevt@example.com", PasswordHash: "hash", Name: "Notif Evt"}
	_ = userRepo.Create(ctx, user)

	// Create enabled and disabled rules for the same event type.
	r1 := &model.NotificationRule{
		UserID: user.ID, Name: "Enabled Enter", EventTypes: []string{"geofenceEnter"},
		Channel: "webhook", Config: map[string]interface{}{}, Template: "t", Enabled: true,
	}
	r2 := &model.NotificationRule{
		UserID: user.ID, Name: "Disabled Enter", EventTypes: []string{"geofenceEnter"},
		Channel: "webhook", Config: map[string]interface{}{}, Template: "t", Enabled: false,
	}
	r3 := &model.NotificationRule{
		UserID: user.ID, Name: "Exit Rule", EventTypes: []string{"geofenceExit"},
		Channel: "webhook", Config: map[string]interface{}{}, Template: "t", Enabled: true,
	}
	_ = notifRepo.Create(ctx, r1)
	_ = notifRepo.Create(ctx, r2)
	_ = notifRepo.Create(ctx, r3)

	// Should only return enabled rules for the specific event type.
	rules, err := notifRepo.GetByEventType(ctx, user.ID, "geofenceEnter")
	if err != nil {
		t.Fatalf("GetByEventType failed: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 enabled rule for geofenceEnter, got %d", len(rules))
	}
	if rules[0].Name != "Enabled Enter" {
		t.Errorf("expected 'Enabled Enter', got %q", rules[0].Name)
	}
}

func TestNotificationRepository_Update(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	notifRepo := repository.NewNotificationRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "notifupd@example.com", PasswordHash: "hash", Name: "Notif Upd"}
	_ = userRepo.Create(ctx, user)

	rule := &model.NotificationRule{
		UserID: user.ID, Name: "Before Update", EventTypes: []string{"deviceOnline"},
		Channel: "webhook", Config: map[string]interface{}{}, Template: "before", Enabled: true,
	}
	_ = notifRepo.Create(ctx, rule)

	rule.Name = "After Update"
	rule.Template = "after"
	rule.Enabled = false

	if err := notifRepo.Update(ctx, rule); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	found, err := notifRepo.GetByID(ctx, rule.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found.Name != "After Update" {
		t.Errorf("expected name 'After Update', got %q", found.Name)
	}
	if found.Enabled {
		t.Error("expected enabled to be false")
	}
}

func TestNotificationRepository_Delete(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	notifRepo := repository.NewNotificationRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "notifdel@example.com", PasswordHash: "hash", Name: "Notif Del"}
	_ = userRepo.Create(ctx, user)

	rule := &model.NotificationRule{
		UserID: user.ID, Name: "Delete Me", EventTypes: []string{"deviceOffline"},
		Channel: "webhook", Config: map[string]interface{}{}, Template: "t", Enabled: true,
	}
	_ = notifRepo.Create(ctx, rule)

	if err := notifRepo.Delete(ctx, rule.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := notifRepo.GetByID(ctx, rule.ID)
	if err == nil {
		t.Error("expected error after deletion")
	}
}

func TestNotificationRepository_LogDelivery(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	notifRepo := repository.NewNotificationRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	eventRepo := repository.NewEventRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "notiflog@example.com", PasswordHash: "hash", Name: "Notif Log"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "notiflog-dev-" + time.Now().Format("150405.000"), Name: "Log Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// Create an event to reference in the log.
	event := &model.Event{DeviceID: device.ID, Type: "geofenceEnter", Timestamp: time.Now().UTC()}
	_ = eventRepo.Create(ctx, event)

	rule := &model.NotificationRule{
		UserID: user.ID, Name: "Log Rule", EventTypes: []string{"geofenceEnter"},
		Channel: "webhook", Config: map[string]interface{}{}, Template: "t", Enabled: true,
	}
	_ = notifRepo.Create(ctx, rule)

	sentAt := time.Now().UTC()
	entry := &model.NotificationLog{
		RuleID:       rule.ID,
		EventID:      &event.ID,
		Status:       "sent",
		SentAt:       &sentAt,
		ResponseCode: 200,
	}

	if err := notifRepo.LogDelivery(ctx, entry); err != nil {
		t.Fatalf("LogDelivery failed: %v", err)
	}
	if entry.ID == 0 {
		t.Error("expected log entry ID to be set")
	}
}

func TestNotificationRepository_GetLogsByRule(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	notifRepo := repository.NewNotificationRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	eventRepo := repository.NewEventRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "logsget@example.com", PasswordHash: "hash", Name: "Logs Get"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "logsget-dev-" + time.Now().Format("150405.000"), Name: "Logs Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	event := &model.Event{DeviceID: device.ID, Type: "geofenceEnter", Timestamp: time.Now().UTC()}
	_ = eventRepo.Create(ctx, event)

	rule := &model.NotificationRule{
		UserID: user.ID, Name: "Logs Rule", EventTypes: []string{"geofenceEnter"},
		Channel: "webhook", Config: map[string]interface{}{}, Template: "t", Enabled: true,
	}
	_ = notifRepo.Create(ctx, rule)

	sentAt := time.Now().UTC()
	for i := 0; i < 3; i++ {
		_ = notifRepo.LogDelivery(ctx, &model.NotificationLog{
			RuleID: rule.ID, EventID: &event.ID, Status: "sent",
			SentAt: &sentAt, ResponseCode: 200,
		})
	}

	logs, err := notifRepo.GetLogsByRule(ctx, rule.ID, 10)
	if err != nil {
		t.Fatalf("GetLogsByRule failed: %v", err)
	}
	if len(logs) != 3 {
		t.Errorf("expected 3 logs, got %d", len(logs))
	}
}

func TestNotificationRepository_GetLogsByRule_LimitCap(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	notifRepo := repository.NewNotificationRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "logscap@example.com", PasswordHash: "hash", Name: "Logs Cap"}
	_ = userRepo.Create(ctx, user)

	rule := &model.NotificationRule{
		UserID: user.ID, Name: "Cap Rule", EventTypes: []string{"deviceOnline"},
		Channel: "webhook", Config: map[string]interface{}{}, Template: "t", Enabled: true,
	}
	_ = notifRepo.Create(ctx, rule)

	// Invalid limit should default to 100.
	logs, err := notifRepo.GetLogsByRule(ctx, rule.ID, 0)
	if err != nil {
		t.Fatalf("GetLogsByRule failed: %v", err)
	}
	if logs != nil {
		t.Errorf("expected nil for no logs, got %d", len(logs))
	}
}

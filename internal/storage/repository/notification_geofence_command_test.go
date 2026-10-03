package repository_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

// TestNotificationRepository_GeofenceIDsAndCommandChannel verifies that the
// geofence filter round-trips through every read path and that the command
// channel is accepted by the channel CHECK constraint (migration 00047).
func TestNotificationRepository_GeofenceIDsAndCommandChannel(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	notifRepo := repository.NewNotificationRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "notifgeo@example.com", PasswordHash: "hash", Name: "Notif Geo"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	rule := &model.NotificationRule{
		UserID:      user.ID,
		Name:        "Left home",
		EventTypes:  []string{"geofenceExit"},
		GeofenceIDs: []int64{11, 12},
		Channel:     model.NotificationChannelCommand,
		Config: map[string]any{
			"commandType": model.CommandPositionPeriodic,
			"attributes":  map[string]any{"frequency": 20},
		},
		Enabled: true,
	}
	if err := notifRepo.Create(ctx, rule); err != nil {
		t.Fatalf("Create command rule: %v", err)
	}

	got, err := notifRepo.GetByID(ctx, rule.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !slices.Equal(got.GeofenceIDs, []int64{11, 12}) {
		t.Errorf("GetByID GeofenceIDs = %v, want [11 12]", got.GeofenceIDs)
	}
	if got.Channel != model.NotificationChannelCommand || got.Config["commandType"] != model.CommandPositionPeriodic {
		t.Errorf("unexpected command rule: %+v", got)
	}

	byType, err := notifRepo.GetByEventType(ctx, user.ID, "geofenceExit")
	if err != nil || len(byType) != 1 || !slices.Equal(byType[0].GeofenceIDs, []int64{11, 12}) {
		t.Fatalf("GetByEventType = %+v, %v", byType, err)
	}
	byUser, err := notifRepo.GetByUser(ctx, user.ID)
	if err != nil || len(byUser) != 1 || !slices.Equal(byUser[0].GeofenceIDs, []int64{11, 12}) {
		t.Fatalf("GetByUser = %+v, %v", byUser, err)
	}
	all, err := notifRepo.GetAll(ctx)
	if err != nil || len(all) != 1 || !slices.Equal(all[0].GeofenceIDs, []int64{11, 12}) {
		t.Fatalf("GetAll = %+v, %v", all, err)
	}

	// Clearing the filter (nil) must persist as "all geofences".
	rule.GeofenceIDs = nil
	if err := notifRepo.Update(ctx, rule); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ = notifRepo.GetByID(ctx, rule.ID)
	if len(got.GeofenceIDs) != 0 {
		t.Errorf("expected empty geofence filter after update, got %v", got.GeofenceIDs)
	}

	// A webhook rule without a filter stores an empty array (NOT NULL column).
	webhook := &model.NotificationRule{
		UserID:     user.ID,
		Name:       "Webhook",
		EventTypes: []string{"alarm"},
		Channel:    model.NotificationChannelWebhook,
		Config:     map[string]any{"webhookUrl": "https://example.com/hook"},
		Template:   "x",
		Enabled:    true,
	}
	if err := notifRepo.Create(ctx, webhook); err != nil {
		t.Fatalf("Create webhook rule without geofence filter: %v", err)
	}

	// Unknown channels are still rejected by the CHECK constraint.
	bad := &model.NotificationRule{
		UserID: user.ID, Name: "Bad", EventTypes: []string{"alarm"},
		Channel: "ntfy", Config: map[string]any{}, Template: "x",
	}
	if err := notifRepo.Create(ctx, bad); err == nil {
		t.Error("expected CHECK constraint to reject unknown channel")
	}
}

// TestNotificationRepository_LogDeliveryQueued is a regression test for the
// valid_notification_status CHECK constraint: command deliveries to offline
// devices are logged as "queued" and must be accepted (migration 00047).
func TestNotificationRepository_LogDeliveryQueued(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	notifRepo := repository.NewNotificationRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "notifqueued@example.com", PasswordHash: "hash", Name: "Notif Queued"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	rule := &model.NotificationRule{
		UserID:     user.ID,
		Name:       "Back home",
		EventTypes: []string{"geofenceEnter"},
		Channel:    model.NotificationChannelCommand,
		Config:     map[string]any{"commandType": model.CommandPositionSingle},
		Enabled:    true,
	}
	if err := notifRepo.Create(ctx, rule); err != nil {
		t.Fatalf("Create command rule: %v", err)
	}

	sentAt := time.Now().UTC()
	entry := &model.NotificationLog{RuleID: rule.ID, Status: "queued", SentAt: &sentAt}
	if err := notifRepo.LogDelivery(ctx, entry); err != nil {
		t.Fatalf("LogDelivery(queued): %v", err)
	}
	logs, err := notifRepo.GetLogsByRule(ctx, rule.ID, 10)
	if err != nil {
		t.Fatalf("GetLogsByRule: %v", err)
	}
	if len(logs) != 1 || logs[0].Status != "queued" {
		t.Fatalf("logs = %+v, want one queued entry", logs)
	}

	// Unknown statuses are still rejected.
	if err := notifRepo.LogDelivery(ctx, &model.NotificationLog{RuleID: rule.ID, Status: "bogus"}); err == nil {
		t.Error("expected CHECK constraint to reject unknown status")
	}
}

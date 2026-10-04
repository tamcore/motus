package mcp

import (
	"fmt"
	"testing"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/services"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func TestNotificationRuleTools_UseSharedService(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	user := &model.User{Email: "mcp-rules@example.com", PasswordHash: "hash", Name: "MCP"}
	if err := repository.NewUserRepository(pool).Create(t.Context(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	ctx := api.ContextWithUser(t.Context(), user)
	rules := repository.NewNotificationRepository(pool)
	logger := audit.NewLogger(pool)
	deps := Deps{NotificationRules: services.NewNotificationRuleService(rules, repository.NewGeofenceRepository(pool), logger)}

	rule := &model.NotificationRule{
		UserID: user.ID, Name: "Home", EventTypes: []string{model.EventTypeGeofenceEnter},
		Channel: model.NotificationChannelWebhook, Config: map[string]any{"webhookUrl": "http://127.0.0.1:9/hook"},
		Template: "x", Enabled: true, GeofenceIDs: []int64{424242},
	}
	if err := rules.Create(t.Context(), rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}
	id := fmt.Sprint(rule.ID)

	res, _ := handleUpdateNotificationRule(ctx, callToolRequest(map[string]any{"id": id, "event_types": "deviceOnline"}), deps)
	if res.IsError {
		t.Fatalf("update: %s", resultText(t, res))
	}
	got, err := rules.GetByID(t.Context(), rule.ID)
	if err != nil {
		t.Fatalf("get rule: %v", err)
	}
	if len(got.GeofenceIDs) != 0 {
		t.Errorf("geofence filter = %v, want cleared once no geofence event remains", got.GeofenceIDs)
	}

	res, _ = handleDeleteNotificationRule(ctx, callToolRequest(map[string]any{"id": id}), deps)
	if res.IsError {
		t.Fatalf("delete: %s", resultText(t, res))
	}

	for _, action := range []string{audit.ActionNotifUpdate, audit.ActionNotifDelete} {
		if _, total, err := logger.Query(t.Context(), audit.QueryParams{UserID: &user.ID, Action: action}); err != nil || total != 1 {
			t.Errorf("%s: %d audit entries (err %v), want 1", action, total, err)
		}
	}
}

package mcp

import (
	"fmt"
	"maps"
	"strings"
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

	res, _ := handleUpdateNotificationRule(ctx, callToolRequest(map[string]any{"id": id, "event_types": "deviceOnline", "enabled": false}), deps)
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
	if got.Enabled {
		t.Error("enabled = true, want false from JSON boolean")
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

func TestCreateNotificationRuleTool(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	user := &model.User{Email: "mcp-create-rule@example.com", PasswordHash: "hash", Name: "MCP"}
	if err := repository.NewUserRepository(pool).Create(t.Context(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	ctx := api.ContextWithUser(t.Context(), user)
	rules := repository.NewNotificationRepository(pool)
	logger := audit.NewLogger(pool)
	deps := Deps{NotificationRules: services.NewNotificationRuleService(rules, repository.NewGeofenceRepository(pool), logger)}
	args := func(overrides map[string]any) map[string]any {
		a := map[string]any{
			"name": "Offline", "event_types": "deviceOffline", "channel": "webhook",
			"webhook_url": "http://127.0.0.1:9/hook", "template": "{{device.name}}",
		}
		maps.Copy(a, overrides)
		return a
	}

	t.Run("success audits and defaults enabled", func(t *testing.T) {
		res, _ := handleCreateNotificationRule(ctx, callToolRequest(args(nil)), deps)
		if res.IsError {
			t.Fatalf("create: %s", resultText(t, res))
		}
		list, err := rules.GetByUser(t.Context(), user.ID)
		if err != nil || len(list) != 1 {
			t.Fatalf("rules: %d (err %v), want 1", len(list), err)
		}
		if !list[0].Enabled {
			t.Error("enabled = false, want true by default")
		}
		entries, _, err := logger.Query(t.Context(), audit.QueryParams{UserID: &user.ID, Action: audit.ActionNotifCreate})
		if err != nil || len(entries) != 1 {
			t.Fatalf("audit entries: %d (err %v), want 1", len(entries), err)
		}
		if rt := entries[0].ResourceType; rt == nil || *rt != audit.ResourceNotification {
			t.Errorf("resourceType = %v, want %q", rt, audit.ResourceNotification)
		}
		if name := entries[0].Details["name"]; name != "Offline" {
			t.Errorf("details name = %v, want Offline", name)
		}
	})

	for name, tc := range map[string]struct {
		overrides map[string]any
		wantErr   string
	}{
		"missing template":    {map[string]any{"template": ""}, "template is required"},
		"missing webhook url": {map[string]any{"webhook_url": ""}, "webhook_url is required for webhook channel"},
		"non-webhook channel": {map[string]any{"channel": "command"}, "unsupported channel (supported: webhook)"},
	} {
		t.Run(name, func(t *testing.T) {
			res, _ := handleCreateNotificationRule(ctx, callToolRequest(args(tc.overrides)), deps)
			if !res.IsError || !strings.Contains(resultText(t, res), tc.wantErr) {
				t.Fatalf("got %q (error %v), want %q", resultText(t, res), res.IsError, tc.wantErr)
			}
		})
	}
}

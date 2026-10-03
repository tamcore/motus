package handlers_test

// Tests for the geofence filter and the "command" channel of notification
// rules (CreateNotification / UpdateNotification / TestNotification).

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/tamcore/motus/internal/api/handlers"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/services"
	"github.com/tamcore/motus/internal/storage/repository"
)

// newNotificationGeofenceTestHandler wires a geofence repo whose access check
// grants only the geofence IDs in allowed.
func newNotificationGeofenceTestHandler(notifications repository.NotificationRepo, allowed ...int64) *handlers.Handler {
	geofences := &auditMockGeofenceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, id int64) bool {
			return slices.Contains(allowed, id)
		},
	}
	svc := services.NewNotificationService(notifications, &mockDeviceRepo{}, geofences, &auditMockPositionRepo{}, nil, nil)
	return handlers.NewHandler(handlers.HandlerConfig{
		Notifications:       notifications,
		Geofences:           geofences,
		NotificationService: svc,
		AuditLogger:         audit.NewLogger(nil),
	})
}

func capturingNotifRepo(created **model.NotificationRule) *auditMockNotificationRepo {
	return &auditMockNotificationRepo{
		createFn: func(_ context.Context, rule *model.NotificationRule) error {
			rule.ID = 8
			*created = rule
			return nil
		},
	}
}

func intervalCommandConfig(seconds int) oas.NotificationRuleConfig {
	return oas.NewNotificationConfigCommandNotificationRuleConfig(oas.NotificationConfigCommand{
		Channel:     oas.NotificationConfigCommandChannelCommand,
		CommandType: model.CommandPositionPeriodic,
		Attributes: oas.NewOptCommandAttributes(oas.NewCommandAttrPositionPeriodicCommandAttributes(oas.CommandAttrPositionPeriodic{
			Type:      oas.CommandAttrPositionPeriodicTypePositionPeriodic,
			Frequency: seconds,
		})),
	})
}

// petExitRuleInput is the "left home -> report every 20 s" rule.
func petExitRuleInput() *oas.NotificationRuleInput {
	return &oas.NotificationRuleInput{
		Name:        "Rex left home",
		EventTypes:  []string{"geofenceExit"},
		Channel:     "command",
		Config:      intervalCommandConfig(20),
		Enabled:     oas.NewOptBool(true),
		GeofenceIds: []int64{100},
	}
}

func TestCreateNotification_CommandRuleWithGeofence(t *testing.T) {
	var created *model.NotificationRule
	h := newNotificationGeofenceTestHandler(capturingNotifRepo(&created), 100)

	res, err := h.CreateNotification(notificationTestUserCtx(1), petExitRuleInput())
	if err != nil {
		t.Fatalf("CreateNotification: %v", err)
	}
	out, ok := res.(*oas.NotificationRule)
	if !ok {
		t.Fatalf("expected *oas.NotificationRule, got %#v", res)
	}
	if created == nil {
		t.Fatal("rule not persisted")
	}
	if created.Channel != model.NotificationChannelCommand || !slices.Equal(created.GeofenceIDs, []int64{100}) {
		t.Errorf("unexpected persisted rule: %+v", created)
	}
	if created.Config["commandType"] != model.CommandPositionPeriodic {
		t.Errorf("commandType = %v", created.Config["commandType"])
	}
	attrs, _ := created.Config["attributes"].(map[string]any)
	if attrs["frequency"] != 20 {
		t.Errorf("attributes = %#v, want frequency 20", created.Config["attributes"])
	}
	if created.Template != "" {
		t.Errorf("command rules store no template, got %q", created.Template)
	}

	// Response round-trips the command config and the geofence filter.
	cmdCfg, ok := out.Config.GetNotificationConfigCommand()
	if !ok || cmdCfg.CommandType != model.CommandPositionPeriodic {
		t.Fatalf("response config = %#v", out.Config)
	}
	if !cmdCfg.Attributes.Set || cmdCfg.Attributes.Value.CommandAttrPositionPeriodic.Frequency != 20 {
		t.Errorf("response attributes = %#v", cmdCfg.Attributes)
	}
	if !slices.Equal(out.GeofenceIds, []int64{100}) {
		t.Errorf("response geofenceIds = %v", out.GeofenceIds)
	}
}

func TestCreateNotification_WebhookGeofenceFilterDeduplicated(t *testing.T) {
	var created *model.NotificationRule
	h := newNotificationGeofenceTestHandler(capturingNotifRepo(&created), 3, 4)

	in := validNotificationInput(t)
	in.GeofenceIds = []int64{4, 3, 4}
	res, _ := h.CreateNotification(notificationTestUserCtx(1), in)
	if _, ok := res.(*oas.NotificationRule); !ok {
		t.Fatalf("expected success, got %#v", res)
	}
	if !slices.Equal(created.GeofenceIDs, []int64{3, 4}) {
		t.Errorf("GeofenceIDs = %v, want sorted unique [3 4]", created.GeofenceIDs)
	}
}

func TestCreateNotification_NoGeofenceFilterReturnsEmptyList(t *testing.T) {
	var created *model.NotificationRule
	h := newNotificationGeofenceTestHandler(capturingNotifRepo(&created))

	res, _ := h.CreateNotification(notificationTestUserCtx(1), validNotificationInput(t))
	out, ok := res.(*oas.NotificationRule)
	if !ok {
		t.Fatalf("expected success, got %#v", res)
	}
	if out.GeofenceIds == nil || len(out.GeofenceIds) != 0 {
		t.Errorf("expected empty (non-nil) geofenceIds meaning all geofences, got %#v", out.GeofenceIds)
	}
}

func TestCreateNotification_GeofenceAccessDenied(t *testing.T) {
	var created *model.NotificationRule
	h := newNotificationGeofenceTestHandler(capturingNotifRepo(&created), 100)

	in := petExitRuleInput()
	in.GeofenceIds = []int64{100, 666}
	res, _ := h.CreateNotification(notificationTestUserCtx(1), in)
	bad, ok := res.(*oas.CreateNotificationBadRequest)
	if !ok || !strings.Contains(bad.Error, "geofence") {
		t.Fatalf("expected geofence access error, got %#v", res)
	}
	if created != nil {
		t.Error("rule must not be persisted")
	}
}

func TestCreateNotification_GeofenceFilterRequiresGeofenceEvent(t *testing.T) {
	var created *model.NotificationRule
	h := newNotificationGeofenceTestHandler(capturingNotifRepo(&created), 100)

	in := petExitRuleInput()
	in.EventTypes = []string{"deviceOffline"}
	res, _ := h.CreateNotification(notificationTestUserCtx(1), in)
	if _, ok := res.(*oas.CreateNotificationBadRequest); !ok {
		t.Fatalf("expected BadRequest, got %#v", res)
	}
}

func TestCreateNotification_CommandValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(in *oas.NotificationRuleInput)
		want   string
	}{
		{"factoryReset not allowed", func(in *oas.NotificationRuleInput) {
			in.Config = oas.NewNotificationConfigCommandNotificationRuleConfig(oas.NotificationConfigCommand{
				Channel: oas.NotificationConfigCommandChannelCommand, CommandType: model.CommandFactoryReset,
			})
		}, "command type"},
		{"unknown command type", func(in *oas.NotificationRuleInput) {
			in.Config = oas.NewNotificationConfigCommandNotificationRuleConfig(oas.NotificationConfigCommand{
				Channel: oas.NotificationConfigCommandChannelCommand, CommandType: "selfDestruct",
			})
		}, "command type"},
		{"interval missing", func(in *oas.NotificationRuleInput) {
			in.Config = oas.NewNotificationConfigCommandNotificationRuleConfig(oas.NotificationConfigCommand{
				Channel: oas.NotificationConfigCommandChannelCommand, CommandType: model.CommandPositionPeriodic,
			})
		}, "frequency"},
		{"interval not positive", func(in *oas.NotificationRuleInput) {
			in.Config = intervalCommandConfig(0)
		}, "frequency"},
		{"attributes for other command type", func(in *oas.NotificationRuleInput) {
			in.Config = oas.NewNotificationConfigCommandNotificationRuleConfig(oas.NotificationConfigCommand{
				Channel:     oas.NotificationConfigCommandChannelCommand,
				CommandType: model.CommandSosNumber,
				Attributes: oas.NewOptCommandAttributes(oas.NewCommandAttrPositionPeriodicCommandAttributes(oas.CommandAttrPositionPeriodic{
					Type: oas.CommandAttrPositionPeriodicTypePositionPeriodic, Frequency: 20,
				})),
			})
		}, "attributes"},
		{"custom without text", func(in *oas.NotificationRuleInput) {
			in.Config = oas.NewNotificationConfigCommandNotificationRuleConfig(oas.NotificationConfigCommand{
				Channel: oas.NotificationConfigCommandChannelCommand, CommandType: model.CommandCustom,
			})
		}, "text"},
		{"channel/config mismatch", func(in *oas.NotificationRuleInput) {
			in.Channel = "webhook"
		}, "config"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var created *model.NotificationRule
			h := newNotificationGeofenceTestHandler(capturingNotifRepo(&created), 100)
			in := petExitRuleInput()
			tt.mutate(in)
			res, err := h.CreateNotification(notificationTestUserCtx(1), in)
			if err != nil {
				t.Fatalf("CreateNotification: %v", err)
			}
			bad, ok := res.(*oas.CreateNotificationBadRequest)
			if !ok {
				t.Fatalf("expected BadRequest, got %#v", res)
			}
			if !strings.Contains(bad.Error, tt.want) {
				t.Errorf("error %q should mention %q", bad.Error, tt.want)
			}
			if created != nil {
				t.Error("invalid rule must not be persisted")
			}
		})
	}
}

func TestCreateNotification_ParameterlessCommandAllowed(t *testing.T) {
	var created *model.NotificationRule
	h := newNotificationGeofenceTestHandler(capturingNotifRepo(&created))

	in := &oas.NotificationRuleInput{
		Name:       "Locate on alarm",
		EventTypes: []string{"alarm"},
		Channel:    "command",
		Config: oas.NewNotificationConfigCommandNotificationRuleConfig(oas.NotificationConfigCommand{
			Channel: oas.NotificationConfigCommandChannelCommand, CommandType: model.CommandPositionSingle,
		}),
	}
	res, _ := h.CreateNotification(notificationTestUserCtx(1), in)
	if _, ok := res.(*oas.NotificationRule); !ok {
		t.Fatalf("expected success, got %#v", res)
	}
	if _, has := created.Config["attributes"]; has {
		t.Errorf("parameterless command must not store attributes: %#v", created.Config)
	}
}

func TestUpdateNotification_CommandRuleAndGeofenceAccess(t *testing.T) {
	var updated *model.NotificationRule
	mock := &auditMockNotificationRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.NotificationRule, error) {
			return &model.NotificationRule{ID: id, UserID: 1, Channel: "webhook", EventTypes: []string{"geofenceEnter"}}, nil
		},
		updateFn: func(_ context.Context, rule *model.NotificationRule) error {
			updated = rule
			return nil
		},
	}
	h := newNotificationGeofenceTestHandler(mock, 100)

	res, _ := h.UpdateNotification(notificationTestUserCtx(1), petExitRuleInput(), oas.UpdateNotificationParams{ID: 5})
	if _, ok := res.(*oas.NotificationRule); !ok {
		t.Fatalf("expected success, got %#v", res)
	}
	if updated == nil || updated.Channel != "command" || !slices.Equal(updated.GeofenceIDs, []int64{100}) {
		t.Fatalf("unexpected update: %+v", updated)
	}

	updated = nil
	in := petExitRuleInput()
	in.GeofenceIds = []int64{666}
	res, _ = h.UpdateNotification(notificationTestUserCtx(1), in, oas.UpdateNotificationParams{ID: 5})
	if _, ok := res.(*oas.UpdateNotificationBadRequest); !ok {
		t.Fatalf("expected BadRequest for inaccessible geofence, got %#v", res)
	}
	if updated != nil {
		t.Error("rule must not be updated")
	}
}

func TestCreateNotification_RejectsReconnectLoopCommands(t *testing.T) {
	commandCfg := func(cmdType string, attrs oas.OptCommandAttributes) oas.NotificationRuleConfig {
		return oas.NewNotificationConfigCommandNotificationRuleConfig(oas.NotificationConfigCommand{
			Channel: oas.NotificationConfigCommandChannelCommand, CommandType: cmdType, Attributes: attrs,
		})
	}
	customAttrs := oas.NewOptCommandAttributes(oas.NewCommandAttrCustomCommandAttributes(oas.CommandAttrCustom{
		Type: oas.CommandAttrCustomTypeCustom, Text: "RESET",
	}))
	tests := []struct {
		name       string
		cfg        oas.NotificationRuleConfig
		eventTypes []string
		wantOK     bool
	}{
		{"reboot on deviceOnline", commandCfg(model.CommandRebootDevice, oas.OptCommandAttributes{}), []string{"deviceOnline"}, false},
		{"reboot on deviceOffline", commandCfg(model.CommandRebootDevice, oas.OptCommandAttributes{}), []string{"alarm", "deviceOffline"}, false},
		{"custom on deviceOnline", commandCfg(model.CommandCustom, customAttrs), []string{"deviceOnline"}, false},
		{"custom on deviceOffline", commandCfg(model.CommandCustom, customAttrs), []string{"deviceOffline"}, false},
		{"reboot on alarm", commandCfg(model.CommandRebootDevice, oas.OptCommandAttributes{}), []string{"alarm"}, true},
		{"interval on deviceOnline", intervalCommandConfig(60), []string{"deviceOnline"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var created *model.NotificationRule
			h := newNotificationGeofenceTestHandler(capturingNotifRepo(&created))
			in := &oas.NotificationRuleInput{Name: "r", EventTypes: tt.eventTypes, Channel: "command", Config: tt.cfg}
			res, _ := h.CreateNotification(notificationTestUserCtx(1), in)
			if tt.wantOK {
				if _, ok := res.(*oas.NotificationRule); !ok {
					t.Fatalf("expected success, got %#v", res)
				}
				return
			}
			bad, ok := res.(*oas.CreateNotificationBadRequest)
			if !ok || !strings.Contains(bad.Error, "cannot be triggered") {
				t.Fatalf("expected reconnect-loop rejection, got %#v", res)
			}
			if created != nil {
				t.Error("rule must not be persisted")
			}
		})
	}
}

func TestCreateNotification_IntervalUpperBound(t *testing.T) {
	var created *model.NotificationRule
	h := newNotificationGeofenceTestHandler(capturingNotifRepo(&created), 100)

	in := petExitRuleInput()
	in.Config = intervalCommandConfig(model.MaxReportingIntervalSeconds)
	res, _ := h.CreateNotification(notificationTestUserCtx(1), in)
	if _, ok := res.(*oas.NotificationRule); !ok {
		t.Fatalf("interval of exactly the maximum must be accepted, got %#v", res)
	}

	created = nil
	in.Config = intervalCommandConfig(model.MaxReportingIntervalSeconds + 1)
	res, _ = h.CreateNotification(notificationTestUserCtx(1), in)
	bad, ok := res.(*oas.CreateNotificationBadRequest)
	if !ok || !strings.Contains(bad.Error, "frequency") {
		t.Fatalf("expected frequency upper-bound rejection, got %#v", res)
	}
	if created != nil {
		t.Error("rule must not be persisted")
	}
}

// TestCommandAttrPositionPeriodic_SpecMaximum checks that the OpenAPI schema
// (maximum: 86400) rejects larger intervals at request decoding.
func TestCommandAttrPositionPeriodic_SpecMaximum(t *testing.T) {
	ok := oas.CommandAttrPositionPeriodic{Type: oas.CommandAttrPositionPeriodicTypePositionPeriodic, Frequency: model.MaxReportingIntervalSeconds}
	if err := ok.Validate(); err != nil {
		t.Errorf("frequency %d must be valid: %v", ok.Frequency, err)
	}
	tooLarge := ok
	tooLarge.Frequency = model.MaxReportingIntervalSeconds + 1
	if err := tooLarge.Validate(); err == nil {
		t.Errorf("frequency %d must be rejected by the spec", tooLarge.Frequency)
	}
}

// geofenceUpdateHandler returns a handler whose stored rule 5 has the given
// geofence filter and records the updated rule.
func geofenceUpdateHandler(stored []int64, updated **model.NotificationRule, allowed ...int64) *handlers.Handler {
	mock := &auditMockNotificationRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.NotificationRule, error) {
			return &model.NotificationRule{
				ID: id, UserID: 1, Channel: "command", EventTypes: []string{"geofenceExit"},
				GeofenceIDs: slices.Clone(stored),
				Config:      map[string]any{"commandType": model.CommandPositionPeriodic, "attributes": map[string]any{"frequency": 20}},
			}, nil
		},
		updateFn: func(_ context.Context, rule *model.NotificationRule) error {
			*updated = rule
			return nil
		},
	}
	return newNotificationGeofenceTestHandler(mock, allowed...)
}

func TestUpdateNotification_AbsentGeofenceIdsKeepsFilter(t *testing.T) {
	var updated *model.NotificationRule
	h := geofenceUpdateHandler([]int64{100}, &updated, 100)

	in := petExitRuleInput()
	in.GeofenceIds = nil // field absent from the request body
	res, _ := h.UpdateNotification(notificationTestUserCtx(1), in, oas.UpdateNotificationParams{ID: 5})
	out, ok := res.(*oas.NotificationRule)
	if !ok {
		t.Fatalf("expected success, got %#v", res)
	}
	if !slices.Equal(updated.GeofenceIDs, []int64{100}) || !slices.Equal(out.GeofenceIds, []int64{100}) {
		t.Errorf("absent geofenceIds must keep the stored filter, got %v / %v", updated.GeofenceIDs, out.GeofenceIds)
	}
}

func TestUpdateNotification_EmptyGeofenceIdsClearsFilter(t *testing.T) {
	var updated *model.NotificationRule
	h := geofenceUpdateHandler([]int64{100}, &updated, 100)

	in := petExitRuleInput()
	in.GeofenceIds = []int64{} // explicit []
	res, _ := h.UpdateNotification(notificationTestUserCtx(1), in, oas.UpdateNotificationParams{ID: 5})
	if _, ok := res.(*oas.NotificationRule); !ok {
		t.Fatalf("expected success, got %#v", res)
	}
	if updated.GeofenceIDs == nil || len(updated.GeofenceIDs) != 0 {
		t.Errorf("explicit [] must clear the filter, got %#v", updated.GeofenceIDs)
	}
}

func TestUpdateNotification_AbsentGeofenceIdsWithoutGeofenceEventClearsFilter(t *testing.T) {
	var updated *model.NotificationRule
	h := geofenceUpdateHandler([]int64{100}, &updated, 100)

	in := petExitRuleInput()
	in.EventTypes = []string{"alarm"}
	in.GeofenceIds = nil
	res, _ := h.UpdateNotification(notificationTestUserCtx(1), in, oas.UpdateNotificationParams{ID: 5})
	if _, ok := res.(*oas.NotificationRule); !ok {
		t.Fatalf("expected success, got %#v", res)
	}
	if len(updated.GeofenceIDs) != 0 {
		t.Errorf("a filter without geofence event types has no effect and is dropped, got %v", updated.GeofenceIDs)
	}
}

func TestUpdateNotification_DeletedStoredGeofenceAllowed(t *testing.T) {
	// Geofence 666 was deleted (no longer accessible) but is still stored in
	// the rule. Re-sending it unchanged (Edit, enabled toggle) must work.
	var updated *model.NotificationRule
	h := geofenceUpdateHandler([]int64{100, 666}, &updated, 100, 101)

	in := petExitRuleInput()
	in.GeofenceIds = []int64{666, 100}
	in.Enabled = oas.NewOptBool(false)
	res, _ := h.UpdateNotification(notificationTestUserCtx(1), in, oas.UpdateNotificationParams{ID: 5})
	if _, ok := res.(*oas.NotificationRule); !ok {
		t.Fatalf("unchanged stored geofence IDs must be accepted, got %#v", res)
	}
	if !slices.Equal(updated.GeofenceIDs, []int64{100, 666}) {
		t.Errorf("GeofenceIDs = %v, want [100 666]", updated.GeofenceIDs)
	}

	// Explicitly removing the deleted geofence and adding a replacement works.
	updated = nil
	in.GeofenceIds = []int64{100, 101}
	res, _ = h.UpdateNotification(notificationTestUserCtx(1), in, oas.UpdateNotificationParams{ID: 5})
	if _, ok := res.(*oas.NotificationRule); !ok {
		t.Fatalf("expected success, got %#v", res)
	}
	if !slices.Equal(updated.GeofenceIDs, []int64{100, 101}) {
		t.Errorf("GeofenceIDs = %v, want [100 101]", updated.GeofenceIDs)
	}

	// Newly added IDs are still validated.
	updated = nil
	in.GeofenceIds = []int64{100, 666, 777}
	res, _ = h.UpdateNotification(notificationTestUserCtx(1), in, oas.UpdateNotificationParams{ID: 5})
	bad, ok := res.(*oas.UpdateNotificationBadRequest)
	if !ok || !strings.Contains(bad.Error, "777") {
		t.Fatalf("expected rejection of new inaccessible geofence 777, got %#v", res)
	}
	if updated != nil {
		t.Error("rule must not be updated")
	}
}

func TestTestNotification_CommandRuleNotTestable(t *testing.T) {
	mock := &auditMockNotificationRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.NotificationRule, error) {
			return &model.NotificationRule{
				ID: id, UserID: 1, Channel: "command", EventTypes: []string{"geofenceExit"},
				Config: map[string]any{"commandType": "positionSingle"},
			}, nil
		},
	}
	h := newNotificationGeofenceTestHandler(mock)

	res, err := h.TestNotification(notificationTestUserCtx(1), oas.TestNotificationParams{ID: 4})
	if err != nil {
		t.Fatalf("TestNotification: %v", err)
	}
	if _, ok := res.(*oas.TestNotificationBadRequest); !ok {
		t.Fatalf("expected *oas.TestNotificationBadRequest, got %#v", res)
	}
}

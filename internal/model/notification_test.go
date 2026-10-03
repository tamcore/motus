package model

import (
	"reflect"
	"testing"
)

func TestNotificationRule_MatchesEvent(t *testing.T) {
	tests := []struct {
		name  string
		rule  NotificationRule
		event Event
		want  bool
	}{
		{
			name:  "event type not subscribed",
			rule:  NotificationRule{EventTypes: []string{"geofenceEnter"}},
			event: Event{Type: "geofenceExit", GeofenceID: new(int64(1))},
			want:  false,
		},
		{
			name:  "no geofence filter matches every geofence",
			rule:  NotificationRule{EventTypes: []string{"geofenceEnter"}},
			event: Event{Type: "geofenceEnter", GeofenceID: new(int64(42))},
			want:  true,
		},
		{
			name:  "empty geofence filter matches every geofence",
			rule:  NotificationRule{EventTypes: []string{"geofenceExit"}, GeofenceIDs: []int64{}},
			event: Event{Type: "geofenceExit", GeofenceID: new(int64(42))},
			want:  true,
		},
		{
			name:  "geofence filter matches selected geofence",
			rule:  NotificationRule{EventTypes: []string{"geofenceEnter"}, GeofenceIDs: []int64{7, 42}},
			event: Event{Type: "geofenceEnter", GeofenceID: new(int64(42))},
			want:  true,
		},
		{
			name:  "geofence filter rejects other geofence",
			rule:  NotificationRule{EventTypes: []string{"geofenceEnter"}, GeofenceIDs: []int64{7}},
			event: Event{Type: "geofenceEnter", GeofenceID: new(int64(42))},
			want:  false,
		},
		{
			name:  "geofence filter rejects geofence event without geofence",
			rule:  NotificationRule{EventTypes: []string{"geofenceExit"}, GeofenceIDs: []int64{7}},
			event: Event{Type: "geofenceExit"},
			want:  false,
		},
		{
			name:  "geofence filter does not apply to non-geofence events",
			rule:  NotificationRule{EventTypes: []string{"geofenceEnter", "alarm"}, GeofenceIDs: []int64{7}},
			event: Event{Type: "alarm"},
			want:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rule.MatchesEvent(&tt.event); got != tt.want {
				t.Errorf("MatchesEvent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsGeofenceEventType(t *testing.T) {
	if !IsGeofenceEventType("geofenceEnter") || !IsGeofenceEventType("geofenceExit") {
		t.Error("geofenceEnter/geofenceExit must be geofence event types")
	}
	if IsGeofenceEventType("alarm") {
		t.Error("alarm must not be a geofence event type")
	}
}

func TestNotificationRule_DeviceCommand(t *testing.T) {
	t.Run("builds positionPeriodic command for device", func(t *testing.T) {
		rule := NotificationRule{
			Channel: NotificationChannelCommand,
			Config: map[string]any{
				"commandType": CommandPositionPeriodic,
				// JSON round-trip through JSONB yields float64 numbers.
				"attributes": map[string]any{"frequency": float64(20)},
			},
		}
		cmd, err := rule.DeviceCommand(9)
		if err != nil {
			t.Fatalf("DeviceCommand: %v", err)
		}
		if cmd.DeviceID != 9 || cmd.Type != CommandPositionPeriodic || cmd.Status != CommandStatusPending {
			t.Errorf("unexpected command: %+v", cmd)
		}
		if !reflect.DeepEqual(cmd.Attributes, map[string]any{"frequency": float64(20)}) {
			t.Errorf("unexpected attributes: %#v", cmd.Attributes)
		}
	})

	t.Run("parameterless command has no attributes", func(t *testing.T) {
		rule := NotificationRule{
			Channel: NotificationChannelCommand,
			Config:  map[string]any{"commandType": CommandPositionSingle},
		}
		cmd, err := rule.DeviceCommand(1)
		if err != nil {
			t.Fatalf("DeviceCommand: %v", err)
		}
		if cmd.Attributes != nil {
			t.Errorf("expected nil attributes, got %#v", cmd.Attributes)
		}
	})

	t.Run("attributes are copied, not shared with the rule", func(t *testing.T) {
		attrs := map[string]any{"frequency": float64(20)}
		rule := NotificationRule{
			Channel: NotificationChannelCommand,
			Config:  map[string]any{"commandType": CommandPositionPeriodic, "attributes": attrs},
		}
		cmd, _ := rule.DeviceCommand(1)
		cmd.Attributes["frequency"] = float64(1)
		if attrs["frequency"] != float64(20) {
			t.Error("mutating the command must not change the rule config")
		}
	})

	t.Run("errors", func(t *testing.T) {
		cases := map[string]NotificationRule{
			"wrong channel":   {Channel: NotificationChannelWebhook, Config: map[string]any{"commandType": CommandPositionSingle}},
			"no command type": {Channel: NotificationChannelCommand, Config: map[string]any{}},
			"unknown type":    {Channel: NotificationChannelCommand, Config: map[string]any{"commandType": "selfDestruct"}},
			"nil config":      {Channel: NotificationChannelCommand},
		}
		for name, rule := range cases {
			if _, err := rule.DeviceCommand(1); err == nil {
				t.Errorf("%s: expected error", name)
			}
		}
	})
}

func TestNotificationAutomationCommandTypes(t *testing.T) {
	types := NotificationCommandTypes()
	for _, ct := range types {
		if ct == CommandFactoryReset {
			t.Fatal("factoryReset must not be available as an automated notification command")
		}
	}
	for _, want := range []string{CommandPositionPeriodic, CommandPositionSingle, CommandRebootDevice} {
		found := false
		for _, ct := range types {
			if ct == want {
				found = true
			}
		}
		if !found {
			t.Errorf("expected %s to be an automation command type", want)
		}
	}
}

func TestValidateCommandEventTypes(t *testing.T) {
	tests := []struct {
		name       string
		cmdType    string
		eventTypes []string
		wantErr    bool
	}{
		{"reboot on deviceOnline loops", CommandRebootDevice, []string{EventTypeDeviceOnline}, true},
		{"reboot on deviceOffline loops", CommandRebootDevice, []string{"alarm", EventTypeDeviceOffline}, true},
		{"custom on deviceOnline loops", CommandCustom, []string{EventTypeDeviceOnline}, true},
		{"custom on deviceOffline loops", CommandCustom, []string{EventTypeDeviceOffline}, true},
		{"reboot on geofence exit is fine", CommandRebootDevice, []string{EventTypeGeofenceExit}, false},
		{"custom on alarm is fine", CommandCustom, []string{"alarm"}, false},
		{"interval on deviceOnline is fine", CommandPositionPeriodic, []string{EventTypeDeviceOnline, EventTypeDeviceOffline}, false},
		{"position request on deviceOnline is fine", CommandPositionSingle, []string{EventTypeDeviceOnline}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCommandEventTypes(tt.cmdType, tt.eventTypes)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCommandEventTypes(%s, %v) = %v, wantErr %v", tt.cmdType, tt.eventTypes, err, tt.wantErr)
			}
		})
	}
}

func TestNotificationRule_DeviceCommand_RejectsReconnectLoop(t *testing.T) {
	rule := NotificationRule{
		Channel:    NotificationChannelCommand,
		EventTypes: []string{"alarm", EventTypeDeviceOnline},
		Config:     map[string]any{"commandType": CommandRebootDevice},
	}
	if _, err := rule.DeviceCommand(1); err == nil {
		t.Fatal("a reboot rule triggered by deviceOnline must never build a command")
	}
}

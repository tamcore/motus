package model

import "testing"

func TestSupportedCommandTypes(t *testing.T) {
	types := SupportedCommandTypes()

	expected := map[string]bool{
		CommandRebootDevice:     true,
		CommandPositionPeriodic: true,
		CommandPositionSingle:   true,
		CommandSosNumber:        true,
		CommandCustom:           true,
		CommandSetSpeedAlarm:    true,
		CommandFactoryReset:     true,
	}

	if len(types) != len(expected) {
		t.Fatalf("expected %d command types, got %d", len(expected), len(types))
	}

	for _, ct := range types {
		if !expected[ct] {
			t.Errorf("unexpected command type: %q", ct)
		}
	}
}

func TestCommandConstants(t *testing.T) {
	if CommandRebootDevice != "rebootDevice" {
		t.Errorf("expected 'rebootDevice', got %q", CommandRebootDevice)
	}
	if CommandPositionPeriodic != "positionPeriodic" {
		t.Errorf("expected 'positionPeriodic', got %q", CommandPositionPeriodic)
	}
	if CommandPositionSingle != "positionSingle" {
		t.Errorf("expected 'positionSingle', got %q", CommandPositionSingle)
	}
	if CommandSosNumber != "sosNumber" {
		t.Errorf("expected 'sosNumber', got %q", CommandSosNumber)
	}
	if CommandCustom != "custom" {
		t.Errorf("expected 'custom', got %q", CommandCustom)
	}
	if CommandSetSpeedAlarm != "setSpeedAlarm" {
		t.Errorf("expected 'setSpeedAlarm', got %q", CommandSetSpeedAlarm)
	}
	if CommandFactoryReset != "factoryReset" {
		t.Errorf("expected 'factoryReset', got %q", CommandFactoryReset)
	}
	if CommandStatusPending != "pending" {
		t.Errorf("expected 'pending', got %q", CommandStatusPending)
	}
	if CommandStatusSent != "sent" {
		t.Errorf("expected 'sent', got %q", CommandStatusSent)
	}
}

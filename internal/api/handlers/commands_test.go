package handlers

import (
	"testing"

	"github.com/tamcore/motus/internal/model"
)

func TestIsValidCommandType(t *testing.T) {
	tests := []struct {
		name     string
		cmdType  string
		expected bool
	}{
		{"valid rebootDevice", model.CommandRebootDevice, true},
		{"valid positionPeriodic", model.CommandPositionPeriodic, true},
		{"valid positionSingle", model.CommandPositionSingle, true},
		{"valid sosNumber", model.CommandSosNumber, true},
		{"invalid empty string", "", false},
		{"invalid arbitrary type", "deleteAllData", false},
		{"invalid SQL injection", "'; DROP TABLE commands;--", false},
		{"invalid similar type", "reboot_device", false},
		{"invalid case variation", "RebootDevice", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidCommandType(tt.cmdType)
			if got != tt.expected {
				t.Errorf("isValidCommandType(%q) = %v, want %v", tt.cmdType, got, tt.expected)
			}
		})
	}
}

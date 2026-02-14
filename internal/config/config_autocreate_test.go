package config_test

import (
	"os"
	"testing"

	"github.com/tamcore/motus/internal/config"
)

// TestDeviceAutoCreate_LoadFromEnv_Defaults tests that device auto-create
// settings load correctly with default values when env vars are not set.
func TestDeviceAutoCreate_LoadFromEnv_Defaults(t *testing.T) {
	_ = os.Unsetenv("MOTUS_DEVICE_AUTO_CREATE")
	_ = os.Unsetenv("MOTUS_DEVICE_AUTO_CREATE_USER")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() failed: %v", err)
	}

	// Default: auto-create should be disabled.
	if cfg.Device.AutoCreateDevices {
		t.Error("expected AutoCreateDevices to default to false")
	}

	expectedUser := "admin@motus.local"
	if cfg.Device.AutoCreateDefaultUser != expectedUser {
		t.Errorf("AutoCreateDefaultUser = %q, want %q",
			cfg.Device.AutoCreateDefaultUser, expectedUser)
	}
}

// TestDeviceAutoCreate_LoadFromEnv_CustomEnabled tests loading custom
// enabled flag from environment.
func TestDeviceAutoCreate_LoadFromEnv_CustomEnabled(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     bool
	}{
		{
			name:     "explicitly enabled",
			envValue: "true",
			want:     true,
		},
		{
			name:     "disabled",
			envValue: "false",
			want:     false,
		},
		{
			name:     "enabled with 1",
			envValue: "1",
			want:     true,
		},
		{
			name:     "disabled with 0",
			envValue: "0",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MOTUS_DEVICE_AUTO_CREATE", tt.envValue)

			cfg, err := config.LoadFromEnv()
			if err != nil {
				t.Fatalf("LoadFromEnv() failed: %v", err)
			}

			if cfg.Device.AutoCreateDevices != tt.want {
				t.Errorf("AutoCreateDevices = %v, want %v",
					cfg.Device.AutoCreateDevices, tt.want)
			}
		})
	}
}

// TestDeviceAutoCreate_LoadFromEnv_CustomUser tests loading custom
// default user from environment.
func TestDeviceAutoCreate_LoadFromEnv_CustomUser(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     string
	}{
		{
			name:     "custom email",
			envValue: "custom@example.com",
			want:     "custom@example.com",
		},
		{
			name:     "empty string uses default",
			envValue: "",
			want:     "admin@motus.local",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("MOTUS_DEVICE_AUTO_CREATE_USER", tt.envValue)
			} else {
				_ = os.Unsetenv("MOTUS_DEVICE_AUTO_CREATE_USER")
			}

			cfg, err := config.LoadFromEnv()
			if err != nil {
				t.Fatalf("LoadFromEnv() failed: %v", err)
			}

			if cfg.Device.AutoCreateDefaultUser != tt.want {
				t.Errorf("AutoCreateDefaultUser = %q, want %q",
					cfg.Device.AutoCreateDefaultUser, tt.want)
			}
		})
	}
}

// TestDeviceAutoCreate_LoadFromEnv_BothSettings tests loading both
// auto-create settings together.
func TestDeviceAutoCreate_LoadFromEnv_BothSettings(t *testing.T) {
	t.Setenv("MOTUS_DEVICE_AUTO_CREATE", "true")
	t.Setenv("MOTUS_DEVICE_AUTO_CREATE_USER", "operator@company.com")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() failed: %v", err)
	}

	if !cfg.Device.AutoCreateDevices {
		t.Error("expected AutoCreateDevices to be true")
	}

	if cfg.Device.AutoCreateDefaultUser != "operator@company.com" {
		t.Errorf("AutoCreateDefaultUser = %q, want %q",
			cfg.Device.AutoCreateDefaultUser, "operator@company.com")
	}
}

// TestDeviceAutoCreate_InvalidBoolValue tests that invalid boolean values
// fallback to default behavior.
func TestDeviceAutoCreate_InvalidBoolValue(t *testing.T) {
	t.Setenv("MOTUS_DEVICE_AUTO_CREATE", "invalid")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() failed: %v", err)
	}

	// Should fallback to default (false).
	if cfg.Device.AutoCreateDevices {
		t.Error("expected AutoCreateDevices to fallback to default false")
	}
}

// Note: validation of auto-create enabled without user is tested in
// validate_test.go via direct struct manipulation, since getEnv()
// always returns the default when the env var is empty.

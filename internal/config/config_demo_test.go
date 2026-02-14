package config_test

import (
	"testing"

	"github.com/tamcore/motus/internal/config"
)

func TestLoadFromEnv_DemoDefaults(t *testing.T) {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Demo.Enabled {
		t.Error("demo mode should be disabled by default")
	}
	if cfg.Demo.GPXDir != "data/demo" {
		t.Errorf("expected GPXDir 'data/demo', got %q", cfg.Demo.GPXDir)
	}
	if cfg.Demo.ResetTime != "00:00" {
		t.Errorf("expected ResetTime '00:00', got %q", cfg.Demo.ResetTime)
	}
	if len(cfg.Demo.DeviceIMEIs) != 2 {
		t.Fatalf("expected 2 default device IMEIs, got %d", len(cfg.Demo.DeviceIMEIs))
	}
	if cfg.Demo.DeviceIMEIs[0] != "9000000000001" {
		t.Errorf("expected first IMEI '9000000000001', got %q", cfg.Demo.DeviceIMEIs[0])
	}
	if cfg.Demo.SpeedMultiplier != 1.0 {
		t.Errorf("expected SpeedMultiplier 1.0, got %f", cfg.Demo.SpeedMultiplier)
	}
	if cfg.Demo.InterpolationInterval != 100.0 {
		t.Errorf("expected InterpolationInterval 100.0, got %f", cfg.Demo.InterpolationInterval)
	}
}

func TestLoadFromEnv_DemoEnabled(t *testing.T) {
	t.Setenv("MOTUS_DEMO_ENABLED", "true")
	t.Setenv("MOTUS_DEMO_GPX_DIR", "/custom/path")
	t.Setenv("MOTUS_DEMO_RESET_TIME", "03:00")
	t.Setenv("MOTUS_DEMO_DEVICE_IMEIS", "DEV001,DEV002,DEV003")
	t.Setenv("MOTUS_DEMO_SPEED_MULTIPLIER", "10.0")
	t.Setenv("MOTUS_DEMO_INTERPOLATION_INTERVAL", "50.0")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.Demo.Enabled {
		t.Error("expected demo mode to be enabled")
	}
	if cfg.Demo.GPXDir != "/custom/path" {
		t.Errorf("expected GPXDir '/custom/path', got %q", cfg.Demo.GPXDir)
	}
	if cfg.Demo.ResetTime != "03:00" {
		t.Errorf("expected ResetTime '03:00', got %q", cfg.Demo.ResetTime)
	}
	if len(cfg.Demo.DeviceIMEIs) != 3 {
		t.Fatalf("expected 3 device IMEIs, got %d", len(cfg.Demo.DeviceIMEIs))
	}
	if cfg.Demo.DeviceIMEIs[2] != "DEV003" {
		t.Errorf("expected third IMEI 'DEV003', got %q", cfg.Demo.DeviceIMEIs[2])
	}
	if cfg.Demo.SpeedMultiplier != 10.0 {
		t.Errorf("expected SpeedMultiplier 10.0, got %f", cfg.Demo.SpeedMultiplier)
	}
	if cfg.Demo.InterpolationInterval != 50.0 {
		t.Errorf("expected InterpolationInterval 50.0, got %f", cfg.Demo.InterpolationInterval)
	}
}

func TestLoadFromEnv_DemoInvalidValues(t *testing.T) {
	t.Setenv("MOTUS_DEMO_ENABLED", "not-a-bool")
	t.Setenv("MOTUS_DEMO_SPEED_MULTIPLIER", "not-a-float")
	t.Setenv("MOTUS_DEMO_INTERPOLATION_INTERVAL", "not-a-float")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Demo.Enabled {
		t.Error("expected demo mode to fall back to disabled for invalid bool")
	}
	if cfg.Demo.SpeedMultiplier != 1.0 {
		t.Errorf("expected SpeedMultiplier 1.0 (default), got %f", cfg.Demo.SpeedMultiplier)
	}
	if cfg.Demo.InterpolationInterval != 100.0 {
		t.Errorf("expected InterpolationInterval 100.0 (default), got %f", cfg.Demo.InterpolationInterval)
	}
}

package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/config"
)

func TestDeviceConfig_Timeout(t *testing.T) {
	dc := config.DeviceConfig{TimeoutMinutes: 5, CheckIntervalMinutes: 1}

	timeout := dc.Timeout()
	if timeout.Minutes() != 5 {
		t.Errorf("expected 5 minutes, got %v", timeout)
	}
}

func TestDeviceConfig_CheckInterval(t *testing.T) {
	dc := config.DeviceConfig{TimeoutMinutes: 5, CheckIntervalMinutes: 2}

	interval := dc.CheckInterval()
	if interval.Minutes() != 2 {
		t.Errorf("expected 2 minutes, got %v", interval)
	}
}

func TestLoadFromEnv_DeviceConfig(t *testing.T) {
	t.Setenv("MOTUS_DEVICE_TIMEOUT_MINUTES", "10")
	t.Setenv("MOTUS_DEVICE_CHECK_INTERVAL_MINUTES", "3")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Device.TimeoutMinutes != 10 {
		t.Errorf("expected timeout 10, got %d", cfg.Device.TimeoutMinutes)
	}
	if cfg.Device.CheckIntervalMinutes != 3 {
		t.Errorf("expected check interval 3, got %d", cfg.Device.CheckIntervalMinutes)
	}
}

func TestLoadFromEnv_InvalidIntFallsBackToDefault(t *testing.T) {
	t.Setenv("MOTUS_DEVICE_TIMEOUT_MINUTES", "not-a-number")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Device.TimeoutMinutes != 5 {
		t.Errorf("expected default timeout 5, got %d", cfg.Device.TimeoutMinutes)
	}
}

func TestLoadFromEnv_WebSocketAllowedOrigins(t *testing.T) {
	t.Setenv("MOTUS_WS_ALLOWED_ORIGINS", "https://a.example.com, https://b.example.com")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.WebSocket.AllowedOrigins) != 2 {
		t.Fatalf("expected 2 allowed origins, got %d", len(cfg.WebSocket.AllowedOrigins))
	}
	if cfg.WebSocket.AllowedOrigins[0] != "https://a.example.com" {
		t.Errorf("expected first origin 'https://a.example.com', got %q", cfg.WebSocket.AllowedOrigins[0])
	}
	if cfg.WebSocket.AllowedOrigins[1] != "https://b.example.com" {
		t.Errorf("expected second origin 'https://b.example.com', got %q", cfg.WebSocket.AllowedOrigins[1])
	}
}

func TestLoadFromEnv_WebSocketEmptyOrigins(t *testing.T) {
	_ = os.Unsetenv("MOTUS_WS_ALLOWED_ORIGINS")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.WebSocket.AllowedOrigins != nil {
		t.Errorf("expected nil allowed origins, got %v", cfg.WebSocket.AllowedOrigins)
	}
}

func TestLoadFromEnv_GPSConfig(t *testing.T) {
	t.Setenv("MOTUS_GPS_H02_PORT", "6013")
	t.Setenv("MOTUS_GPS_WATCH_PORT", "6093")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.GPS.H02Port != "6013" {
		t.Errorf("expected H02 port 6013, got %q", cfg.GPS.H02Port)
	}
	if cfg.GPS.WatchPort != "6093" {
		t.Errorf("expected WATCH port 6093, got %q", cfg.GPS.WatchPort)
	}
}

func TestLoadFromEnv_RedisDefaults(t *testing.T) {
	_ = os.Unsetenv("MOTUS_REDIS_URL")
	_ = os.Unsetenv("MOTUS_REDIS_ENABLED")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Redis.Enabled {
		t.Error("expected Redis disabled by default")
	}
	if cfg.Redis.URL != "" {
		t.Errorf("expected empty Redis URL by default, got %q", cfg.Redis.URL)
	}
}

func TestLoadFromEnv_RedisCustomValues(t *testing.T) {
	t.Setenv("MOTUS_REDIS_URL", "redis://redis:6379")
	t.Setenv("MOTUS_REDIS_ENABLED", "true")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.Redis.Enabled {
		t.Error("expected Redis enabled")
	}
	if cfg.Redis.URL != "redis://redis:6379" {
		t.Errorf("expected Redis URL 'redis://redis:6379', got %q", cfg.Redis.URL)
	}
}

func TestLoadFromEnv_AllowedOriginsWithEmptyParts(t *testing.T) {
	t.Setenv("MOTUS_WS_ALLOWED_ORIGINS", "https://a.example.com, , https://b.example.com, ")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Empty parts should be filtered out.
	if len(cfg.WebSocket.AllowedOrigins) != 2 {
		t.Fatalf("expected 2 allowed origins (empty parts filtered), got %d: %v",
			len(cfg.WebSocket.AllowedOrigins), cfg.WebSocket.AllowedOrigins)
	}
}

func TestLoadFromEnv_PoolDefaults(t *testing.T) {
	// Ensure pool env vars are not set.
	_ = os.Unsetenv("MOTUS_DB_MAX_CONNS")
	_ = os.Unsetenv("MOTUS_DB_MIN_CONNS")
	_ = os.Unsetenv("MOTUS_DB_MAX_CONN_LIFETIME")
	_ = os.Unsetenv("MOTUS_DB_MAX_CONN_IDLE_TIME")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Database.Pool.MaxConns != 25 {
		t.Errorf("expected default MaxConns 25, got %d", cfg.Database.Pool.MaxConns)
	}
	if cfg.Database.Pool.MinConns != 5 {
		t.Errorf("expected default MinConns 5, got %d", cfg.Database.Pool.MinConns)
	}
	if cfg.Database.Pool.MaxConnLifetime != 1*time.Hour {
		t.Errorf("expected default MaxConnLifetime 1h, got %v", cfg.Database.Pool.MaxConnLifetime)
	}
	if cfg.Database.Pool.MaxConnIdleTime != 30*time.Minute {
		t.Errorf("expected default MaxConnIdleTime 30m, got %v", cfg.Database.Pool.MaxConnIdleTime)
	}
}

func TestLoadFromEnv_PoolCustomValues(t *testing.T) {
	t.Setenv("MOTUS_DB_MAX_CONNS", "50")
	t.Setenv("MOTUS_DB_MIN_CONNS", "10")
	t.Setenv("MOTUS_DB_MAX_CONN_LIFETIME", "2h")
	t.Setenv("MOTUS_DB_MAX_CONN_IDLE_TIME", "15m")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Database.Pool.MaxConns != 50 {
		t.Errorf("expected MaxConns 50, got %d", cfg.Database.Pool.MaxConns)
	}
	if cfg.Database.Pool.MinConns != 10 {
		t.Errorf("expected MinConns 10, got %d", cfg.Database.Pool.MinConns)
	}
	if cfg.Database.Pool.MaxConnLifetime != 2*time.Hour {
		t.Errorf("expected MaxConnLifetime 2h, got %v", cfg.Database.Pool.MaxConnLifetime)
	}
	if cfg.Database.Pool.MaxConnIdleTime != 15*time.Minute {
		t.Errorf("expected MaxConnIdleTime 15m, got %v", cfg.Database.Pool.MaxConnIdleTime)
	}
}

func TestLoadFromEnv_PoolInvalidDurationFallsBackToDefault(t *testing.T) {
	t.Setenv("MOTUS_DB_MAX_CONN_LIFETIME", "not-a-duration")
	t.Setenv("MOTUS_DB_MAX_CONN_IDLE_TIME", "bad")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Database.Pool.MaxConnLifetime != 1*time.Hour {
		t.Errorf("expected default MaxConnLifetime 1h on bad input, got %v", cfg.Database.Pool.MaxConnLifetime)
	}
	if cfg.Database.Pool.MaxConnIdleTime != 30*time.Minute {
		t.Errorf("expected default MaxConnIdleTime 30m on bad input, got %v", cfg.Database.Pool.MaxConnIdleTime)
	}
}

func TestValidate_PoolMinConnsExceedsMaxConns(t *testing.T) {
	t.Setenv("MOTUS_DB_MAX_CONNS", "5")
	t.Setenv("MOTUS_DB_MIN_CONNS", "10")

	_, err := config.LoadFromEnv()
	if err == nil {
		t.Fatal("expected validation error when MinConns > MaxConns")
	}
}

func TestLoadFromEnv_LogDefaults(t *testing.T) {
	_ = os.Unsetenv("MOTUS_LOG_LEVEL")
	_ = os.Unsetenv("MOTUS_LOG_FORMAT")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Log.Level != "INFO" {
		t.Errorf("expected default log level 'INFO', got %q", cfg.Log.Level)
	}
	if cfg.Log.Format != "" {
		t.Errorf("expected empty log format (auto-detect), got %q", cfg.Log.Format)
	}
}

func TestLoadFromEnv_LogCustomValues(t *testing.T) {
	t.Setenv("MOTUS_LOG_LEVEL", "DEBUG")
	t.Setenv("MOTUS_LOG_FORMAT", "text")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Log.Level != "DEBUG" {
		t.Errorf("expected log level 'DEBUG', got %q", cfg.Log.Level)
	}
	if cfg.Log.Format != "text" {
		t.Errorf("expected log format 'text', got %q", cfg.Log.Format)
	}
}

func TestLoadFromEnv_DeviceAutoCreateDefaults(t *testing.T) {
	_ = os.Unsetenv("MOTUS_DEVICE_AUTO_CREATE")
	_ = os.Unsetenv("MOTUS_DEVICE_AUTO_CREATE_USER")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Device.AutoCreateDevices {
		t.Error("expected auto-create disabled by default")
	}
	if cfg.Device.AutoCreateDefaultUser != "admin@motus.local" {
		t.Errorf("expected default user 'admin@motus.local', got %q", cfg.Device.AutoCreateDefaultUser)
	}
}

func TestLoadFromEnv_DeviceAutoCreateEnabled(t *testing.T) {
	t.Setenv("MOTUS_DEVICE_AUTO_CREATE", "true")
	t.Setenv("MOTUS_DEVICE_AUTO_CREATE_USER", "gps@example.com")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.Device.AutoCreateDevices {
		t.Error("expected auto-create enabled")
	}
	if cfg.Device.AutoCreateDefaultUser != "gps@example.com" {
		t.Errorf("expected user 'gps@example.com', got %q", cfg.Device.AutoCreateDefaultUser)
	}
}

func TestMetricsPortHandlesKubernetesServiceURL(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		wantPort string
	}{
		{"plain port", "9090", "9090"},
		{"kubernetes tcp url", "tcp://10.233.24.213:9090", "9090"},
		{"kubernetes tcp url with path", "tcp://10.233.24.213:9090/metrics", "9090"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MOTUS_METRICS_PORT", tt.envValue)
			t.Setenv("MOTUS_METRICS_ENABLED", "true")

			cfg, err := config.LoadFromEnv()
			if err != nil {
				t.Fatalf("LoadFromEnv() error = %v", err)
			}

			if cfg.Metrics.Port != tt.wantPort {
				t.Errorf("Metrics.Port = %q, want %q", cfg.Metrics.Port, tt.wantPort)
			}
		})
	}
}

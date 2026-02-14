package config

import (
	"strings"
	"testing"
	"time"
)

func TestValidate_DefaultConfig(t *testing.T) {
	// Default config from LoadFromEnv should be valid.
	cfg := validConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("default config should be valid, got: %v", err)
	}
}

func TestValidate_PortRanges(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr string
	}{
		{
			name:    "server port zero",
			modify:  func(c *Config) { c.Server.Port = "0" },
			wantErr: "MOTUS_SERVER_PORT: port 0 is out of range",
		},
		{
			name:    "server port negative",
			modify:  func(c *Config) { c.Server.Port = "-1" },
			wantErr: "MOTUS_SERVER_PORT: port -1 is out of range",
		},
		{
			name:    "server port too high",
			modify:  func(c *Config) { c.Server.Port = "70000" },
			wantErr: "MOTUS_SERVER_PORT: port 70000 is out of range",
		},
		{
			name:    "server port not a number",
			modify:  func(c *Config) { c.Server.Port = "abc" },
			wantErr: "MOTUS_SERVER_PORT: \"abc\" is not a valid port number",
		},
		{
			name:   "server port 1 valid",
			modify: func(c *Config) { c.Server.Port = "1" },
		},
		{
			name:   "server port 65535 valid",
			modify: func(c *Config) { c.Server.Port = "65535" },
		},
		{
			name:    "h02 port invalid",
			modify:  func(c *Config) { c.GPS.H02Port = "0" },
			wantErr: "MOTUS_GPS_H02_PORT",
		},
		{
			name:    "watch port invalid",
			modify:  func(c *Config) { c.GPS.WatchPort = "99999" },
			wantErr: "MOTUS_GPS_WATCH_PORT",
		},
		{
			name:    "metrics port invalid when enabled",
			modify:  func(c *Config) { c.Metrics.Enabled = true; c.Metrics.Port = "0" },
			wantErr: "MOTUS_METRICS_PORT",
		},
		{
			name:   "metrics port ignored when disabled",
			modify: func(c *Config) { c.Metrics.Enabled = false; c.Metrics.Port = "invalid" },
		},
		{
			name:    "database port invalid",
			modify:  func(c *Config) { c.Database.Port = "not-a-port" },
			wantErr: "MOTUS_DATABASE_PORT",
		},
		{
			name: "database port skipped when URI set",
			modify: func(c *Config) {
				c.Database.URI = "postgres://user:pass@host:5432/db"
				c.Database.Port = "invalid"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.modify(cfg)

			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidate_DatabaseConfig(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr string
	}{
		{
			name:    "no host and no URI",
			modify:  func(c *Config) { c.Database.Host = ""; c.Database.URI = "" },
			wantErr: "either POSTGRES_URI or MOTUS_DATABASE_HOST must be set",
		},
		{
			name:   "host set no URI",
			modify: func(c *Config) { c.Database.Host = "localhost"; c.Database.URI = "" },
		},
		{
			name:   "URI set no host",
			modify: func(c *Config) { c.Database.URI = "postgres://localhost/db"; c.Database.Host = "" },
		},
		{
			name: "both URI and host set",
			modify: func(c *Config) {
				c.Database.URI = "postgres://localhost/db"
				c.Database.Host = "localhost"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.modify(cfg)

			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidate_DeviceConfig(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr string
	}{
		{
			name:    "timeout zero",
			modify:  func(c *Config) { c.Device.TimeoutMinutes = 0 },
			wantErr: "MOTUS_DEVICE_TIMEOUT_MINUTES must be > 0",
		},
		{
			name:    "timeout negative",
			modify:  func(c *Config) { c.Device.TimeoutMinutes = -1 },
			wantErr: "MOTUS_DEVICE_TIMEOUT_MINUTES must be > 0",
		},
		{
			name:    "check interval zero",
			modify:  func(c *Config) { c.Device.CheckIntervalMinutes = 0 },
			wantErr: "MOTUS_DEVICE_CHECK_INTERVAL_MINUTES must be > 0",
		},
		{
			name:    "check interval negative",
			modify:  func(c *Config) { c.Device.CheckIntervalMinutes = -5 },
			wantErr: "MOTUS_DEVICE_CHECK_INTERVAL_MINUTES must be > 0",
		},
		{
			name: "auto-create enabled with user set",
			modify: func(c *Config) {
				c.Device.AutoCreateDevices = true
				c.Device.AutoCreateDefaultUser = "admin@motus.local"
			},
		},
		{
			name: "auto-create enabled without user",
			modify: func(c *Config) {
				c.Device.AutoCreateDevices = true
				c.Device.AutoCreateDefaultUser = ""
			},
			wantErr: "MOTUS_DEVICE_AUTO_CREATE_USER must be set when device auto-creation is enabled",
		},
		{
			name: "auto-create disabled without user is ok",
			modify: func(c *Config) {
				c.Device.AutoCreateDevices = false
				c.Device.AutoCreateDefaultUser = ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.modify(cfg)

			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidate_DemoConfig(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr string
	}{
		{
			name: "disabled demo skips validation",
			modify: func(c *Config) {
				c.Demo.Enabled = false
				c.Demo.SpeedMultiplier = -1 // would be invalid if enabled
			},
		},
		{
			name: "enabled demo with valid config",
			modify: func(c *Config) {
				c.Demo.Enabled = true
				c.Demo.SpeedMultiplier = 10
				c.Demo.InterpolationInterval = 100
				c.Demo.GPXDir = "/data/demo"
				c.Demo.DeviceIMEIs = []string{"DEV001"}
			},
		},
		{
			name: "speed multiplier zero",
			modify: func(c *Config) {
				c.Demo.Enabled = true
				c.Demo.SpeedMultiplier = 0
				c.Demo.InterpolationInterval = 100
				c.Demo.GPXDir = "/data/demo"
				c.Demo.DeviceIMEIs = []string{"DEV001"}
			},
			wantErr: "MOTUS_DEMO_SPEED_MULTIPLIER must be > 0",
		},
		{
			name: "speed multiplier negative",
			modify: func(c *Config) {
				c.Demo.Enabled = true
				c.Demo.SpeedMultiplier = -5
				c.Demo.InterpolationInterval = 100
				c.Demo.GPXDir = "/data/demo"
				c.Demo.DeviceIMEIs = []string{"DEV001"}
			},
			wantErr: "MOTUS_DEMO_SPEED_MULTIPLIER must be > 0",
		},
		{
			name: "interpolation interval zero",
			modify: func(c *Config) {
				c.Demo.Enabled = true
				c.Demo.SpeedMultiplier = 1
				c.Demo.InterpolationInterval = 0
				c.Demo.GPXDir = "/data/demo"
				c.Demo.DeviceIMEIs = []string{"DEV001"}
			},
			wantErr: "MOTUS_DEMO_INTERPOLATION_INTERVAL must be > 0",
		},
		{
			name: "gpx dir empty",
			modify: func(c *Config) {
				c.Demo.Enabled = true
				c.Demo.SpeedMultiplier = 1
				c.Demo.InterpolationInterval = 100
				c.Demo.GPXDir = ""
				c.Demo.DeviceIMEIs = []string{"DEV001"}
			},
			wantErr: "MOTUS_DEMO_GPX_DIR must be set",
		},
		{
			name: "device IMEIs empty",
			modify: func(c *Config) {
				c.Demo.Enabled = true
				c.Demo.SpeedMultiplier = 1
				c.Demo.InterpolationInterval = 100
				c.Demo.GPXDir = "/data/demo"
				c.Demo.DeviceIMEIs = nil
			},
			wantErr: "MOTUS_DEMO_DEVICE_IMEIS must have at least one device",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.modify(cfg)

			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidate_RedisConfig(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr string
	}{
		{
			name:   "disabled redis skips validation",
			modify: func(c *Config) { c.Redis.Enabled = false; c.Redis.URL = "" },
		},
		{
			name:    "enabled redis without URL",
			modify:  func(c *Config) { c.Redis.Enabled = true; c.Redis.URL = "" },
			wantErr: "MOTUS_REDIS_URL must be set when Redis is enabled",
		},
		{
			name:   "enabled redis with URL",
			modify: func(c *Config) { c.Redis.Enabled = true; c.Redis.URL = "redis://localhost:6379" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.modify(cfg)

			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	cfg := &Config{
		Database: DatabaseConfig{
			Host: "", URI: "", Port: "invalid",
			Pool: PoolConfig{
				MaxConns:        25,
				MinConns:        5,
				MaxConnLifetime: 1 * time.Hour,
				MaxConnIdleTime: 30 * time.Minute,
			},
		},
		Server:  ServerConfig{Port: "0"},
		GPS:     GPSConfig{H02Port: "99999", WatchPort: "-1"},
		Device:  DeviceConfig{TimeoutMinutes: 0, CheckIntervalMinutes: -1},
		Metrics: MetricsConfig{Enabled: false},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errMsg := err.Error()

	// Should contain multiple errors.
	expected := []string{
		"MOTUS_SERVER_PORT",
		"MOTUS_GPS_H02_PORT",
		"MOTUS_GPS_WATCH_PORT",
		"POSTGRES_URI or MOTUS_DATABASE_HOST",
		"MOTUS_DEVICE_TIMEOUT_MINUTES",
		"MOTUS_DEVICE_CHECK_INTERVAL_MINUTES",
	}
	for _, exp := range expected {
		if !strings.Contains(errMsg, exp) {
			t.Errorf("error should contain %q, got: %s", exp, errMsg)
		}
	}
}

func TestValidate_LoadFromEnv_CallsValidate(t *testing.T) {
	// Set a clearly invalid port to confirm Validate is called from LoadFromEnv.
	t.Setenv("MOTUS_SERVER_PORT", "99999")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected LoadFromEnv to return error for invalid port")
	}
	if !strings.Contains(err.Error(), "MOTUS_SERVER_PORT") {
		t.Errorf("error should mention MOTUS_SERVER_PORT, got: %v", err)
	}
}

func TestValidatePort(t *testing.T) {
	tests := []struct {
		port    string
		wantErr bool
	}{
		{"1", false},
		{"80", false},
		{"8080", false},
		{"65535", false},
		{"0", true},
		{"65536", true},
		{"-1", true},
		{"abc", true},
		{"", true},
		{"12.5", true},
	}

	for _, tt := range tests {
		t.Run(tt.port, func(t *testing.T) {
			err := validatePort(tt.port, "TEST_PORT")
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePort(%q) error = %v, wantErr %v", tt.port, err, tt.wantErr)
			}
		})
	}
}

func TestValidate_PositionsConfig(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr string
	}{
		{
			name:   "retention days zero (disabled) is valid",
			modify: func(c *Config) { c.Positions.RetentionDays = 0 },
		},
		{
			name:   "retention days positive is valid",
			modify: func(c *Config) { c.Positions.RetentionDays = 90 },
		},
		{
			name:    "retention days negative is invalid",
			modify:  func(c *Config) { c.Positions.RetentionDays = -1 },
			wantErr: "MOTUS_POSITION_RETENTION_DAYS must be >= 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.modify(cfg)

			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidate_PoolConfig(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr string
	}{
		{
			name:    "max conns zero",
			modify:  func(c *Config) { c.Database.Pool.MaxConns = 0 },
			wantErr: "MOTUS_DB_MAX_CONNS must be > 0",
		},
		{
			name:    "max conns negative",
			modify:  func(c *Config) { c.Database.Pool.MaxConns = -1 },
			wantErr: "MOTUS_DB_MAX_CONNS must be > 0",
		},
		{
			name:    "min conns negative",
			modify:  func(c *Config) { c.Database.Pool.MinConns = -1 },
			wantErr: "MOTUS_DB_MIN_CONNS must be >= 0",
		},
		{
			name:   "min conns zero valid",
			modify: func(c *Config) { c.Database.Pool.MinConns = 0 },
		},
		{
			name: "min conns greater than max conns",
			modify: func(c *Config) {
				c.Database.Pool.MaxConns = 5
				c.Database.Pool.MinConns = 10
			},
			wantErr: "MOTUS_DB_MIN_CONNS must be <= MOTUS_DB_MAX_CONNS",
		},
		{
			name: "min conns equal to max conns valid",
			modify: func(c *Config) {
				c.Database.Pool.MaxConns = 10
				c.Database.Pool.MinConns = 10
			},
		},
		{
			name:    "max conn lifetime zero",
			modify:  func(c *Config) { c.Database.Pool.MaxConnLifetime = 0 },
			wantErr: "MOTUS_DB_MAX_CONN_LIFETIME must be > 0",
		},
		{
			name:    "max conn idle time zero",
			modify:  func(c *Config) { c.Database.Pool.MaxConnIdleTime = 0 },
			wantErr: "MOTUS_DB_MAX_CONN_IDLE_TIME must be > 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.modify(cfg)

			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidate_GeocodingConfig(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr string
	}{
		{
			name:    "disabled geocoding skips validation",
			modify:  func(c *Config) { c.Geocoding.Enabled = false },
			wantErr: "",
		},
		{
			name: "enabled geocoding with valid config",
			modify: func(c *Config) {
				c.Geocoding.Enabled = true
				c.Geocoding.URL = "https://nominatim.openstreetmap.org/reverse"
				c.Geocoding.CacheTTL = 30 * time.Second
				c.Geocoding.RateLimit = 1.0
			},
			wantErr: "",
		},
		{
			name: "empty URL",
			modify: func(c *Config) {
				c.Geocoding.Enabled = true
				c.Geocoding.URL = ""
				c.Geocoding.CacheTTL = 30 * time.Second
				c.Geocoding.RateLimit = 1.0
			},
			wantErr: "MOTUS_GEOCODING_URL must be set",
		},
		{
			name: "cache TTL zero",
			modify: func(c *Config) {
				c.Geocoding.Enabled = true
				c.Geocoding.URL = "https://example.com/reverse"
				c.Geocoding.CacheTTL = 0
				c.Geocoding.RateLimit = 1.0
			},
			wantErr: "MOTUS_GEOCODING_CACHE_TTL must be > 0",
		},
		{
			name: "rate limit zero",
			modify: func(c *Config) {
				c.Geocoding.Enabled = true
				c.Geocoding.URL = "https://example.com/reverse"
				c.Geocoding.CacheTTL = 30 * time.Second
				c.Geocoding.RateLimit = 0
			},
			wantErr: "MOTUS_GEOCODING_RATE_LIMIT must be > 0",
		},
		{
			name: "rate limit negative",
			modify: func(c *Config) {
				c.Geocoding.Enabled = true
				c.Geocoding.URL = "https://example.com/reverse"
				c.Geocoding.CacheTTL = 30 * time.Second
				c.Geocoding.RateLimit = -1.0
			},
			wantErr: "MOTUS_GEOCODING_RATE_LIMIT must be > 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.modify(cfg)

			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// validConfig returns a Config that passes all validation checks.
func validConfig() *Config {
	return &Config{
		Database: DatabaseConfig{
			Host:    "localhost",
			Port:    "5432",
			User:    "motus",
			Name:    "motus",
			SSLMode: "disable",
			Pool: PoolConfig{
				MaxConns:        25,
				MinConns:        5,
				MaxConnLifetime: 1 * time.Hour,
				MaxConnIdleTime: 30 * time.Minute,
			},
		},
		Server:  ServerConfig{Port: "8080"},
		GPS:     GPSConfig{H02Port: "5013", WatchPort: "5093"},
		Device:  DeviceConfig{TimeoutMinutes: 5, CheckIntervalMinutes: 1},
		Metrics: MetricsConfig{Port: "9090", Enabled: true},
	}
}

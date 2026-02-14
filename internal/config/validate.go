package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Validate checks the configuration for invalid or inconsistent values.
// It returns an error describing all validation failures found, or nil
// if the configuration is valid. This method is called automatically by
// LoadFromEnv to fail fast on startup.
func (c *Config) Validate() error {
	var errs []string

	// Server port validation.
	if err := validatePort(c.Server.Port, "MOTUS_SERVER_PORT"); err != nil {
		errs = append(errs, err.Error())
	}

	// GPS port validation.
	if err := validatePort(c.GPS.H02Port, "MOTUS_GPS_H02_PORT"); err != nil {
		errs = append(errs, err.Error())
	}
	if err := validatePort(c.GPS.WatchPort, "MOTUS_GPS_WATCH_PORT"); err != nil {
		errs = append(errs, err.Error())
	}

	// Metrics port validation (only when enabled).
	if c.Metrics.Enabled {
		if err := validatePort(c.Metrics.Port, "MOTUS_METRICS_PORT"); err != nil {
			errs = append(errs, err.Error())
		}
	}

	// Database validation: must have either URI or host configured.
	if c.Database.URI == "" && c.Database.Host == "" {
		errs = append(errs, "database: either POSTGRES_URI or MOTUS_DATABASE_HOST must be set")
	}

	// Database port validation (only when using individual fields).
	if c.Database.URI == "" {
		if err := validatePort(c.Database.Port, "MOTUS_DATABASE_PORT"); err != nil {
			errs = append(errs, err.Error())
		}
	}

	// Connection pool validation.
	if c.Database.Pool.MaxConns <= 0 {
		errs = append(errs, "MOTUS_DB_MAX_CONNS must be > 0")
	}
	if c.Database.Pool.MinConns < 0 {
		errs = append(errs, "MOTUS_DB_MIN_CONNS must be >= 0")
	}
	if c.Database.Pool.MinConns > c.Database.Pool.MaxConns {
		errs = append(errs, "MOTUS_DB_MIN_CONNS must be <= MOTUS_DB_MAX_CONNS")
	}
	if c.Database.Pool.MaxConnLifetime <= 0 {
		errs = append(errs, "MOTUS_DB_MAX_CONN_LIFETIME must be > 0")
	}
	if c.Database.Pool.MaxConnIdleTime <= 0 {
		errs = append(errs, "MOTUS_DB_MAX_CONN_IDLE_TIME must be > 0")
	}

	// Device config validation.
	if c.Device.TimeoutMinutes <= 0 {
		errs = append(errs, "MOTUS_DEVICE_TIMEOUT_MINUTES must be > 0")
	}
	if c.Device.CheckIntervalMinutes <= 0 {
		errs = append(errs, "MOTUS_DEVICE_CHECK_INTERVAL_MINUTES must be > 0")
	}
	if c.Device.AutoCreateDevices && c.Device.AutoCreateDefaultUser == "" {
		errs = append(errs, "MOTUS_DEVICE_AUTO_CREATE_USER must be set when device auto-creation is enabled")
	}

	// Demo config validation (only when enabled).
	if c.Demo.Enabled {
		if c.Demo.SpeedMultiplier <= 0 {
			errs = append(errs, "MOTUS_DEMO_SPEED_MULTIPLIER must be > 0")
		}
		if c.Demo.InterpolationInterval <= 0 {
			errs = append(errs, "MOTUS_DEMO_INTERPOLATION_INTERVAL must be > 0")
		}
		if c.Demo.GPXDir == "" {
			errs = append(errs, "MOTUS_DEMO_GPX_DIR must be set when demo mode is enabled")
		}
		if len(c.Demo.DeviceIMEIs) == 0 {
			errs = append(errs, "MOTUS_DEMO_DEVICE_IMEIS must have at least one device when demo mode is enabled")
		}
	}

	// Redis validation (only when enabled).
	if c.Redis.Enabled && c.Redis.URL == "" {
		errs = append(errs, "MOTUS_REDIS_URL must be set when Redis is enabled")
	}

	// Position retention validation.
	if c.Positions.RetentionDays < 0 {
		errs = append(errs, "MOTUS_POSITION_RETENTION_DAYS must be >= 0 (0 disables retention)")
	}

	// Geocoding validation (only when enabled).
	if c.Geocoding.Enabled {
		if c.Geocoding.URL == "" {
			errs = append(errs, "MOTUS_GEOCODING_URL must be set when geocoding is enabled")
		}
		if c.Geocoding.CacheTTL <= 0 {
			errs = append(errs, "MOTUS_GEOCODING_CACHE_TTL must be > 0")
		}
		if c.Geocoding.RateLimit <= 0 {
			errs = append(errs, "MOTUS_GEOCODING_RATE_LIMIT must be > 0")
		}
	}

	// OIDC validation (only when enabled).
	if c.OIDC.Enabled {
		if c.OIDC.Issuer == "" {
			errs = append(errs, "MOTUS_OIDC_ISSUER must be set when OIDC is enabled")
		}
		if c.OIDC.ClientID == "" {
			errs = append(errs, "MOTUS_OIDC_CLIENT_ID must be set when OIDC is enabled")
		}
		if c.OIDC.ClientSecret == "" {
			errs = append(errs, "MOTUS_OIDC_CLIENT_SECRET must be set when OIDC is enabled")
		}
		if c.OIDC.RedirectURL == "" {
			errs = append(errs, "MOTUS_OIDC_REDIRECT_URL must be set when OIDC is enabled")
		}
		if c.OIDC.AdminEmailRegex != "" {
			if _, err := regexp.Compile(c.OIDC.AdminEmailRegex); err != nil {
				errs = append(errs, fmt.Sprintf("MOTUS_OIDC_ADMIN_EMAIL_REGEX: invalid regular expression: %v", err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("configuration validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// validatePort checks that the given string is a valid TCP port number (1-65535).
func validatePort(port string, envVar string) error {
	n, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("%s: %q is not a valid port number", envVar, port)
	}
	if n < 1 || n > 65535 {
		return fmt.Errorf("%s: port %d is out of range (1-65535)", envVar, n)
	}
	return nil
}

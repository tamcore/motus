package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all application configuration.
type Config struct {
	Database  DatabaseConfig
	Server    ServerConfig
	GPS       GPSConfig
	Device    DeviceConfig
	WebSocket WebSocketConfig
	Redis     RedisConfig
	Demo      DemoConfig
	Metrics   MetricsConfig
	Security  SecurityConfig
	Positions PositionsConfig
	Log       LogConfig
	Geocoding GeocodingConfig
	OIDC      OIDCConfig
}

// OIDCConfig holds OpenID Connect authentication settings.
type OIDCConfig struct {
	// Enabled activates OIDC login.
	// Loaded from MOTUS_OIDC_ENABLED. Default: false.
	Enabled bool
	// Issuer is the OIDC provider discovery URL (e.g. https://accounts.google.com).
	// Loaded from MOTUS_OIDC_ISSUER.
	Issuer string
	// ClientID is the OAuth2 client ID.
	// Loaded from MOTUS_OIDC_CLIENT_ID.
	ClientID string
	// ClientSecret is the OAuth2 client secret.
	// Loaded from MOTUS_OIDC_CLIENT_SECRET.
	ClientSecret string
	// RedirectURL is the absolute callback URL for the OIDC provider.
	// Example: https://app.example.com/api/auth/oidc/callback
	// Loaded from MOTUS_OIDC_REDIRECT_URL.
	RedirectURL string
	// SignupEnabled controls whether new Motus accounts are created on first OIDC login.
	// When false, only pre-existing accounts can authenticate via OIDC.
	// Loaded from MOTUS_OIDC_SIGNUP_ENABLED. Default: false.
	SignupEnabled bool
	// AdminEmailRegex is an optional regular expression matched against the user's
	// email address. A match grants the admin role on login.
	// Loaded from MOTUS_OIDC_ADMIN_EMAIL_REGEX.
	AdminEmailRegex string
	// AdminClaim is an optional claim key (e.g. "groups") that is checked for
	// the admin role in conjunction with AdminClaimValue.
	// Loaded from MOTUS_OIDC_ADMIN_CLAIM.
	AdminClaim string
	// AdminClaimValue is the value in AdminClaim that grants the admin role
	// (e.g. "motus-admin"). The claim may be a string or an array of strings.
	// Loaded from MOTUS_OIDC_ADMIN_CLAIM_VALUE.
	AdminClaimValue string
	// Scopes is a space-separated list of additional OAuth2 scopes to request
	// beyond openid, email, and profile.
	// Loaded from MOTUS_OIDC_SCOPES.
	Scopes string
}

// GeocodingConfig holds reverse geocoding settings.
type GeocodingConfig struct {
	// Enabled controls whether server-side reverse geocoding is active.
	// Loaded from MOTUS_GEOCODING_ENABLED. Default: true.
	Enabled bool
	// Provider selects the geocoding backend. Currently only "nominatim" is supported.
	// Loaded from MOTUS_GEOCODING_PROVIDER. Default: "nominatim".
	Provider string
	// URL is the reverse geocoding API endpoint.
	// Loaded from MOTUS_GEOCODING_URL. Default: "https://nominatim.openstreetmap.org/reverse".
	URL string
	// CacheTTL is how long geocoded addresses are cached in memory.
	// Loaded from MOTUS_GEOCODING_CACHE_TTL. Default: 30s.
	CacheTTL time.Duration
	// RateLimit is the maximum number of geocoding requests per second.
	// Nominatim's usage policy requires at most 1 req/sec.
	// Loaded from MOTUS_GEOCODING_RATE_LIMIT. Default: 1.
	RateLimit float64
}

// LogConfig holds structured logging settings.
type LogConfig struct {
	// Level is the minimum log level: DEBUG, INFO, WARN, ERROR.
	// Loaded from MOTUS_LOG_LEVEL. Default: "INFO".
	Level string
	// Format is the log output format: "json" or "text".
	// Loaded from MOTUS_LOG_FORMAT. Default: "json" in production, "text" in development.
	// When empty, determined by MOTUS_ENV.
	Format string
}

// PositionsConfig holds position data management settings.
type PositionsConfig struct {
	// RetentionDays is the number of days to retain position data.
	// Partitions older than this are automatically dropped.
	// Set to 0 (default) to disable automatic retention/deletion.
	// Loaded from MOTUS_POSITION_RETENTION_DAYS.
	RetentionDays int
}

// SecurityConfig holds security-related settings.
type SecurityConfig struct {
	// CSRFSecret is the 32-byte key used to authenticate CSRF tokens.
	// Loaded from MOTUS_CSRF_SECRET. If empty, a random key is generated
	// at startup (tokens will not survive restarts).
	CSRFSecret string
	// Environment controls security behavior (e.g., Secure cookie flag).
	// Loaded from MOTUS_ENV. Default: "production".
	Env string
}

// MetricsConfig holds Prometheus metrics server settings.
type MetricsConfig struct {
	// Port is the port for the Prometheus metrics endpoint.
	// Loaded from MOTUS_METRICS_PORT. Default: "9090".
	Port string
	// Enabled controls whether the metrics server is started.
	// Loaded from MOTUS_METRICS_ENABLED. Default: true.
	Enabled bool
}

// DemoConfig holds demo mode settings.
type DemoConfig struct {
	// Enabled activates demo mode with simulated GPS devices.
	Enabled bool
	// GPXDir is the directory containing GPX route files.
	// Default: "data/demo"
	GPXDir string
	// ResetTime is the time of day (HH:MM) to reset the database.
	// Default: "00:00"
	ResetTime string
	// DeviceIMEIs is the list of simulated device identifiers.
	// Must be numeric-only to be compatible with Traccar's H02 decoder.
	// Default: ["9000000000001", "9000000000002"]
	DeviceIMEIs []string
	// SpeedMultiplier controls simulation speed. 1.0 = real time, 10.0 = 10x faster.
	// Default: 1.0
	SpeedMultiplier float64
	// InterpolationInterval is the maximum distance in meters between consecutive
	// route points after interpolation. Lower values produce more points and slower,
	// smoother visual movement. Default: 100.0 meters.
	InterpolationInterval float64
	// H02Target is the host:port of the H02 GPS server to send demo messages to.
	// Default: "localhost:5013"
	H02Target string
}

// RedisConfig holds Redis connection settings for cross-pod pub/sub.
type RedisConfig struct {
	// URL is the Redis connection string (e.g. "redis://localhost:6379").
	// Loaded from MOTUS_REDIS_URL.
	URL string
	// Enabled controls whether Redis pub/sub is used for cross-pod WebSocket
	// broadcasting. Loaded from MOTUS_REDIS_ENABLED.
	Enabled bool
}

// WebSocketConfig holds WebSocket-related settings.
type WebSocketConfig struct {
	// AllowedOrigins is the list of allowed WebSocket origins.
	// Loaded from MOTUS_WS_ALLOWED_ORIGINS (comma-separated).
	// An empty list means no origin restriction is enforced (dev mode).
	AllowedOrigins []string
}

// DeviceConfig holds device monitoring settings.
type DeviceConfig struct {
	// TimeoutMinutes is how long (in minutes) a device can be silent before
	// being marked offline. Default: 5.
	TimeoutMinutes int
	// CheckIntervalMinutes is how often (in minutes) the timeout service
	// checks for inactive devices. Default: 1.
	CheckIntervalMinutes int
	// AutoCreateDevices controls whether unknown devices are automatically
	// created when they first connect via a GPS protocol. When enabled, the
	// device is assigned to the user specified by AutoCreateDefaultUser.
	// Loaded from MOTUS_DEVICE_AUTO_CREATE. Default: false.
	AutoCreateDevices bool
	// AutoCreateDefaultUser is the email address of the user that auto-created
	// devices are assigned to. This user must already exist in the database.
	// Loaded from MOTUS_DEVICE_AUTO_CREATE_USER. Default: "admin@motus.local".
	AutoCreateDefaultUser string
	// UniqueIDPrefix is prepended to device UniqueID values in API responses.
	// This allows running motus alongside another Traccar-compatible server
	// (e.g. the real Traccar) without unique ID collisions in Home Assistant.
	// Loaded from MOTUS_DEVICE_UNIQUE_ID_PREFIX. Default: "" (no prefix).
	UniqueIDPrefix string
}

// Timeout returns the device timeout as a time.Duration.
func (c DeviceConfig) Timeout() time.Duration {
	return time.Duration(c.TimeoutMinutes) * time.Minute
}

// CheckInterval returns the check interval as a time.Duration.
func (c DeviceConfig) CheckInterval() time.Duration {
	return time.Duration(c.CheckIntervalMinutes) * time.Minute
}

// DatabaseConfig holds database connection settings.
type DatabaseConfig struct {
	URI      string // Full connection URI (takes precedence if set)
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
	Pool     PoolConfig
}

// PoolConfig holds connection pool tuning parameters.
type PoolConfig struct {
	// MaxConns is the maximum number of connections in the pool.
	// Loaded from MOTUS_DB_MAX_CONNS. Default: 25.
	MaxConns int32
	// MinConns is the minimum number of connections kept open.
	// Loaded from MOTUS_DB_MIN_CONNS. Default: 5.
	MinConns int32
	// MaxConnLifetime is the maximum time a connection can be reused.
	// Loaded from MOTUS_DB_MAX_CONN_LIFETIME. Default: 1h.
	MaxConnLifetime time.Duration
	// MaxConnIdleTime is the maximum time a connection can sit idle.
	// Loaded from MOTUS_DB_MAX_CONN_IDLE_TIME. Default: 30m.
	MaxConnIdleTime time.Duration
}

// URL returns the PostgreSQL connection string.
// If URI is set, returns it directly. Otherwise constructs from individual fields.
func (c DatabaseConfig) URL() string {
	if c.URI != "" {
		return c.URI
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.Name, c.SSLMode)
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port string
}

// GPSConfig holds GPS protocol listener settings.
type GPSConfig struct {
	H02Port          string
	WatchPort        string
	H02RelayTarget   string // optional "host:port" to forward raw H02 messages
	WatchRelayTarget string // optional "host:port" to forward raw WATCH messages
}

// LoadFromEnv loads configuration from environment variables with defaults.
func LoadFromEnv() (*Config, error) {
	cfg := &Config{
		Database: DatabaseConfig{
			URI:      getEnv("POSTGRES_URI", ""), // Cloudnative-PG compatibility
			Host:     getEnv("MOTUS_DATABASE_HOST", "localhost"),
			Port:     getEnv("MOTUS_DATABASE_PORT", "5432"),
			User:     getEnv("MOTUS_DATABASE_USER", "motus"),
			Password: getEnv("MOTUS_DATABASE_PASSWORD", "motus"),
			Name:     getEnv("MOTUS_DATABASE_NAME", "motus"),
			SSLMode:  getEnv("MOTUS_DATABASE_SSLMODE", "require"),
			Pool: PoolConfig{
				MaxConns:        int32(getEnvInt("MOTUS_DB_MAX_CONNS", 25)),
				MinConns:        int32(getEnvInt("MOTUS_DB_MIN_CONNS", 5)),
				MaxConnLifetime: getEnvDuration("MOTUS_DB_MAX_CONN_LIFETIME", 1*time.Hour),
				MaxConnIdleTime: getEnvDuration("MOTUS_DB_MAX_CONN_IDLE_TIME", 30*time.Minute),
			},
		},
		Server: ServerConfig{
			Port: getEnv("MOTUS_SERVER_PORT", "8080"),
		},
		GPS: GPSConfig{
			H02Port:          getEnv("MOTUS_GPS_H02_PORT", "5013"),
			WatchPort:        getEnv("MOTUS_GPS_WATCH_PORT", "5093"),
			H02RelayTarget:   getEnv("MOTUS_GPS_H02_RELAY_TARGET", ""),
			WatchRelayTarget: getEnv("MOTUS_GPS_WATCH_RELAY_TARGET", ""),
		},
		Device: DeviceConfig{
			TimeoutMinutes:        getEnvInt("MOTUS_DEVICE_TIMEOUT_MINUTES", 5),
			CheckIntervalMinutes:  getEnvInt("MOTUS_DEVICE_CHECK_INTERVAL_MINUTES", 1),
			AutoCreateDevices:     getEnvBool("MOTUS_DEVICE_AUTO_CREATE", false),
			AutoCreateDefaultUser: getEnv("MOTUS_DEVICE_AUTO_CREATE_USER", "admin@motus.local"),
			UniqueIDPrefix:        getEnv("MOTUS_DEVICE_UNIQUE_ID_PREFIX", ""),
		},
		WebSocket: WebSocketConfig{
			AllowedOrigins: getEnvSlice("MOTUS_WS_ALLOWED_ORIGINS"),
		},
		Redis: RedisConfig{
			URL:     getEnv("MOTUS_REDIS_URL", ""),
			Enabled: getEnvBool("MOTUS_REDIS_ENABLED", false),
		},
		Demo: DemoConfig{
			Enabled:               getEnvBool("MOTUS_DEMO_ENABLED", false),
			GPXDir:                getEnv("MOTUS_DEMO_GPX_DIR", "data/demo"),
			ResetTime:             getEnv("MOTUS_DEMO_RESET_TIME", "00:00"),
			DeviceIMEIs:           getEnvSliceDefault("MOTUS_DEMO_DEVICE_IMEIS", []string{"9000000000001", "9000000000002"}),
			SpeedMultiplier:       getEnvFloat("MOTUS_DEMO_SPEED_MULTIPLIER", 1.0),
			InterpolationInterval: getEnvFloat("MOTUS_DEMO_INTERPOLATION_INTERVAL", 100.0),
			H02Target:             getEnv("MOTUS_DEMO_H02_TARGET", "localhost:5013"),
		},
		Metrics: MetricsConfig{
			Port:    getPort("MOTUS_METRICS_PORT", "9090"),
			Enabled: getEnvBool("MOTUS_METRICS_ENABLED", true),
		},
		Security: SecurityConfig{
			CSRFSecret: getEnv("MOTUS_CSRF_SECRET", ""),
			Env:        getEnv("MOTUS_ENV", "production"),
		},
		Positions: PositionsConfig{
			RetentionDays: getEnvInt("MOTUS_POSITION_RETENTION_DAYS", 0),
		},
		Log: LogConfig{
			Level:  getEnv("MOTUS_LOG_LEVEL", "INFO"),
			Format: getEnv("MOTUS_LOG_FORMAT", ""),
		},
		Geocoding: GeocodingConfig{
			Enabled:   getEnvBool("MOTUS_GEOCODING_ENABLED", true),
			Provider:  getEnv("MOTUS_GEOCODING_PROVIDER", "nominatim"),
			URL:       getEnv("MOTUS_GEOCODING_URL", "https://nominatim.openstreetmap.org/reverse"),
			CacheTTL:  getEnvDuration("MOTUS_GEOCODING_CACHE_TTL", 30*time.Second),
			RateLimit: getEnvFloat("MOTUS_GEOCODING_RATE_LIMIT", 1.0),
		},
		OIDC: OIDCConfig{
			Enabled:         getEnvBool("MOTUS_OIDC_ENABLED", false),
			Issuer:          getEnv("MOTUS_OIDC_ISSUER", ""),
			ClientID:        getEnv("MOTUS_OIDC_CLIENT_ID", ""),
			ClientSecret:    getEnv("MOTUS_OIDC_CLIENT_SECRET", ""),
			RedirectURL:     getEnv("MOTUS_OIDC_REDIRECT_URL", ""),
			SignupEnabled:   getEnvBool("MOTUS_OIDC_SIGNUP_ENABLED", false),
			AdminEmailRegex: getEnv("MOTUS_OIDC_ADMIN_EMAIL_REGEX", ""),
			AdminClaim:      getEnv("MOTUS_OIDC_ADMIN_CLAIM", ""),
			AdminClaimValue: getEnv("MOTUS_OIDC_ADMIN_CLAIM_VALUE", ""),
			Scopes:          getEnv("MOTUS_OIDC_SCOPES", ""),
		},
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// getPort retrieves a port from environment, handling Kubernetes service URLs.
// Kubernetes injects service env vars like MOTUS_METRICS_PORT=tcp://10.233.x.x:9090
// This function extracts just the port number.
func getPort(key, defaultValue string) string {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}

	// Handle Kubernetes service URL format: tcp://host:port or tcp://host:port/path
	if strings.HasPrefix(v, "tcp://") || strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		// Extract port from URL
		parts := strings.Split(v, ":")
		if len(parts) >= 3 {
			// Format: tcp://host:port
			port := parts[2]
			// Remove any path suffix
			if idx := strings.Index(port, "/"); idx > 0 {
				port = port[:idx]
			}
			return port
		}
	}

	// Plain port number
	return v
}

func getEnvSlice(key string) []string {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func getEnvInt(key string, defaultValue int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultValue
	}
	return n
}

func getEnvBool(key string, defaultValue bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return defaultValue
	}
	return b
}

func getEnvFloat(key string, defaultValue float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return defaultValue
	}
	return f
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return defaultValue
	}
	return d
}

func getEnvSliceDefault(key string, defaultValue []string) []string {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	parts := strings.Split(v, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

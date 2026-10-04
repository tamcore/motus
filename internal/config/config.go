package config

import (
	"cmp"
	"fmt"
	"net/netip"
	"net/url"
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
	AI        AIConfig
	WebAuthn  WebAuthnConfig
}

// WebAuthnConfig holds passkey (WebAuthn/FIDO2) settings.
type WebAuthnConfig struct {
	// Enabled activates passkey registration and login.
	// Loaded from MOTUS_WEBAUTHN_ENABLED. Default: false.
	Enabled bool
	// RPID is the Relying Party ID: the registrable domain the credentials are
	// bound to, without scheme or port (e.g. "app.example.com"). It must match
	// the site's effective domain or authentication will fail, and changing it
	// permanently invalidates existing credentials.
	// Loaded from MOTUS_WEBAUTHN_RPID. Required when Enabled.
	RPID string
	// RPOrigins is the list of full origins (scheme + host + optional port) that
	// may initiate ceremonies, e.g. "https://app.example.com".
	// Loaded from MOTUS_WEBAUTHN_ORIGINS (comma-separated). Required when Enabled.
	RPOrigins []string
	// RPDisplayName is the human-readable Relying Party name shown by
	// authenticators. Loaded from MOTUS_WEBAUTHN_DISPLAY_NAME. Default: "Motus".
	RPDisplayName string
}

// AIConfig holds settings for the OpenAI-compatible chat/MCP feature.
type AIConfig struct {
	// Enabled activates the /api/chat endpoint and in-process MCP tools.
	// Loaded from MOTUS_AI_ENABLED. Default: false.
	Enabled bool
	// BaseURL is the base URL of the OpenAI-compatible API.
	// Loaded from MOTUS_AI_BASE_URL. Default: "https://api.openai.com/v1".
	BaseURL string
	// APIKey is the API key for the AI provider.
	// Loaded from MOTUS_AI_API_KEY. Required when Enabled.
	APIKey string
	// Model is the chat completion model to use.
	// Loaded from MOTUS_AI_MODEL. Default: "gpt-4o-mini".
	Model string
	// MaxTokens is the maximum number of tokens in a completion.
	// Loaded from MOTUS_AI_MAX_TOKENS. Default: 4096.
	MaxTokens int
	// Temperature controls output randomness (0.0–2.0).
	// Loaded from MOTUS_AI_TEMPERATURE. Default: 0.2.
	Temperature float64
	// Timeout is the maximum duration for a single chat request including all
	// tool-call iterations.
	// Loaded from MOTUS_AI_TIMEOUT. Default: 90s.
	Timeout time.Duration
	// MaxToolLoops is the maximum number of tool-call iterations per chat turn.
	// Loaded from MOTUS_AI_MAX_TOOL_LOOPS. Default: 8.
	MaxToolLoops int
	// SystemPrompt overrides the built-in system prompt when non-empty.
	// Loaded from MOTUS_AI_SYSTEM_PROMPT.
	SystemPrompt string
	// GuardrailEnabled turns on the pre-flight topic classifier that refuses
	// off-topic chat messages before they reach the main model.
	// Loaded from MOTUS_AI_GUARDRAIL_ENABLED. Default: true.
	GuardrailEnabled bool
	// GuardrailModel is the model used for topic classification.
	// Loaded from MOTUS_AI_GUARDRAIL_MODEL. Default: same as Model.
	GuardrailModel string
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
	// TrustUnverifiedEmail allows linking an OIDC subject to an existing local
	// account even when the IdP does not assert email_verified. Leave false
	// unless the IdP is known to only issue verified addresses; otherwise an
	// attacker-controlled IdP account could take over a local account with a
	// matching email.
	// Loaded from MOTUS_OIDC_TRUST_UNVERIFIED_EMAIL. Default: false.
	TrustUnverifiedEmail bool
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
	// URL is the reverse geocoding API endpoint.
	// Loaded from MOTUS_GEOCODING_URL. Default: "https://nominatim.openstreetmap.org/reverse".
	URL string
	// CacheTTL is how long geocoded addresses are cached in memory.
	// Loaded from MOTUS_GEOCODING_CACHE_TTL. Default: 1h.
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
	// WebhookAllowedHosts is the list of hostnames whose webhook URLs may
	// legitimately resolve to private IP addresses. Used to allow
	// self-hosted services on internal networks (e.g. ntfy.example.lan)
	// without disabling SSRF protection globally.
	// Loaded from MOTUS_WEBHOOK_ALLOWED_HOSTS (comma-separated).
	WebhookAllowedHosts []string
	// TrustedProxies lists the IPs or CIDRs of reverse proxies whose
	// X-Forwarded-For / X-Real-Ip headers are honoured for the client IP.
	// Loaded from MOTUS_TRUSTED_PROXIES (comma-separated).
	// Default: loopback and private ranges.
	TrustedProxies []string
}

// IsDevelopment reports whether MOTUS_ENV is "development" (case-insensitive).
func (c SecurityConfig) IsDevelopment() bool {
	return strings.EqualFold(c.Env, "development")
}

// MetricsConfig holds Prometheus metrics server settings.
type MetricsConfig struct {
	// Port is the port for the Prometheus metrics endpoint.
	// Loaded from MOTUS_METRICS_PORT. Default: "9090".
	Port string
	// Enabled controls whether the metrics server is started.
	// Loaded from MOTUS_METRICS_ENABLED. Default: true.
	Enabled bool
	// Pprof exposes net/http/pprof under /debug/pprof/ on the metrics port.
	// Loaded from MOTUS_PPROF_ENABLED. Default: false.
	Pprof bool
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
	// Pod marks the dedicated demo pod that runs the simulator and resets.
	// Loaded from MOTUS_DEMO_POD.
	Pod bool
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
	OsmAndPort       string // OsmAnd / Traccar Client HTTP protocol port
}

// LoadFromEnv loads configuration from environment variables with defaults.
func LoadFromEnv() (*Config, error) {
	cfg := &Config{
		Database: DatabaseConfig{
			URI:      getEnv("POSTGRES_URI", ""), // Cloudnative-PG compatibility
			Host:     getEnv("MOTUS_DATABASE_HOST", "localhost"),
			Port:     getEnv("MOTUS_DATABASE_PORT", "5432"),
			User:     getEnv("MOTUS_DATABASE_USER", "motus"),
			Password: getEnv("MOTUS_DATABASE_PASSWORD", ""),
			Name:     getEnv("MOTUS_DATABASE_NAME", "motus"),
			SSLMode:  getEnv("MOTUS_DATABASE_SSLMODE", "require"),
			Pool: PoolConfig{
				MaxConns:        parseEnv("MOTUS_DB_MAX_CONNS", 25, parseInt32),
				MinConns:        parseEnv("MOTUS_DB_MIN_CONNS", 5, parseInt32),
				MaxConnLifetime: parseEnv("MOTUS_DB_MAX_CONN_LIFETIME", 1*time.Hour, time.ParseDuration),
				MaxConnIdleTime: parseEnv("MOTUS_DB_MAX_CONN_IDLE_TIME", 30*time.Minute, time.ParseDuration),
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
			OsmAndPort:       getEnv("MOTUS_GPS_OSMAND_PORT", "5055"),
		},
		Device: DeviceConfig{
			TimeoutMinutes:        parseEnv("MOTUS_DEVICE_TIMEOUT_MINUTES", 5, strconv.Atoi),
			CheckIntervalMinutes:  parseEnv("MOTUS_DEVICE_CHECK_INTERVAL_MINUTES", 1, strconv.Atoi),
			AutoCreateDevices:     parseEnv("MOTUS_DEVICE_AUTO_CREATE", false, strconv.ParseBool),
			AutoCreateDefaultUser: getEnv("MOTUS_DEVICE_AUTO_CREATE_USER", "admin@motus.local"),
			UniqueIDPrefix:        getEnv("MOTUS_DEVICE_UNIQUE_ID_PREFIX", ""),
		},
		WebSocket: WebSocketConfig{
			AllowedOrigins: parseEnv("MOTUS_WS_ALLOWED_ORIGINS", nil, parseList),
		},
		Redis: RedisConfig{
			URL:     getEnv("MOTUS_REDIS_URL", ""),
			Enabled: parseEnv("MOTUS_REDIS_ENABLED", false, strconv.ParseBool),
		},
		Demo: DemoConfig{
			Enabled:               parseEnv("MOTUS_DEMO_ENABLED", false, strconv.ParseBool),
			GPXDir:                getEnv("MOTUS_DEMO_GPX_DIR", "data/demo"),
			ResetTime:             getEnv("MOTUS_DEMO_RESET_TIME", "00:00"),
			DeviceIMEIs:           parseEnv("MOTUS_DEMO_DEVICE_IMEIS", []string{"9000000000001", "9000000000002"}, parseList),
			SpeedMultiplier:       parseEnv("MOTUS_DEMO_SPEED_MULTIPLIER", 1.0, parseFloat),
			InterpolationInterval: parseEnv("MOTUS_DEMO_INTERPOLATION_INTERVAL", 100.0, parseFloat),
			H02Target:             getEnv("MOTUS_DEMO_H02_TARGET", "localhost:5013"),
			Pod:                   parseEnv("MOTUS_DEMO_POD", false, strconv.ParseBool),
		},
		Metrics: MetricsConfig{
			Port:    getPort("MOTUS_METRICS_PORT", "9090"),
			Enabled: parseEnv("MOTUS_METRICS_ENABLED", true, strconv.ParseBool),
			Pprof:   parseEnv("MOTUS_PPROF_ENABLED", false, strconv.ParseBool),
		},
		Security: SecurityConfig{
			CSRFSecret:          getEnv("MOTUS_CSRF_SECRET", ""),
			Env:                 getEnv("MOTUS_ENV", "production"),
			WebhookAllowedHosts: parseEnv("MOTUS_WEBHOOK_ALLOWED_HOSTS", nil, parseList),
			TrustedProxies:      parseEnv("MOTUS_TRUSTED_PROXIES", defaultTrustedProxies, parseList),
		},
		Positions: PositionsConfig{
			RetentionDays: parseEnv("MOTUS_POSITION_RETENTION_DAYS", 0, strconv.Atoi),
		},
		Log: LogConfig{
			Level:  getEnv("MOTUS_LOG_LEVEL", "INFO"),
			Format: getEnv("MOTUS_LOG_FORMAT", ""),
		},
		Geocoding: GeocodingConfig{
			Enabled:   parseEnv("MOTUS_GEOCODING_ENABLED", true, strconv.ParseBool),
			URL:       getEnv("MOTUS_GEOCODING_URL", "https://nominatim.openstreetmap.org/reverse"),
			CacheTTL:  parseEnv("MOTUS_GEOCODING_CACHE_TTL", time.Hour, time.ParseDuration),
			RateLimit: parseEnv("MOTUS_GEOCODING_RATE_LIMIT", 1.0, parseFloat),
		},
		OIDC: OIDCConfig{
			Enabled:              parseEnv("MOTUS_OIDC_ENABLED", false, strconv.ParseBool),
			Issuer:               getEnv("MOTUS_OIDC_ISSUER", ""),
			ClientID:             getEnv("MOTUS_OIDC_CLIENT_ID", ""),
			ClientSecret:         getEnv("MOTUS_OIDC_CLIENT_SECRET", ""),
			RedirectURL:          getEnv("MOTUS_OIDC_REDIRECT_URL", ""),
			SignupEnabled:        parseEnv("MOTUS_OIDC_SIGNUP_ENABLED", false, strconv.ParseBool),
			TrustUnverifiedEmail: parseEnv("MOTUS_OIDC_TRUST_UNVERIFIED_EMAIL", false, strconv.ParseBool),
			AdminEmailRegex:      getEnv("MOTUS_OIDC_ADMIN_EMAIL_REGEX", ""),
			AdminClaim:           getEnv("MOTUS_OIDC_ADMIN_CLAIM", ""),
			AdminClaimValue:      getEnv("MOTUS_OIDC_ADMIN_CLAIM_VALUE", ""),
			Scopes:               getEnv("MOTUS_OIDC_SCOPES", ""),
		},
		AI: AIConfig{
			Enabled:          parseEnv("MOTUS_AI_ENABLED", false, strconv.ParseBool),
			BaseURL:          getEnv("MOTUS_AI_BASE_URL", "https://api.openai.com/v1"),
			APIKey:           getEnv("MOTUS_AI_API_KEY", ""),
			Model:            getEnv("MOTUS_AI_MODEL", "gpt-4o-mini"),
			MaxTokens:        parseEnv("MOTUS_AI_MAX_TOKENS", 4096, strconv.Atoi),
			Temperature:      parseEnv("MOTUS_AI_TEMPERATURE", 0.2, parseFloat),
			Timeout:          parseEnv("MOTUS_AI_TIMEOUT", 90*time.Second, time.ParseDuration),
			MaxToolLoops:     parseEnv("MOTUS_AI_MAX_TOOL_LOOPS", 8, strconv.Atoi),
			SystemPrompt:     getEnv("MOTUS_AI_SYSTEM_PROMPT", ""),
			GuardrailEnabled: parseEnv("MOTUS_AI_GUARDRAIL_ENABLED", true, strconv.ParseBool),
			GuardrailModel:   getEnv("MOTUS_AI_GUARDRAIL_MODEL", ""),
		},
		WebAuthn: WebAuthnConfig{
			Enabled:       parseEnv("MOTUS_WEBAUTHN_ENABLED", false, strconv.ParseBool),
			RPID:          getEnv("MOTUS_WEBAUTHN_RPID", ""),
			RPOrigins:     parseEnv("MOTUS_WEBAUTHN_ORIGINS", nil, parseList),
			RPDisplayName: getEnv("MOTUS_WEBAUTHN_DISPLAY_NAME", "Motus"),
		},
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	return cmp.Or(os.Getenv(key), defaultValue)
}

// getPort reads a port, accepting Kubernetes service URLs such as tcp://10.0.0.1:9090.
func getPort(key, defaultValue string) string {
	v := getEnv(key, defaultValue)
	if u, err := url.Parse(v); err == nil && u.Port() != "" {
		return u.Port()
	}
	return v
}

// defaultTrustedProxies covers reverse proxies on the same host or private
// network, such as a Kubernetes ingress or a Docker Compose proxy.
var defaultTrustedProxies = []string{
	"127.0.0.0/8", "::1/128", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7",
}

// TrustedProxyPrefixes parses TrustedProxies. A bare IP becomes a single-host prefix.
func (s SecurityConfig) TrustedProxyPrefixes() ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(s.TrustedProxies))
	for _, v := range s.TrustedProxies {
		if p, err := netip.ParsePrefix(v); err == nil {
			prefixes = append(prefixes, p.Masked())
			continue
		}
		ip, err := netip.ParseAddr(v)
		if err != nil {
			return nil, fmt.Errorf("MOTUS_TRUSTED_PROXIES: %q is not an IP or CIDR", v)
		}
		prefixes = append(prefixes, netip.PrefixFrom(ip, ip.BitLen()))
	}
	return prefixes, nil
}

// parseEnv returns parse(os.Getenv(key)), or defaultValue when unset or invalid.
func parseEnv[T any](key string, defaultValue T, parse func(string) (T, error)) T {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	x, err := parse(v)
	if err != nil {
		return defaultValue
	}
	return x
}

func parseInt32(s string) (int32, error) {
	n, err := strconv.ParseInt(s, 10, 32)
	return int32(n), err
}

func parseFloat(s string) (float64, error) { return strconv.ParseFloat(s, 64) }

func parseList(s string) ([]string, error) { return SplitList(s), nil }

// SplitList splits a comma-separated string, trims each item and drops empty items.
func SplitList(s string) []string {
	result := []string{}
	for p := range strings.SplitSeq(s, ",") {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

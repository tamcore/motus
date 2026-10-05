// Package audit provides a structured audit logging system for tracking
// admin actions and significant system events.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// auditFilterRE validates audit filter strings (action and resource_type).
// Allows lowercase letters, digits, dots, underscores; max 64 chars.
// e.g. "session.login", "device.create", "user"
var auditFilterRE = regexp.MustCompile(`^[a-z][a-z0-9._]{0,63}$`)

// validateAuditFilter returns an error if the filter string contains characters
// outside the expected audit action/resource-type pattern.
func validateAuditFilter(field, value string) error {
	if !auditFilterRE.MatchString(value) {
		return fmt.Errorf("invalid %s filter %q", field, value)
	}
	return nil
}

// Standard audit actions.
const (
	// Session lifecycle actions.
	ActionSessionLogin       = "session.login"
	ActionSessionLoginFailed = "session.login_failed"
	ActionSessionLogout      = "session.logout"

	// User CRUD actions.
	ActionUserCreate = "user.create"
	ActionUserUpdate = "user.update"
	ActionUserDelete = "user.delete"

	// Device CRUD actions.
	ActionDeviceCreate   = "device.create"
	ActionDeviceUpdate   = "device.update"
	ActionDeviceDelete   = "device.delete"
	ActionDeviceAssign   = "device.assign"
	ActionDeviceUnassign = "device.unassign"

	// Geofence CRUD actions.
	ActionGeofenceCreate = "geofence.create"
	ActionGeofenceUpdate = "geofence.update"
	ActionGeofenceDelete = "geofence.delete"

	// Calendar CRUD actions.
	ActionCalendarCreate = "calendar.create"
	ActionCalendarUpdate = "calendar.update"
	ActionCalendarDelete = "calendar.delete"

	// Trail bookmark CRUD actions.
	ActionTrailBookmarkCreate = "trail_bookmark.create"
	ActionTrailBookmarkUpdate = "trail_bookmark.update"
	ActionTrailBookmarkDelete = "trail_bookmark.delete"

	// Notification rule CRUD actions.
	ActionNotifCreate = "notification.create"
	ActionNotifUpdate = "notification.update"
	ActionNotifDelete = "notification.delete"

	// API key actions.
	ActionApiKeyCreate = "apikey.create"
	ActionApiKeyDelete = "apikey.delete"

	// Device share actions.
	ActionShareCreate = "share.create"
	ActionShareDelete = "share.delete"

	// GPX import action.
	ActionGPXImport = "device.gpx_import"

	// Command actions.
	ActionCommandSend = "command.send"

	// Session lifecycle actions (continued).
	ActionSessionSudo    = "session.sudo"
	ActionSessionSudoEnd = "session.sudo_end"
	ActionSessionRevoke  = "session.revoke"
)

// Standard resource types.
const (
	ResourceUser          = "user"
	ResourceDevice        = "device"
	ResourceGeofence      = "geofence"
	ResourceCalendar      = "calendar"
	ResourceTrailBookmark = "trail_bookmark"
	ResourceNotification  = "notification"
	ResourceSession       = "session"
	ResourceApiKey        = "apikey"
	ResourceShare         = "share"
	ResourceCommand       = "command"
)

// Entry represents a single audit log entry.
type Entry struct {
	ID           int64          `json:"id"`
	Timestamp    time.Time      `json:"timestamp"`
	UserID       *int64         `json:"userId,omitempty"`
	Action       string         `json:"action"`
	ResourceType *string        `json:"resourceType,omitempty"`
	ResourceID   *int64         `json:"resourceId,omitempty"`
	Details      map[string]any `json:"details,omitempty"`
	IPAddress    *string        `json:"ipAddress,omitempty"`
	UserAgent    *string        `json:"userAgent,omitempty"`
}

// Logger provides audit logging backed by a PostgreSQL table.
type Logger struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewLogger creates a new audit logger.
func NewLogger(pool *pgxpool.Pool) *Logger {
	return &Logger{pool: pool, logger: slog.Default()}
}

type requestMetaKey struct{}

type requestMeta struct{ ip, userAgent string }

// Middleware stores the client IP and User-Agent in the request context so
// Log records them.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), requestMetaKey{}, requestMeta{ip: ExtractIP(r), userAgent: r.UserAgent()})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Log records an audit event, with the client IP and User-Agent stored by
// Middleware. Errors are logged but never returned to callers, because audit
// logging must not break application flow.
func (l *Logger) Log(ctx context.Context, userID *int64, action, resourceType string, resourceID *int64, details map[string]any) {
	if l == nil || l.pool == nil {
		return
	}

	var detailsJSON []byte
	if details != nil {
		var err error
		detailsJSON, err = json.Marshal(details)
		if err != nil {
			l.logger.Warn("failed to marshal audit details",
				slog.String("action", action),
				slog.Any("error", err),
			)
			detailsJSON = nil
		}
	}

	meta, _ := ctx.Value(requestMetaKey{}).(requestMeta)
	var ip string
	if parsed := net.ParseIP(meta.ip); parsed != nil {
		ip = parsed.String()
	}

	_, err := l.pool.Exec(ctx, `
		INSERT INTO audit_log (user_id, action, resource_type, resource_id, details, ip_address, user_agent)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, NULLIF($6, '')::inet, NULLIF($7, ''))
	`, userID, action, resourceType, resourceID, detailsJSON, ip, meta.userAgent)
	if err != nil {
		l.logger.Error("failed to write audit log",
			slog.String("action", action),
			slog.Any("error", err),
		)
	}

	// Mirror audit event to stdout for log aggregation pipelines.
	attrs := []slog.Attr{
		slog.String("type", "audit"),
		slog.String("action", action),
	}
	if resourceType != "" {
		attrs = append(attrs, slog.String("resource_type", resourceType))
	}
	if resourceID != nil {
		attrs = append(attrs, slog.Int64("resource_id", *resourceID))
	}
	if userID != nil {
		attrs = append(attrs, slog.Int64("user_id", *userID))
	}
	if ip != "" {
		attrs = append(attrs, slog.String("ip", ip))
	}
	if details != nil {
		attrs = append(attrs, slog.Any("details", details))
	}
	l.logger.LogAttrs(ctx, slog.LevelInfo, "audit", attrs...)
}

// Query retrieves audit log entries with optional filtering.
type QueryParams struct {
	UserID       *int64
	Action       string
	ResourceType string
	Limit        int
	Offset       int
}

// Query returns audit log entries matching the given parameters.
func (l *Logger) Query(ctx context.Context, params QueryParams) ([]Entry, int64, error) {
	if params.Limit <= 0 || params.Limit > 100 {
		params.Limit = 50
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	if params.Action != "" {
		if err := validateAuditFilter("action", params.Action); err != nil {
			return nil, 0, err
		}
	}
	if params.ResourceType != "" {
		if err := validateAuditFilter("resource_type", params.ResourceType); err != nil {
			return nil, 0, err
		}
	}

	const where = `WHERE ($1::bigint IS NULL OR user_id = $1)
		AND ($2::text = '' OR action = $2)
		AND ($3::text = '' OR resource_type = $3)`
	args := []any{params.UserID, params.Action, params.ResourceType}

	var total int64
	if err := l.pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_log `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit entries: %w", err)
	}

	rows, err := l.pool.Query(ctx, `
		SELECT id, timestamp, user_id, action, resource_type, resource_id, details,
		       host(ip_address)::text, user_agent
		FROM audit_log `+where+`
		ORDER BY timestamp DESC
		LIMIT $4 OFFSET $5`, append(args, params.Limit, params.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query audit log: %w", err)
	}
	entries, err := pgx.AppendRows([]Entry(nil), rows, func(row pgx.CollectableRow) (Entry, error) {
		var e Entry
		var detailsJSON []byte
		if err := row.Scan(&e.ID, &e.Timestamp, &e.UserID, &e.Action,
			&e.ResourceType, &e.ResourceID, &detailsJSON, &e.IPAddress, &e.UserAgent); err != nil {
			return Entry{}, fmt.Errorf("scan audit entry: %w", err)
		}
		if len(detailsJSON) > 0 {
			_ = json.Unmarshal(detailsJSON, &e.Details)
		}
		return e, nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("audit rows: %w", err)
	}
	return entries, total, nil
}

// ExtractIP returns the client IP from a request. Chi's RealIP middleware
// rewrites RemoteAddr to the real IP, so we only need to strip the port.
func ExtractIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

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
	ActionDeviceOnline   = "device.online"
	ActionDeviceOffline  = "device.offline"
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

	// Notification rule CRUD actions.
	ActionNotifCreate = "notification.create"
	ActionNotifUpdate = "notification.update"
	ActionNotifDelete = "notification.delete"
	ActionNotifSent   = "notification.sent"
	ActionNotifFailed = "notification.failed"

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
	ResourceUser         = "user"
	ResourceDevice       = "device"
	ResourceGeofence     = "geofence"
	ResourceCalendar     = "calendar"
	ResourceNotification = "notification"
	ResourceSession      = "session"
	ResourceApiKey       = "apikey"
	ResourceShare        = "share"
	ResourceCommand      = "command"
)

// Entry represents a single audit log entry.
type Entry struct {
	ID           int64                  `json:"id"`
	Timestamp    time.Time              `json:"timestamp"`
	UserID       *int64                 `json:"userId,omitempty"`
	Action       string                 `json:"action"`
	ResourceType *string                `json:"resourceType,omitempty"`
	ResourceID   *int64                 `json:"resourceId,omitempty"`
	Details      map[string]interface{} `json:"details,omitempty"`
	IPAddress    *string                `json:"ipAddress,omitempty"`
	UserAgent    *string                `json:"userAgent,omitempty"`
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

// SetLogger configures the structured logger for audit operations.
func (l *Logger) SetLogger(sl *slog.Logger) {
	if l != nil && sl != nil {
		l.logger = sl
	}
}

// Log records an audit event. Errors are logged but never returned to
// callers, because audit logging must not break application flow.
func (l *Logger) Log(ctx context.Context, userID *int64, action, resourceType string, resourceID *int64, details map[string]interface{}, ip, userAgent string) {
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

	var resType *string
	if resourceType != "" {
		resType = &resourceType
	}

	var ipAddr *string
	if ip != "" {
		// Validate the IP to avoid INET parse errors.
		if parsed := net.ParseIP(ip); parsed != nil {
			ipStr := parsed.String()
			ipAddr = &ipStr
		}
	}

	var ua *string
	if userAgent != "" {
		ua = &userAgent
	}

	_, err := l.pool.Exec(ctx, `
		INSERT INTO audit_log (user_id, action, resource_type, resource_id, details, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, userID, action, resType, resourceID, detailsJSON, ipAddr, ua)
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

// LogFromRequest is a convenience method that extracts IP and User-Agent
// from an HTTP request.
func (l *Logger) LogFromRequest(r *http.Request, userID *int64, action, resourceType string, resourceID *int64, details map[string]interface{}) {
	ip := extractIP(r)
	ua := r.UserAgent()
	l.Log(r.Context(), userID, action, resourceType, resourceID, details, ip, ua)
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

	// Validate filter strings before they reach the query builder.
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

	// Build the WHERE clause dynamically.
	where := "WHERE 1=1"
	args := []any{}
	argIdx := 1

	if params.UserID != nil {
		where += fmt.Sprintf(" AND user_id = $%d", argIdx)
		args = append(args, *params.UserID)
		argIdx++
	}
	if params.Action != "" {
		where += fmt.Sprintf(" AND action = $%d", argIdx)
		args = append(args, params.Action)
		argIdx++
	}
	if params.ResourceType != "" {
		where += fmt.Sprintf(" AND resource_type = $%d", argIdx)
		args = append(args, params.ResourceType)
		argIdx++
	}

	// Get total count for pagination.
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_log %s", where)
	var total int64
	err := l.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count audit entries: %w", err)
	}

	// Fetch entries.
	query := fmt.Sprintf(`
		SELECT id, timestamp, user_id, action, resource_type, resource_id, details,
		       host(ip_address)::text, user_agent
		FROM audit_log %s
		ORDER BY timestamp DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, params.Limit, params.Offset)

	rows, err := l.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query audit log: %w", err)
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		var detailsJSON []byte
		err := rows.Scan(&e.ID, &e.Timestamp, &e.UserID, &e.Action,
			&e.ResourceType, &e.ResourceID, &detailsJSON, &e.IPAddress, &e.UserAgent)
		if err != nil {
			return nil, 0, fmt.Errorf("scan audit entry: %w", err)
		}
		if len(detailsJSON) > 0 {
			_ = json.Unmarshal(detailsJSON, &e.Details)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("audit rows: %w", err)
	}

	return entries, total, nil
}

// extractIP returns the client IP from a request, handling X-Forwarded-For.
func extractIP(r *http.Request) string {
	// Chi's RealIP middleware sets RemoteAddr to the real IP.
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

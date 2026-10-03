package demo

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamcore/motus/internal/storage/repository"
	"golang.org/x/crypto/bcrypt"
)

// DefaultDeviceIMEIs are the default demo device identifiers.
// Must be numeric-only for Traccar H02 protocol compatibility.
var DefaultDeviceIMEIs = []string{"9000000000001", "9000000000002"}

// DemoGeofenceNames are the names of geofences created by demo mode.
// Used for selective cleanup.
var DemoGeofenceNames = []string{
	"Cologne Start",
	"Munich End",
	"Berlin Start",
	"Stuttgart End",
}

// DemoNotificationPrefix is the prefix for demo notification rules.
const DemoNotificationPrefix = "Demo "

// ResetResult contains counts of affected resources during a reset.
type ResetResult struct {
	UsersReset               int
	DevicesDeleted           int
	GeofencesDeleted         int
	GeofencesCreated         int
	NotificationRulesDeleted int
	NotificationRulesCreated int
	PositionsDeleted         int
	EventsDeleted            int
	SessionsDeleted          int
	SharesDeleted            int
	CommandsDeleted          int
	NotificationLogsDeleted  int
	AuditLogsDeleted         int
	ApiKeysDeleted           int
	ApiKeysCreated           int
	PasskeysDeleted          int
	TrailBookmarksDeleted    int
}

// Reset performs a comprehensive demo environment reset. It:
//  1. Deletes all demo-managed transient data (positions, events, sessions, etc.)
//  2. Deletes demo-managed resources (devices, geofences, notification rules)
//  3. Re-creates all demo resources from scratch (users, devices, geofences, notifications)
//
// Only resources identified as demo-managed are affected. User-created resources
// (e.g., Traccar imports, manually created devices) are preserved.
//
// This function is the single source of truth for demo reset logic. It is called by:
//   - The nightly reset timer (Service.Start)
//   - The `motus reset-demo` CLI command
//   - Init containers at startup
func Reset(ctx context.Context, pool *pgxpool.Pool, accounts []DemoAccount, deviceIMEIs []string) (*ResetResult, error) {
	result := &ResetResult{}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin reset transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	demoEmails := make([]string, len(accounts))
	for i, a := range accounts {
		demoEmails[i] = a.Email
	}

	const (
		byDevice = `SELECT id FROM devices WHERE unique_id = ANY($1)`
		byUser   = `SELECT id FROM users WHERE email = ANY($1)`
	)
	// Passkeys and trail bookmarks are deleted explicitly: demo users are upserted, so ON DELETE CASCADE never fires.
	deletes := []struct {
		what  string
		query string
		args  []any
		count *int
	}{
		{"positions", `DELETE FROM positions WHERE device_id IN (` + byDevice + `)`, []any{deviceIMEIs}, &result.PositionsDeleted},
		{"events", `DELETE FROM events WHERE device_id IN (` + byDevice + `)`, []any{deviceIMEIs}, &result.EventsDeleted},
		{"commands", `DELETE FROM commands WHERE device_id IN (` + byDevice + `)`, []any{deviceIMEIs}, &result.CommandsDeleted},
		{"device shares", `DELETE FROM device_shares WHERE device_id IN (` + byDevice + `)`, []any{deviceIMEIs}, &result.SharesDeleted},
		{"sessions", `DELETE FROM sessions WHERE user_id IN (` + byUser + `)`, []any{demoEmails}, &result.SessionsDeleted},
		{"notification logs", `DELETE FROM notification_log WHERE rule_id IN (SELECT id FROM notification_rules WHERE user_id IN (` + byUser + `))`, []any{demoEmails}, &result.NotificationLogsDeleted},
		{"audit logs", `DELETE FROM audit_log WHERE user_id IN (` + byUser + `)`, []any{demoEmails}, &result.AuditLogsDeleted},
		{"api keys", `DELETE FROM api_keys WHERE user_id IN (` + byUser + `)`, []any{demoEmails}, &result.ApiKeysDeleted},
		{"passkey credentials", `DELETE FROM passkey_credentials WHERE user_id IN (` + byUser + `)`, []any{demoEmails}, &result.PasskeysDeleted},
		{"trail bookmarks", `DELETE FROM trail_bookmarks WHERE user_id IN (` + byUser + `)`, []any{demoEmails}, &result.TrailBookmarksDeleted},
		{"notification rules", `DELETE FROM notification_rules WHERE user_id IN (` + byUser + `) AND name LIKE $2`, []any{demoEmails, DemoNotificationPrefix + "%"}, &result.NotificationRulesDeleted},
		{"user-device associations", `DELETE FROM user_devices WHERE device_id IN (` + byDevice + `)`, []any{deviceIMEIs}, nil},
		{"devices", `DELETE FROM devices WHERE unique_id = ANY($1)`, []any{deviceIMEIs}, &result.DevicesDeleted},
		{"user-geofence associations", `DELETE FROM user_geofences WHERE geofence_id IN (SELECT id FROM geofences WHERE name = ANY($1))`, []any{DemoGeofenceNames}, nil},
		{"geofences", `DELETE FROM geofences WHERE name = ANY($1)`, []any{DemoGeofenceNames}, &result.GeofencesDeleted},
	}
	for _, d := range deletes {
		tag, err := tx.Exec(ctx, d.query, d.args...)
		if err != nil {
			return nil, fmt.Errorf("delete demo %s: %w", d.what, err)
		}
		if d.count != nil {
			*d.count = int(tag.RowsAffected())
		}
	}

	// The legacy token (email local part) is stored hashed to match GetByToken, and
	// mirrored into api_keys as readonly because auth checks api_keys first.
	userIDs := make(map[string]int64)
	for _, acct := range accounts {
		hash, err := bcrypt.GenerateFromPassword([]byte(acct.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash password for %s: %w", acct.Email, err)
		}

		token := acct.Email
		if name, _, ok := strings.Cut(acct.Email, "@"); ok && name != "" {
			token = name
		}
		hashedToken := repository.HashToken(token)

		var userID int64
		err = tx.QueryRow(ctx,
			`INSERT INTO users (email, password_hash, name, role, token)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (email) DO UPDATE SET password_hash = $2, name = $3, role = $4, token = $5
			 RETURNING id`,
			acct.Email, string(hash), acct.Name, acct.Role, hashedToken,
		).Scan(&userID)
		if err != nil {
			return nil, fmt.Errorf("upsert user %s: %w", acct.Email, err)
		}
		userIDs[acct.Email] = userID
		result.UsersReset++

		_, err = tx.Exec(ctx,
			`INSERT INTO api_keys (user_id, token, name, permissions)
			 VALUES ($1, $2, $3, 'readonly')`,
			userID, hashedToken, acct.Name+" API Key",
		)
		if err != nil {
			return nil, fmt.Errorf("create readonly api key for %s: %w", acct.Email, err)
		}
		result.ApiKeysCreated++
	}

	// Demo devices are not pre-registered; the protocol server auto-registers them.
	demoUserID := userIDs["demo@motus.local"]

	demoGeofences := []struct {
		Name string
		Lat  float64
		Lon  float64
	}{
		{"Cologne Start", 50.9375, 6.9603},
		{"Munich End", 48.1351, 11.5820},
		{"Berlin Start", 52.5200, 13.4050},
		{"Stuttgart End", 48.7758, 9.1829},
	}

	for _, gf := range demoGeofences {
		_, err = tx.Exec(ctx, `
			INSERT INTO geofences (name, geometry, created_at, updated_at)
			VALUES ($1, ST_Buffer(ST_MakePoint($2, $3)::geography, 1000)::geometry, NOW(), NOW())
		`, gf.Name, gf.Lon, gf.Lat)
		if err != nil {
			return nil, fmt.Errorf("create geofence %s: %w", gf.Name, err)
		}
		result.GeofencesCreated++
	}

	// Associate geofences with demo user only (not admin).
	if demoUserID > 0 {
		_, err = tx.Exec(ctx, `
			INSERT INTO user_geofences (user_id, geofence_id)
			SELECT $1, id FROM geofences
			WHERE name = ANY($2)
			ON CONFLICT DO NOTHING
		`, demoUserID, DemoGeofenceNames)
		if err != nil {
			return nil, fmt.Errorf("associate geofences with demo user: %w", err)
		}
	}

	if demoUserID > 0 {
		_, err = tx.Exec(ctx, `
			INSERT INTO notification_rules (user_id, name, event_types, channel, config, template, enabled)
			VALUES
				($1, 'Demo All Events',
				 '{geofenceEnter,geofenceExit,deviceOnline,deviceOffline,motion,deviceIdle,ignitionOn,ignitionOff,alarm}',
				 'webhook', $2::jsonb, $3, true)
		`, demoUserID,
			`{"webhookUrl":"https://ntfy.sh/motus-gps","headers":{"Title":"Motus GPS Alert"}}`,
			`{{device.name}}: {{event.type}} at {{position.latitude}},{{position.longitude}} ({{position.speed}} km/h)`,
		)
		if err != nil {
			return nil, fmt.Errorf("create demo notification rules: %w", err)
		}
		result.NotificationRulesCreated = 1
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit reset transaction: %w", err)
	}

	return result, nil
}

// LogResult prints a summary of the reset operation.
func LogResult(result *ResetResult) {
	counts := []struct {
		name string
		n    int
	}{
		{"positions", result.PositionsDeleted},
		{"events", result.EventsDeleted},
		{"commands", result.CommandsDeleted},
		{"shares", result.SharesDeleted},
		{"sessions", result.SessionsDeleted},
		{"notificationLogs", result.NotificationLogsDeleted},
		{"auditLogs", result.AuditLogsDeleted},
		{"apiKeys", result.ApiKeysDeleted},
		{"passkeys", result.PasskeysDeleted},
		{"trailBookmarks", result.TrailBookmarksDeleted},
		{"rulesDeleted", result.NotificationRulesDeleted},
		{"devicesDeleted", result.DevicesDeleted},
		{"geofencesDeleted", result.GeofencesDeleted},
	}
	var parts []any
	for _, c := range counts {
		if c.n > 0 {
			parts = append(parts, slog.Int(c.name, c.n))
		}
	}

	deleted := slog.String("deleted", "none")
	if len(parts) > 0 {
		deleted = slog.Group("deleted", parts...)
	}

	slog.Info("demo reset complete",
		deleted,
		slog.Int("usersReset", result.UsersReset),
		slog.Int("geofencesCreated", result.GeofencesCreated),
		slog.Int("rulesCreated", result.NotificationRulesCreated),
		slog.Int("apiKeysCreated", result.ApiKeysCreated),
	)
}

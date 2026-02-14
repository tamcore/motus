package demo

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
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

	// Build list of demo user emails for filtering.
	demoEmails := make([]string, len(accounts))
	for i, a := range accounts {
		demoEmails[i] = a.Email
	}

	// -----------------------------------------------------------------------
	// Phase 1: Clean up transient data tied to demo devices.
	// -----------------------------------------------------------------------

	// Delete positions for demo devices.
	tag, err := tx.Exec(ctx, `
		DELETE FROM positions WHERE device_id IN (
			SELECT id FROM devices WHERE unique_id = ANY($1)
		)
	`, deviceIMEIs)
	if err != nil {
		return nil, fmt.Errorf("delete demo positions: %w", err)
	}
	result.PositionsDeleted = int(tag.RowsAffected())

	// Delete events for demo devices.
	tag, err = tx.Exec(ctx, `
		DELETE FROM events WHERE device_id IN (
			SELECT id FROM devices WHERE unique_id = ANY($1)
		)
	`, deviceIMEIs)
	if err != nil {
		return nil, fmt.Errorf("delete demo events: %w", err)
	}
	result.EventsDeleted = int(tag.RowsAffected())

	// Delete commands for demo devices.
	tag, err = tx.Exec(ctx, `
		DELETE FROM commands WHERE device_id IN (
			SELECT id FROM devices WHERE unique_id = ANY($1)
		)
	`, deviceIMEIs)
	if err != nil {
		return nil, fmt.Errorf("delete demo commands: %w", err)
	}
	result.CommandsDeleted = int(tag.RowsAffected())

	// Delete device shares for demo devices.
	tag, err = tx.Exec(ctx, `
		DELETE FROM device_shares WHERE device_id IN (
			SELECT id FROM devices WHERE unique_id = ANY($1)
		)
	`, deviceIMEIs)
	if err != nil {
		return nil, fmt.Errorf("delete demo device shares: %w", err)
	}
	result.SharesDeleted = int(tag.RowsAffected())

	// -----------------------------------------------------------------------
	// Phase 2: Clean up transient data tied to demo users.
	// -----------------------------------------------------------------------

	// Delete sessions for demo users.
	tag, err = tx.Exec(ctx, `
		DELETE FROM sessions WHERE user_id IN (
			SELECT id FROM users WHERE email = ANY($1)
		)
	`, demoEmails)
	if err != nil {
		return nil, fmt.Errorf("delete demo sessions: %w", err)
	}
	result.SessionsDeleted = int(tag.RowsAffected())

	// Delete notification logs for demo users' notification rules.
	tag, err = tx.Exec(ctx, `
		DELETE FROM notification_log WHERE rule_id IN (
			SELECT id FROM notification_rules WHERE user_id IN (
				SELECT id FROM users WHERE email = ANY($1)
			)
		)
	`, demoEmails)
	if err != nil {
		return nil, fmt.Errorf("delete demo notification logs: %w", err)
	}
	result.NotificationLogsDeleted = int(tag.RowsAffected())

	// Delete audit logs for demo users.
	tag, err = tx.Exec(ctx, `
		DELETE FROM audit_log WHERE user_id IN (
			SELECT id FROM users WHERE email = ANY($1)
		)
	`, demoEmails)
	if err != nil {
		return nil, fmt.Errorf("delete demo audit logs: %w", err)
	}
	result.AuditLogsDeleted = int(tag.RowsAffected())

	// Delete API keys for demo users.
	tag, err = tx.Exec(ctx, `
		DELETE FROM api_keys WHERE user_id IN (
			SELECT id FROM users WHERE email = ANY($1)
		)
	`, demoEmails)
	if err != nil {
		return nil, fmt.Errorf("delete demo api keys: %w", err)
	}
	result.ApiKeysDeleted = int(tag.RowsAffected())

	// -----------------------------------------------------------------------
	// Phase 3: Delete demo-managed resources.
	// -----------------------------------------------------------------------

	// Delete demo notification rules (by name prefix and user ownership).
	tag, err = tx.Exec(ctx, `
		DELETE FROM notification_rules WHERE user_id IN (
			SELECT id FROM users WHERE email = ANY($1)
		) AND name LIKE $2
	`, demoEmails, DemoNotificationPrefix+"%")
	if err != nil {
		return nil, fmt.Errorf("delete demo notification rules: %w", err)
	}
	result.NotificationRulesDeleted = int(tag.RowsAffected())

	// Delete user-device associations for demo devices.
	_, err = tx.Exec(ctx, `
		DELETE FROM user_devices WHERE device_id IN (
			SELECT id FROM devices WHERE unique_id = ANY($1)
		)
	`, deviceIMEIs)
	if err != nil {
		return nil, fmt.Errorf("delete demo user-device associations: %w", err)
	}

	// Delete demo devices.
	tag, err = tx.Exec(ctx, `
		DELETE FROM devices WHERE unique_id = ANY($1)
	`, deviceIMEIs)
	if err != nil {
		return nil, fmt.Errorf("delete demo devices: %w", err)
	}
	result.DevicesDeleted = int(tag.RowsAffected())

	// Delete user-geofence associations for demo geofences.
	_, err = tx.Exec(ctx, `
		DELETE FROM user_geofences WHERE geofence_id IN (
			SELECT id FROM geofences WHERE name = ANY($1)
		)
	`, DemoGeofenceNames)
	if err != nil {
		return nil, fmt.Errorf("delete demo user-geofence associations: %w", err)
	}

	// Delete demo geofences.
	tag, err = tx.Exec(ctx, `
		DELETE FROM geofences WHERE name = ANY($1)
	`, DemoGeofenceNames)
	if err != nil {
		return nil, fmt.Errorf("delete demo geofences: %w", err)
	}
	result.GeofencesDeleted = int(tag.RowsAffected())

	// -----------------------------------------------------------------------
	// Phase 4: Re-create demo users (upsert to handle existing).
	// -----------------------------------------------------------------------
	userIDs := make(map[string]int64)
	for _, acct := range accounts {
		hash, err := bcrypt.GenerateFromPassword([]byte(acct.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash password for %s: %w", acct.Email, err)
		}

		// Extract username from email for simple token (demo@motus.local → demo)
		token := acct.Email
		if idx := strings.Index(acct.Email, "@"); idx > 0 {
			token = acct.Email[:idx]
		}

		var userID int64
		err = tx.QueryRow(ctx,
			`INSERT INTO users (email, password_hash, name, role, token)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (email) DO UPDATE SET password_hash = $2, name = $3, role = $4, token = $5
			 RETURNING id`,
			acct.Email, string(hash), acct.Name, acct.Role, token, // token = username part (demo@motus.local → "demo")
		).Scan(&userID)
		if err != nil {
			return nil, fmt.Errorf("upsert user %s: %w", acct.Email, err)
		}
		userIDs[acct.Email] = userID
		result.UsersReset++
	}

	// -----------------------------------------------------------------------
	// Phase 4b: Re-create readonly API keys for demo users.
	//
	// Each demo user's legacy token (stored in users.token) is also inserted
	// into api_keys with readonly permissions. Because the auth middleware
	// checks api_keys before the legacy users.token column, this ensures
	// demo tokens are treated as read-only, preventing state-changing API
	// requests (POST, PUT, DELETE, PATCH).
	// -----------------------------------------------------------------------
	for _, acct := range accounts {
		userID := userIDs[acct.Email]
		token := acct.Email
		if idx := strings.Index(acct.Email, "@"); idx > 0 {
			token = acct.Email[:idx]
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO api_keys (user_id, token, name, permissions)
			 VALUES ($1, $2, $3, 'readonly')`,
			userID, token, acct.Name+" API Key",
		)
		if err != nil {
			return nil, fmt.Errorf("create readonly api key for %s: %w", acct.Email, err)
		}
		result.ApiKeysCreated++
	}

	// -----------------------------------------------------------------------
	// Phase 5: Re-create demo geofences.
	// (Demo devices are no longer pre-registered — they are auto-registered
	// by the GPS protocol server when the simulator sends the first position.)
	// -----------------------------------------------------------------------
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

	// -----------------------------------------------------------------------
	// Phase 6: Re-create demo notification rules.
	// -----------------------------------------------------------------------
	if demoUserID > 0 {
		_, err = tx.Exec(ctx, `
			INSERT INTO notification_rules (user_id, name, event_types, channel, config, template, enabled)
			VALUES
				($1, 'Demo All Events',
				 '{geofenceEnter,geofenceExit,deviceOnline,deviceOffline,overspeed,motion,deviceIdle,ignitionOn,ignitionOff,alarm}',
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

	// Commit the entire reset as one atomic operation.
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit reset transaction: %w", err)
	}

	return result, nil
}

// LogResult prints a summary of the reset operation.
func LogResult(result *ResetResult) {
	var parts []string

	if result.PositionsDeleted > 0 {
		parts = append(parts, fmt.Sprintf("positions=%d", result.PositionsDeleted))
	}
	if result.EventsDeleted > 0 {
		parts = append(parts, fmt.Sprintf("events=%d", result.EventsDeleted))
	}
	if result.CommandsDeleted > 0 {
		parts = append(parts, fmt.Sprintf("commands=%d", result.CommandsDeleted))
	}
	if result.SharesDeleted > 0 {
		parts = append(parts, fmt.Sprintf("shares=%d", result.SharesDeleted))
	}
	if result.SessionsDeleted > 0 {
		parts = append(parts, fmt.Sprintf("sessions=%d", result.SessionsDeleted))
	}
	if result.NotificationLogsDeleted > 0 {
		parts = append(parts, fmt.Sprintf("notificationLogs=%d", result.NotificationLogsDeleted))
	}
	if result.AuditLogsDeleted > 0 {
		parts = append(parts, fmt.Sprintf("auditLogs=%d", result.AuditLogsDeleted))
	}
	if result.ApiKeysDeleted > 0 {
		parts = append(parts, fmt.Sprintf("apiKeys=%d", result.ApiKeysDeleted))
	}
	if result.NotificationRulesDeleted > 0 {
		parts = append(parts, fmt.Sprintf("rulesDeleted=%d", result.NotificationRulesDeleted))
	}
	if result.DevicesDeleted > 0 {
		parts = append(parts, fmt.Sprintf("devicesDeleted=%d", result.DevicesDeleted))
	}
	if result.GeofencesDeleted > 0 {
		parts = append(parts, fmt.Sprintf("geofencesDeleted=%d", result.GeofencesDeleted))
	}

	deleted := "none"
	if len(parts) > 0 {
		deleted = strings.Join(parts, ", ")
	}

	slog.Info("demo reset complete",
		slog.String("deleted", deleted),
		slog.Int("usersReset", result.UsersReset),
		slog.Int("geofencesCreated", result.GeofencesCreated),
		slog.Int("rulesCreated", result.NotificationRulesCreated),
		slog.Int("apiKeysCreated", result.ApiKeysCreated),
	)
}

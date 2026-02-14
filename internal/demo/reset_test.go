package demo_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamcore/motus/internal/demo"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func setupPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	return pool
}

func TestReset_CreatesAllResources(t *testing.T) {
	pool := setupPool(t)
	ctx := context.Background()

	result, err := demo.Reset(ctx, pool, demo.DefaultAccounts, demo.DefaultDeviceIMEIs)
	if err != nil {
		t.Fatalf("Reset() returned error: %v", err)
	}

	// Verify users created.
	if result.UsersReset != 2 {
		t.Errorf("UsersReset = %d, want 2", result.UsersReset)
	}

	// Verify geofences created.
	if result.GeofencesCreated != 4 {
		t.Errorf("GeofencesCreated = %d, want 4", result.GeofencesCreated)
	}

	// Verify notification rules created.
	if result.NotificationRulesCreated != 1 {
		t.Errorf("NotificationRulesCreated = %d, want 1", result.NotificationRulesCreated)
	}

	// Verify readonly API keys created (one per demo account).
	if result.ApiKeysCreated != 2 {
		t.Errorf("ApiKeysCreated = %d, want 2", result.ApiKeysCreated)
	}

	// Verify data exists in the database.
	assertRowCount(t, pool, "SELECT COUNT(*) FROM users WHERE email LIKE '%@motus.local'", 2)
	assertRowCount(t, pool, "SELECT COUNT(*) FROM geofences WHERE name = ANY($1)", 4, demo.DemoGeofenceNames)
	assertRowCount(t, pool, "SELECT COUNT(*) FROM notification_rules WHERE name LIKE 'Demo %'", 1)

	// Verify API keys exist with readonly permissions.
	assertRowCount(t, pool, `
		SELECT COUNT(*) FROM api_keys
		WHERE user_id IN (SELECT id FROM users WHERE email LIKE '%@motus.local')
		  AND permissions = 'readonly'
	`, 2)

	// Verify no full-permission demo API keys exist.
	assertRowCount(t, pool, `
		SELECT COUNT(*) FROM api_keys
		WHERE user_id IN (SELECT id FROM users WHERE email LIKE '%@motus.local')
		  AND permissions = 'full'
	`, 0)

	// Verify demo token matches the username part of the email.
	assertRowCount(t, pool, `
		SELECT COUNT(*) FROM api_keys WHERE token = 'demo' AND permissions = 'readonly'
	`, 1)
	assertRowCount(t, pool, `
		SELECT COUNT(*) FROM api_keys WHERE token = 'admin' AND permissions = 'readonly'
	`, 1)

	// Verify user-geofence associations (only demo user, not admin).
	assertRowCount(t, pool, `
		SELECT COUNT(*) FROM user_geofences
		WHERE user_id = (SELECT id FROM users WHERE email = 'demo@motus.local')
	`, 4)

	// Admin should have no geofence associations.
	assertRowCount(t, pool, `
		SELECT COUNT(*) FROM user_geofences
		WHERE user_id = (SELECT id FROM users WHERE email = 'admin@motus.local')
	`, 0)
}

func TestReset_CleansExistingDemoData(t *testing.T) {
	pool := setupPool(t)
	ctx := context.Background()

	// First reset: seed users and geofences.
	_, err := demo.Reset(ctx, pool, demo.DefaultAccounts, demo.DefaultDeviceIMEIs)
	if err != nil {
		t.Fatalf("first Reset() failed: %v", err)
	}

	// Simulate an auto-registered demo device (as the GPS simulator would create it).
	var demoDeviceID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO devices (unique_id, name, protocol, status, created_at, updated_at)
		VALUES ('9000000000001', 'Demo Car 1', 'h02', 'online', NOW(), NOW())
		RETURNING id
	`).Scan(&demoDeviceID)
	if err != nil {
		t.Fatalf("failed to insert demo device: %v", err)
	}

	// Insert some transient data for the demo device.
	_, err = pool.Exec(ctx, `
		INSERT INTO positions (device_id, latitude, longitude, speed, course, timestamp)
		VALUES ($1, 50.0, 10.0, 60.0, 180.0, NOW())
	`, demoDeviceID)
	if err != nil {
		t.Fatalf("failed to insert position: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO events (device_id, type, timestamp)
		VALUES ($1, 'deviceOnline', NOW())
	`, demoDeviceID)
	if err != nil {
		t.Fatalf("failed to insert event: %v", err)
	}

	// Insert a session for demo user.
	var demoUserID int64
	err = pool.QueryRow(ctx, "SELECT id FROM users WHERE email = 'demo@motus.local'").Scan(&demoUserID)
	if err != nil {
		t.Fatalf("failed to get demo user ID: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO sessions (id, user_id, created_at, expires_at)
		VALUES ('test-session-123', $1, NOW(), NOW() + INTERVAL '1 hour')
	`, demoUserID)
	if err != nil {
		t.Fatalf("failed to insert session: %v", err)
	}

	// Second reset: should clean all transient data including the auto-registered device.
	result, err := demo.Reset(ctx, pool, demo.DefaultAccounts, demo.DefaultDeviceIMEIs)
	if err != nil {
		t.Fatalf("second Reset() failed: %v", err)
	}

	// Verify transient data was cleaned.
	if result.PositionsDeleted < 1 {
		t.Errorf("PositionsDeleted = %d, want >= 1", result.PositionsDeleted)
	}
	if result.EventsDeleted < 1 {
		t.Errorf("EventsDeleted = %d, want >= 1", result.EventsDeleted)
	}
	if result.SessionsDeleted < 1 {
		t.Errorf("SessionsDeleted = %d, want >= 1", result.SessionsDeleted)
	}
	if result.DevicesDeleted < 1 {
		t.Errorf("DevicesDeleted = %d, want >= 1", result.DevicesDeleted)
	}

	// Verify device was removed (will be re-registered by simulator on next position).
	assertRowCount(t, pool, "SELECT COUNT(*) FROM devices WHERE unique_id IN ('9000000000001','9000000000002')", 0)

	// Verify geofences were re-created.
	if result.GeofencesCreated != 4 {
		t.Errorf("GeofencesCreated = %d, want 4", result.GeofencesCreated)
	}
}

func TestReset_PreservesNonDemoResources(t *testing.T) {
	pool := setupPool(t)
	ctx := context.Background()

	// Create a non-demo user.
	_, err := pool.Exec(ctx, `
		INSERT INTO users (email, password_hash, name, role)
		VALUES ('real@example.com', 'hash123', 'Real User', 'user')
	`)
	if err != nil {
		t.Fatalf("failed to create non-demo user: %v", err)
	}

	// Create a non-demo device.
	var realDeviceID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO devices (unique_id, name, protocol, status, created_at, updated_at)
		VALUES ('REAL001', 'Real Device', 'h02', 'online', NOW(), NOW())
		RETURNING id
	`).Scan(&realDeviceID)
	if err != nil {
		t.Fatalf("failed to create non-demo device: %v", err)
	}

	// Create a non-demo geofence.
	_, err = pool.Exec(ctx, `
		INSERT INTO geofences (name, geometry, created_at, updated_at)
		VALUES ('Home', ST_Buffer(ST_MakePoint(10.0, 50.0)::geography, 500)::geometry, NOW(), NOW())
	`)
	if err != nil {
		t.Fatalf("failed to create non-demo geofence: %v", err)
	}

	// Insert a position for the non-demo device.
	_, err = pool.Exec(ctx, `
		INSERT INTO positions (device_id, latitude, longitude, speed, course, timestamp)
		VALUES ($1, 50.0, 10.0, 30.0, 90.0, NOW())
	`, realDeviceID)
	if err != nil {
		t.Fatalf("failed to insert non-demo position: %v", err)
	}

	// Run demo reset.
	_, err = demo.Reset(ctx, pool, demo.DefaultAccounts, demo.DefaultDeviceIMEIs)
	if err != nil {
		t.Fatalf("Reset() returned error: %v", err)
	}

	// Verify non-demo user still exists.
	assertRowCount(t, pool, "SELECT COUNT(*) FROM users WHERE email = 'real@example.com'", 1)

	// Verify non-demo device still exists.
	assertRowCount(t, pool, "SELECT COUNT(*) FROM devices WHERE unique_id = 'REAL001'", 1)

	// Verify non-demo geofence still exists.
	assertRowCount(t, pool, "SELECT COUNT(*) FROM geofences WHERE name = 'Home'", 1)

	// Verify non-demo position still exists.
	assertRowCount(t, pool, `
		SELECT COUNT(*) FROM positions WHERE device_id IN (
			SELECT id FROM devices WHERE unique_id = 'REAL001'
		)
	`, 1)
}

func TestReset_Idempotent(t *testing.T) {
	pool := setupPool(t)
	ctx := context.Background()

	// Run reset three times in a row.
	for i := 0; i < 3; i++ {
		result, err := demo.Reset(ctx, pool, demo.DefaultAccounts, demo.DefaultDeviceIMEIs)
		if err != nil {
			t.Fatalf("Reset() iteration %d returned error: %v", i+1, err)
		}

		// Each run should produce the same creation counts.
		if result.UsersReset != 2 {
			t.Errorf("iteration %d: UsersReset = %d, want 2", i+1, result.UsersReset)
		}
		if result.GeofencesCreated != 4 {
			t.Errorf("iteration %d: GeofencesCreated = %d, want 4", i+1, result.GeofencesCreated)
		}
		if result.ApiKeysCreated != 2 {
			t.Errorf("iteration %d: ApiKeysCreated = %d, want 2", i+1, result.ApiKeysCreated)
		}
	}

	// Final state should have exactly the expected counts.
	assertRowCount(t, pool, "SELECT COUNT(*) FROM users WHERE email LIKE '%@motus.local'", 2)
	assertRowCount(t, pool, "SELECT COUNT(*) FROM geofences WHERE name = ANY($1)", 4, demo.DemoGeofenceNames)

	// Verify exactly 2 readonly API keys remain after repeated resets.
	assertRowCount(t, pool, `
		SELECT COUNT(*) FROM api_keys
		WHERE user_id IN (SELECT id FROM users WHERE email LIKE '%@motus.local')
		  AND permissions = 'readonly'
	`, 2)
}

func TestReset_ResetsUserPasswords(t *testing.T) {
	pool := setupPool(t)
	ctx := context.Background()

	// First reset to seed.
	_, err := demo.Reset(ctx, pool, demo.DefaultAccounts, demo.DefaultDeviceIMEIs)
	if err != nil {
		t.Fatalf("first Reset() failed: %v", err)
	}

	// Tamper with the demo user's password.
	_, err = pool.Exec(ctx, `
		UPDATE users SET password_hash = 'tampered_hash' WHERE email = 'demo@motus.local'
	`)
	if err != nil {
		t.Fatalf("failed to tamper password: %v", err)
	}

	// Reset should restore the password.
	_, err = demo.Reset(ctx, pool, demo.DefaultAccounts, demo.DefaultDeviceIMEIs)
	if err != nil {
		t.Fatalf("second Reset() failed: %v", err)
	}

	// Verify password hash was restored (not the tampered value).
	var hash string
	err = pool.QueryRow(ctx, "SELECT password_hash FROM users WHERE email = 'demo@motus.local'").Scan(&hash)
	if err != nil {
		t.Fatalf("failed to read password hash: %v", err)
	}
	if hash == "tampered_hash" {
		t.Error("password hash was not reset by Reset()")
	}
}

func TestLogResult(t *testing.T) {
	// Capture slog output by setting a temporary default logger.
	var buf bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(oldLogger)

	result := &demo.ResetResult{
		UsersReset:               2,
		DevicesDeleted:           3,
		GeofencesDeleted:         4,
		GeofencesCreated:         4,
		NotificationRulesDeleted: 2,
		NotificationRulesCreated: 1,
		PositionsDeleted:         100,
		EventsDeleted:            50,
		SessionsDeleted:          5,
		ApiKeysDeleted:           3,
		ApiKeysCreated:           2,
	}

	demo.LogResult(result)

	output := buf.String()
	if !strings.Contains(output, "positions=100") {
		t.Errorf("log output missing positions count: %s", output)
	}
	if !strings.Contains(output, "events=50") {
		t.Errorf("log output missing events count: %s", output)
	}
	if !strings.Contains(output, "usersReset=2") {
		t.Errorf("log output missing users count: %s", output)
	}
	if !strings.Contains(output, "apiKeys=3") {
		t.Errorf("log output missing apiKeys count: %s", output)
	}
	if !strings.Contains(output, "apiKeysCreated=2") {
		t.Errorf("log output missing apiKeysCreated count: %s", output)
	}
}

func TestLogResult_NothingDeleted(t *testing.T) {
	var buf bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(oldLogger)

	result := &demo.ResetResult{
		UsersReset:               2,
		GeofencesCreated:         4,
		NotificationRulesCreated: 1,
		ApiKeysCreated:           2,
	}

	demo.LogResult(result)

	output := buf.String()
	if !strings.Contains(output, "deleted=none") {
		t.Errorf("expected 'deleted=none' in output, got: %s", output)
	}
	if !strings.Contains(output, "apiKeysCreated=2") {
		t.Errorf("expected 'apiKeysCreated=2' in output, got: %s", output)
	}
}

func TestDemoGeofenceNames(t *testing.T) {
	names := demo.DemoGeofenceNames
	if len(names) != 4 {
		t.Fatalf("DemoGeofenceNames has %d entries, want 4", len(names))
	}

	expected := map[string]bool{
		"Cologne Start": true,
		"Munich End":    true,
		"Berlin Start":  true,
		"Stuttgart End": true,
	}
	for _, name := range names {
		if !expected[name] {
			t.Errorf("unexpected geofence name: %q", name)
		}
	}
}

func TestReset_CleansAutoRegisteredDemoDevices(t *testing.T) {
	pool := setupPool(t)
	ctx := context.Background()

	// Seed demo users.
	_, err := demo.Reset(ctx, pool, demo.DefaultAccounts, demo.DefaultDeviceIMEIs)
	if err != nil {
		t.Fatalf("Reset() returned error: %v", err)
	}

	// Simulate two auto-registered demo devices (as the GPS simulator would create them).
	var devID1, devID2 int64
	err = pool.QueryRow(ctx, `
		INSERT INTO devices (unique_id, name, protocol, status, created_at, updated_at)
		VALUES ('9000000000001', 'Demo Car 1', 'h02', 'online', NOW(), NOW())
		RETURNING id
	`).Scan(&devID1)
	if err != nil {
		t.Fatalf("failed to insert demo device 1: %v", err)
	}
	err = pool.QueryRow(ctx, `
		INSERT INTO devices (unique_id, name, protocol, status, created_at, updated_at)
		VALUES ('9000000000002', 'Demo Car 2', 'h02', 'online', NOW(), NOW())
		RETURNING id
	`).Scan(&devID2)
	if err != nil {
		t.Fatalf("failed to insert demo device 2: %v", err)
	}

	// Insert positions for both devices.
	for _, devID := range []int64{devID1, devID2} {
		_, err = pool.Exec(ctx, `
			INSERT INTO positions (device_id, latitude, longitude, speed, course, timestamp)
			VALUES ($1, 50.0, 10.0, 80.0, 180.0, NOW())
		`, devID)
		if err != nil {
			t.Fatalf("failed to insert position: %v", err)
		}
	}

	assertRowCount(t, pool, "SELECT COUNT(*) FROM devices WHERE unique_id IN ('9000000000001','9000000000002')", 2)
	assertRowCount(t, pool, "SELECT COUNT(*) FROM positions WHERE device_id IN (SELECT id FROM devices WHERE unique_id IN ('9000000000001','9000000000002'))", 2)

	// Reset should delete the auto-registered devices and all their data.
	result, err := demo.Reset(ctx, pool, demo.DefaultAccounts, demo.DefaultDeviceIMEIs)
	if err != nil {
		t.Fatalf("second Reset() returned error: %v", err)
	}

	if result.DevicesDeleted != 2 {
		t.Errorf("DevicesDeleted = %d, want 2", result.DevicesDeleted)
	}
	if result.PositionsDeleted != 2 {
		t.Errorf("PositionsDeleted = %d, want 2", result.PositionsDeleted)
	}

	assertRowCount(t, pool, "SELECT COUNT(*) FROM devices WHERE unique_id IN ('9000000000001','9000000000002')", 0)
	assertRowCount(t, pool, "SELECT COUNT(*) FROM positions WHERE device_id IN (SELECT id FROM devices WHERE unique_id IN ('9000000000001','9000000000002'))", 0)
}

func TestReset_CleansApiKeys(t *testing.T) {
	pool := setupPool(t)
	ctx := context.Background()

	// First reset to seed demo users.
	_, err := demo.Reset(ctx, pool, demo.DefaultAccounts, demo.DefaultDeviceIMEIs)
	if err != nil {
		t.Fatalf("first Reset() failed: %v", err)
	}

	// Get demo user IDs.
	var demoUserID, adminUserID int64
	err = pool.QueryRow(ctx, "SELECT id FROM users WHERE email = 'demo@motus.local'").Scan(&demoUserID)
	if err != nil {
		t.Fatalf("failed to get demo user ID: %v", err)
	}
	err = pool.QueryRow(ctx, "SELECT id FROM users WHERE email = 'admin@motus.local'").Scan(&adminUserID)
	if err != nil {
		t.Fatalf("failed to get admin user ID: %v", err)
	}

	// The first reset already created 2 readonly API keys (one per demo user).
	// Insert additional API keys for both demo users to test cleanup.
	_, err = pool.Exec(ctx, `
		INSERT INTO api_keys (user_id, token, name, permissions) VALUES
			($1, 'demo-token-aaa', 'Demo Key 1', 'full'),
			($1, 'demo-token-bbb', 'Demo Key 2', 'readonly'),
			($2, 'admin-token-ccc', 'Admin Key 1', 'full')
	`, demoUserID, adminUserID)
	if err != nil {
		t.Fatalf("failed to insert API keys: %v", err)
	}

	// Verify keys exist before reset: 2 auto-created + 3 manually inserted = 5.
	assertRowCount(t, pool, "SELECT COUNT(*) FROM api_keys WHERE user_id = ANY($1)", 5, []int64{demoUserID, adminUserID})

	// Create a non-demo user with an API key that should survive reset.
	var realUserID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, name, role)
		VALUES ('real@example.com', 'hash123', 'Real User', 'user')
		RETURNING id
	`).Scan(&realUserID)
	if err != nil {
		t.Fatalf("failed to create non-demo user: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO api_keys (user_id, token, name, permissions)
		VALUES ($1, 'real-token-ddd', 'Real Key', 'full')
	`, realUserID)
	if err != nil {
		t.Fatalf("failed to insert non-demo API key: %v", err)
	}

	// Second reset: should clean ALL demo API keys (auto-created + manual) and re-create readonly ones.
	result, err := demo.Reset(ctx, pool, demo.DefaultAccounts, demo.DefaultDeviceIMEIs)
	if err != nil {
		t.Fatalf("second Reset() failed: %v", err)
	}

	// Verify API keys deleted count: 2 auto-created + 3 manually inserted = 5.
	if result.ApiKeysDeleted != 5 {
		t.Errorf("ApiKeysDeleted = %d, want 5", result.ApiKeysDeleted)
	}

	// Verify new readonly API keys were created (one per demo account).
	if result.ApiKeysCreated != 2 {
		t.Errorf("ApiKeysCreated = %d, want 2", result.ApiKeysCreated)
	}

	// Verify only the auto-created readonly keys remain for demo users.
	assertRowCount(t, pool, `
		SELECT COUNT(*) FROM api_keys WHERE user_id IN (
			SELECT id FROM users WHERE email LIKE '%@motus.local'
		)
	`, 2)

	// Verify they are all readonly.
	assertRowCount(t, pool, `
		SELECT COUNT(*) FROM api_keys WHERE user_id IN (
			SELECT id FROM users WHERE email LIKE '%@motus.local'
		) AND permissions = 'readonly'
	`, 2)

	// Verify non-demo API key still exists.
	assertRowCount(t, pool, "SELECT COUNT(*) FROM api_keys WHERE user_id = $1", 1, realUserID)
}

// assertRowCount verifies a COUNT(*) query returns the expected value.
func assertRowCount(t *testing.T, pool *pgxpool.Pool, query string, want int, args ...interface{}) {
	t.Helper()
	ctx := context.Background()
	var count int
	err := pool.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		t.Fatalf("assertRowCount query failed: %v\nquery: %s", err, query)
	}
	if count != want {
		t.Errorf("row count = %d, want %d\nquery: %s", count, want, query)
	}
}

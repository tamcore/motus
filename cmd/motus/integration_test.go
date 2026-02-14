package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

// TestMain provides setup/teardown for the integration test suite.
func TestMain(m *testing.M) {
	code := m.Run()
	testutil.Cleanup()
	os.Exit(code)
}

// injectTestDB overrides connectDBFn so every cobra command Run invocation
// connects to the test container. Each call returns a freshly allocated pool
// (which the command will Close) so the testutil shared pool is unaffected.
func injectTestDB(t *testing.T) (cleanup func()) {
	t.Helper()
	connStr := testutil.ConnStr(t)
	orig := connectDBFn
	connectDBFn = func() (*pgxpool.Pool, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		pool, err := pgxpool.New(ctx, connStr)
		if err != nil {
			return nil, err
		}
		if err := pool.Ping(ctx); err != nil {
			pool.Close()
			return nil, err
		}
		return pool, nil
	}
	return func() { connectDBFn = orig }
}

// captureStdout captures everything written to os.Stdout during f().
func captureStdout(f func()) string {
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	old := os.Stdout
	os.Stdout = w

	f()

	_ = w.Close()
	os.Stdout = old
	var buf strings.Builder
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

// mustExec runs a SQL statement and fails the test on error.
func mustExec(t *testing.T, pool *pgxpool.Pool, ctx context.Context, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("mustExec: %v", err)
	}
}

// --- User commands ---

func TestUserAdd_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	defer injectTestDB(t)()
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	cmd := newUserAddCmd()
	if err := cmd.Flags().Set("email", "add@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("name", "Add User"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("password", "Password1!"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("role", "user"); err != nil {
		t.Fatal(err)
	}
	cmd.Run(cmd, nil)

	var email string
	if err := pool.QueryRow(ctx, `SELECT email FROM users WHERE email = $1`, "add@example.com").Scan(&email); err != nil {
		t.Fatalf("user not found in DB: %v", err)
	}
}

func TestUserList_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	defer injectTestDB(t)()
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	mustExec(t, pool, ctx, `INSERT INTO users (email, name, password_hash, role, created_at) VALUES ($1,$2,$3,$4,NOW())`,
		"list1@example.com", "List One", "hash1", "admin")
	mustExec(t, pool, ctx, `INSERT INTO users (email, name, password_hash, role, created_at) VALUES ($1,$2,$3,$4,NOW())`,
		"list2@example.com", "List Two", "hash2", "user")

	for _, output := range []string{"table", "json", "csv"} {
		cmd := newUserListCmd()
		if err := cmd.Flags().Set("output", output); err != nil {
			t.Fatal(err)
		}
		out := captureStdout(func() { cmd.Run(cmd, nil) })
		if !strings.Contains(out, "list1@example.com") {
			t.Errorf("output=%s missing list1@example.com: %q", output, out)
		}
	}
}

func TestUserList_WithFilter_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	defer injectTestDB(t)()
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	mustExec(t, pool, ctx, `INSERT INTO users (email, name, password_hash, role, created_at) VALUES ($1,$2,$3,$4,NOW())`,
		"admin@example.com", "Admin User", "hash", "admin")
	mustExec(t, pool, ctx, `INSERT INTO users (email, name, password_hash, role, created_at) VALUES ($1,$2,$3,$4,NOW())`,
		"user@example.com", "Regular User", "hash", "user")

	cmd := newUserListCmd()
	if err := cmd.Flags().Set("filter", "role=admin"); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(func() { cmd.Run(cmd, nil) })
	if !strings.Contains(out, "admin@example.com") {
		t.Errorf("filter=role=admin: missing admin user in output: %q", out)
	}
	if strings.Contains(out, "user@example.com") {
		t.Errorf("filter=role=admin: regular user should not be in output: %q", out)
	}
}

func TestUserList_Empty_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	defer injectTestDB(t)()
	testutil.CleanTables(t, pool)

	cmd := newUserListCmd()
	out := captureStdout(func() { cmd.Run(cmd, nil) })
	if !strings.Contains(out, "No users found") {
		t.Errorf("expected 'No users found', got: %q", out)
	}
}

func TestUserDelete_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	defer injectTestDB(t)()
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	mustExec(t, pool, ctx, `INSERT INTO users (email, name, password_hash, role, created_at) VALUES ($1,$2,$3,$4,NOW())`,
		"delete@example.com", "Delete Me", "hash", "user")

	cmd := newUserDeleteCmd()
	if err := cmd.Flags().Set("email", "delete@example.com"); err != nil {
		t.Fatal(err)
	}
	cmd.Run(cmd, nil)

	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE email = $1`, "delete@example.com").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Error("user should have been deleted")
	}
}

func TestUserUpdate_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	defer injectTestDB(t)()
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	mustExec(t, pool, ctx, `INSERT INTO users (email, name, password_hash, role, created_at) VALUES ($1,$2,$3,$4,NOW())`,
		"update@example.com", "Original Name", "hash", "user")

	cmd := newUserUpdateCmd()
	if err := cmd.Flags().Set("email", "update@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("name", "Updated Name"); err != nil {
		t.Fatal(err)
	}
	cmd.Run(cmd, nil)

	var name string
	if err := pool.QueryRow(ctx, `SELECT name FROM users WHERE email = $1`, "update@example.com").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Updated Name" {
		t.Errorf("name = %q, want 'Updated Name'", name)
	}
}

func TestUserSetPassword_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	defer injectTestDB(t)()
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	mustExec(t, pool, ctx, `INSERT INTO users (email, name, password_hash, role, created_at) VALUES ($1,$2,$3,$4,NOW())`,
		"setpwd@example.com", "Set Pwd", "oldhash", "user")

	cmd := newUserSetPasswordCmd()
	if err := cmd.Flags().Set("email", "setpwd@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("password", "NewPassword1!"); err != nil {
		t.Fatal(err)
	}
	cmd.Run(cmd, nil)

	var hash string
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE email = $1`, "setpwd@example.com").Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if hash == "oldhash" {
		t.Error("password hash should have been updated")
	}
}

func TestUserSetPassword_Generated_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	defer injectTestDB(t)()
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	mustExec(t, pool, ctx, `INSERT INTO users (email, name, password_hash, role, created_at) VALUES ($1,$2,$3,$4,NOW())`,
		"genpwd@example.com", "Gen Pwd", "oldhash", "user")

	cmd := newUserSetPasswordCmd()
	if err := cmd.Flags().Set("email", "genpwd@example.com"); err != nil {
		t.Fatal(err)
	}
	// No --password flag → auto-generate
	out := captureStdout(func() { cmd.Run(cmd, nil) })
	if !strings.Contains(out, "Generated password:") {
		t.Errorf("expected 'Generated password:' in output, got: %q", out)
	}
}

// --- Device commands ---

func TestDeviceAdd_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	defer injectTestDB(t)()
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	cmd := newDeviceAddCmd()
	if err := cmd.Flags().Set("unique-id", "DEVICE-ADD-001"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("name", "Add Device"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("protocol", "h02"); err != nil {
		t.Fatal(err)
	}
	cmd.Run(cmd, nil)

	var uid string
	if err := pool.QueryRow(ctx, `SELECT unique_id FROM devices WHERE unique_id = $1`, "DEVICE-ADD-001").Scan(&uid); err != nil {
		t.Fatalf("device not found in DB: %v", err)
	}
}

func TestDeviceList_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	defer injectTestDB(t)()
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	mustExec(t, pool, ctx, `INSERT INTO devices (unique_id, name, protocol, status, created_at, updated_at) VALUES ($1,$2,$3,$4,NOW(),NOW())`,
		"LIST-001", "List Device One", "h02", "online")
	mustExec(t, pool, ctx, `INSERT INTO devices (unique_id, name, protocol, status, created_at, updated_at) VALUES ($1,$2,$3,$4,NOW(),NOW())`,
		"LIST-002", "List Device Two", "watch", "offline")

	for _, output := range []string{"table", "json", "csv"} {
		cmd := newDeviceListCmd()
		if err := cmd.Flags().Set("output", output); err != nil {
			t.Fatal(err)
		}
		out := captureStdout(func() { cmd.Run(cmd, nil) })
		if !strings.Contains(out, "LIST-001") {
			t.Errorf("output=%s missing LIST-001: %q", output, out)
		}
	}
}

func TestDeviceList_WithFilter_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	defer injectTestDB(t)()
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	mustExec(t, pool, ctx, `INSERT INTO devices (unique_id, name, protocol, status, created_at, updated_at) VALUES ($1,$2,$3,$4,NOW(),NOW())`,
		"ONLINE-001", "Online Device", "h02", "online")
	mustExec(t, pool, ctx, `INSERT INTO devices (unique_id, name, protocol, status, created_at, updated_at) VALUES ($1,$2,$3,$4,NOW(),NOW())`,
		"OFFLINE-001", "Offline Device", "h02", "offline")

	cmd := newDeviceListCmd()
	if err := cmd.Flags().Set("filter", "status=online"); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(func() { cmd.Run(cmd, nil) })
	if !strings.Contains(out, "ONLINE-001") {
		t.Errorf("missing ONLINE-001: %q", out)
	}
	if strings.Contains(out, "OFFLINE-001") {
		t.Errorf("OFFLINE-001 should be filtered out: %q", out)
	}
}

func TestDeviceList_Empty_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	defer injectTestDB(t)()
	testutil.CleanTables(t, pool)

	cmd := newDeviceListCmd()
	out := captureStdout(func() { cmd.Run(cmd, nil) })
	if !strings.Contains(out, "No devices found") {
		t.Errorf("expected 'No devices found', got: %q", out)
	}
}

func TestDeviceDelete_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	defer injectTestDB(t)()
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	mustExec(t, pool, ctx, `INSERT INTO devices (unique_id, name, protocol, status, created_at, updated_at) VALUES ($1,$2,$3,$4,NOW(),NOW())`,
		"DELETE-001", "Delete Device", "h02", "offline")

	cmd := newDeviceDeleteCmd()
	if err := cmd.Flags().Set("unique-id", "DELETE-001"); err != nil {
		t.Fatal(err)
	}
	cmd.Run(cmd, nil)

	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM devices WHERE unique_id = $1`, "DELETE-001").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Error("device should have been deleted")
	}
}

func TestDeviceUpdate_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	defer injectTestDB(t)()
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	mustExec(t, pool, ctx, `INSERT INTO devices (unique_id, name, protocol, status, created_at, updated_at) VALUES ($1,$2,$3,$4,NOW(),NOW())`,
		"UPDATE-001", "Old Name", "h02", "offline")

	cmd := newDeviceUpdateCmd()
	if err := cmd.Flags().Set("unique-id", "UPDATE-001"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("name", "New Name"); err != nil {
		t.Fatal(err)
	}
	cmd.Run(cmd, nil)

	var name string
	if err := pool.QueryRow(ctx, `SELECT name FROM devices WHERE unique_id = $1`, "UPDATE-001").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "New Name" {
		t.Errorf("name = %q, want 'New Name'", name)
	}
}

// --- API Key commands ---

func TestApiKeyAdd_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	defer injectTestDB(t)()
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	mustExec(t, pool, ctx, `INSERT INTO users (email, name, password_hash, role, created_at) VALUES ($1,$2,$3,$4,NOW())`,
		"keyuser@example.com", "Key User", "hash", "user")

	cmd := newUserKeysAddCmd()
	if err := cmd.Flags().Set("email", "keyuser@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("name", "my-key"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("permissions", "full"); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(func() { cmd.Run(cmd, nil) })
	if !strings.Contains(out, "my-key") {
		t.Errorf("expected key name in output, got: %q", out)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM api_keys WHERE name = $1`, "my-key").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 api_key row, got %d", count)
	}
}

func TestApiKeyList_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	defer injectTestDB(t)()
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	mustExec(t, pool, ctx, `INSERT INTO users (email, name, password_hash, role, created_at) VALUES ($1,$2,$3,$4,NOW())`,
		"listkey@example.com", "List Key User", "hash", "user")
	var userID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, "listkey@example.com").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	mustExec(t, pool, ctx, `INSERT INTO api_keys (user_id, name, token, permissions, created_at) VALUES ($1,$2,$3,$4,NOW())`,
		userID, "listed-key", "tok123", "readonly")

	for _, output := range []string{"table", "json", "csv"} {
		cmd := newUserKeysListCmd()
		if err := cmd.Flags().Set("email", "listkey@example.com"); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Flags().Set("output", output); err != nil {
			t.Fatal(err)
		}
		out := captureStdout(func() { cmd.Run(cmd, nil) })
		if !strings.Contains(out, "listed-key") {
			t.Errorf("output=%s missing 'listed-key': %q", output, out)
		}
	}
}

func TestApiKeyList_Empty_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	defer injectTestDB(t)()
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	mustExec(t, pool, ctx, `INSERT INTO users (email, name, password_hash, role, created_at) VALUES ($1,$2,$3,$4,NOW())`,
		"nokeys@example.com", "No Keys", "hash", "user")

	cmd := newUserKeysListCmd()
	if err := cmd.Flags().Set("email", "nokeys@example.com"); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(func() { cmd.Run(cmd, nil) })
	if !strings.Contains(out, "No API keys") {
		t.Errorf("expected 'No API keys' in output, got: %q", out)
	}
}

func TestApiKeyDelete_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	defer injectTestDB(t)()
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	mustExec(t, pool, ctx, `INSERT INTO users (email, name, password_hash, role, created_at) VALUES ($1,$2,$3,$4,NOW())`,
		"delkey@example.com", "Del Key", "hash", "user")
	var userID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, "delkey@example.com").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	mustExec(t, pool, ctx, `INSERT INTO api_keys (user_id, name, token, permissions, created_at) VALUES ($1,$2,$3,$4,NOW())`,
		userID, "to-delete", "tok456", "full")
	var keyID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM api_keys WHERE name = $1`, "to-delete").Scan(&keyID); err != nil {
		t.Fatal(err)
	}

	cmd := newUserKeysDeleteCmd()
	if err := cmd.Flags().Set("id", fmt.Sprint(keyID)); err != nil {
		t.Fatal(err)
	}
	cmd.Run(cmd, nil)

	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM api_keys WHERE id = $1`, keyID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Error("api key should have been deleted")
	}
}

// --- Session commands ---

func TestSessionList_Empty_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	defer injectTestDB(t)()
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	mustExec(t, pool, ctx, `INSERT INTO users (email, name, password_hash, role, created_at) VALUES ($1,$2,$3,$4,NOW())`,
		"nosessions@example.com", "No Sessions", "hash", "user")

	cmd := newUserSessionsListCmd()
	if err := cmd.Flags().Set("email", "nosessions@example.com"); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(func() { cmd.Run(cmd, nil) })
	if !strings.Contains(out, "No active sessions") {
		t.Errorf("expected 'No active sessions' in output, got: %q", out)
	}
}

func TestSessionList_WithSessions_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	defer injectTestDB(t)()
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	mustExec(t, pool, ctx, `INSERT INTO users (email, name, password_hash, role, created_at) VALUES ($1,$2,$3,$4,NOW())`,
		"hassessions@example.com", "Has Sessions", "hash", "user")
	var userID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, "hassessions@example.com").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	mustExec(t, pool, ctx, `INSERT INTO sessions (id, user_id, remember_me, is_sudo, created_at, expires_at) VALUES ($1,$2,$3,$4,NOW(),NOW()+INTERVAL '1 day')`,
		"test-session-id-12345", userID, false, false)

	for _, output := range []string{"table", "json", "csv"} {
		cmd := newUserSessionsListCmd()
		if err := cmd.Flags().Set("email", "hassessions@example.com"); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Flags().Set("output", output); err != nil {
			t.Fatal(err)
		}
		out := captureStdout(func() { cmd.Run(cmd, nil) })
		if len(out) == 0 {
			t.Errorf("output=%s: expected non-empty output", output)
		}
	}
}

func TestSessionRevoke_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	defer injectTestDB(t)()
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	mustExec(t, pool, ctx, `INSERT INTO users (email, name, password_hash, role, created_at) VALUES ($1,$2,$3,$4,NOW())`,
		"revoke@example.com", "Revoke User", "hash", "user")
	var userID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, "revoke@example.com").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	sessionID := "revoke-session-id-xyz"
	mustExec(t, pool, ctx, `INSERT INTO sessions (id, user_id, remember_me, is_sudo, created_at, expires_at) VALUES ($1,$2,$3,$4,NOW(),NOW()+INTERVAL '1 day')`,
		sessionID, userID, false, false)

	cmd := newUserSessionsRevokeCmd()
	if err := cmd.Flags().Set("id", sessionID); err != nil {
		t.Fatal(err)
	}
	cmd.Run(cmd, nil)

	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions WHERE id = $1`, sessionID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Error("session should have been revoked")
	}
}

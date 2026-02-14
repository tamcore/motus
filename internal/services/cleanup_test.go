package services

import (
	"context"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func TestCleanupService_CleanExpiredSessions(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	// Create a test user first
	var userID int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, password_hash, name, role)
		 VALUES ('test@example.com', 'hash', 'Test User', 'user')
		 RETURNING id`,
	).Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Create test sessions: one expired recently, one expired 8 days ago
	recentExpired := time.Now().Add(-1 * time.Hour)
	oldExpired := time.Now().Add(-8 * 24 * time.Hour)

	_, err = pool.Exec(context.Background(),
		`INSERT INTO sessions (id, user_id, expires_at) VALUES
		 ('recent', $1, $2), ('old', $1, $3)`,
		userID, recentExpired, oldExpired,
	)
	if err != nil {
		t.Fatalf("Failed to create test sessions: %v", err)
	}

	// Run cleanup
	svc := NewCleanupService(pool, 24*time.Hour)
	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	// Verify only old session was deleted
	var count int
	err = pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM sessions WHERE id IN ('recent', 'old')`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count sessions: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 session remaining, got %d", count)
	}

	// Verify it's the recent one that remained
	var remaining string
	err = pool.QueryRow(context.Background(),
		`SELECT id FROM sessions WHERE id IN ('recent', 'old')`,
	).Scan(&remaining)
	if err != nil {
		t.Fatalf("Failed to get remaining session: %v", err)
	}

	if remaining != "recent" {
		t.Errorf("Expected 'recent' session to remain, got %s", remaining)
	}
}

func TestCleanupService_CleanExpiredShares(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	// Create a test user
	var userID int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, password_hash, name, role)
		 VALUES ('test@example.com', 'hash', 'Test User', 'user')
		 RETURNING id`,
	).Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Create a test device
	var deviceID int64
	err = pool.QueryRow(context.Background(),
		`INSERT INTO devices (unique_id, name, status)
		 VALUES ('test-dev', 'Test Device', 'unknown') RETURNING id`,
	).Scan(&deviceID)
	if err != nil {
		t.Fatalf("Failed to create test device: %v", err)
	}

	// Create test shares
	recentExpired := time.Now().Add(-1 * time.Hour)
	oldExpired := time.Now().Add(-8 * 24 * time.Hour)

	_, err = pool.Exec(context.Background(),
		`INSERT INTO device_shares (device_id, token, created_by, expires_at) VALUES
		 ($1, 'recent', $2, $3),
		 ($1, 'old', $2, $4),
		 ($1, 'never', $2, NULL)`,
		deviceID, userID, recentExpired, oldExpired,
	)
	if err != nil {
		t.Fatalf("Failed to create test shares: %v", err)
	}

	// Run cleanup
	svc := NewCleanupService(pool, 24*time.Hour)
	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	// Verify count: recent + never = 2 (old deleted)
	var count int
	err = pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM device_shares WHERE device_id = $1`,
		deviceID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count shares: %v", err)
	}

	if count != 2 {
		t.Errorf("Expected 2 shares remaining (recent + never), got %d", count)
	}
}

func TestCleanupService_NoExpiredData(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	svc := NewCleanupService(pool, 24*time.Hour)

	// Should not error when there's nothing to clean
	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce failed with no data: %v", err)
	}
}

func TestCleanupService_Start_ContextCancel(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	svc := NewCleanupService(pool, 24*time.Hour)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.Start(ctx)
	}()

	// Give Start time to complete the initial RunOnce and enter the select loop.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Good: goroutine exited after cancellation.
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not exit after context cancellation")
	}
}

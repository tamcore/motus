// Package testutil provides helpers for repository integration tests.
package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	// pgx driver for goose migrations.
	_ "github.com/jackc/pgx/v5/stdlib"
)

var (
	sharedPool      *pgxpool.Pool
	sharedContainer testcontainers.Container
	sharedConnStr   string
	once            sync.Once
	initErr         error
)

// SetupTestDB returns a shared connection pool to a PostGIS testcontainer.
// The container is started once and reused across all tests in the package.
// The caller should call CleanTables before each test for isolation.
func SetupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test (requires Docker/PostgreSQL) in short mode")
	}

	once.Do(func() {
		sharedPool, sharedContainer, sharedConnStr, initErr = startContainer()
	})

	if initErr != nil {
		t.Fatalf("failed to setup test DB: %v", initErr)
	}

	return sharedPool
}

// ConnStr returns the raw PostgreSQL connection string for the shared test
// container. Useful for tests that need to build their own connection pool
// (e.g. when testing functions that accept connection parameters rather than
// an already-open pool).
func ConnStr(t *testing.T) string {
	t.Helper()
	SetupTestDB(t) // ensure container is running
	return sharedConnStr
}

// Cleanup terminates the shared container and closes the pool.
// Call this from TestMain after m.Run().
func Cleanup() {
	if sharedPool != nil {
		sharedPool.Close()
	}
	if sharedContainer != nil {
		_ = sharedContainer.Terminate(context.Background())
	}
}

func startContainer() (*pgxpool.Pool, testcontainers.Container, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	req := testcontainers.ContainerRequest{
		Image:        "postgis/postgis:16-3.4",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_DB":       "motus_test",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).
			WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, nil, "", fmt.Errorf("start container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, nil, "", fmt.Errorf("get container host: %w", err)
	}
	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, nil, "", fmt.Errorf("get mapped port: %w", err)
	}

	connStr := fmt.Sprintf("postgres://postgres:test@%s:%s/motus_test?sslmode=disable", host, port.Port())

	if err := runMigrations(connStr); err != nil {
		_ = container.Terminate(ctx)
		return nil, nil, "", fmt.Errorf("run migrations: %w", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, nil, "", fmt.Errorf("connect pool: %w", err)
	}

	return pool, container, connStr, nil
}

// migrationsDir locates the project's migrations directory relative to this
// source file. This avoids issues with the working directory during tests.
func migrationsDir() string {
	_, filename, _, _ := runtime.Caller(0)
	// filename = .../internal/storage/repository/testutil/db.go
	// project root = 4 levels up (not 5)
	projectRoot := filepath.Join(filepath.Dir(filename), "..", "..", "..", "..")
	// Clean the path to resolve all .. references
	projectRoot = filepath.Clean(projectRoot)
	dir := filepath.Join(projectRoot, "migrations")

	// Validate the directory exists.
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		// Fallback: try from working directory.
		if cwd, err := os.Getwd(); err == nil {
			alt := filepath.Join(cwd, "migrations")
			if info, err := os.Stat(alt); err == nil && info.IsDir() {
				return alt
			}

			// Fallback 2: search upward from cwd
			current := cwd
			for i := 0; i < 10; i++ {
				alt = filepath.Join(current, "migrations")
				if info, err := os.Stat(alt); err == nil && info.IsDir() {
					return alt
				}
				current = filepath.Dir(current)
			}
		}
	}
	return dir
}

func runMigrations(connStr string) error {
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Enable PostGIS extension before running schema migrations.
	if _, err := db.Exec("CREATE EXTENSION IF NOT EXISTS postgis"); err != nil {
		return fmt.Errorf("create postgis extension: %w", err)
	}

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}

	dir := migrationsDir()
	if err := goose.Up(db, dir); err != nil {
		return fmt.Errorf("goose up from %s: %w", dir, err)
	}

	return nil
}

// CleanTables truncates all data tables for test isolation.
func CleanTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx := context.Background()
	// Single TRUNCATE CASCADE is faster than per-table truncates.
	_, err := pool.Exec(ctx, `TRUNCATE TABLE
		audit_log,
		notification_log,
		notification_rules,
		events,
		user_geofences,
		geofences,
		commands,
		device_shares,
		api_keys,
		positions,
		user_devices,
		sessions,
		user_calendars,
		devices,
		calendars,
		users,
		oidc_states
		CASCADE`)
	if err != nil {
		t.Fatalf("failed to truncate tables: %v", err)
	}
}

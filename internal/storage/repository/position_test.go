package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func createTestDevice(t *testing.T, pool interface {
	// We use the pool directly via repos.
}, deviceRepo *repository.DeviceRepository, userRepo *repository.UserRepository) (*model.User, *model.Device) {
	t.Helper()
	user := createTestUser(t, userRepo)
	device := &model.Device{
		UniqueID: "pos-dev-" + time.Now().Format("20060102150405.000000000"),
		Name:     "Position Test Device",
		Status:   "online",
	}
	if err := deviceRepo.Create(context.Background(), device, user.ID); err != nil {
		t.Fatalf("failed to create test device: %v", err)
	}
	return user, device
}

func TestPositionRepository_Create(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	_, device := createTestDevice(t, pool, deviceRepo, userRepo)

	speed := 45.5
	altitude := 100.0
	course := 180.0
	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.520008,
		Longitude: 13.404954,
		Speed:     &speed,
		Altitude:  &altitude,
		Course:    &course,
		Timestamp: time.Now().UTC(),
		Attributes: map[string]interface{}{
			"protocol": "h02",
			"flags":    "F",
		},
	}

	if err := posRepo.Create(ctx, pos); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if pos.ID == 0 {
		t.Error("expected position ID to be set")
	}
}

func TestPositionRepository_GetLatestByDevice(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	_, device := createTestDevice(t, pool, deviceRepo, userRepo)

	// Insert two positions at different times.
	now := time.Now().UTC()
	p1 := &model.Position{
		DeviceID: device.ID, Latitude: 52.0, Longitude: 13.0,
		Timestamp: now.Add(-10 * time.Minute),
	}
	p2 := &model.Position{
		DeviceID: device.ID, Latitude: 52.1, Longitude: 13.1,
		Timestamp: now,
	}

	if err := posRepo.Create(ctx, p1); err != nil {
		t.Fatalf("Create p1 failed: %v", err)
	}
	if err := posRepo.Create(ctx, p2); err != nil {
		t.Fatalf("Create p2 failed: %v", err)
	}

	latest, err := posRepo.GetLatestByDevice(ctx, device.ID)
	if err != nil {
		t.Fatalf("GetLatestByDevice failed: %v", err)
	}
	if latest.ID != p2.ID {
		t.Errorf("expected latest position ID %d, got %d", p2.ID, latest.ID)
	}
}

func TestPositionRepository_GetByDeviceAndTimeRange(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	_, device := createTestDevice(t, pool, deviceRepo, userRepo)

	now := time.Now().UTC()
	positions := []*model.Position{
		{DeviceID: device.ID, Latitude: 52.0, Longitude: 13.0, Timestamp: now.Add(-3 * time.Hour)},
		{DeviceID: device.ID, Latitude: 52.1, Longitude: 13.1, Timestamp: now.Add(-2 * time.Hour)},
		{DeviceID: device.ID, Latitude: 52.2, Longitude: 13.2, Timestamp: now.Add(-1 * time.Hour)},
		{DeviceID: device.ID, Latitude: 52.3, Longitude: 13.3, Timestamp: now},
	}

	for _, p := range positions {
		if err := posRepo.Create(ctx, p); err != nil {
			t.Fatalf("Create position failed: %v", err)
		}
	}

	// Query for positions in the last 2.5 hours.
	from := now.Add(-150 * time.Minute)
	to := now.Add(time.Minute)

	results, err := posRepo.GetByDeviceAndTimeRange(ctx, device.ID, from, to, 100)
	if err != nil {
		t.Fatalf("GetByDeviceAndTimeRange failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 positions in range, got %d", len(results))
	}
}

func TestPositionRepository_GetByDeviceAndTimeRange_LimitDefault(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	_, device := createTestDevice(t, pool, deviceRepo, userRepo)

	now := time.Now().UTC()
	pos := &model.Position{DeviceID: device.ID, Latitude: 52.0, Longitude: 13.0, Timestamp: now}
	if err := posRepo.Create(ctx, pos); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Invalid limit should default to maxPositionsPerQuery (10000).
	results, err := posRepo.GetByDeviceAndTimeRange(ctx, device.ID, now.Add(-time.Hour), now.Add(time.Minute), -1)
	if err != nil {
		t.Fatalf("GetByDeviceAndTimeRange failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 position, got %d", len(results))
	}
}

func TestPositionRepository_GetPreviousByDevice(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	_, device := createTestDevice(t, pool, deviceRepo, userRepo)

	now := time.Now().UTC()
	p1 := &model.Position{DeviceID: device.ID, Latitude: 52.0, Longitude: 13.0, Timestamp: now.Add(-10 * time.Minute)}
	p2 := &model.Position{DeviceID: device.ID, Latitude: 52.1, Longitude: 13.1, Timestamp: now}

	if err := posRepo.Create(ctx, p1); err != nil {
		t.Fatalf("Create p1 failed: %v", err)
	}
	if err := posRepo.Create(ctx, p2); err != nil {
		t.Fatalf("Create p2 failed: %v", err)
	}

	prev, err := posRepo.GetPreviousByDevice(ctx, device.ID, now)
	if err != nil {
		t.Fatalf("GetPreviousByDevice failed: %v", err)
	}
	if prev.ID != p1.ID {
		t.Errorf("expected previous position ID %d, got %d", p1.ID, prev.ID)
	}
}

func TestPositionRepository_GetByID(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	_, device := createTestDevice(t, pool, deviceRepo, userRepo)

	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.520008,
		Longitude: 13.404954,
		Timestamp: time.Now().UTC(),
		Attributes: map[string]interface{}{
			"key": "value",
		},
	}
	if err := posRepo.Create(ctx, pos); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	found, err := posRepo.GetByID(ctx, pos.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found.Latitude != 52.520008 {
		t.Errorf("expected latitude 52.520008, got %f", found.Latitude)
	}
	if found.Attributes["key"] != "value" {
		t.Errorf("expected attribute key=value, got %v", found.Attributes["key"])
	}
}

// TestPositionRepository_NullProtocol verifies that positions with NULL protocol
// (pre-migration 00014 rows) can be scanned without error.
func TestPositionRepository_NullProtocol(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user, device := createTestDevice(t, pool, deviceRepo, userRepo)

	// Insert a position with NULL protocol directly via SQL to simulate
	// pre-migration data that exists in production.
	now := time.Now().UTC()
	var posID int64
	err := pool.QueryRow(ctx,
		`INSERT INTO positions (device_id, latitude, longitude, timestamp, server_time, device_time, valid, outdated)
		 VALUES ($1, $2, $3, $4, $5, $6, true, false)
		 RETURNING id`,
		device.ID, 52.520008, 13.404954, now, now, now,
	).Scan(&posID)
	if err != nil {
		t.Fatalf("insert position with NULL protocol: %v", err)
	}

	// Verify GetByID works with NULL protocol.
	pos, err := posRepo.GetByID(ctx, posID)
	if err != nil {
		t.Fatalf("GetByID failed for NULL protocol position: %v", err)
	}
	if pos.Protocol != "" {
		t.Errorf("expected empty protocol for NULL value, got %q", pos.Protocol)
	}

	// Verify GetLatestByUser works (this is the production-failing path).
	positions, err := posRepo.GetLatestByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetLatestByUser failed for NULL protocol position: %v", err)
	}
	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}
	if positions[0].Protocol != "" {
		t.Errorf("expected empty protocol, got %q", positions[0].Protocol)
	}
}

func TestPositionRepository_GetByIDs(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	_, device := createTestDevice(t, pool, deviceRepo, userRepo)

	now := time.Now().UTC()
	p1 := &model.Position{DeviceID: device.ID, Latitude: 52.0, Longitude: 13.0, Timestamp: now.Add(-10 * time.Minute)}
	p2 := &model.Position{DeviceID: device.ID, Latitude: 52.1, Longitude: 13.1, Timestamp: now.Add(-5 * time.Minute)}
	p3 := &model.Position{DeviceID: device.ID, Latitude: 52.2, Longitude: 13.2, Timestamp: now}

	for _, p := range []*model.Position{p1, p2, p3} {
		if err := posRepo.Create(ctx, p); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// Fetch two of three by ID.
	results, err := posRepo.GetByIDs(ctx, []int64{p1.ID, p3.ID})
	if err != nil {
		t.Fatalf("GetByIDs failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(results))
	}

	// Verify IDs returned are the ones we asked for.
	gotIDs := map[int64]bool{}
	for _, p := range results {
		gotIDs[p.ID] = true
	}
	if !gotIDs[p1.ID] || !gotIDs[p3.ID] {
		t.Errorf("expected IDs %d and %d, got %v", p1.ID, p3.ID, gotIDs)
	}
	if gotIDs[p2.ID] {
		t.Error("should not have returned p2")
	}
}

func TestPositionRepository_GetByIDs_Empty(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	posRepo := repository.NewPositionRepository(pool)
	ctx := context.Background()

	results, err := posRepo.GetByIDs(ctx, []int64{})
	if err != nil {
		t.Fatalf("GetByIDs with empty slice failed: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil for empty IDs, got %v", results)
	}
}

func TestPositionRepository_GetByIDs_NonExistent(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	posRepo := repository.NewPositionRepository(pool)
	ctx := context.Background()

	results, err := posRepo.GetByIDs(ctx, []int64{999999, 999998})
	if err != nil {
		t.Fatalf("GetByIDs with non-existent IDs failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for non-existent IDs, got %d", len(results))
	}
}

func TestPositionRepository_GetLatestByUser(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user, device1 := createTestDevice(t, pool, deviceRepo, userRepo)
	device2 := &model.Device{
		UniqueID: "latuser-dev-2-" + time.Now().Format("20060102150405.000000000"),
		Name:     "Second Device",
		Status:   "online",
	}
	if err := deviceRepo.Create(ctx, device2, user.ID); err != nil {
		t.Fatalf("Create device2 failed: %v", err)
	}

	now := time.Now().UTC()
	// Positions for device 1
	_ = posRepo.Create(ctx, &model.Position{DeviceID: device1.ID, Latitude: 52.0, Longitude: 13.0, Timestamp: now.Add(-10 * time.Minute)})
	_ = posRepo.Create(ctx, &model.Position{DeviceID: device1.ID, Latitude: 52.1, Longitude: 13.1, Timestamp: now})
	// Positions for device 2
	_ = posRepo.Create(ctx, &model.Position{DeviceID: device2.ID, Latitude: 51.0, Longitude: 12.0, Timestamp: now.Add(-5 * time.Minute)})
	_ = posRepo.Create(ctx, &model.Position{DeviceID: device2.ID, Latitude: 51.1, Longitude: 12.1, Timestamp: now})

	latest, err := posRepo.GetLatestByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetLatestByUser failed: %v", err)
	}
	if len(latest) != 2 {
		t.Fatalf("expected 2 latest positions (one per device), got %d", len(latest))
	}
}

func TestPositionRepository_UpdateGeofenceIDs(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	_, device := createTestDevice(t, pool, deviceRepo, userRepo)

	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Timestamp: time.Now().UTC(),
	}
	if err := posRepo.Create(ctx, pos); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	ids := []int64{101, 202}
	if err := posRepo.UpdateGeofenceIDs(ctx, pos.ID, ids); err != nil {
		t.Fatalf("UpdateGeofenceIDs failed: %v", err)
	}

	// Read back and verify.
	got, err := posRepo.GetByID(ctx, pos.ID)
	if err != nil {
		t.Fatalf("GetByID after update failed: %v", err)
	}
	if len(got.GeofenceIDs) != 2 {
		t.Errorf("expected 2 geofence IDs, got %d", len(got.GeofenceIDs))
	}
}

func TestPositionRepository_UpdateAddress(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	_, device := createTestDevice(t, pool, deviceRepo, userRepo)

	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.37,
		Timestamp: time.Now().UTC(),
	}
	if err := posRepo.Create(ctx, pos); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	addr := "Alexanderplatz, Berlin, Germany"
	if err := posRepo.UpdateAddress(ctx, pos.ID, addr); err != nil {
		t.Fatalf("UpdateAddress failed: %v", err)
	}

	got, err := posRepo.GetByID(ctx, pos.ID)
	if err != nil {
		t.Fatalf("GetByID after update failed: %v", err)
	}
	if got.Address == nil || *got.Address != addr {
		t.Errorf("expected address %q, got %v", addr, got.Address)
	}
}

func TestPositionRepository_GetLatestAll(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	// Create two users with one device each.
	user1, device1 := createTestDevice(t, pool, deviceRepo, userRepo)
	_ = user1
	user2 := &model.User{
		Email:        "getlatestall2@example.com",
		PasswordHash: "$2a$10$hash",
		Name:         "User Two",
	}
	if err := userRepo.Create(ctx, user2); err != nil {
		t.Fatalf("Create user2 failed: %v", err)
	}
	device2 := &model.Device{
		UniqueID: "latall-dev-2-" + time.Now().Format("20060102150405.000000000"),
		Name:     "Second User Device",
		Status:   "online",
	}
	if err := deviceRepo.Create(ctx, device2, user2.ID); err != nil {
		t.Fatalf("Create device2 failed: %v", err)
	}

	now := time.Now().UTC()
	// Positions for device 1 (user 1)
	_ = posRepo.Create(ctx, &model.Position{DeviceID: device1.ID, Latitude: 52.0, Longitude: 13.0, Timestamp: now.Add(-10 * time.Minute)})
	_ = posRepo.Create(ctx, &model.Position{DeviceID: device1.ID, Latitude: 52.1, Longitude: 13.1, Timestamp: now})
	// Positions for device 2 (user 2)
	_ = posRepo.Create(ctx, &model.Position{DeviceID: device2.ID, Latitude: 51.0, Longitude: 12.0, Timestamp: now.Add(-5 * time.Minute)})
	_ = posRepo.Create(ctx, &model.Position{DeviceID: device2.ID, Latitude: 51.1, Longitude: 12.1, Timestamp: now})

	// GetLatestAll should return the latest position for EVERY device (across all users).
	latest, err := posRepo.GetLatestAll(ctx)
	if err != nil {
		t.Fatalf("GetLatestAll failed: %v", err)
	}
	if len(latest) != 2 {
		t.Fatalf("expected 2 latest positions (one per device), got %d", len(latest))
	}
}

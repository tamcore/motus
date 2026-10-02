package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func createTestDevice(t *testing.T, pool any, deviceRepo *repository.DeviceRepository, userRepo *repository.UserRepository) (*model.User, *model.Device) {
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
		Attributes: map[string]any{
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

func TestPositionRepository_StreamByDeviceAndTimeRange_NegativeLimitTreatedAsUnlimited(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	_, device := createTestDevice(t, pool, deviceRepo, userRepo)

	now := time.Now().UTC()
	for i := range 3 {
		p := &model.Position{
			DeviceID:  device.ID,
			Latitude:  52.0 + float64(i)*0.01,
			Longitude: 13.0,
			Timestamp: now.Add(time.Duration(-3+i) * time.Minute),
		}
		if err := posRepo.Create(ctx, p); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// Negative limit must behave the same as 0 (unlimited).
	var results []*model.Position
	err := posRepo.StreamByDeviceAndTimeRange(ctx, device.ID, now.Add(-time.Hour), now.Add(time.Minute), -1, func(p *model.Position) error {
		results = append(results, p)
		return nil
	})
	if err != nil {
		t.Fatalf("StreamByDeviceAndTimeRange failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 positions for negative limit (treated as unlimited), got %d", len(results))
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
		Attributes: map[string]any{
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

	empty := &model.Device{UniqueID: "latuser-empty-" + time.Now().Format("20060102150405.000000000"), Name: "No Positions", Status: "online"}
	if err := deviceRepo.Create(ctx, empty, user.ID); err != nil {
		t.Fatalf("Create empty device failed: %v", err)
	}

	latest, err := posRepo.GetLatestByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetLatestByUser failed: %v", err)
	}
	if len(latest) != 2 {
		t.Fatalf("expected 2 latest positions (one per device with positions), got %d", len(latest))
	}
	assertLatest(t, latest, map[int64]float64{device1.ID: 52.1, device2.ID: 51.1})
}

func assertLatest(t *testing.T, latest []*model.Position, wantLat map[int64]float64) {
	t.Helper()
	for _, p := range latest {
		if want, ok := wantLat[p.DeviceID]; !ok || p.Latitude != want {
			t.Errorf("device %d: latitude %v, want newest position %v", p.DeviceID, p.Latitude, want)
		}
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

func TestPositionRepository_StreamByDeviceAndTimeRange_AllRows(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	_, device := createTestDevice(t, pool, deviceRepo, userRepo)

	now := time.Now().UTC()
	for i := range 10 {
		p := &model.Position{
			DeviceID:  device.ID,
			Latitude:  52.0 + float64(i)*0.01,
			Longitude: 13.0,
			Timestamp: now.Add(time.Duration(-10+i) * time.Minute),
		}
		if err := posRepo.Create(ctx, p); err != nil {
			t.Fatalf("Create position %d failed: %v", i, err)
		}
	}

	var count int
	err := posRepo.StreamByDeviceAndTimeRange(ctx, device.ID, now.Add(-time.Hour), now.Add(time.Minute), 0, func(p *model.Position) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("StreamByDeviceAndTimeRange failed: %v", err)
	}
	if count != 10 {
		t.Errorf("expected 10 positions streamed, got %d", count)
	}
}

func TestPositionRepository_StreamByDeviceAndTimeRange_WithLimit(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	_, device := createTestDevice(t, pool, deviceRepo, userRepo)

	now := time.Now().UTC()
	for i := range 10 {
		p := &model.Position{
			DeviceID:  device.ID,
			Latitude:  52.0 + float64(i)*0.01,
			Longitude: 13.0,
			Timestamp: now.Add(time.Duration(-10+i) * time.Minute),
		}
		if err := posRepo.Create(ctx, p); err != nil {
			t.Fatalf("Create position %d failed: %v", i, err)
		}
	}

	var count int
	err := posRepo.StreamByDeviceAndTimeRange(ctx, device.ID, now.Add(-time.Hour), now.Add(time.Minute), 3, func(p *model.Position) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("StreamByDeviceAndTimeRange failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 positions with limit=3, got %d", count)
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
	assertLatest(t, latest, map[int64]float64{device1.ID: 52.1, device2.ID: 51.1})
}

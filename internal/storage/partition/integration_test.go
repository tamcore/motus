package partition_test

import (
	"context"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/partition"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
	"os"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testutil.Cleanup()
	os.Exit(code)
}

func TestPartitionManager_EnsureFuturePartitions(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	mgr := partition.NewManager(pool, 0, 1*time.Hour)

	ctx := context.Background()
	if err := mgr.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	// Verify partitions were created.
	partitions, err := mgr.ListPartitions(ctx)
	if err != nil {
		t.Fatalf("ListPartitions failed: %v", err)
	}

	// Should have at least current month + 3 future months = 4 partitions.
	// Migration may have created additional historical partitions, so we check >= 4.
	if len(partitions) < 4 {
		t.Errorf("expected at least 4 partitions, got %d", len(partitions))
		for _, p := range partitions {
			t.Logf("  partition: %s (%s to %s)", p.Name, p.RangeStart, p.RangeEnd)
		}
	}

	// Verify current month's partition exists.
	now := time.Now().UTC()
	currentName := partition.PartitionName(now)
	found := false
	for _, p := range partitions {
		if p.Name == currentName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("current month partition %q not found in: %v", currentName, partitions)
	}
}

func TestPartitionManager_Idempotent(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	mgr := partition.NewManager(pool, 0, 1*time.Hour)

	ctx := context.Background()
	// Run twice - should not error on existing partitions.
	if err := mgr.RunOnce(ctx); err != nil {
		t.Fatalf("first RunOnce failed: %v", err)
	}
	if err := mgr.RunOnce(ctx); err != nil {
		t.Fatalf("second RunOnce failed: %v", err)
	}

	partitions, err := mgr.ListPartitions(ctx)
	if err != nil {
		t.Fatalf("ListPartitions failed: %v", err)
	}

	if len(partitions) < 4 {
		t.Errorf("expected at least 4 partitions after idempotent runs, got %d", len(partitions))
	}
}

func TestPartitionManager_InsertIntoPartition(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	mgr := partition.NewManager(pool, 0, 1*time.Hour)
	ctx := context.Background()

	if err := mgr.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	// Create a test device and insert a position.
	userRepo := repository.NewUserRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	posRepo := repository.NewPositionRepository(pool)

	user := &model.User{
		Email:        "parttest@example.com",
		PasswordHash: "$2a$10$fakehashforfasttest",
		Name:         "Partition Test",
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	device := &model.Device{
		UniqueID: "part-test-" + time.Now().Format("20060102150405.000000000"),
		Name:     "Partition Test Device",
		Status:   "online",
	}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	speed := 45.5
	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.520008,
		Longitude: 13.404954,
		Speed:     &speed,
		Timestamp: time.Now().UTC(),
	}

	if err := posRepo.Create(ctx, pos); err != nil {
		t.Fatalf("create position: %v", err)
	}

	if pos.ID == 0 {
		t.Error("expected position ID to be set after insert into partitioned table")
	}

	// Verify we can read it back.
	got, err := posRepo.GetByID(ctx, pos.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Latitude != 52.520008 {
		t.Errorf("expected latitude 52.520008, got %f", got.Latitude)
	}
}

func TestPartitionManager_TimeRangeQuery(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	mgr := partition.NewManager(pool, 0, 1*time.Hour)
	ctx := context.Background()

	if err := mgr.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	userRepo := repository.NewUserRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	posRepo := repository.NewPositionRepository(pool)

	user := &model.User{
		Email:        "rangetest@example.com",
		PasswordHash: "$2a$10$fakehashforfasttest",
		Name:         "Range Test",
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	device := &model.Device{
		UniqueID: "range-test-" + time.Now().Format("20060102150405.000000000"),
		Name:     "Range Test Device",
		Status:   "online",
	}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	now := time.Now().UTC()
	positions := []*model.Position{
		{DeviceID: device.ID, Latitude: 52.0, Longitude: 13.0, Timestamp: now.Add(-3 * time.Hour)},
		{DeviceID: device.ID, Latitude: 52.1, Longitude: 13.1, Timestamp: now.Add(-2 * time.Hour)},
		{DeviceID: device.ID, Latitude: 52.2, Longitude: 13.2, Timestamp: now.Add(-1 * time.Hour)},
		{DeviceID: device.ID, Latitude: 52.3, Longitude: 13.3, Timestamp: now},
	}
	for _, p := range positions {
		if err := posRepo.Create(ctx, p); err != nil {
			t.Fatalf("create position: %v", err)
		}
	}

	// Query the last 2.5 hours.
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

func TestPartitionManager_DropRetentionDisabled(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	// retentionDays = 0 means disabled.
	mgr := partition.NewManager(pool, 0, 1*time.Hour)
	ctx := context.Background()

	if err := mgr.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	partitions, err := mgr.ListPartitions(ctx)
	if err != nil {
		t.Fatalf("ListPartitions failed: %v", err)
	}

	initialCount := len(partitions)

	// Run again - no partitions should be dropped since retention is disabled.
	if err := mgr.RunOnce(ctx); err != nil {
		t.Fatalf("second RunOnce failed: %v", err)
	}

	partitions, err = mgr.ListPartitions(ctx)
	if err != nil {
		t.Fatalf("ListPartitions after second run failed: %v", err)
	}

	if len(partitions) != initialCount {
		t.Errorf("partition count changed from %d to %d (retention disabled, should not drop)",
			initialCount, len(partitions))
	}
}

func TestPartitionManager_DropExpiredPartitions(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	ctx := context.Background()

	// First, ensure current/future partitions are created.
	setupMgr := partition.NewManager(pool, 0, time.Hour)
	if err := setupMgr.RunOnce(ctx); err != nil {
		t.Fatalf("setup RunOnce failed: %v", err)
	}

	// Manually create an old partition (2020-01) that should be expired.
	oldPartName := "positions_y2020m01"
	_, err := pool.Exec(ctx, `
		ALTER TABLE positions DETACH PARTITION positions_default
	`)
	if err != nil {
		t.Fatalf("detach default partition: %v", err)
	}
	_, err = pool.Exec(ctx, `
		CREATE TABLE `+oldPartName+` PARTITION OF positions
		FOR VALUES FROM ('2020-01-01') TO ('2020-02-01')
	`)
	if err != nil {
		// Re-attach and fail.
		_, _ = pool.Exec(ctx, `ALTER TABLE positions ATTACH PARTITION positions_default DEFAULT`)
		t.Fatalf("create old partition: %v", err)
	}
	_, err = pool.Exec(ctx, `ALTER TABLE positions ATTACH PARTITION positions_default DEFAULT`)
	if err != nil {
		t.Fatalf("re-attach default partition: %v", err)
	}

	// Verify old partition exists.
	partitionsBefore, err := setupMgr.ListPartitions(ctx)
	if err != nil {
		t.Fatalf("ListPartitions before: %v", err)
	}
	foundOld := false
	for _, p := range partitionsBefore {
		if p.Name == oldPartName {
			foundOld = true
			break
		}
	}
	if !foundOld {
		t.Fatalf("old partition %q was not created", oldPartName)
	}

	// Run with 1-day retention — should drop the 2020 partition.
	retentionMgr := partition.NewManager(pool, 1, time.Hour)
	if err := retentionMgr.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce with retention failed: %v", err)
	}

	// Verify old partition was dropped.
	partitionsAfter, err := retentionMgr.ListPartitions(ctx)
	if err != nil {
		t.Fatalf("ListPartitions after: %v", err)
	}
	for _, p := range partitionsAfter {
		if p.Name == oldPartName {
			t.Errorf("expected old partition %q to be dropped, but it still exists", oldPartName)
		}
	}
}

func TestPartitionManager_Start_ContextCancel(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	// Use a very long check interval so the goroutine blocks in the select loop.
	mgr := partition.NewManager(pool, 0, 24*time.Hour)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		mgr.Start(ctx)
	}()

	// Give Start time to complete its initial runMaintenance and enter the select loop.
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Good: goroutine exited after cancellation.
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not exit after context cancellation within 5 seconds")
	}
}

// TestPartitionManager_Start_ErrorPath verifies that runMaintenance logs
// errors gracefully when called with a cancelled context (DB queries fail).
// This covers the error branch in runMaintenance and the error return path
// in runMaintenanceWithError.
func TestPartitionManager_Start_ErrorPath(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	mgr := partition.NewManager(pool, 0, 24*time.Hour)

	// Cancel the context before calling Start. The initial runMaintenance(ctx)
	// inside Start will fail because DB queries reject a cancelled context.
	// This exercises the error-logging branch in runMaintenance.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		mgr.Start(ctx)
	}()

	select {
	case <-done:
		// Good: Start exited quickly after discovering the cancelled context.
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not exit after pre-cancelled context")
	}
}

// TestPartitionManager_RunOnce_WithRetention verifies the retention branch
// in runMaintenanceWithError when retentionDays > 0 and no partitions are expired.
func TestPartitionManager_RunOnce_WithRetention_NoExpired(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	// 365-day retention — nothing should be dropped (all partitions are recent).
	mgr := partition.NewManager(pool, 365, time.Hour)
	ctx := context.Background()

	if err := mgr.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce with retention failed: %v", err)
	}
}

// TestPartitionManager_CreatePartitionIfNotExists_CreationPath exercises the
// transaction path inside createPartitionIfNotExists by dropping a future
// partition and forcing RunOnce to re-create it.
func TestPartitionManager_CreatePartitionIfNotExists_CreationPath(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	mgr := partition.NewManager(pool, 0, time.Hour)
	ctx := context.Background()

	// Initial run — creates current + 3 months ahead (all via the creation path
	// only if they do not already exist; if migrations already created them the
	// idempotent path fires instead). Regardless, we need a stable baseline.
	if err := mgr.RunOnce(ctx); err != nil {
		t.Fatalf("initial RunOnce failed: %v", err)
	}

	// Pick the partition 3 months ahead so it is guaranteed to be empty
	// (no position data). Drop it so RunOnce must re-create it.
	now := time.Now().UTC()
	future := time.Date(now.Year(), now.Month()+3, 1, 0, 0, 0, 0, time.UTC)
	futureName := partition.PartitionName(future)

	if _, err := pool.Exec(ctx, "DROP TABLE IF EXISTS "+futureName); err != nil {
		t.Fatalf("drop future partition %q: %v", futureName, err)
	}

	// Confirm it no longer appears in the list.
	before, err := mgr.ListPartitions(ctx)
	if err != nil {
		t.Fatalf("ListPartitions before: %v", err)
	}
	for _, p := range before {
		if p.Name == futureName {
			t.Fatalf("partition %q should be absent after manual DROP", futureName)
		}
	}

	// RunOnce should now hit the creation path in createPartitionIfNotExists
	// (detach default → CREATE TABLE → move rows → re-attach → commit).
	if err := mgr.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce after drop failed: %v", err)
	}

	// Verify the partition was successfully re-created.
	after, err := mgr.ListPartitions(ctx)
	if err != nil {
		t.Fatalf("ListPartitions after: %v", err)
	}
	found := false
	for _, p := range after {
		if p.Name == futureName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("partition %q was not re-created by RunOnce", futureName)
	}
}

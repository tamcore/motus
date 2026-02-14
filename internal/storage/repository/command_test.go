package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func TestCommandRepository_Create(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	cmdRepo := repository.NewCommandRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := createTestUser(t, userRepo)
	device := &model.Device{UniqueID: "cmd-dev-" + time.Now().Format("150405.000"), Name: "Cmd Device", Status: "online"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("Create device failed: %v", err)
	}

	cmd := &model.Command{
		DeviceID: device.ID,
		Type:     "rebootDevice",
		Attributes: map[string]interface{}{
			"reason": "test",
		},
		Status: model.CommandStatusPending,
	}

	if err := cmdRepo.Create(ctx, cmd); err != nil {
		t.Fatalf("Create command failed: %v", err)
	}
	if cmd.ID == 0 {
		t.Error("expected command ID to be set")
	}
	if cmd.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestCommandRepository_GetPendingByDevice(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	cmdRepo := repository.NewCommandRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := createTestUser(t, userRepo)
	device := &model.Device{UniqueID: "cmdpend-" + time.Now().Format("150405.000"), Name: "Pending Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// Create 2 pending and 1 executed command.
	c1 := &model.Command{DeviceID: device.ID, Type: "rebootDevice", Status: "pending"}
	c2 := &model.Command{DeviceID: device.ID, Type: "positionSingle", Status: "pending"}
	c3 := &model.Command{DeviceID: device.ID, Type: "positionPeriodic", Status: "executed"}

	_ = cmdRepo.Create(ctx, c1)
	_ = cmdRepo.Create(ctx, c2)
	_ = cmdRepo.Create(ctx, c3)

	pending, err := cmdRepo.GetPendingByDevice(ctx, device.ID)
	if err != nil {
		t.Fatalf("GetPendingByDevice failed: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("expected 2 pending commands, got %d", len(pending))
	}
}

func TestCommandRepository_GetPendingByDevice_Empty(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	cmdRepo := repository.NewCommandRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := createTestUser(t, userRepo)
	device := &model.Device{UniqueID: "cmdnone-" + time.Now().Format("150405.000"), Name: "No Cmd Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	pending, err := cmdRepo.GetPendingByDevice(ctx, device.ID)
	if err != nil {
		t.Fatalf("GetPendingByDevice failed: %v", err)
	}
	if pending != nil {
		t.Errorf("expected nil for no pending commands, got %d", len(pending))
	}
}

func TestCommandRepository_UpdateStatus(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	cmdRepo := repository.NewCommandRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := createTestUser(t, userRepo)
	device := &model.Device{UniqueID: "cmdupd-" + time.Now().Format("150405.000"), Name: "Update Cmd Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	cmd := &model.Command{DeviceID: device.ID, Type: "rebootDevice", Status: "pending"}
	_ = cmdRepo.Create(ctx, cmd)

	if err := cmdRepo.UpdateStatus(ctx, cmd.ID, "executed"); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Verify it's no longer pending.
	pending, err := cmdRepo.GetPendingByDevice(ctx, device.ID)
	if err != nil {
		t.Fatalf("GetPendingByDevice failed: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending commands after status update, got %d", len(pending))
	}
}

func TestCommandRepository_ListByDevice(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	cmdRepo := repository.NewCommandRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := createTestUser(t, userRepo)
	device := &model.Device{UniqueID: "cmdlist-" + time.Now().Format("150405.000"), Name: "List Cmd Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// Create 3 commands.
	for i := 0; i < 3; i++ {
		cmd := &model.Command{DeviceID: device.ID, Type: "rebootDevice", Status: "pending"}
		_ = cmdRepo.Create(ctx, cmd)
	}

	cmds, err := cmdRepo.ListByDevice(ctx, device.ID, 10)
	if err != nil {
		t.Fatalf("ListByDevice failed: %v", err)
	}
	if len(cmds) != 3 {
		t.Errorf("expected 3 commands, got %d", len(cmds))
	}
}

func TestCommandRepository_ListByDevice_Empty(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	cmdRepo := repository.NewCommandRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := createTestUser(t, userRepo)
	device := &model.Device{UniqueID: "cmdlistempty-" + time.Now().Format("150405.000"), Name: "Empty List Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	cmds, err := cmdRepo.ListByDevice(ctx, device.ID, 10)
	if err != nil {
		t.Fatalf("ListByDevice failed: %v", err)
	}
	// nil or empty slice are both acceptable for no results.
	if len(cmds) != 0 {
		t.Errorf("expected 0 commands for new device, got %d", len(cmds))
	}
}

func TestCommandRepository_AppendResult(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	cmdRepo := repository.NewCommandRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := createTestUser(t, userRepo)
	device := &model.Device{UniqueID: "cmdresult-" + time.Now().Format("150405.000"), Name: "Result Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	cmd := &model.Command{DeviceID: device.ID, Type: "rebootDevice", Status: "sent"}
	_ = cmdRepo.Create(ctx, cmd)

	if err := cmdRepo.AppendResult(ctx, cmd.ID, "OK, device rebooted"); err != nil {
		t.Fatalf("AppendResult failed: %v", err)
	}
}

func TestCommandRepository_GetLatestSentByDevice(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	cmdRepo := repository.NewCommandRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := createTestUser(t, userRepo)
	device := &model.Device{UniqueID: "cmdlatest-" + time.Now().Format("150405.000"), Name: "Latest Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// Create a pending and a sent command.
	c1 := &model.Command{DeviceID: device.ID, Type: "rebootDevice", Status: "pending"}
	c2 := &model.Command{DeviceID: device.ID, Type: "positionSingle", Status: "sent"}
	_ = cmdRepo.Create(ctx, c1)
	_ = cmdRepo.Create(ctx, c2)

	latest, err := cmdRepo.GetLatestSentByDevice(ctx, device.ID)
	if err != nil {
		t.Fatalf("GetLatestSentByDevice failed: %v", err)
	}
	if latest == nil {
		t.Fatal("expected to find the sent command, got nil")
	}
	if latest.Status != "sent" {
		t.Errorf("expected status 'sent', got %q", latest.Status)
	}
}

func TestCommandRepository_GetLatestSentByDevice_None(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	cmdRepo := repository.NewCommandRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := createTestUser(t, userRepo)
	device := &model.Device{UniqueID: "cmdnone2-" + time.Now().Format("150405.000"), Name: "No Sent Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// No commands at all — should return an error (pgx.ErrNoRows).
	_, err := cmdRepo.GetLatestSentByDevice(ctx, device.ID)
	if err == nil {
		t.Error("expected error for device with no sent commands (pgx.ErrNoRows), got nil")
	}
}

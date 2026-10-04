package repository_test

import (
	"context"
	"slices"
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
	ctx := context.Background()

	user := testutil.CreateUser(t, "command-1@example.com")
	device := testutil.CreateDevice(t, user.ID, "cmd-dev-"+time.Now().Format("150405.000"))

	cmd := &model.Command{
		DeviceID: device.ID,
		Type:     "rebootDevice",
		Attributes: map[string]any{
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

func TestCommandRepository_GetPendingByUniqueIDs(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	cmdRepo := repository.NewCommandRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	ctx := context.Background()

	user := testutil.CreateUser(t, "command-2@example.com")
	a := &model.Device{UniqueID: "cmdpend-a", Name: "A", Status: "online", Protocol: "h02"}
	b := &model.Device{UniqueID: "cmdpend-b", Name: "B", Status: "online", Protocol: "watch"}
	other := &model.Device{UniqueID: "cmdpend-other", Name: "Other", Status: "online"}
	for _, d := range []*model.Device{a, b, other} {
		if err := deviceRepo.Create(ctx, d, user.ID); err != nil {
			t.Fatalf("Create device: %v", err)
		}
	}
	for _, c := range []*model.Command{
		{DeviceID: a.ID, Type: "rebootDevice", Status: "pending"},
		{DeviceID: b.ID, Type: "positionSingle", Status: "pending"},
		{DeviceID: a.ID, Type: "positionPeriodic", Status: "executed"},
		{DeviceID: other.ID, Type: "rebootDevice", Status: "pending"},
	} {
		if err := cmdRepo.Create(ctx, c); err != nil {
			t.Fatalf("Create command: %v", err)
		}
	}

	pending, err := cmdRepo.GetPendingByUniqueIDs(ctx, []string{"cmdpend-a", "cmdpend-b", "unknown"})
	if err != nil {
		t.Fatalf("GetPendingByUniqueIDs: %v", err)
	}
	got := make([]string, 0, len(pending))
	for _, pc := range pending {
		got = append(got, pc.UniqueID+"/"+pc.Protocol+"/"+pc.Command.Type)
	}
	want := []string{"cmdpend-a/h02/rebootDevice", "cmdpend-b/watch/positionSingle"}
	if !slices.Equal(got, want) {
		t.Errorf("pending = %v, want %v (oldest first, pending only, requested devices only)", got, want)
	}

	none, err := cmdRepo.GetPendingByUniqueIDs(ctx, []string{"unknown"})
	if err != nil || len(none) != 0 {
		t.Errorf("unknown device: got %d commands, err %v; want none", len(none), err)
	}
}

func TestCommandRepository_UpdateStatus(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	cmdRepo := repository.NewCommandRepository(pool)
	ctx := context.Background()

	user := testutil.CreateUser(t, "command-3@example.com")
	device := testutil.CreateDevice(t, user.ID, "cmdupd-"+time.Now().Format("150405.000"))

	cmd := &model.Command{DeviceID: device.ID, Type: "rebootDevice", Status: "pending"}
	_ = cmdRepo.Create(ctx, cmd)

	if err := cmdRepo.UpdateStatus(ctx, cmd.ID, "executed"); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Verify it's no longer pending.
	pending, err := cmdRepo.GetPendingByUniqueIDs(ctx, []string{device.UniqueID})
	if err != nil {
		t.Fatalf("GetPendingByUniqueIDs failed: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending commands after status update, got %d", len(pending))
	}
}

func TestCommandRepository_ListByDevice(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	cmdRepo := repository.NewCommandRepository(pool)
	ctx := context.Background()

	user := testutil.CreateUser(t, "command-4@example.com")
	device := testutil.CreateDevice(t, user.ID, "cmdlist-"+time.Now().Format("150405.000"))

	// Create 3 commands.
	for range 3 {
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
	ctx := context.Background()

	user := testutil.CreateUser(t, "command-5@example.com")
	device := testutil.CreateDevice(t, user.ID, "cmdlistempty-"+time.Now().Format("150405.000"))

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
	ctx := context.Background()

	user := testutil.CreateUser(t, "command-6@example.com")
	device := testutil.CreateDevice(t, user.ID, "cmdresult-"+time.Now().Format("150405.000"))

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
	ctx := context.Background()

	user := testutil.CreateUser(t, "command-7@example.com")
	device := testutil.CreateDevice(t, user.ID, "cmdlatest-"+time.Now().Format("150405.000"))

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
		return
	}
	if latest.Status != "sent" {
		t.Errorf("expected status 'sent', got %q", latest.Status)
	}
}

func TestCommandRepository_GetLatestSentByDevice_None(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	cmdRepo := repository.NewCommandRepository(pool)
	ctx := context.Background()

	user := testutil.CreateUser(t, "command-8@example.com")
	device := testutil.CreateDevice(t, user.ID, "cmdnone2-"+time.Now().Format("150405.000"))

	// No commands at all — should return an error (pgx.ErrNoRows).
	_, err := cmdRepo.GetLatestSentByDevice(ctx, device.ID)
	if err == nil {
		t.Error("expected error for device with no sent commands (pgx.ErrNoRows), got nil")
	}
}

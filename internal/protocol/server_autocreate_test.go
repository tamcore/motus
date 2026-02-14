package protocol

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
	"github.com/tamcore/motus/internal/websocket"
)

// TestDeviceAutoCreate_H02_Enabled_UnknownDevice tests that when auto-create
// is enabled, an unknown device sending a position message gets automatically
// created and the position is stored successfully.
func TestDeviceAutoCreate_H02_Enabled_UnknownDevice(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	positionRepo := repository.NewPositionRepository(pool)
	ctx := context.Background()

	// Create the default admin user for auto-created devices.
	adminUser := &model.User{
		Email:        "admin@motus.local",
		PasswordHash: "hash",
		Name:         "Admin",
		Role:         model.RoleAdmin,
	}
	if err := userRepo.Create(ctx, adminUser); err != nil {
		t.Fatalf("create admin user: %v", err)
	}

	srv := &Server{
		name:    "h02",
		devices: deviceRepo,
	}
	srv.SetAutoCreate(AutoCreateConfig{
		Enabled:          true,
		DefaultUserEmail: "admin@motus.local",
	}, userRepo)

	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })
	handler := NewPositionHandler(positionRepo, deviceRepo, hub, nil)
	srv.handler = handler

	// Send a V1 position from an unknown device.
	unknownDeviceID := "9999999999"
	raw := "*HQ,9999999999,V1,212250,A,4948.8999,N,00958.2106,E,015.50,180,110226,FFFFFBFF,262,03,49032,46083637#"

	pos, devID, resp, err := srv.decodeH02(ctx, raw)
	if err != nil {
		t.Fatalf("decodeH02 failed: %v", err)
	}

	// Position should be created successfully.
	if pos == nil {
		t.Fatal("expected non-nil position after auto-create")
	}
	if devID != unknownDeviceID {
		t.Errorf("deviceID: got %q, want %q", devID, unknownDeviceID)
	}
	if resp == "" {
		t.Error("expected non-empty response")
	}

	// Verify the device was auto-created in the database.
	device, err := deviceRepo.GetByUniqueID(ctx, unknownDeviceID)
	if err != nil {
		t.Fatalf("device not found after auto-create: %v", err)
	}

	// Device name should be set to the unique ID.
	if device.Name != unknownDeviceID {
		t.Errorf("device.Name: got %q, want %q", device.Name, unknownDeviceID)
	}

	// Protocol should be set correctly.
	if device.Protocol != "h02" {
		t.Errorf("device.Protocol: got %q, want %q", device.Protocol, "h02")
	}

	// Status should be "unknown".
	if device.Status != "unknown" {
		t.Errorf("device.Status: got %q, want %q", device.Status, "unknown")
	}

	// Device should be assigned to the admin user.
	userDevices, err := deviceRepo.GetByUser(ctx, adminUser.ID)
	if err != nil {
		t.Fatalf("get user devices: %v", err)
	}
	if len(userDevices) != 1 {
		t.Fatalf("expected 1 device for admin user, got %d", len(userDevices))
	}
	if userDevices[0].ID != device.ID {
		t.Error("device not assigned to admin user")
	}

	// Verify position references the new device.
	if pos.DeviceID != device.ID {
		t.Errorf("position.DeviceID: got %d, want %d", pos.DeviceID, device.ID)
	}

	// Verify position can be stored (HandlePosition should succeed).
	if err := handler.HandlePosition(ctx, pos); err != nil {
		t.Errorf("HandlePosition failed: %v", err)
	}

	// Verify position was stored in database.
	latestPos, err := positionRepo.GetLatestByDevice(ctx, device.ID)
	if err != nil {
		t.Fatalf("get latest position: %v", err)
	}
	if latestPos == nil {
		t.Fatal("expected position to be stored")
	}
}

// TestDeviceAutoCreate_H02_Enabled_SubsequentPositions tests that after
// auto-creating a device, subsequent positions from the same device work normally.
func TestDeviceAutoCreate_H02_Enabled_SubsequentPositions(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	positionRepo := repository.NewPositionRepository(pool)
	ctx := context.Background()

	// Create admin user.
	adminUser := &model.User{
		Email:        "admin@motus.local",
		PasswordHash: "hash",
		Name:         "Admin",
		Role:         model.RoleAdmin,
	}
	_ = userRepo.Create(ctx, adminUser)

	srv := &Server{
		name:    "h02",
		devices: deviceRepo,
	}
	srv.SetAutoCreate(AutoCreateConfig{
		Enabled:          true,
		DefaultUserEmail: "admin@motus.local",
	}, userRepo)

	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })
	handler := NewPositionHandler(positionRepo, deviceRepo, hub, nil)
	srv.handler = handler

	unknownDeviceID := "8888888888"

	// First position - should trigger auto-create.
	raw1 := "*HQ,8888888888,V1,120000,A,4948.8999,N,00958.2106,E,010.00,090,110226,FFFFFBFF,0,0,0,0#"
	pos1, _, _, err := srv.decodeH02(ctx, raw1)
	if err != nil {
		t.Fatalf("first decodeH02 failed: %v", err)
	}
	if pos1 == nil {
		t.Fatal("expected non-nil position on first decode")
	}

	device, err := deviceRepo.GetByUniqueID(ctx, unknownDeviceID)
	if err != nil {
		t.Fatalf("device not found after auto-create: %v", err)
	}

	// Store first position.
	if err := handler.HandlePosition(ctx, pos1); err != nil {
		t.Fatalf("HandlePosition for first position failed: %v", err)
	}

	// Second position from same device - should NOT trigger another auto-create.
	raw2 := "*HQ,8888888888,V1,130000,A,4948.9000,N,00958.2200,E,020.00,180,110226,FFFFFBFF,0,0,0,0#"
	pos2, _, _, err := srv.decodeH02(ctx, raw2)
	if err != nil {
		t.Fatalf("second decodeH02 failed: %v", err)
	}
	if pos2 == nil {
		t.Fatal("expected non-nil position on second decode")
	}

	// Should reference the same device.
	if pos2.DeviceID != device.ID {
		t.Errorf("second position.DeviceID: got %d, want %d", pos2.DeviceID, device.ID)
	}

	// Store second position.
	if err := handler.HandlePosition(ctx, pos2); err != nil {
		t.Fatalf("HandlePosition for second position failed: %v", err)
	}

	// Verify we still have exactly one device (no duplicate created).
	allDevices, err := deviceRepo.GetAll(ctx)
	if err != nil {
		t.Fatalf("get all devices: %v", err)
	}
	if len(allDevices) != 1 {
		t.Errorf("expected exactly 1 device, got %d", len(allDevices))
	}

	// Verify both positions were stored successfully by checking latest position.
	latestPos, err := positionRepo.GetLatestByDevice(ctx, device.ID)
	if err != nil {
		t.Fatalf("get latest position: %v", err)
	}
	if latestPos == nil {
		t.Fatal("expected latest position to exist")
	}
	// Latest position should be the second one (with latitude 4948.9000 / 49.815)
	if latestPos.Latitude < 49.814 || latestPos.Latitude > 49.816 {
		t.Errorf("latest position latitude: got %f, expected ~49.815 (second position)",
			latestPos.Latitude)
	}
}

// TestDeviceAutoCreate_H02_Disabled_UnknownDevice tests that when auto-create
// is disabled, an unknown device gets an error and no device is created.
func TestDeviceAutoCreate_H02_Disabled_UnknownDevice(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	// Create admin user (shouldn't be used).
	adminUser := &model.User{
		Email:        "admin@motus.local",
		PasswordHash: "hash",
		Name:         "Admin",
		Role:         model.RoleAdmin,
	}
	_ = userRepo.Create(ctx, adminUser)

	// Configure auto-create: DISABLED.
	srv := &Server{
		name:    "h02",
		devices: deviceRepo,
	}
	srv.SetAutoCreate(AutoCreateConfig{
		Enabled:          false,
		DefaultUserEmail: "admin@motus.local",
	}, userRepo)

	// Try to send a position from an unknown device.
	unknownDeviceID := "7777777777"
	raw := "*HQ,7777777777,V1,120000,A,4948.8999,N,00958.2106,E,010.00,090,110226,FFFFFBFF,0,0,0,0#"

	pos, devID, _, err := srv.decodeH02(ctx, raw)

	// Should return an error.
	if err == nil {
		t.Fatal("expected error for unknown device when auto-create is disabled")
	}
	if !strings.Contains(err.Error(), "unknown device") {
		t.Errorf("error should mention 'unknown device', got: %v", err)
	}

	// Position should be nil.
	if pos != nil {
		t.Error("expected nil position when device lookup fails")
	}

	// Device ID should still be returned for logging purposes.
	if devID != unknownDeviceID {
		t.Errorf("deviceID: got %q, want %q", devID, unknownDeviceID)
	}

	// Verify no device was created.
	_, err = deviceRepo.GetByUniqueID(ctx, unknownDeviceID)
	if err == nil {
		t.Error("expected device NOT to exist, but it was found")
	}

	// Verify admin user has no devices assigned.
	userDevices, err := deviceRepo.GetByUser(ctx, adminUser.ID)
	if err != nil {
		t.Fatalf("get user devices: %v", err)
	}
	if len(userDevices) != 0 {
		t.Errorf("expected 0 devices for admin user, got %d", len(userDevices))
	}
}

// TestDeviceAutoCreate_H02_DefaultUserNotFound tests that when auto-create
// is enabled but the default user doesn't exist, an error is returned and
// no device is created.
func TestDeviceAutoCreate_H02_DefaultUserNotFound(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	// Do NOT create the default user.

	srv := &Server{
		name:    "h02",
		devices: deviceRepo,
	}
	srv.SetAutoCreate(AutoCreateConfig{
		Enabled:          true,
		DefaultUserEmail: "nonexistent@example.com",
	}, userRepo)

	unknownDeviceID := "6666666666"
	raw := "*HQ,6666666666,V1,120000,A,4948.8999,N,00958.2106,E,010.00,090,110226,FFFFFBFF,0,0,0,0#"

	pos, devID, _, err := srv.decodeH02(ctx, raw)

	// Should return an error about default user not found.
	if err == nil {
		t.Fatal("expected error when default user doesn't exist")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}

	// Position should be nil.
	if pos != nil {
		t.Error("expected nil position when user lookup fails")
	}

	// Device ID should be returned.
	if devID != unknownDeviceID {
		t.Errorf("deviceID: got %q, want %q", devID, unknownDeviceID)
	}

	// Verify no device was created.
	_, err = deviceRepo.GetByUniqueID(ctx, unknownDeviceID)
	if err == nil {
		t.Error("expected device NOT to exist, but it was found")
	}
}

// TestDeviceAutoCreate_H02_ExistingDevice tests that when a device already
// exists, the existing one is used (no duplicate created).
func TestDeviceAutoCreate_H02_ExistingDevice(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	// Create admin user.
	adminUser := &model.User{
		Email:        "admin@motus.local",
		PasswordHash: "hash",
		Name:         "Admin",
		Role:         model.RoleAdmin,
	}
	_ = userRepo.Create(ctx, adminUser)

	srv := &Server{
		name:    "h02",
		devices: deviceRepo,
	}
	srv.SetAutoCreate(AutoCreateConfig{
		Enabled:          true,
		DefaultUserEmail: "admin@motus.local",
	}, userRepo)

	// Create a device manually first.
	existingDevice := &model.Device{
		UniqueID: "5555555555",
		Name:     "Existing",
		Status:   "offline",
	}
	if err := deviceRepo.Create(ctx, existingDevice, adminUser.ID); err != nil {
		t.Fatalf("create existing device: %v", err)
	}

	// Now decode a position for the existing device (auto-create enabled).
	raw := "*HQ,5555555555,V1,120000,A,4948.8999,N,00958.2106,E,010.00,090,110226,FFFFFBFF,0,0,0,0#"

	pos, devID, _, err := srv.decodeH02(ctx, raw)

	// Should use the existing device, NOT create a duplicate.
	if err != nil {
		t.Fatalf("decodeH02 should succeed with existing device: %v", err)
	}

	if pos == nil {
		t.Fatal("expected non-nil position")
	}

	if pos.DeviceID != existingDevice.ID {
		t.Errorf("position.DeviceID: got %d, want %d (existing device)",
			pos.DeviceID, existingDevice.ID)
	}

	if devID != "5555555555" {
		t.Errorf("deviceID: got %q, want %q", devID, "5555555555")
	}

	// Verify still only one device exists.
	allDevices, err := deviceRepo.GetAll(ctx)
	if err != nil {
		t.Fatalf("get all devices: %v", err)
	}
	if len(allDevices) != 1 {
		t.Errorf("expected 1 device, got %d", len(allDevices))
	}
}

// TestDeviceAutoCreate_Watch_Enabled_UnknownDevice tests auto-create for
// the WATCH protocol.
func TestDeviceAutoCreate_Watch_Enabled_UnknownDevice(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	positionRepo := repository.NewPositionRepository(pool)
	ctx := context.Background()

	// Create admin user.
	adminUser := &model.User{
		Email:        "admin@motus.local",
		PasswordHash: "hash",
		Name:         "Admin",
		Role:         model.RoleAdmin,
	}
	_ = userRepo.Create(ctx, adminUser)

	srv := &Server{
		name:    "watch",
		devices: deviceRepo,
	}
	srv.SetAutoCreate(AutoCreateConfig{
		Enabled:          true,
		DefaultUserEmail: "admin@motus.local",
	}, userRepo)

	hub := websocket.NewHub(nil, nil, func(r *http.Request) int64 { return 0 })
	handler := NewPositionHandler(positionRepo, deviceRepo, hub, nil)
	srv.handler = handler

	// Send a UD position from an unknown watch device.
	unknownDeviceID := "4444444444"
	raw := "[3G*4444444444*0078*UD,14022026,153045,A,49.814998,N,9.970177,E,15.50,270.0,0.0,8,100,460,0,9527,3661]"

	pos, devID, _, err := srv.decodeWatch(ctx, raw)
	if err != nil {
		t.Fatalf("decodeWatch failed: %v", err)
	}

	if pos == nil {
		t.Fatal("expected non-nil position after auto-create")
	}
	if devID != unknownDeviceID {
		t.Errorf("deviceID: got %q, want %q", devID, unknownDeviceID)
	}

	// Verify device was auto-created.
	device, err := deviceRepo.GetByUniqueID(ctx, unknownDeviceID)
	if err != nil {
		t.Fatalf("device not found after auto-create: %v", err)
	}

	if device.Name != unknownDeviceID {
		t.Errorf("device.Name: got %q, want %q", device.Name, unknownDeviceID)
	}

	if device.Protocol != "watch" {
		t.Errorf("device.Protocol: got %q, want %q", device.Protocol, "watch")
	}

	// Verify device is assigned to admin user.
	userDevices, err := deviceRepo.GetByUser(ctx, adminUser.ID)
	if err != nil {
		t.Fatalf("get user devices: %v", err)
	}
	if len(userDevices) != 1 {
		t.Fatalf("expected 1 device for admin user, got %d", len(userDevices))
	}

	// Verify position references the new device.
	if pos.DeviceID != device.ID {
		t.Errorf("position.DeviceID: got %d, want %d", pos.DeviceID, device.ID)
	}
}

// TestDeviceAutoCreate_Watch_Disabled_UnknownDevice tests that auto-create
// respects the disabled flag for WATCH protocol.
func TestDeviceAutoCreate_Watch_Disabled_UnknownDevice(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	// Create admin user.
	adminUser := &model.User{
		Email:        "admin@motus.local",
		PasswordHash: "hash",
		Name:         "Admin",
		Role:         model.RoleAdmin,
	}
	_ = userRepo.Create(ctx, adminUser)

	// Auto-create DISABLED.
	srv := &Server{
		name:    "watch",
		devices: deviceRepo,
	}
	srv.SetAutoCreate(AutoCreateConfig{
		Enabled:          false,
		DefaultUserEmail: "admin@motus.local",
	}, userRepo)

	unknownDeviceID := "3333333333"
	raw := "[3G*3333333333*0078*UD,14022026,153045,A,49.814998,N,9.970177,E,15.50,270.0,0.0,8,100,460,0,9527,3661]"

	pos, devID, _, err := srv.decodeWatch(ctx, raw)

	// Should return an error.
	if err == nil {
		t.Fatal("expected error for unknown device when auto-create is disabled")
	}
	if !strings.Contains(err.Error(), "unknown device") {
		t.Errorf("error should mention 'unknown device', got: %v", err)
	}

	if pos != nil {
		t.Error("expected nil position")
	}
	if devID != unknownDeviceID {
		t.Errorf("deviceID: got %q, want %q", devID, unknownDeviceID)
	}

	// Verify no device was created.
	_, err = deviceRepo.GetByUniqueID(ctx, unknownDeviceID)
	if err == nil {
		t.Error("expected device NOT to exist, but it was found")
	}
}

// TestDeviceAutoCreate_UserLookupCaching tests that the user ID lookup is
// cached to avoid repeated database queries.
func TestDeviceAutoCreate_UserLookupCaching(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	// Create admin user.
	adminUser := &model.User{
		Email:        "admin@motus.local",
		PasswordHash: "hash",
		Name:         "Admin",
		Role:         model.RoleAdmin,
	}
	_ = userRepo.Create(ctx, adminUser)

	srv := &Server{
		name:    "h02",
		devices: deviceRepo,
	}
	srv.SetAutoCreate(AutoCreateConfig{
		Enabled:          true,
		DefaultUserEmail: "admin@motus.local",
	}, userRepo)

	// First auto-create should trigger user lookup.
	raw1 := "*HQ,1111111111,V1,120000,A,4948.8999,N,00958.2106,E,010.00,090,110226,FFFFFBFF,0,0,0,0#"
	pos1, _, _, err := srv.decodeH02(ctx, raw1)
	if err != nil {
		t.Fatalf("first decodeH02 failed: %v", err)
	}
	if pos1 == nil {
		t.Fatal("expected position")
	}

	// Check that user ID is cached internally.
	if srv.defaultUserID != adminUser.ID {
		t.Errorf("cached defaultUserID: got %d, want %d", srv.defaultUserID, adminUser.ID)
	}

	// Second auto-create should use cached user ID (no additional DB query).
	raw2 := "*HQ,2222222222,V1,120000,A,4948.9000,N,00958.2200,E,020.00,180,110226,FFFFFBFF,0,0,0,0#"
	pos2, _, _, err := srv.decodeH02(ctx, raw2)
	if err != nil {
		t.Fatalf("second decodeH02 failed: %v", err)
	}
	if pos2 == nil {
		t.Fatal("expected position")
	}

	// Verify both devices were created.
	_, err = deviceRepo.GetByUniqueID(ctx, "1111111111")
	if err != nil {
		t.Fatalf("device1 not found: %v", err)
	}
	_, err = deviceRepo.GetByUniqueID(ctx, "2222222222")
	if err != nil {
		t.Fatalf("device2 not found: %v", err)
	}

	// Verify both devices are assigned to the same admin user.
	userDevices, err := deviceRepo.GetByUser(ctx, adminUser.ID)
	if err != nil {
		t.Fatalf("get user devices: %v", err)
	}
	if len(userDevices) != 2 {
		t.Errorf("expected 2 devices for admin user, got %d", len(userDevices))
	}
}

// TestDeviceAutoCreate_H02_Heartbeat_NoAutoCreate tests that heartbeat
// messages do NOT trigger auto-create (they return before device lookup).
func TestDeviceAutoCreate_H02_Heartbeat_NoAutoCreate(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	// Create admin user.
	_ = userRepo.Create(ctx, &model.User{
		Email:        "admin@motus.local",
		PasswordHash: "hash",
		Name:         "Admin",
		Role:         model.RoleAdmin,
	})

	srv := &Server{
		name:    "h02",
		devices: deviceRepo,
	}
	srv.SetAutoCreate(AutoCreateConfig{
		Enabled:          true,
		DefaultUserEmail: "admin@motus.local",
	}, userRepo)

	// Send a V4 heartbeat from an unknown device.
	unknownDeviceID := "9876543210"
	raw := "*HQ,9876543210,V4,V1,20260211212008#"

	pos, devID, _, err := srv.decodeH02(ctx, raw)

	// Heartbeat should not trigger auto-create and should not error.
	if err != nil {
		t.Errorf("decodeH02 heartbeat should not error: %v", err)
	}
	if pos != nil {
		t.Error("expected nil position for heartbeat")
	}
	if devID != unknownDeviceID {
		t.Errorf("deviceID: got %q, want %q", devID, unknownDeviceID)
	}

	// Verify no device was created.
	_, err = deviceRepo.GetByUniqueID(ctx, unknownDeviceID)
	if err == nil {
		t.Error("expected device NOT to exist after heartbeat")
	}
}

// TestDeviceAutoCreate_Watch_Heartbeat_NoAutoCreate tests that heartbeat
// messages for WATCH protocol do NOT trigger auto-create.
func TestDeviceAutoCreate_Watch_Heartbeat_NoAutoCreate(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	// Create admin user.
	_ = userRepo.Create(ctx, &model.User{
		Email:        "admin@motus.local",
		PasswordHash: "hash",
		Name:         "Admin",
		Role:         model.RoleAdmin,
	})

	srv := &Server{
		name:    "watch",
		devices: deviceRepo,
	}
	srv.SetAutoCreate(AutoCreateConfig{
		Enabled:          true,
		DefaultUserEmail: "admin@motus.local",
	}, userRepo)

	// Send a LK heartbeat from an unknown device.
	unknownDeviceID := "9998887777"
	raw := "[3G*9998887777*0005*LK,85]"

	pos, devID, resp, err := srv.decodeWatch(ctx, raw)

	// Heartbeat should not trigger auto-create.
	if err != nil {
		t.Errorf("decodeWatch heartbeat should not error: %v", err)
	}
	if pos != nil {
		t.Error("expected nil position for heartbeat")
	}
	if devID != unknownDeviceID {
		t.Errorf("deviceID: got %q, want %q", devID, unknownDeviceID)
	}
	if resp == "" {
		t.Error("expected heartbeat response")
	}

	// Verify no device was created.
	_, err = deviceRepo.GetByUniqueID(ctx, unknownDeviceID)
	if err == nil {
		t.Error("expected device NOT to exist after heartbeat")
	}
}

// TestDeviceAutoCreate_NilUserRepo tests that auto-create with nil user repo
// returns a clear error.
func TestDeviceAutoCreate_NilUserRepo(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	ctx := context.Background()

	srv := &Server{
		name:    "h02",
		devices: deviceRepo,
		autoCreate: AutoCreateConfig{
			Enabled:          true,
			DefaultUserEmail: "admin@motus.local",
		},
		// users is nil -- not set via SetAutoCreate.
	}

	raw := "*HQ,1234567890,V1,120000,A,4948.8999,N,00958.2106,E,010.00,090,110226,FFFFFBFF,0,0,0,0#"
	pos, _, _, err := srv.decodeH02(ctx, raw)

	if err == nil {
		t.Fatal("expected error with nil user repo")
	}
	if !strings.Contains(err.Error(), "user repository not configured") {
		t.Errorf("error should mention 'user repository not configured', got: %v", err)
	}
	if pos != nil {
		t.Error("expected nil position")
	}
}

// TestDeviceAutoCreate_DefaultDisabled tests that without calling
// SetAutoCreate, the server behaves as before (rejects unknown devices).
func TestDeviceAutoCreate_DefaultDisabled(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	ctx := context.Background()

	// Standard server without auto-create configured.
	srv := &Server{
		name:    "h02",
		devices: deviceRepo,
	}

	raw := "*HQ,0000099999,V1,120000,A,4948.8999,N,00958.2106,E,010.00,090,110226,FFFFFBFF,0,0,0,0#"
	pos, devID, _, err := srv.decodeH02(ctx, raw)

	if err == nil {
		t.Fatal("expected error for unknown device")
	}
	if !strings.Contains(err.Error(), "unknown device") {
		t.Errorf("error should mention 'unknown device', got: %v", err)
	}
	if pos != nil {
		t.Error("expected nil position")
	}
	if devID != "0000099999" {
		t.Errorf("deviceID: got %q, want %q", devID, "0000099999")
	}
}

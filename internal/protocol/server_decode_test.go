package protocol

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func TestDecodeH02_FullDecode(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	// Create a user and device to satisfy the lookup.
	user := &model.User{Email: "h02full@example.com", PasswordHash: "hash", Name: "H02 Full"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	device := &model.Device{UniqueID: "123456789012345", Name: "Test H02 Device", Status: "offline"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	srv := &Server{
		name:    "h02",
		devices: deviceRepo,
	}

	// V1 position message.
	raw := "*HQ,123456789012345,V1,212250,A,4948.8999,N,00958.2106,E,000.00,000,110226,FFFFFBFF,262,03,49032,46083637#"
	pos, devID, resp, err := srv.decodeH02(ctx, raw)
	if err != nil {
		t.Fatalf("decodeH02 error: %v", err)
	}

	if pos == nil {
		t.Fatal("expected non-nil position for V1 message")
	}
	if pos.DeviceID != device.ID {
		t.Errorf("DeviceID: got %d, want %d", pos.DeviceID, device.ID)
	}
	if pos.Protocol != "h02" {
		t.Errorf("Protocol: got %q, want %q", pos.Protocol, "h02")
	}
	if !pos.Valid {
		t.Error("expected valid position")
	}
	if pos.Latitude == 0 || pos.Longitude == 0 {
		t.Error("expected non-zero coordinates")
	}
	if pos.Speed == nil {
		t.Error("expected non-nil speed")
	}
	if pos.Course == nil {
		t.Error("expected non-nil course")
	}
	if pos.Altitude == nil {
		t.Error("expected non-nil altitude (H02 always sets 0)")
	}
	if pos.ServerTime == nil {
		t.Error("expected non-nil server time")
	}
	if pos.DeviceTime == nil {
		t.Error("expected non-nil device time")
	}
	if pos.Attributes == nil {
		t.Error("expected non-nil attributes")
	}
	if pos.Attributes["flags"] != "FFFFFBFF" {
		t.Errorf("flags: got %v, want FFFFFBFF", pos.Attributes["flags"])
	}
	// Cell tower info should be present.
	if pos.Attributes["mcc"] != 262 {
		t.Errorf("mcc: got %v, want 262", pos.Attributes["mcc"])
	}

	if devID != "123456789012345" {
		t.Errorf("deviceID: got %q, want %q", devID, "123456789012345")
	}
	if resp == "" {
		t.Error("expected non-empty response for V1")
	}
	if !strings.HasPrefix(resp, "*HQ,123456789012345,V4,V1,") {
		t.Errorf("response prefix: got %q", resp)
	}
}

func TestDecodeH02_V6WithICCID(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "h02v6@example.com", PasswordHash: "hash", Name: "H02 V6"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "123456789012345", Name: "V6 Device", Status: "offline"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	srv := &Server{
		name:    "h02",
		devices: deviceRepo,
	}

	raw := "*HQ,123456789012345,V6,211755,A,4948.8999,N,00958.2106,E,000.00,000,110226,FFFFFBFF,262,03,49032,46083637,8949227221106570251F#"
	pos, devID, _, err := srv.decodeH02(ctx, raw)
	if err != nil {
		t.Fatalf("decodeH02 V6 error: %v", err)
	}
	if pos == nil {
		t.Fatal("expected non-nil position for V6 message")
	}
	if pos.Attributes["iccid"] != "8949227221106570251F" {
		t.Errorf("iccid: got %v, want 8949227221106570251F", pos.Attributes["iccid"])
	}
	if devID != "123456789012345" {
		t.Errorf("deviceID: got %q", devID)
	}
}

func TestDecodeH02_UnknownDevice(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	srv := &Server{
		name:    "h02",
		devices: deviceRepo,
	}

	// V1 message for a device that doesn't exist.
	raw := "*HQ,0000000000,V1,212250,A,4948.8999,N,00958.2106,E,000.00,000,110226,FFFFFBFF,0,0,0,0#"
	pos, devID, _, err := srv.decodeH02(context.Background(), raw)
	if err == nil {
		t.Fatal("expected error for unknown device")
	}
	if pos != nil {
		t.Error("expected nil position for unknown device")
	}
	if devID != "0000000000" {
		t.Errorf("deviceID: got %q, want %q", devID, "0000000000")
	}
}

func TestDecodeWatch_FullDecode(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "watchfull@example.com", PasswordHash: "hash", Name: "Watch Full"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "1234567890", Name: "Watch Device", Status: "offline"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	srv := &Server{
		name:    "watch",
		devices: deviceRepo,
	}

	raw := "[3G*1234567890*0078*UD,14022026,153045,A,49.814998,N,9.970177,E,15.50,270.0,0.0,8,100,460,0,9527,3661]"
	pos, devID, resp, err := srv.decodeWatch(ctx, raw)
	if err != nil {
		t.Fatalf("decodeWatch error: %v", err)
	}

	if pos == nil {
		t.Fatal("expected non-nil position for UD message")
	}
	if pos.DeviceID != device.ID {
		t.Errorf("DeviceID: got %d, want %d", pos.DeviceID, device.ID)
	}
	if pos.Protocol != "watch" {
		t.Errorf("Protocol: got %q, want %q", pos.Protocol, "watch")
	}
	if !pos.Valid {
		t.Error("expected valid position")
	}
	if pos.Speed == nil || *pos.Speed != 15.50 {
		t.Errorf("Speed: got %v, want 15.50", pos.Speed)
	}
	if pos.Course == nil || *pos.Course != 270.0 {
		t.Errorf("Course: got %v, want 270.0", pos.Course)
	}
	if pos.Attributes == nil || pos.Attributes["satellites"] != 8 {
		t.Errorf("satellites: got %v, want 8", pos.Attributes["satellites"])
	}
	if pos.ServerTime == nil {
		t.Error("expected non-nil server time")
	}
	if pos.DeviceTime == nil {
		t.Error("expected non-nil device time")
	}

	if devID != "1234567890" {
		t.Errorf("deviceID: got %q, want %q", devID, "1234567890")
	}
	// UD messages don't produce a response.
	if resp != "" {
		t.Errorf("expected empty response for UD, got %q", resp)
	}
}

func TestDecodeWatch_Heartbeat_FromServer(t *testing.T) {
	srv := &Server{
		name:    "watch",
		devices: nil,
	}

	raw := "[3G*1234567890*0005*LK,85]"
	pos, devID, resp, err := srv.decodeWatch(context.Background(), raw)
	if err != nil {
		t.Fatalf("decodeWatch heartbeat error: %v", err)
	}

	if pos != nil {
		t.Error("expected nil position for heartbeat")
	}
	if devID != "1234567890" {
		t.Errorf("deviceID: got %q, want %q", devID, "1234567890")
	}
	if resp == "" {
		t.Error("expected non-empty response for LK heartbeat")
	}
	if !strings.Contains(resp, "LK") {
		t.Errorf("response should contain LK: %q", resp)
	}
}

func TestDecodeWatch_UnknownDevice(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	srv := &Server{
		name:    "watch",
		devices: deviceRepo,
	}

	raw := "[3G*0000000000*0078*UD,14022026,153045,A,49.814998,N,9.970177,E,15.50,270.0,0.0,8,100,460,0,9527,3661]"
	pos, devID, _, err := srv.decodeWatch(context.Background(), raw)
	if err == nil {
		t.Fatal("expected error for unknown device")
	}
	if pos != nil {
		t.Error("expected nil position for unknown device")
	}
	if devID != "0000000000" {
		t.Errorf("deviceID: got %q, want %q", devID, "0000000000")
	}
}

func TestDecodeWatch_InvalidPosition(t *testing.T) {
	srv := &Server{
		name:    "watch",
		devices: nil,
	}

	// V (invalid) GPS fix -- msg.Valid is false so decodeWatch returns nil position.
	raw := "[3G*5555555555*0078*UD,14022026,120000,V,0.000000,N,0.000000,E,0.00,0.0,0.0,0,100,460,0,0,0]"
	pos, devID, _, err := srv.decodeWatch(context.Background(), raw)
	if err != nil {
		t.Fatalf("decodeWatch error: %v", err)
	}
	// msg.Valid is false, so decodeWatch should return nil position.
	if pos != nil {
		t.Error("expected nil position for invalid GPS fix")
	}
	if devID != "5555555555" {
		t.Errorf("deviceID: got %q, want %q", devID, "5555555555")
	}
}

func TestMarkDeviceOffline_WithRepo(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "offline@example.com", PasswordHash: "hash", Name: "Offline Test"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "offline-dev", Name: "Offline Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	srv := &Server{
		name:    "test",
		devices: deviceRepo,
	}

	srv.markDeviceOffline(ctx, "offline-dev")

	// Device should now be offline.
	updated, err := deviceRepo.GetByID(ctx, device.ID)
	if err != nil {
		t.Fatalf("get device: %v", err)
	}
	if updated.Status != "offline" {
		t.Errorf("expected status 'offline', got %q", updated.Status)
	}
	if updated.LastUpdate == nil {
		t.Error("expected last_update to be set")
	}
}

func TestMarkDeviceOffline_UnknownDevice(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	srv := &Server{
		name:    "test",
		devices: deviceRepo,
	}

	// Should not panic for non-existent device.
	srv.markDeviceOffline(context.Background(), "nonexistent-device")
}

func TestServer_ConcurrentConnections(t *testing.T) {
	// Multiple clients connecting simultaneously.
	decodeCalls := make(chan string, 100)

	srv := &Server{
		name:    "test",
		port:    "0",
		handler: &PositionHandler{},
		decoder: func(_ context.Context, line string) (*model.Position, string, string, error) {
			decodeCalls <- line
			return nil, "dev", "", nil
		},
	}

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen error: %v", err)
	}
	srv.listener = listener
	port := listener.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.acceptLoop(ctx)

	const numClients = 10
	var wg sync.WaitGroup
	wg.Add(numClients)

	for i := 0; i < numClients; i++ {
		go func(id int) {
			defer wg.Done()
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
			if err != nil {
				t.Errorf("client %d dial error: %v", id, err)
				return
			}
			defer func() { _ = conn.Close() }()

			msg := fmt.Sprintf("client-%d-msg\r\n", id)
			if _, err := fmt.Fprint(conn, msg); err != nil {
				t.Errorf("client %d write error: %v", id, err)
				return
			}
			// Small delay so the server has time to process.
			time.Sleep(100 * time.Millisecond)
		}(i)
	}

	wg.Wait()

	// Cancel the context and wait for all server goroutines to finish
	// before closing the channel to avoid a send-on-closed-channel race.
	cancel()
	srv.activeConns.Wait()

	close(decodeCalls)
	var count int
	for range decodeCalls {
		count++
	}

	if count != numClients {
		t.Errorf("expected %d decoded messages, got %d", numClients, count)
	}
}

func TestServer_MalformedMessages(t *testing.T) {
	decodeCalls := make(chan string, 100)
	var decodeErrors atomic.Int32

	srv := &Server{
		name:    "test",
		port:    "0",
		handler: &PositionHandler{},
		decoder: func(_ context.Context, line string) (*model.Position, string, string, error) {
			decodeCalls <- line
			if strings.HasPrefix(line, "BAD") {
				decodeErrors.Add(1)
				return nil, "", "", fmt.Errorf("malformed: %s", line)
			}
			return nil, "dev", "", nil
		},
	}

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen error: %v", err)
	}
	srv.listener = listener
	port := listener.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.acceptLoop(ctx)

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}

	// Send mix of valid and malformed messages.
	messages := []string{
		"GOOD-1\r\n",
		"BAD-1\r\n",
		"GOOD-2\r\n",
		"BAD-2\r\n",
		"GOOD-3\r\n",
	}
	for _, msg := range messages {
		_, _ = fmt.Fprint(conn, msg)
	}

	time.Sleep(300 * time.Millisecond)
	_ = conn.Close()

	// Cancel the context and wait for all server goroutines to finish
	// before closing the channel to avoid a send-on-closed-channel race.
	cancel()
	srv.activeConns.Wait()

	// All 5 messages should have been decoded (errors don't stop processing).
	close(decodeCalls)
	var totalCalls int
	for range decodeCalls {
		totalCalls++
	}

	if totalCalls != 5 {
		t.Errorf("expected 5 decode calls, got %d", totalCalls)
	}
	if decodeErrors.Load() != 2 {
		t.Errorf("expected 2 decode errors, got %d", decodeErrors.Load())
	}
}

func TestServer_ConnectionTimeout(t *testing.T) {
	// A client that connects but sends nothing should eventually be cleaned up
	// when the context is cancelled.
	srv := &Server{
		name:    "test",
		port:    "0",
		handler: &PositionHandler{},
		decoder: func(_ context.Context, line string) (*model.Position, string, string, error) {
			return nil, "dev", "", nil
		},
	}

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen error: %v", err)
	}
	srv.listener = listener
	port := listener.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.acceptLoop(ctx)

	// Connect but don't send anything.
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Verify the connection count is tracked.
	if srv.connCount.Load() != 1 {
		t.Errorf("expected 1 active connection, got %d", srv.connCount.Load())
	}

	// Cancel the context and close the idle connection.
	cancel()
	_ = conn.Close()
	time.Sleep(200 * time.Millisecond)

	if srv.connCount.Load() != 0 {
		t.Errorf("expected 0 active connections after cancel, got %d", srv.connCount.Load())
	}
}

func TestServer_BufferOverflow(t *testing.T) {
	// Messages larger than the 4096 buffer should be handled gracefully.
	srv := &Server{
		name:    "test",
		port:    "0",
		handler: &PositionHandler{},
		decoder: func(_ context.Context, line string) (*model.Position, string, string, error) {
			return nil, "dev", "", nil
		},
	}

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen error: %v", err)
	}
	srv.listener = listener
	port := listener.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.acceptLoop(ctx)

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}

	// Send a message that exceeds the 4096 buffer limit.
	// bufio.Scanner with 4096 buffer should cause a scanner error for lines > 4096.
	bigLine := strings.Repeat("A", 5000) + "\r\n"
	_, _ = fmt.Fprint(conn, bigLine)

	// After the big line, the scanner should have errored and the connection should close.
	time.Sleep(200 * time.Millisecond)

	_ = conn.Close()
	cancel()
	// The key thing is the server doesn't crash.
}

func TestServer_GracefulShutdown(t *testing.T) {
	decoded := make(chan struct{}, 100)

	srv := &Server{
		name:    "test",
		port:    "0",
		handler: &PositionHandler{},
		decoder: func(_ context.Context, line string) (*model.Position, string, string, error) {
			decoded <- struct{}{}
			return nil, "dev", "", nil
		},
	}

	// Listen on a random port.
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen error: %v", err)
	}
	srv.listener = listener
	port := listener.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithCancel(context.Background())

	shutdownDone := make(chan struct{})
	go func() {
		// Simulate the shutdown flow from Start().
		go srv.acceptLoop(ctx)
		<-ctx.Done()
		_ = srv.listener.Close()

		done := make(chan struct{})
		go func() {
			srv.activeConns.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
		close(shutdownDone)
	}()

	// Connect a client.
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}

	_, _ = fmt.Fprint(conn, "test-msg\r\n")
	select {
	case <-decoded:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for decode")
	}

	// Cancel the context (initiate shutdown).
	cancel()

	// Close the connection (simulating client disconnect).
	_ = conn.Close()

	// Wait for shutdown to complete.
	select {
	case <-shutdownDone:
		// Shutdown completed.
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not complete in time")
	}
}

func TestDecodeH02_NoCellTowerInfo(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "h02nocell@example.com", PasswordHash: "hash", Name: "No Cell"}
	_ = userRepo.Create(ctx, user)

	device := &model.Device{UniqueID: "1111111111", Name: "No Cell Device", Status: "offline"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	srv := &Server{
		name:    "h02",
		devices: deviceRepo,
	}

	// V1 message without cell tower info (only 12 fields).
	raw := "*HQ,1111111111,V1,120000,A,4948.8999,N,00958.2106,E,010.50,180,150226,FFFFFFFF#"
	pos, _, _, err := srv.decodeH02(ctx, raw)
	if err != nil {
		t.Fatalf("decodeH02 error: %v", err)
	}
	if pos == nil {
		t.Fatal("expected non-nil position")
	}
	// MCC should not be in attributes (no cell tower data).
	if _, ok := pos.Attributes["mcc"]; ok {
		t.Error("expected no mcc attribute when cell tower info is absent")
	}
}

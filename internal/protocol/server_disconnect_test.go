package protocol

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
)

// TestServer_HandleConnection_NoPanicOnConcurrentCommandDuringDisconnect verifies
// that concurrent registry.Send calls during connection teardown never cause a
// send-on-closed-channel panic.
func TestServer_HandleConnection_NoPanicOnConcurrentCommandDuringDisconnect(t *testing.T) {
	const deviceID = "TESTDEVICE001"

	registry := NewDeviceRegistry()

	srv := &Server{
		name:     "test",
		port:     "0",
		registry: registry,
		decoder: func(_ context.Context, _ string) (*model.Position, string, string, error) {
			return nil, deviceID, "", nil
		},
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv.listener = listener
	port := listener.Addr().(*net.TCPAddr).Port

	ctx := t.Context()

	go srv.acceptLoop(ctx)

	// Connect and send one line so handleConnection learns the device ID and registers outCh.
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := fmt.Fprintf(conn, "hello\r\n"); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Wait until the device is registered.
	deadline := time.Now().Add(2 * time.Second)
	for !registry.IsOnline(deviceID) {
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for device registration")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Spawn senders that hammer Send() while we disconnect.
	const senders = 20
	var wg sync.WaitGroup
	stop := make(chan struct{})

	for range senders {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
					registry.Send(deviceID, []byte("cmd"))
				}
			}
		})
	}

	// Disconnect — triggers handleConnection teardown (Deregister + close(outCh) on old code).
	_ = conn.Close()

	// Give teardown time to complete.
	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()
}

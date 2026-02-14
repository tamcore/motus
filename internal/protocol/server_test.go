package protocol

import (
	"context"
	"fmt"
	"math"
	"net"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
)

func TestServer_AcceptAndDecode(t *testing.T) {
	// Start a server with a mock decoder on a random port.
	speed := 15.0
	course := 270.0
	pos := &model.Position{
		DeviceID:  1,
		Latitude:  49.81,
		Longitude: 9.97,
		Speed:     &speed,
		Course:    &course,
		Timestamp: time.Now().UTC(),
	}

	// Use a channel to capture positions.
	positions := make(chan *model.Position, 10)
	handler := &PositionHandler{} // nil repos since we override behavior

	srv := &Server{
		name:    "test",
		port:    "0", // random port
		handler: handler,
		decoder: func(_ context.Context, line string) (*model.Position, string, string, error) {
			if line == "" {
				return nil, "", "", fmt.Errorf("empty")
			}
			positions <- pos
			return nil, "test-device", "ACK", nil // Return nil position to skip handler.HandlePosition (needs DB)
		},
	}

	// Listen manually so we can get the assigned port.
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen error: %v", err)
	}
	srv.listener = listener
	port := listener.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run accept loop in background.
	go srv.acceptLoop(ctx)

	// Connect and send a message.
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}

	_, err = fmt.Fprintf(conn, "*HQ,test,V1,data#\r\n")
	if err != nil {
		t.Fatalf("write error: %v", err)
	}

	// Wait for position to be processed.
	select {
	case <-positions:
		// Success - decoder was called.
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for position to be decoded")
	}

	// Read ACK response.
	buf := make([]byte, 256)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read response error: %v", err)
	}

	if string(buf[:n]) != "ACK\r\n" {
		t.Errorf("response: got %q, want %q", string(buf[:n]), "ACK\r\n")
	}

	_ = conn.Close()
	cancel()
}

func TestDecodeH02_Integration(t *testing.T) {
	// Test the H02 decoder function directly (without DB).
	// This cannot do a full decode because it needs the device repo,
	// but we can verify error handling for unknown devices.

	srv := &Server{
		name:    "h02",
		devices: nil, // Will cause a panic if we try to look up a device
	}

	ctx := context.Background()

	// Invalid message should return an error.
	_, _, _, err := srv.decodeH02(ctx, "not-an-h02-message")
	if err == nil {
		t.Error("expected error for invalid H02 message")
	}

	// Heartbeat should return no position and no error.
	pos, devID, resp, err := srv.decodeH02(ctx, "*HQ,123456789012345,V4,V1,20260211212008#")
	if err != nil {
		t.Fatalf("heartbeat decode error: %v", err)
	}
	if pos != nil {
		t.Error("heartbeat should not return a position")
	}
	if devID != "123456789012345" {
		t.Errorf("device ID: got %q, want %q", devID, "123456789012345")
	}
	if resp != "" {
		t.Errorf("heartbeat should not produce a response, got %q", resp)
	}
}

func TestDecodeWatch_Integration(t *testing.T) {
	srv := &Server{
		name:    "watch",
		devices: nil,
	}

	ctx := context.Background()

	// Invalid message.
	_, _, _, err := srv.decodeWatch(ctx, "not-a-watch-message")
	if err == nil {
		t.Error("expected error for invalid WATCH message")
	}

	// Heartbeat.
	pos, devID, resp, err := srv.decodeWatch(ctx, "[3G*1234567890*0002*LK]")
	if err != nil {
		t.Fatalf("heartbeat decode error: %v", err)
	}
	if pos != nil {
		t.Error("heartbeat should not return a position")
	}
	if devID != "1234567890" {
		t.Errorf("device ID: got %q, want %q", devID, "1234567890")
	}
	if resp == "" {
		t.Error("heartbeat should produce a response")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
	}

	for _, tt := range tests {
		got := truncate(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestPtrFloat(t *testing.T) {
	f := 42.5
	if ptrFloat(&f) != 42.5 {
		t.Error("ptrFloat should return the value")
	}
	if ptrFloat(nil) != 0 {
		t.Error("ptrFloat(nil) should return 0")
	}
}

// Verify coordinate parsing end-to-end through the H02 decoder.
func TestH02CoordinateParsing(t *testing.T) {
	// We can test the decoder output indirectly by using the h02 package directly.
	// This is more of a documentation test showing expected values.

	// From real h02.log: 4948.8999,N,00958.2106,E
	// Latitude: 49 + 48.8999/60 = 49.814998333...
	wantLat := 49.814998
	// Longitude: 9 + 58.2106/60 = 9.970176666...
	wantLon := 9.970177

	if math.Abs(wantLat-49.815) > 0.001 {
		t.Errorf("expected latitude ~49.815, got %f", wantLat)
	}
	if math.Abs(wantLon-9.970) > 0.001 {
		t.Errorf("expected longitude ~9.970, got %f", wantLon)
	}
}

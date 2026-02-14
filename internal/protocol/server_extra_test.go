package protocol

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
)

func TestNewH02Server(t *testing.T) {
	srv := NewH02Server("5013", nil, nil)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	if srv.name != "h02" {
		t.Errorf("expected name 'h02', got %q", srv.name)
	}
	if srv.port != "5013" {
		t.Errorf("expected port '5013', got %q", srv.port)
	}
	if srv.decoder == nil {
		t.Error("expected decoder to be set")
	}
}

func TestNewWatchServer(t *testing.T) {
	srv := NewWatchServer("5093", nil, nil)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	if srv.name != "watch" {
		t.Errorf("expected name 'watch', got %q", srv.name)
	}
	if srv.port != "5093" {
		t.Errorf("expected port '5093', got %q", srv.port)
	}
	if srv.decoder == nil {
		t.Error("expected decoder to be set")
	}
}

func TestServer_HandleConnection_DecodeError(t *testing.T) {
	// Test that decode errors are handled gracefully (logged, not fatal).
	decoded := make(chan string, 10)

	srv := &Server{
		name:    "test",
		port:    "0",
		handler: &PositionHandler{},
		decoder: func(_ context.Context, line string) (*model.Position, string, string, error) {
			decoded <- line
			return nil, "", "", fmt.Errorf("decode error for: %s", line)
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

	// Send a line that will cause a decode error.
	_, _ = fmt.Fprintf(conn, "bad-data\r\n")

	select {
	case line := <-decoded:
		if line != "bad-data" {
			t.Errorf("expected 'bad-data', got %q", line)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for decode")
	}

	_ = conn.Close()
	cancel()
}

func TestServer_HandleConnection_HeartbeatNoPosition(t *testing.T) {
	// Test that heartbeat messages (nil position) don't trigger HandlePosition.
	decoded := make(chan bool, 10)

	srv := &Server{
		name:    "test",
		port:    "0",
		handler: &PositionHandler{},
		decoder: func(_ context.Context, line string) (*model.Position, string, string, error) {
			decoded <- true
			return nil, "dev123", "ACK", nil // nil position = heartbeat
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

	_, _ = fmt.Fprintf(conn, "heartbeat\r\n")

	select {
	case <-decoded:
		// Good, decoder was called.
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for decode")
	}

	// Read the ACK response.
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

func TestServer_HandleConnection_EmptyLinesSkipped(t *testing.T) {
	decodeCalls := make(chan string, 10)

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

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}

	// Send empty line then real data.
	_, _ = fmt.Fprintf(conn, "\r\n")
	_, _ = fmt.Fprintf(conn, "real-data\r\n")

	select {
	case line := <-decodeCalls:
		if line != "real-data" {
			t.Errorf("expected 'real-data', got %q", line)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for decode")
	}

	_ = conn.Close()
	cancel()
}

func TestServer_MarkDeviceOffline_NilDevices(t *testing.T) {
	srv := &Server{
		name:    "test",
		devices: nil,
	}

	// Should not panic with nil devices repo.
	srv.markDeviceOffline(context.Background(), "test-device")
}

func TestServer_Start_InvalidPort(t *testing.T) {
	// Use an already-bound port to cause listen failure.
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen error: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	defer func() { _ = listener.Close() }()

	srv := &Server{
		name:    "test",
		port:    fmt.Sprintf("%d", port),
		handler: &PositionHandler{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = srv.Start(ctx)
	if err == nil {
		t.Error("expected error when port is already bound")
	}
}

func TestTruncate_AdditionalCases(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"exactly3", 8, "exactly3"},
		{"long string here", 4, "long..."},
		{"a", 1, "a"},
		{"ab", 1, "a..."},
	}

	for _, tt := range tests {
		got := truncate(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestPtrFloat_Values(t *testing.T) {
	zero := 0.0
	negative := -42.5
	large := 999999.99

	tests := []struct {
		name string
		f    *float64
		want float64
	}{
		{"nil", nil, 0},
		{"zero", &zero, 0},
		{"negative", &negative, -42.5},
		{"large", &large, 999999.99},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ptrFloat(tt.f)
			if got != tt.want {
				t.Errorf("ptrFloat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestServer_SetLogger(t *testing.T) {
	srv := NewH02Server("5014", nil, nil)
	srv.SetLogger(nil) // nil should not change the logger
	custom := slog.Default()
	srv.SetLogger(custom)
	if srv.logger != custom {
		t.Error("expected logger to be set")
	}
}

func TestServer_SetRegistry(t *testing.T) {
	srv := NewH02Server("5015", nil, nil)
	reg := NewDeviceRegistry()
	srv.SetRegistry(reg)
	if srv.registry != reg {
		t.Error("expected registry to be set")
	}
}

func TestServer_SetCommandRepo(t *testing.T) {
	srv := NewH02Server("5016", nil, nil)
	srv.SetCommandRepo(nil) // nil is valid
	if srv.commands != nil {
		t.Error("expected commands to be nil")
	}
}

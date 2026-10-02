package protocol

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
)

func TestServer_ShutdownClosesDeviceConnections(t *testing.T) {
	srv := &Server{
		name:    "test",
		handler: &PositionHandler{},
		decoder: func(context.Context, string) (*model.Position, string, string, error) {
			return nil, "dev-1", "ACK", nil
		},
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv.listener = listener
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go srv.acceptLoop(ctx)

	conn, err := net.DialTimeout("tcp", listener.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := fmt.Fprint(conn, "hello\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 64)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Read(buf); err != nil {
		t.Fatalf("read ACK: %v", err)
	}

	cancel()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Read(buf); !errors.Is(err, io.EOF) {
		t.Fatalf("after shutdown: read err = %v, want io.EOF (server closed the connection)", err)
	}
}

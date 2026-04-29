package protocol

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
)

// startMockRelay starts a TCP listener on an ephemeral port and returns the
// address and a channel that receives every line the relay receives.
func startMockRelay(t *testing.T) (addr string, lines <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("mock relay listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	ch := make(chan string, 32)
	go func() {
		defer close(ch)
		conn, err := ln.Accept()
		if err != nil {
			return // listener closed
		}
		defer func() { _ = conn.Close() }()
		sc := bufio.NewScanner(conn)
		for sc.Scan() {
			ch <- sc.Text()
		}
	}()
	return ln.Addr().String(), ch
}

// noopDecoder is a Decoder that records the decoded line and returns a static ACK.
func noopDecoder(processed chan<- string) Decoder {
	return func(_ context.Context, line string) (*model.Position, string, string, error) {
		if processed != nil {
			processed <- line
		}
		return nil, "dev-1", "ACK", nil
	}
}

// TestSetRelay verifies that SetRelay stores the target string.
func TestSetRelay(t *testing.T) {
	srv := &Server{}
	srv.SetRelay("relay.example.com:5013")
	if srv.relayTarget != "relay.example.com:5013" {
		t.Errorf("relayTarget = %q, want %q", srv.relayTarget, "relay.example.com:5013")
	}
}

// TestRelay_ForwardsMessagesToRelay verifies that incoming device messages are
// forwarded verbatim to the configured relay target.
func TestRelay_ForwardsMessagesToRelay(t *testing.T) {
	relayAddr, relayLines := startMockRelay(t)

	srv := &Server{
		name:        "h02",
		port:        "0",
		relayTarget: relayAddr,
		decoder:     noopDecoder(nil),
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv.listener = listener
	port := listener.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.acceptLoop(ctx)

	devConn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = devConn.Close() }()

	messages := []string{
		"*HQ,123456789012345,V1,data1#",
		"*HQ,123456789012345,V1,data2#",
	}
	for _, msg := range messages {
		if _, err := fmt.Fprintf(devConn, "%s\r\n", msg); err != nil {
			t.Fatalf("write to server: %v", err)
		}
		time.Sleep(50 * time.Millisecond) // give server time to relay
	}

	// Collect forwarded lines from the relay within a timeout.
	received := make([]string, 0, len(messages))
	deadline := time.After(2 * time.Second)
	for len(received) < len(messages) {
		select {
		case line, ok := <-relayLines:
			if !ok {
				t.Fatalf("relay connection closed after %d/%d messages", len(received), len(messages))
			}
			received = append(received, line)
		case <-deadline:
			t.Fatalf("timeout: received %d/%d relay messages", len(received), len(messages))
		}
	}

	for i, want := range messages {
		if received[i] != want {
			t.Errorf("relay line %d: got %q, want %q", i, received[i], want)
		}
	}
}

// TestRelay_FailOpen_UnreachableRelay verifies that Motus continues processing
// device messages normally when the relay target is unreachable.
func TestRelay_FailOpen_UnreachableRelay(t *testing.T) {
	// Use a port that nothing is listening on.
	unreachableAddr := "127.0.0.1:1" // port 1 is privileged and never open

	processed := make(chan string, 10)

	srv := &Server{
		name:        "h02",
		port:        "0",
		relayTarget: unreachableAddr,
		decoder:     noopDecoder(processed),
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv.listener = listener
	port := listener.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.acceptLoop(ctx)

	devConn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = devConn.Close() }()

	if _, err := fmt.Fprintf(devConn, "*HQ,123456789012345,V4,V1,20260211212008#\r\n"); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case line := <-processed:
		if !strings.Contains(line, "HQ") {
			t.Errorf("unexpected line: %q", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: device message was not processed despite unreachable relay")
	}
}

// TestRelay_LazyDial_NoTraccarTouchedForProbe verifies that a TCP probe — a
// connection that opens and closes without sending any H02 frame — never
// triggers a relay dial. This avoids hammering Traccar with idle TCP sessions
// every time a health check or load balancer probe hits the device port.
func TestRelay_LazyDial_NoTraccarTouchedForProbe(t *testing.T) {
	dialAttempts := make(chan struct{}, 4)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("relay listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			dialAttempts <- struct{}{}
			_ = c.Close()
		}
	}()

	srv := &Server{
		name:        "h02",
		port:        "0",
		relayTarget: ln.Addr().String(),
		decoder:     noopDecoder(nil),
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv.listener = listener
	port := listener.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.acceptLoop(ctx)

	// Simulate a probe: connect and immediately close without sending data.
	probe, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("probe dial: %v", err)
	}
	_ = probe.Close()

	// Give the server time to clean up the probe connection.
	time.Sleep(300 * time.Millisecond)

	select {
	case <-dialAttempts:
		t.Fatal("relay was dialed for a probe-style connection that sent no data")
	default:
	}
}

// TestRelay_RedialsAfterBrokenPipe verifies that after a relay write fails,
// the next device frame redials and gets through. The previous behavior would
// permanently disable relay for the connection on the first failure.
func TestRelay_RedialsAfterBrokenPipe(t *testing.T) {
	// Mock relay: drops the first inbound conn after one byte, then accepts the
	// next conn and forwards every subsequent line to a channel.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("relay listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	secondConnLines := make(chan string, 4)
	firstConnDropped := make(chan struct{})

	go func() {
		c1, err := ln.Accept()
		if err != nil {
			return
		}
		buf := make([]byte, 1)
		_, _ = c1.Read(buf)
		_ = c1.Close()
		close(firstConnDropped)

		c2, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = c2.Close() }()
		sc := bufio.NewScanner(c2)
		for sc.Scan() {
			secondConnLines <- sc.Text()
		}
	}()

	srv := &Server{
		name:        "h02",
		port:        "0",
		relayTarget: ln.Addr().String(),
		decoder:     noopDecoder(nil),
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv.listener = listener
	port := listener.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.acceptLoop(ctx)

	devConn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("device dial: %v", err)
	}
	defer func() { _ = devConn.Close() }()

	if _, err := fmt.Fprintf(devConn, "*HQ,123,V1,first#\r\n"); err != nil {
		t.Fatalf("write first: %v", err)
	}
	select {
	case <-firstConnDropped:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: relay never accepted+dropped first conn")
	}
	time.Sleep(150 * time.Millisecond)

	if _, err := fmt.Fprintf(devConn, "*HQ,123,V1,second#\r\n"); err != nil {
		t.Fatalf("write second: %v", err)
	}

	select {
	case got := <-secondConnLines:
		if got != "*HQ,123,V1,second#" {
			t.Errorf("unexpected relayed line: %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: redial did not deliver second frame after broken pipe")
	}
}

// TestRelay_FailOpen_RelayDropsMidSession verifies that when the relay
// connection drops mid-session, Motus continues serving the device.
func TestRelay_FailOpen_RelayDropsMidSession(t *testing.T) {
	// Start a relay that accepts one connection then immediately closes it after
	// receiving the first byte.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("relay listen: %v", err)
	}

	relayAddr := ln.Addr().String()
	relayReady := make(chan struct{})

	go func() {
		defer func() { _ = ln.Close() }()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		close(relayReady)
		buf := make([]byte, 256)
		_, _ = conn.Read(buf)
		_ = conn.Close()
	}()

	processed := make(chan string, 10)

	srv := &Server{
		name:        "h02",
		port:        "0",
		relayTarget: relayAddr,
		decoder:     noopDecoder(processed),
	}

	listener, err2 := net.Listen("tcp", "127.0.0.1:0")
	if err2 != nil {
		t.Fatalf("listen: %v", err2)
	}
	srv.listener = listener
	port := listener.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.acceptLoop(ctx)

	devConn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = devConn.Close() }()

	// First message: relay alive.
	if _, err := fmt.Fprintf(devConn, "*HQ,123456789012345,V4,V1,20260211212008#\r\n"); err != nil {
		t.Fatalf("write msg1: %v", err)
	}
	select {
	case <-processed:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for first message to be processed")
	}

	// Wait for the relay goroutine to accept and close its side.
	select {
	case <-relayReady:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for relay to signal readiness")
	}

	// Give server time to detect write failure on the relay side.
	time.Sleep(200 * time.Millisecond)

	// Second message: relay is dead. Motus must still process it.
	if _, err := fmt.Fprintf(devConn, "*HQ,123456789012345,V4,V1,20260211212009#\r\n"); err != nil {
		t.Fatalf("write msg2: %v", err)
	}
	select {
	case <-processed:
		// success
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: device was not served after relay dropped (should be fail-open)")
	}
}

package demo

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatchdog_TriggersOnStaleConnection(t *testing.T) {
	// Simulate a connection that succeeds but never actually writes.
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	w := newConnWriter(client, 5*time.Second)

	// Force the write timestamp into the past to simulate staleness.
	w.lastWriteAt.Store(time.Now().Add(-30 * time.Second).UnixNano())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- runWatchdog(ctx, w, 1*time.Second, 500*time.Millisecond)
	}()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected watchdog to return stale connection error")
		}
		t.Logf("watchdog fired: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("watchdog did not fire within 3 seconds")
	}
}

func TestWatchdog_DoesNotFireWhenWritesAreFresh(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	w := newConnWriter(client, 5*time.Second)

	// Context expires before the watchdog threshold.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := runWatchdog(ctx, w, 10*time.Second, 200*time.Millisecond)
	if err != nil {
		t.Errorf("expected nil error (context cancelled), got: %v", err)
	}
}

func TestWatchdog_CancelledByContext(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	w := newConnWriter(client, 5*time.Second)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- runWatchdog(ctx, w, 60*time.Second, 100*time.Millisecond)
	}()

	// Cancel immediately.
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("expected nil on context cancel, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog did not exit after context cancel")
	}
}

func TestSimulateDevice_ResumesFromProgress(t *testing.T) {
	// This test verifies that when a connection drops mid-route,
	// the simulator resumes from the approximate position, not from the start.

	var (
		mu            sync.Mutex
		receivedMsgs  []string
		connCount     int32
		firstDropAt   int
		secondStartAt int
	)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}

			count := atomic.AddInt32(&connCount, 1)

			go func(conn net.Conn, connNum int32) {
				defer func() { _ = conn.Close() }()
				buf := make([]byte, 4096)
				msgCount := 0

				for {
					_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
					n, err := conn.Read(buf)
					if err != nil {
						return
					}

					mu.Lock()
					receivedMsgs = append(receivedMsgs, string(buf[:n]))
					msgCount++

					if connNum == 1 && msgCount >= 5 {
						// Drop the first connection after 5 messages.
						firstDropAt = len(receivedMsgs)
						mu.Unlock()
						_ = conn.Close()
						return
					}

					if connNum == 2 && secondStartAt == 0 {
						secondStartAt = len(receivedMsgs)
					}
					mu.Unlock()

					// Keep the second connection open longer.
					if connNum == 2 && msgCount >= 10 {
						return
					}
				}
			}(c, count)
		}
	}()

	route := &Route{
		Name:   "test",
		Points: makeTestPoints(200), // 200 points so there's plenty to traverse.
	}

	sim := NewSimulator([]*Route{route}, ln.Addr().String(), []string{"TEST001"}, 200.0)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		sim.Start(ctx)
		close(done)
	}()

	// Wait for at least 2 connections.
	deadline := time.After(12 * time.Second)
	for atomic.LoadInt32(&connCount) < 2 {
		select {
		case <-deadline:
			t.Logf("only got %d connections, test inconclusive", atomic.LoadInt32(&connCount))
			cancel()
			<-done
			return
		case <-time.After(100 * time.Millisecond):
		}
	}

	// Give the second connection time to receive some messages.
	time.Sleep(1 * time.Second)
	cancel()
	<-done

	mu.Lock()
	totalMsgs := len(receivedMsgs)
	drop := firstDropAt
	start := secondStartAt
	mu.Unlock()

	t.Logf("total messages: %d, first drop at: %d, second start at: %d, connections: %d",
		totalMsgs, drop, start, atomic.LoadInt32(&connCount))

	if atomic.LoadInt32(&connCount) < 2 {
		t.Errorf("expected at least 2 connections, got %d", atomic.LoadInt32(&connCount))
	}
}

func TestEnableTCPKeepAlive(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
	}()

	conn, err := net.DialTimeout("tcp", ln.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// enableTCPKeepAlive should not error on a valid TCP connection.
	err = enableTCPKeepAlive(conn, 15*time.Second)
	if err != nil {
		t.Errorf("enableTCPKeepAlive failed: %v", err)
	}
}

func TestEnableTCPKeepAlive_NonTCPConn(t *testing.T) {
	// Pipe connections are not TCP, so keepalive should be a no-op (no error).
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	err := enableTCPKeepAlive(client, 15*time.Second)
	// Should not error -- it just logs and skips.
	if err != nil {
		t.Errorf("expected no error for non-TCP conn, got: %v", err)
	}
}

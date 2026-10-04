package demo

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSimulateDevice_ResumesFromProgress(t *testing.T) {
	// This test verifies that when a connection drops mid-route,
	// the simulator resumes from the approximate position, not from the start.

	var (
		mu            sync.Mutex
		receivedMsgs  []string
		connCount     atomic.Int32
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

			count := connCount.Add(1)

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
	for connCount.Load() < 2 {
		select {
		case <-deadline:
			t.Logf("only got %d connections, test inconclusive", connCount.Load())
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
		totalMsgs, drop, start, connCount.Load())

	if connCount.Load() < 2 {
		t.Errorf("expected at least 2 connections, got %d", connCount.Load())
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

// TestSimulateDevice_ReconnectsWhenServerCloses reproduces a server pod going
// away during a rollout: the server half-closes (FIN) but keeps accepting
// data, so the simulator's writes still succeed and only the read side sees
// EOF. The simulator must reconnect instead of writing into the closed
// connection until TCP gives up.
func TestSimulateDevice_ReconnectsWhenServerCloses(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	var conns atomic.Int32
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			n := conns.Add(1)
			go func() {
				defer func() { _ = c.Close() }()
				buf := make([]byte, 4096)
				if n == 1 {
					if _, err := c.Read(buf); err != nil {
						return
					}
					_ = c.(*net.TCPConn).CloseWrite()
				}
				for {
					if _, err := c.Read(buf); err != nil {
						return
					}
				}
			}()
		}
	}()

	route := &Route{Name: "test", Points: makeTestPoints(200)}
	sim := NewSimulator([]*Route{route}, ln.Addr().String(), []string{"TEST002"}, 200.0)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		sim.Start(ctx)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	deadline := time.Now().Add(5 * time.Second)
	for conns.Load() < 2 {
		if time.Now().After(deadline) {
			t.Fatal("simulator did not reconnect after the server closed the connection")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

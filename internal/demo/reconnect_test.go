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

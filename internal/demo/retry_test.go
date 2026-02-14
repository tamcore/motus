package demo

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

func TestBackoff_InitialDelay(t *testing.T) {
	b := newBackoff()
	if b.next() != initialBackoff {
		t.Errorf("first backoff = %v, want %v", b.next(), initialBackoff)
	}
}

func TestBackoff_ExponentialGrowth(t *testing.T) {
	b := newBackoff()

	// Collect delays and verify exponential growth.
	var delays []time.Duration
	for i := 0; i < 10; i++ {
		delays = append(delays, b.next())
		b.increment()
	}

	// Each delay should be double the previous (until cap).
	for i := 1; i < len(delays); i++ {
		if delays[i] <= delays[i-1] && delays[i] < maxBackoff {
			t.Errorf("delay[%d]=%v should be > delay[%d]=%v", i, delays[i], i-1, delays[i-1])
		}
	}
}

func TestBackoff_CapsAtMaximum(t *testing.T) {
	b := newBackoff()

	// Increment many times past the cap.
	for i := 0; i < 20; i++ {
		b.increment()
	}

	delay := b.next()
	if delay > maxBackoff {
		t.Errorf("backoff = %v exceeds max %v", delay, maxBackoff)
	}
}

func TestBackoff_Reset(t *testing.T) {
	b := newBackoff()
	// Increment several times.
	for i := 0; i < 5; i++ {
		b.increment()
	}
	// Verify it grew.
	if b.next() <= initialBackoff {
		t.Fatal("expected backoff to have grown")
	}

	b.reset()
	if b.next() != initialBackoff {
		t.Errorf("after reset, backoff = %v, want %v", b.next(), initialBackoff)
	}
}

func TestBackoff_NextIsIdempotent(t *testing.T) {
	b := newBackoff()
	b.increment()
	b.increment()
	d1 := b.next()
	d2 := b.next()
	if d1 != d2 {
		t.Errorf("next() not idempotent: %v != %v", d1, d2)
	}
}

func TestConnectWithBackoff_ImmediateSuccess(t *testing.T) {
	// Start a TCP listener that accepts connections.
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
			_ = c.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	b := newBackoff()
	conn, err := connectWithBackoff(ctx, ln.Addr().String(), &b)
	if err != nil {
		t.Fatalf("connectWithBackoff: %v", err)
	}
	_ = conn.Close()

	// Backoff should not have incremented since connection succeeded first try.
	if b.next() != initialBackoff {
		t.Errorf("backoff should be at initial after immediate success, got %v", b.next())
	}
}

func TestConnectWithBackoff_RetriesUntilAvailable(t *testing.T) {
	// Start a listener after a short delay to simulate server restart.
	addr := findFreeAddr(t)

	var ln net.Listener
	var mu sync.Mutex

	// Start the listener after 500ms.
	go func() {
		time.Sleep(500 * time.Millisecond)
		l, err := net.Listen("tcp", addr)
		if err != nil {
			return
		}
		mu.Lock()
		ln = l
		mu.Unlock()

		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	defer func() {
		mu.Lock()
		if ln != nil {
			_ = ln.Close()
		}
		mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	b := newBackoff()
	conn, err := connectWithBackoff(ctx, addr, &b)
	if err != nil {
		t.Fatalf("connectWithBackoff should have succeeded after retry: %v", err)
	}
	_ = conn.Close()
}

func TestConnectWithBackoff_RespectsContextCancellation(t *testing.T) {
	// Use an address where nothing is listening.
	addr := findFreeAddr(t)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	b := newBackoff()
	_, err := connectWithBackoff(ctx, addr, &b)
	if err == nil {
		t.Fatal("expected error when context cancelled, got nil")
	}
}

func TestConnectWithBackoff_BackoffGrowsDuringRetries(t *testing.T) {
	// Use an address where nothing is listening. Cancel quickly.
	addr := findFreeAddr(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	b := newBackoff()
	_, _ = connectWithBackoff(ctx, addr, &b)

	// After failed retries, backoff should have grown beyond initial.
	if b.next() <= initialBackoff {
		t.Errorf("backoff should have grown during retries, got %v", b.next())
	}
}

func TestSimulateDevice_ReconnectsAfterConnectionDrop(t *testing.T) {
	// This is an integration-style test that verifies the simulator's
	// outer reconnection loop works when a connection drops.

	// Start a TCP listener that accepts one connection, reads a few messages,
	// then closes the connection to simulate a server restart.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	connectionCount := 0
	var mu sync.Mutex

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}

			mu.Lock()
			connectionCount++
			count := connectionCount
			mu.Unlock()

			if count == 1 {
				// First connection: read a few bytes then close abruptly.
				buf := make([]byte, 512)
				_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
				_, _ = c.Read(buf) // read at least one message
				_ = c.Close()      // simulate server going away
			} else {
				// Second connection: just hold it open briefly then close.
				time.Sleep(500 * time.Millisecond)
				_ = c.Close()
			}
		}
	}()

	defer func() { _ = ln.Close() }()

	route := &Route{
		Name:   "test",
		Points: makeTestPoints(100), // 100 points to keep the simulator busy
	}

	sim := NewSimulator([]*Route{route}, ln.Addr().String(), []string{"TEST001"}, 100.0)

	// Run the simulator for a few seconds -- long enough for one drop + reconnect.
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	// Start the simulation in a goroutine.
	done := make(chan struct{})
	go func() {
		sim.Start(ctx)
		close(done)
	}()

	<-done

	mu.Lock()
	finalCount := connectionCount
	mu.Unlock()

	if finalCount < 2 {
		t.Errorf("expected at least 2 connections (initial + reconnect), got %d", finalCount)
	}
	t.Logf("total connections made: %d", finalCount)
}

// makeTestPoints creates n route points in a line for testing.
func makeTestPoints(n int) []RoutePoint {
	points := make([]RoutePoint, n)
	for i := range points {
		points[i] = RoutePoint{
			Lat:      48.0 + float64(i)*0.001,
			Lon:      11.0 + float64(i)*0.001,
			Speed:    80,
			Course:   45,
			Distance: 100,
		}
	}
	points[0].Distance = 0
	return points
}

// findFreeAddr returns a free TCP address on localhost.
func findFreeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free addr: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	// Small sleep to ensure the port is released.
	time.Sleep(10 * time.Millisecond)
	return addr
}

func TestBackoff_SpecificValues(t *testing.T) {
	b := newBackoff()

	expected := []time.Duration{
		1 * time.Second,  // initial
		2 * time.Second,  // 2x
		4 * time.Second,  // 4x
		8 * time.Second,  // 8x
		16 * time.Second, // 16x
		32 * time.Second, // 32x
		60 * time.Second, // capped at max
		60 * time.Second, // stays at max
	}

	for i, want := range expected {
		got := b.next()
		if got != want {
			t.Errorf("step %d: backoff = %v, want %v", i, got, want)
		}
		b.increment()
	}
}

func TestConnectWithBackoff_ResetsBackoffOnSuccess(t *testing.T) {
	// Start a listener immediately.
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
			_ = c.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Pre-increment backoff to simulate prior failures.
	b := newBackoff()
	for i := 0; i < 5; i++ {
		b.increment()
	}
	if b.next() <= initialBackoff {
		t.Fatal("expected pre-incremented backoff to be greater than initial")
	}

	conn, err := connectWithBackoff(ctx, ln.Addr().String(), &b)
	if err != nil {
		t.Fatalf("connectWithBackoff: %v", err)
	}
	_ = conn.Close()

	// After successful connection, backoff should be reset.
	if b.next() != initialBackoff {
		t.Errorf("backoff should be reset after success, got %v", b.next())
	}
}

func TestSimulatorLogsConnectionAttempts(t *testing.T) {
	// This test verifies the simulator doesn't panic when the server is
	// unavailable and context is cancelled during retry.
	addr := findFreeAddr(t)

	route := &Route{
		Name:   "test",
		Points: makeTestPoints(10),
	}

	sim := NewSimulator([]*Route{route}, addr, []string{"TEST001"}, 100.0)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Should not panic; just exit when context expires.
	done := make(chan struct{})
	go func() {
		sim.Start(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Good - exited cleanly.
	case <-time.After(5 * time.Second):
		t.Fatal("simulator did not exit after context cancellation")
	}
}

func TestBackoff_JitterRange(t *testing.T) {
	// Verify that withJitter returns values within expected range.
	b := newBackoff()

	// Run many iterations to check jitter stays within bounds.
	for i := 0; i < 100; i++ {
		base := b.next()
		jittered := b.withJitter()
		// Jitter should be within [0.5*base, 1.5*base].
		minVal := time.Duration(float64(base) * 0.5)
		maxVal := time.Duration(float64(base) * 1.5)
		if jittered < minVal || jittered > maxVal {
			t.Errorf("jittered %v outside range [%v, %v] for base %v", jittered, minVal, maxVal, base)
		}
	}
}

func TestBackoff_Workflow(t *testing.T) {
	b := newBackoff()

	if got := b.next(); got != 1*time.Second {
		t.Errorf("initial = %v, want 1s", got)
	}
	b.increment()
	if got := b.next(); got != 2*time.Second {
		t.Errorf("after 1 increment = %v, want 2s", got)
	}
	b.increment()
	if got := b.next(); got != 4*time.Second {
		t.Errorf("after 2 increments = %v, want 4s", got)
	}
	b.reset()
	if got := b.next(); got != 1*time.Second {
		t.Errorf("after reset = %v, want 1s", got)
	}
}

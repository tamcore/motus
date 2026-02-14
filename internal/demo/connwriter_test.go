package demo

import (
	"net"
	"testing"
	"time"
)

func TestConnWriter_WriteTracksTimestamp(t *testing.T) {
	// Create a pipe-based connection pair.
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	w := newConnWriter(client, 5*time.Second)

	// Drain reads on the server side so writes don't block.
	go func() {
		buf := make([]byte, 1024)
		for {
			if _, err := server.Read(buf); err != nil {
				return
			}
		}
	}()

	beforeWrite := time.Now()
	time.Sleep(10 * time.Millisecond) // Ensure measurable time passes.

	_, err := w.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	age := w.LastWriteAge()
	if age > time.Since(beforeWrite) {
		t.Errorf("LastWriteAge %v should be less than time since before write %v", age, time.Since(beforeWrite))
	}
	if age > 1*time.Second {
		t.Errorf("LastWriteAge %v too large after immediate write", age)
	}
}

func TestConnWriter_WriteStringAppendsTerminator(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	w := newConnWriter(client, 5*time.Second)

	done := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 1024)
		n, _ := server.Read(buf)
		done <- buf[:n]
	}()

	err := w.WriteString("*HQ,TEST,V1#")
	if err != nil {
		t.Fatalf("WriteString failed: %v", err)
	}

	received := <-done
	expected := "*HQ,TEST,V1#\r\n"
	if string(received) != expected {
		t.Errorf("received %q, want %q", string(received), expected)
	}
}

func TestConnWriter_IsStale_FreshConnection(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	w := newConnWriter(client, 5*time.Second)

	// Immediately after creation, connection should not be stale.
	if w.IsStale(1 * time.Second) {
		t.Error("new connWriter should not be stale")
	}
}

func TestConnWriter_IsStale_AfterDelay(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	w := newConnWriter(client, 5*time.Second)

	// Force the lastWriteAt to a time in the past.
	w.lastWriteAt.Store(time.Now().Add(-2 * time.Second).UnixNano())

	if !w.IsStale(1 * time.Second) {
		t.Error("connWriter should be stale after 2s with 1s threshold")
	}
}

func TestConnWriter_IsStale_ResetByWrite(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	w := newConnWriter(client, 5*time.Second)

	// Drain reads.
	go func() {
		buf := make([]byte, 1024)
		for {
			if _, err := server.Read(buf); err != nil {
				return
			}
		}
	}()

	// Force stale.
	w.lastWriteAt.Store(time.Now().Add(-5 * time.Second).UnixNano())
	if !w.IsStale(1 * time.Second) {
		t.Fatal("should be stale before write")
	}

	// Write resets the timestamp.
	_, err := w.Write([]byte("ping"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if w.IsStale(1 * time.Second) {
		t.Error("should not be stale after successful write")
	}
}

func TestConnWriter_WriteFailsOnClosedConn(t *testing.T) {
	server, client := net.Pipe()
	_ = server.Close()
	_ = client.Close()

	w := newConnWriter(client, 1*time.Second)

	_, err := w.Write([]byte("hello"))
	if err == nil {
		t.Error("expected error writing to closed connection")
	}
}

func TestConnWriter_ConcurrentWrite_DoesNotRace(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	w := newConnWriter(client, 5*time.Second)

	// Drain server side so writes don't block.
	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := server.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// Two goroutines writing concurrently must not data-race.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 5; i++ {
			_ = w.WriteString("hello from goroutine 1")
		}
	}()
	for i := 0; i < 5; i++ {
		_ = w.WriteString("hello from goroutine 2")
	}
	<-done
}

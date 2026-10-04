package demo

import (
	"net"
	"testing"
	"time"
)

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
		for range 5 {
			_ = w.WriteString("hello from goroutine 1")
		}
	}()
	for range 5 {
		_ = w.WriteString("hello from goroutine 2")
	}
	<-done
}

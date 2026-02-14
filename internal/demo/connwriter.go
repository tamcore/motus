package demo

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// connWriter wraps a net.Conn and tracks write health.
//
// After each successful write, it records the timestamp so the watchdog
// goroutine can detect stale connections where the TCP write "succeeds"
// (data accepted into the kernel buffer) but the remote end is gone.
type connWriter struct {
	conn          net.Conn
	writeDeadline time.Duration
	lastWriteAt   atomic.Int64 // unix nanoseconds of last successful write
	mu            sync.Mutex   // guards SetWriteDeadline + conn.Write
}

// newConnWriter creates a connWriter with the given write deadline.
// It records the current time as the initial "last write" timestamp.
func newConnWriter(conn net.Conn, writeDeadline time.Duration) *connWriter {
	w := &connWriter{
		conn:          conn,
		writeDeadline: writeDeadline,
	}
	w.lastWriteAt.Store(time.Now().UnixNano())
	return w
}

// Write sends data with a write deadline and tracks the last successful write time.
// It is safe to call concurrently (e.g. from the route loop and the command reader).
func (w *connWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.conn.SetWriteDeadline(time.Now().Add(w.writeDeadline)); err != nil {
		return 0, fmt.Errorf("set write deadline: %w", err)
	}

	n, err := w.conn.Write(data)
	if err != nil {
		return n, fmt.Errorf("write to connection: %w", err)
	}

	w.lastWriteAt.Store(time.Now().UnixNano())
	return n, nil
}

// WriteString sends a string message followed by CRLF.
func (w *connWriter) WriteString(msg string) error {
	_, err := w.Write([]byte(msg + "\r\n"))
	return err
}

// LastWriteAge returns how long ago the last successful write occurred.
func (w *connWriter) LastWriteAge() time.Duration {
	lastNano := w.lastWriteAt.Load()
	return time.Since(time.Unix(0, lastNano))
}

// IsStale returns true if no successful write has occurred within the given threshold.
func (w *connWriter) IsStale(threshold time.Duration) bool {
	return w.LastWriteAge() > threshold
}

// Close closes the underlying connection.
func (w *connWriter) Close() error {
	return w.conn.Close()
}

// RemoteAddr returns the remote address of the underlying connection.
func (w *connWriter) RemoteAddr() net.Addr {
	return w.conn.RemoteAddr()
}

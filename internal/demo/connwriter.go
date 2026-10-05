package demo

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// connWriter wraps a net.Conn and applies a deadline to every write.
type connWriter struct {
	conn          net.Conn
	writeDeadline time.Duration
	mu            sync.Mutex // guards SetWriteDeadline + conn.Write
}

// newConnWriter creates a connWriter with the given write deadline.
func newConnWriter(conn net.Conn, writeDeadline time.Duration) *connWriter {
	return &connWriter{conn: conn, writeDeadline: writeDeadline}
}

// Write sends data with a write deadline.
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
	return n, nil
}

// WriteString sends a string message followed by CRLF.
func (w *connWriter) WriteString(msg string) error {
	_, err := w.Write([]byte(msg + "\r\n"))
	return err
}

// Close closes the underlying connection.
func (w *connWriter) Close() error {
	return w.conn.Close()
}

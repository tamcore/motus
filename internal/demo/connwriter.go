package demo

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// tcpKeepAlivePeriod is the interval between OS-level TCP keepalive probes.
const tcpKeepAlivePeriod = 15 * time.Second

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

// enableTCPKeepAlive enables OS-level TCP keepalive on a connection.
// If the connection is not a *net.TCPConn (e.g., in tests using net.Pipe),
// it is silently skipped.
func enableTCPKeepAlive(conn net.Conn, period time.Duration) error {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		// Not a TCP connection (e.g., net.Pipe in tests). Skip silently.
		return nil
	}

	if err := tcpConn.SetKeepAlive(true); err != nil {
		return fmt.Errorf("enable TCP keepalive: %w", err)
	}

	if err := tcpConn.SetKeepAlivePeriod(period); err != nil {
		return fmt.Errorf("set TCP keepalive period: %w", err)
	}

	return nil
}

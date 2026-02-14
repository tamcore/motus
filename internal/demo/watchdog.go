package demo

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"
)

// Watchdog constants.
const (
	// defaultStaleThreshold is how long a connection can go without a
	// successful write before the watchdog considers it stale.
	defaultStaleThreshold = 60 * time.Second

	// defaultWatchdogInterval is how often the watchdog checks for staleness.
	defaultWatchdogInterval = 10 * time.Second

	// tcpKeepAlivePeriod is the interval between OS-level TCP keepalive probes.
	tcpKeepAlivePeriod = 15 * time.Second
)

// errStaleConnection is returned when the watchdog detects that no
// successful write has occurred within the stale threshold.
var errStaleConnection = fmt.Errorf("connection stale: no successful write within threshold")

// runWatchdog monitors a connWriter and returns an error when the connection
// is stale (no successful write within the threshold). It returns nil when
// the context is cancelled (normal shutdown).
//
// The caller should run this in a goroutine and select on the returned error
// alongside the main traversal loop.
func runWatchdog(ctx context.Context, w *connWriter, staleThreshold, checkInterval time.Duration) error {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if w.IsStale(staleThreshold) {
				age := w.LastWriteAge()
				slog.Warn("watchdog: connection stale",
					slog.String("lastWriteAge", age.Round(time.Second).String()),
					slog.String("threshold", staleThreshold.String()))
				return errStaleConnection
			}
		}
	}
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

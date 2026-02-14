package demo

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"time"
)

// Backoff constants for connection retry logic.
const (
	// initialBackoff is the delay before the first retry attempt.
	initialBackoff = 1 * time.Second

	// maxBackoff is the maximum delay between retry attempts.
	maxBackoff = 60 * time.Second

	// backoffMultiplier doubles the delay on each failed attempt.
	backoffMultiplier = 2

	// dialTimeout is how long to wait for a single TCP dial attempt.
	dialTimeout = 10 * time.Second
)

// backoff tracks exponential backoff state for connection retries.
// It is not safe for concurrent use; each device goroutine gets its own.
type backoff struct {
	current time.Duration
}

// newBackoff creates a backoff starting at the initial delay.
func newBackoff() backoff {
	return backoff{current: initialBackoff}
}

// next returns the current backoff delay without modifying state.
func (b *backoff) next() time.Duration {
	return b.current
}

// withJitter returns the current backoff with random jitter applied.
// The jittered value falls within [0.5*current, 1.5*current] to prevent
// thundering herd when multiple devices retry simultaneously.
func (b *backoff) withJitter() time.Duration {
	// jitter factor in [0.5, 1.5)
	jitter := 0.5 + rand.Float64()
	return time.Duration(float64(b.current) * jitter)
}

// increment doubles the backoff delay, capping at maxBackoff.
func (b *backoff) increment() {
	b.current *= backoffMultiplier
	if b.current > maxBackoff {
		b.current = maxBackoff
	}
}

// reset returns the backoff to its initial state after a successful connection.
func (b *backoff) reset() {
	b.current = initialBackoff
}

// connectWithBackoff attempts to dial the target TCP address with exponential
// backoff. It retries until a connection succeeds or the context is cancelled.
// On success it resets the backoff state so the next failure starts fresh.
func connectWithBackoff(ctx context.Context, target string, b *backoff) (net.Conn, error) {
	for {
		conn, err := net.DialTimeout("tcp", target, dialTimeout)
		if err == nil {
			// Connection succeeded -- reset backoff for next failure cycle.
			b.reset()
			return conn, nil
		}

		delay := b.withJitter()
		slog.Warn("connection failed, retrying",
			slog.String("target", target),
			slog.Any("error", err),
			slog.String("retryIn", delay.Round(time.Millisecond).String()))
		b.increment()

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("connect to %s: gave up after context cancelled: %w", target, ctx.Err())
		case <-time.After(delay):
			// Retry.
		}
	}
}

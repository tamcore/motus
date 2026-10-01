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

// backoff tracks exponential backoff state; each device goroutine owns one.
type backoff struct {
	current time.Duration
}

func newBackoff() backoff {
	return backoff{current: initialBackoff}
}

// withJitter returns current scaled into [0.5, 1.5) to avoid a thundering herd.
func (b *backoff) withJitter() time.Duration {
	return time.Duration(float64(b.current) * (0.5 + rand.Float64()))
}

func (b *backoff) increment() {
	b.current = min(b.current*backoffMultiplier, maxBackoff)
}

// connectWithBackoff attempts to dial the target TCP address with exponential
// backoff. It retries until a connection succeeds or the context is cancelled.
// On success it resets the backoff state so the next failure starts fresh.
func connectWithBackoff(ctx context.Context, target string, b *backoff) (net.Conn, error) {
	for {
		conn, err := net.DialTimeout("tcp", target, dialTimeout)
		if err == nil {
			b.current = initialBackoff
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

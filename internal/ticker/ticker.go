// Package ticker runs periodic background work.
package ticker

import (
	"context"
	"time"
)

// Every calls fn every d until ctx is cancelled.
func Every(ctx context.Context, d time.Duration, fn func()) {
	t := time.NewTicker(d)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn()
		}
	}
}

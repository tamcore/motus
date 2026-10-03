package ticker

import (
	"context"
	"testing"
	"time"
)

func TestEvery_RunsUntilCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	calls := 0
	Every(ctx, time.Millisecond, func() {
		calls++
		if calls == 3 {
			cancel()
		}
	})
	if calls < 3 {
		t.Fatalf("calls = %d, want at least 3", calls)
	}
}

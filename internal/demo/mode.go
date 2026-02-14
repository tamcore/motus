package demo

import "sync/atomic"

// enabled tracks whether demo mode is active. It is set once at startup
// and read from request handlers to enforce immutable account protections.
var enabled atomic.Bool

// Enable activates demo mode protections globally.
func Enable() {
	enabled.Store(true)
}

// IsEnabled reports whether demo mode is active.
func IsEnabled() bool {
	return enabled.Load()
}

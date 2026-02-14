package demo

import (
	"testing"
)

func TestDemoMode(t *testing.T) {
	// Note: This test modifies global state. It runs in the demo package
	// so it has access to the atomic.Bool directly.

	// Initially disabled.
	if IsEnabled() {
		t.Error("demo mode should be disabled by default")
	}

	Enable()
	if !IsEnabled() {
		t.Error("demo mode should be enabled after Enable()")
	}

	// Reset for other tests (not strictly needed since tests may run
	// in any order, but good practice).
	enabled.Store(false)
}

func TestIsDemoAccount(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  bool
	}{
		{name: "demo", email: "demo@motus.local", want: true},
		{name: "admin", email: "admin@motus.local", want: true},
		{name: "other", email: "user@example.com", want: false},
		{name: "empty", email: "", want: false},
		{name: "case-sensitive", email: "Demo@motus.local", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDemoAccount(tt.email); got != tt.want {
				t.Errorf("IsDemoAccount(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}
}

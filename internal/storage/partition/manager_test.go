package partition

import (
	"log/slog"
	"testing"
	"time"
)

func TestPartitionName(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Time
		expected string
	}{
		{
			name:     "january 2026",
			input:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			expected: "positions_y2026m01",
		},
		{
			name:     "december 2025",
			input:    time.Date(2025, 12, 15, 10, 30, 0, 0, time.UTC),
			expected: "positions_y2025m12",
		},
		{
			name:     "february 2026",
			input:    time.Date(2026, 2, 28, 23, 59, 59, 0, time.UTC),
			expected: "positions_y2026m02",
		},
		{
			name:     "single digit month",
			input:    time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			expected: "positions_y2026m03",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PartitionName(tt.input)
			if got != tt.expected {
				t.Errorf("PartitionName(%v) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNewManager_DefaultsLogger(t *testing.T) {
	if mgr := NewManager(nil, 0, time.Hour, nil); mgr.logger != slog.Default() {
		t.Error("nil logger should default to slog.Default()")
	}
}

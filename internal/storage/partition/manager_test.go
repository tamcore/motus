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

func TestExtractQuotedStrings(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "partition bounds with timezone",
			input:    "FOR VALUES FROM ('2026-01-01 00:00:00+00') TO ('2026-02-01 00:00:00+00')",
			expected: []string{"2026-01-01 00:00:00+00", "2026-02-01 00:00:00+00"},
		},
		{
			name:     "partition bounds date only",
			input:    "FOR VALUES FROM ('2026-01-01') TO ('2026-02-01')",
			expected: []string{"2026-01-01", "2026-02-01"},
		},
		{
			name:     "no quotes",
			input:    "some expression without quotes",
			expected: nil,
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "single quote pair",
			input:    "FROM ('2026-01-01')",
			expected: []string{"2026-01-01"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractQuotedStrings(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("extractQuotedStrings(%q) returned %d strings, want %d: %v",
					tt.input, len(got), len(tt.expected), got)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("extractQuotedStrings(%q)[%d] = %q, want %q",
						tt.input, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestParsePartitionDate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Time
		wantErr  bool
	}{
		{
			name:     "date with timezone offset",
			input:    "2026-01-01 00:00:00+00",
			expected: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "date with negative timezone",
			input:    "2026-06-15 00:00:00-05",
			expected: time.Date(2026, 6, 15, 5, 0, 0, 0, time.UTC),
		},
		{
			name:     "date without time",
			input:    "2026-01-01",
			expected: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "date with time no timezone",
			input:    "2026-01-01 00:00:00",
			expected: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:    "invalid date",
			input:   "not-a-date",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePartitionDate(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parsePartitionDate(%q) expected error, got %v", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePartitionDate(%q) unexpected error: %v", tt.input, err)
			}
			if !got.Equal(tt.expected) {
				t.Errorf("parsePartitionDate(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParsePartitionBounds(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantStart time.Time
		wantEnd   time.Time
		wantErr   bool
	}{
		{
			name:      "standard postgres format",
			input:     "FOR VALUES FROM ('2026-01-01 00:00:00+00') TO ('2026-02-01 00:00:00+00')",
			wantStart: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "date only format",
			input:     "FOR VALUES FROM ('2025-12-01') TO ('2026-01-01')",
			wantStart: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:    "missing dates",
			input:   "DEFAULT",
			wantErr: true,
		},
		{
			name:    "only one date",
			input:   "FOR VALUES FROM ('2026-01-01')",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := parsePartitionBounds(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parsePartitionBounds(%q) expected error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePartitionBounds(%q) unexpected error: %v", tt.input, err)
			}
			if !start.Equal(tt.wantStart) {
				t.Errorf("start = %v, want %v", start, tt.wantStart)
			}
			if !end.Equal(tt.wantEnd) {
				t.Errorf("end = %v, want %v", end, tt.wantEnd)
			}
		})
	}
}

func TestManager_SetLogger_Nil(t *testing.T) {
	// SetLogger with nil should be a no-op (keep existing logger, no panic).
	mgr := NewManager(nil, 0, time.Hour)
	initialLogger := mgr.logger

	mgr.SetLogger(nil) // Should not panic.

	if mgr.logger != initialLogger {
		t.Error("expected logger to remain unchanged after SetLogger(nil)")
	}
}

func TestManager_SetLogger_Custom(t *testing.T) {
	mgr := NewManager(nil, 0, time.Hour)
	customLogger := slog.Default()

	mgr.SetLogger(customLogger)

	if mgr.logger != customLogger {
		t.Error("expected logger to be replaced with the custom logger")
	}
}

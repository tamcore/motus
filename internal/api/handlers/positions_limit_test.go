package handlers

import "testing"

func TestPositionLimit(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{"omitted uses max", 0, maxPositionsPerResponse},
		{"negative uses max", -5, maxPositionsPerResponse},
		{"below max kept", 500, 500},
		{"max kept", maxPositionsPerResponse, maxPositionsPerResponse},
		{"above max clamped", maxPositionsPerResponse + 1, maxPositionsPerResponse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := positionLimit(tt.in); got != tt.want {
				t.Errorf("positionLimit(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

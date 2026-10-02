package handlers

import (
	"errors"
	"math"
	"testing"

	"github.com/tamcore/motus/internal/model"
)

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

func TestCollectPositions(t *testing.T) {
	t.Run("empty stream yields empty non-nil slice", func(t *testing.T) {
		got, err := collectPositions(func(func(*model.Position) error) error { return nil })
		if err != nil || got == nil || len(got) != 0 {
			t.Fatalf("collectPositions(empty) = %v, %v; want [], nil", got, err)
		}
	})
	t.Run("stream error is returned", func(t *testing.T) {
		want := errors.New("boom")
		if _, err := collectPositions(func(func(*model.Position) error) error { return want }); !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
	})
	t.Run("keeps order and converts speed to knots", func(t *testing.T) {
		kmh := 18.52
		got, err := collectPositions(func(fn func(*model.Position) error) error {
			for id := int64(1); id <= 3; id++ {
				if err := fn(&model.Position{ID: id, Speed: &kmh}); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil || len(got) != 3 {
			t.Fatalf("collectPositions = %d positions, %v; want 3, nil", len(got), err)
		}
		for i, p := range got {
			if p.ID != int64(i+1) {
				t.Errorf("position %d has ID %d, want %d", i, p.ID, i+1)
			}
			if math.Abs(p.Speed-10) > 1e-9 {
				t.Errorf("position %d speed = %v knots, want 10", i, p.Speed)
			}
		}
	})
}

package model

import "testing"

func TestPositionSpeedOrZero(t *testing.T) {
	if got := (&Position{}).SpeedOrZero(); got != 0 {
		t.Errorf("nil speed: got %v, want 0", got)
	}
	if got := (&Position{Speed: new(42.5)}).SpeedOrZero(); got != 42.5 {
		t.Errorf("got %v, want 42.5", got)
	}
}

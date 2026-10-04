package model

import (
	"maps"
	"testing"
)

func TestCellAndWifi(t *testing.T) {
	if got, want := Cell(262, 1, 100, 200), (map[string]any{"mobileCountryCode": 262, "mobileNetworkCode": 1, "locationAreaCode": 100, "cellId": 200}); !maps.Equal(got, want) {
		t.Errorf("Cell = %v, want %v", got, want)
	}
	if got := Cell(262, 1, 100, 200, -70)["signalStrength"]; got != -70 {
		t.Errorf("Cell signalStrength = %v, want -70", got)
	}
	if got, want := Wifi("aa:bb", -50), (map[string]any{"macAddress": "aa:bb", "signalStrength": -50}); !maps.Equal(got, want) {
		t.Errorf("Wifi = %v, want %v", got, want)
	}
}

func TestPositionSpeedOrZero(t *testing.T) {
	if got := (&Position{}).SpeedOrZero(); got != 0 {
		t.Errorf("nil speed: got %v, want 0", got)
	}
	if got := (&Position{Speed: new(42.5)}).SpeedOrZero(); got != 42.5 {
		t.Errorf("got %v, want 42.5", got)
	}
}

func TestSpeedUnitConversion(t *testing.T) {
	if got := KnotsToKmh(10); got != 18.52 {
		t.Errorf("KnotsToKmh(10) = %v, want 18.52", got)
	}
	if got := KmhToKnots(18.52); got != 10 {
		t.Errorf("KmhToKnots(18.52) = %v, want 10", got)
	}
}

func TestPositionInKnots(t *testing.T) {
	if got := (&Position{}).InKnots(); got.Speed != nil {
		t.Errorf("nil speed: got %v, want nil", *got.Speed)
	}
	p := &Position{ID: 7, Speed: new(18.52)}
	got := p.InKnots()
	if got.ID != 7 || got.Speed == nil || *got.Speed != 10 {
		t.Errorf("InKnots() = %+v, want ID 7 speed 10", got)
	}
	if *p.Speed != 18.52 {
		t.Errorf("InKnots mutated the original speed: %v", *p.Speed)
	}
}

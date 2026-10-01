package handlers

import (
	"math"
	"testing"
)

func TestGPXBearing(t *testing.T) {
	tests := []struct {
		name         string
		lat1, lon1   float64
		lat2, lon2   float64
		wantApprox   float64
		toleranceDeg float64
	}{
		{
			name: "due north",
			lat1: 0.0, lon1: 0.0,
			lat2: 1.0, lon2: 0.0,
			wantApprox:   0.0,
			toleranceDeg: 1.0,
		},
		{
			name: "due east",
			lat1: 0.0, lon1: 0.0,
			lat2: 0.0, lon2: 1.0,
			wantApprox:   90.0,
			toleranceDeg: 1.0,
		},
		{
			name: "due south",
			lat1: 1.0, lon1: 0.0,
			lat2: 0.0, lon2: 0.0,
			wantApprox:   180.0,
			toleranceDeg: 1.0,
		},
		{
			name: "due west",
			lat1: 0.0, lon1: 1.0,
			lat2: 0.0, lon2: 0.0,
			wantApprox:   270.0,
			toleranceDeg: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bearing := gpxBearing(tt.lat1, tt.lon1, tt.lat2, tt.lon2)
			// Handle wrap-around for north (0/360).
			diff := math.Abs(bearing - tt.wantApprox)
			if diff > 180 {
				diff = 360 - diff
			}
			if diff > tt.toleranceDeg {
				t.Errorf("gpxBearing(%f,%f,%f,%f) = %.2f°, want ~%.2f° (±%.2f°)",
					tt.lat1, tt.lon1, tt.lat2, tt.lon2, bearing, tt.wantApprox, tt.toleranceDeg)
			}
		})
	}
}

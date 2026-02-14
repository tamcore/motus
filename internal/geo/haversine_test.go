package geo

import (
	"math"
	"testing"
)

func TestHaversineDistance_KnownPairs(t *testing.T) {
	tests := []struct {
		name                   string
		lat1, lon1, lat2, lon2 float64
		wantKm                 float64
		tolerance              float64
	}{
		{
			name: "Berlin to Munich",
			lat1: 52.5200, lon1: 13.4050,
			lat2: 48.1351, lon2: 11.5820,
			wantKm:    504.0,
			tolerance: 5.0,
		},
		{
			name: "same point",
			lat1: 51.0, lon1: 9.0,
			lat2: 51.0, lon2: 9.0,
			wantKm:    0.0,
			tolerance: 0.001,
		},
		{
			name: "short distance (~1km)",
			lat1: 52.5200, lon1: 13.4050,
			lat2: 52.5290, lon2: 13.4050,
			wantKm:    1.0,
			tolerance: 0.05,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HaversineDistance(tt.lat1, tt.lon1, tt.lat2, tt.lon2)
			if math.Abs(got-tt.wantKm) > tt.tolerance {
				t.Errorf("HaversineDistance() = %f km, want %f km (±%f)", got, tt.wantKm, tt.tolerance)
			}
		})
	}
}

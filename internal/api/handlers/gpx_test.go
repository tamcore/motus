package handlers

import (
	"math"
	"testing"
)

func TestGPXHaversine(t *testing.T) {
	tests := []struct {
		name         string
		lat1, lon1   float64
		lat2, lon2   float64
		wantApproxKm float64
		toleranceKm  float64
	}{
		{
			name: "Berlin to Hamburg (~255 km)",
			lat1: 52.5200, lon1: 13.4050,
			lat2: 53.5511, lon2: 9.9937,
			wantApproxKm: 255.0,
			toleranceKm:  10.0,
		},
		{
			name: "same point = 0",
			lat1: 52.0, lon1: 13.0,
			lat2: 52.0, lon2: 13.0,
			wantApproxKm: 0.0,
			toleranceKm:  0.001,
		},
		{
			name: "equator short distance (~111 km per degree)",
			lat1: 0.0, lon1: 0.0,
			lat2: 0.0, lon2: 1.0,
			wantApproxKm: 111.3,
			toleranceKm:  1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			distM := gpxHaversine(tt.lat1, tt.lon1, tt.lat2, tt.lon2)
			distKm := distM / 1000.0
			if math.Abs(distKm-tt.wantApproxKm) > tt.toleranceKm {
				t.Errorf("gpxHaversine(%f,%f,%f,%f) = %.2f km, want ~%.2f km (±%.2f km)",
					tt.lat1, tt.lon1, tt.lat2, tt.lon2, distKm, tt.wantApproxKm, tt.toleranceKm)
			}
		})
	}
}

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

func TestNewGPXImportHandler(t *testing.T) {
	h := NewGPXImportHandler(nil, nil, nil)
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
}

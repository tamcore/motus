package demo_test

import (
	"math"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/demo"
	"github.com/tamcore/motus/internal/protocol/h02"
)

// TestH02MessageRoundTrip verifies that H02 messages built by the simulator
// can be decoded by the actual H02 protocol decoder.
func TestH02MessageRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		imei     string
		lat      float64
		lon      float64
		speed    float64
		course   float64
		altitude float64
		ignition bool
	}{
		{
			name:     "Munich driving",
			imei:     "9000000000001",
			lat:      48.1351,
			lon:      11.5820,
			speed:    120.5,
			course:   45,
			altitude: 523.4,
			ignition: true,
		},
		{
			name:     "Berlin parked",
			imei:     "9000000000002",
			lat:      52.5200,
			lon:      13.4050,
			speed:    0,
			course:   0,
			altitude: 34.0,
			ignition: false,
		},
		{
			name:     "Hamburg driving",
			imei:     "9000000000001",
			lat:      53.5511,
			lon:      9.9937,
			speed:    180,
			course:   270.5,
			altitude: 0,
			ignition: true,
		},
		{
			name:     "Frankfurt driving",
			imei:     "TEST999",
			lat:      50.1109,
			lon:      8.6821,
			speed:    50,
			course:   135,
			altitude: 103.0,
			ignition: true,
		},
	}

	ts := time.Date(2026, 2, 15, 14, 30, 45, 0, time.UTC)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := demo.BuildH02Message(tt.imei, tt.lat, tt.lon, tt.speed, tt.course, tt.altitude, tt.ignition, ts)
			t.Logf("Built message: %s", msg)

			decoded, err := h02.Decode(msg)
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}

			// Verify device ID.
			if decoded.DeviceID != tt.imei {
				t.Errorf("DeviceID = %q, want %q", decoded.DeviceID, tt.imei)
			}

			// Verify message type.
			if decoded.Type != "V1" {
				t.Errorf("Type = %q, want V1", decoded.Type)
			}

			// Verify validity.
			if !decoded.Valid {
				t.Error("message should be valid")
			}

			// Verify latitude (within ~100m tolerance due to NMEA format precision).
			if math.Abs(decoded.Latitude-tt.lat) > 0.002 {
				t.Errorf("Latitude = %.6f, want %.6f (diff %.6f)", decoded.Latitude, tt.lat, math.Abs(decoded.Latitude-tt.lat))
			}

			// Verify longitude.
			if math.Abs(decoded.Longitude-tt.lon) > 0.002 {
				t.Errorf("Longitude = %.6f, want %.6f (diff %.6f)", decoded.Longitude, tt.lon, math.Abs(decoded.Longitude-tt.lon))
			}

			// Verify speed (H02 protocol uses knots internally, so there's a conversion).
			decodedSpeed := decoded.Speed // already in km/h after decode
			if math.Abs(decodedSpeed-tt.speed) > 1.0 {
				t.Errorf("Speed = %.1f km/h, want %.1f km/h", decodedSpeed, tt.speed)
			}

			// Verify timestamp.
			if decoded.Timestamp.Hour() != 14 || decoded.Timestamp.Minute() != 30 || decoded.Timestamp.Second() != 45 {
				t.Errorf("Timestamp = %v, want 14:30:45", decoded.Timestamp)
			}

			// Verify ignition state.
			if decoded.Ignition != tt.ignition {
				t.Errorf("Ignition = %v, want %v", decoded.Ignition, tt.ignition)
			}

			// Verify altitude (1 decimal place precision).
			if math.Abs(decoded.Altitude-tt.altitude) > 0.1 {
				t.Errorf("Altitude = %.1f, want %.1f", decoded.Altitude, tt.altitude)
			}

			t.Logf("Decoded: lat=%.6f lon=%.6f speed=%.1f course=%.1f ignition=%v altitude=%.1f",
				decoded.Latitude, decoded.Longitude, decoded.Speed, decoded.Course, decoded.Ignition, decoded.Altitude)
		})
	}
}

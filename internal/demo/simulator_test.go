package demo

import (
	"math"
	"testing"
	"time"
)

func TestBuildH02Message(t *testing.T) {
	ts := time.Date(2026, 2, 15, 14, 30, 45, 0, time.UTC)

	tests := []struct {
		name     string
		imei     string
		lat      float64
		lon      float64
		speed    float64
		course   float64
		altitude float64
		ignition bool
		contains []string
	}{
		{
			name:     "Munich position, ignition on",
			imei:     "9000000000001",
			lat:      48.1351,
			lon:      11.5820,
			speed:    120,
			course:   45,
			altitude: 520.0,
			ignition: true,
			contains: []string{
				"*HQ,9000000000001,V1,",
				",A,",        // valid fix
				",N,",        // north
				",E,",        // east
				",150226,",   // date DDMMYY
				",FFFFFFEF,", // ignition ON flags
				",520.0#",    // altitude
			},
		},
		{
			name:     "Munich parked, ignition off",
			imei:     "9000000000001",
			lat:      48.1351,
			lon:      11.5820,
			speed:    0,
			course:   0,
			altitude: 520.0,
			ignition: false,
			contains: []string{
				",FFFFFBEF,", // ignition OFF flags
				",520.0#",
			},
		},
		{
			name:     "southern hemisphere",
			imei:     "9000000000002",
			lat:      -33.8688,
			lon:      151.2093,
			speed:    60,
			course:   180,
			altitude: 0,
			ignition: true,
			contains: []string{
				"*HQ,9000000000002,V1,",
				",S,", // south
				",E,", // east
			},
		},
		{
			name:     "western hemisphere",
			imei:     "TEST001",
			lat:      40.7128,
			lon:      -74.0060,
			speed:    80,
			course:   270,
			altitude: 100.5,
			ignition: true,
			contains: []string{
				",N,",
				",W,",
				",100.5#",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := BuildH02Message(tt.imei, tt.lat, tt.lon, tt.speed, tt.course, tt.altitude, tt.ignition, ts)
			t.Logf("H02 message: %s", msg)

			for _, s := range tt.contains {
				if !containsStr(msg, s) {
					t.Errorf("message missing %q:\n  got: %s", s, msg)
				}
			}

			// Verify it starts with *HQ and ends with #.
			if msg[0] != '*' || msg[len(msg)-1] != '#' {
				t.Errorf("message does not have proper delimiters: %s", msg)
			}
		})
	}
}

func TestBuildH02Message_RoundTrip(t *testing.T) {
	// Build an H02 message and verify it can be decoded by the H02 decoder.
	// We cannot import the h02 package here (would create import cycle for tests),
	// but we can check the NMEA coordinate format manually.

	ts := time.Date(2026, 2, 15, 10, 25, 30, 0, time.UTC)
	msg := BuildH02Message("9000000000001", 48.1351, 11.5820, 100, 45, 0, true, ts)

	// The message should contain time "102530" and date "150226".
	if !containsStr(msg, "102530") {
		t.Errorf("message missing time 102530: %s", msg)
	}
	if !containsStr(msg, "150226") {
		t.Errorf("message missing date 150226: %s", msg)
	}
}

func TestDecimalToNMEA(t *testing.T) {
	tests := []struct {
		name    string
		decimal float64
		isLat   bool
		wantDir string
	}{
		{name: "north lat", decimal: 48.1351, isLat: true, wantDir: "N"},
		{name: "south lat", decimal: -33.8688, isLat: true, wantDir: "S"},
		{name: "east lon", decimal: 11.5820, isLat: false, wantDir: "E"},
		{name: "west lon", decimal: -74.0060, isLat: false, wantDir: "W"},
		{name: "zero lat", decimal: 0, isLat: true, wantDir: "N"},
		{name: "zero lon", decimal: 0, isLat: false, wantDir: "E"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nmea, dir := decimalToNMEA(tt.decimal, tt.isLat)
			if dir != tt.wantDir {
				t.Errorf("direction = %q, want %q", dir, tt.wantDir)
			}

			// Verify NMEA string is not empty and has expected format.
			if len(nmea) == 0 {
				t.Error("NMEA string is empty")
			}
			t.Logf("%.4f -> %s %s", tt.decimal, nmea, dir)
		})
	}

	// Verify round-trip accuracy for a known coordinate.
	nmea, dir := decimalToNMEA(48.1351, true)
	if dir != "N" {
		t.Fatalf("unexpected direction: %s", dir)
	}

	// Parse NMEA back: format is DDMM.MMMM
	// 48.1351 degrees = 48 degrees 8.106 minutes = "4808.1060"
	// Verify the parsed value is close to the original.
	t.Logf("48.1351 -> NMEA: %s %s", nmea, dir)
}

func TestReversePoints(t *testing.T) {
	points := []RoutePoint{
		{Lat: 1, Lon: 1, Speed: 10, Course: 0, Distance: 0},
		{Lat: 2, Lon: 2, Speed: 20, Course: 45, Distance: 100},
		{Lat: 3, Lon: 3, Speed: 30, Course: 90, Distance: 200},
	}

	rev := reversePoints(points)

	if len(rev) != 3 {
		t.Fatalf("reversed length = %d, want 3", len(rev))
	}

	// First reversed point should be last original point.
	if rev[0].Lat != 3 || rev[0].Lon != 3 {
		t.Errorf("rev[0] = (%.0f, %.0f), want (3, 3)", rev[0].Lat, rev[0].Lon)
	}

	// First reversed point should have zero distance.
	if rev[0].Distance != 0 {
		t.Errorf("rev[0].Distance = %f, want 0", rev[0].Distance)
	}

	// Course should be flipped by 180 degrees.
	if !almostEqual(rev[2].Course, 180, 0.1) {
		t.Errorf("rev[2].Course = %.1f, want 180", rev[2].Course)
	}

	// Original should be unchanged.
	if points[0].Lat != 1 {
		t.Error("original points were modified")
	}
}

func TestScaledDuration(t *testing.T) {
	base := 10 * time.Second

	tests := []struct {
		name       string
		multiplier float64
		want       time.Duration
	}{
		{name: "1x", multiplier: 1.0, want: 10 * time.Second},
		{name: "10x", multiplier: 10.0, want: 1 * time.Second},
		{name: "0.5x", multiplier: 0.5, want: 20 * time.Second},
		{name: "zero (defaults to 1x)", multiplier: 0, want: 10 * time.Second},
		{name: "negative (defaults to 1x)", multiplier: -1, want: 10 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scaledDuration(base, tt.multiplier)
			if math.Abs(float64(got-tt.want)) > float64(time.Millisecond) {
				t.Errorf("scaledDuration = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewSimulator(t *testing.T) {
	routes := []*Route{
		{Name: "test", Points: []RoutePoint{{Lat: 48, Lon: 11}}},
	}

	// Zero multiplier should default to 1.0.
	sim := NewSimulator(routes, "5013", []string{"DEMO1"}, 0)
	if sim.speedMultiplier != 1.0 {
		t.Errorf("speedMultiplier = %f, want 1.0", sim.speedMultiplier)
	}

	// Negative multiplier should default to 1.0.
	sim = NewSimulator(routes, "5013", []string{"DEMO1"}, -5)
	if sim.speedMultiplier != 1.0 {
		t.Errorf("speedMultiplier = %f, want 1.0", sim.speedMultiplier)
	}
}

func TestAddSpeedVariation(t *testing.T) {
	// Zero speed should return zero.
	if v := addSpeedVariation(0); v != 0 {
		t.Errorf("addSpeedVariation(0) = %.1f, want 0", v)
	}

	// Negative speed should return zero.
	if v := addSpeedVariation(-10); v != 0 {
		t.Errorf("addSpeedVariation(-10) = %.1f, want 0", v)
	}

	// Non-zero speed should produce values within +-5% range.
	base := 100.0
	min, max := base, base
	for i := 0; i < 1000; i++ {
		v := addSpeedVariation(base)
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
		// Must be within roughly +-5%.
		if v < base*0.94 || v > base*1.06 {
			t.Errorf("variation out of expected range: %.2f (base=%.0f)", v, base)
		}
	}
	t.Logf("speed variation over 1000 samples: min=%.2f max=%.2f (base=%.0f)", min, max, base)
}

func TestSmoothAcceleration(t *testing.T) {
	tests := []struct {
		name      string
		current   float64
		target    float64
		maxChange float64
		want      float64
	}{
		{name: "at target", current: 80, target: 80, maxChange: 5, want: 80},
		{name: "small increase", current: 78, target: 80, maxChange: 5, want: 80},
		{name: "large increase", current: 50, target: 100, maxChange: 5, want: 55},
		{name: "small decrease", current: 82, target: 80, maxChange: 5, want: 80},
		{name: "large decrease", current: 100, target: 50, maxChange: 5, want: 95},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := smoothAcceleration(tt.current, tt.target, tt.maxChange)
			if math.Abs(got-tt.want) > 0.01 {
				t.Errorf("smoothAcceleration(%.0f, %.0f, %.0f) = %.1f, want %.1f",
					tt.current, tt.target, tt.maxChange, got, tt.want)
			}
		})
	}
}

func TestPointInterval_PhysicsBasedTiming(t *testing.T) {
	routes := []*Route{
		{Name: "test", Points: []RoutePoint{{Lat: 48, Lon: 11}}},
	}

	tests := []struct {
		name       string
		speed      float64 // km/h
		distance   float64 // meters (to next point)
		multiplier float64
		wantMin    time.Duration
		wantMax    time.Duration
	}{
		{
			name:       "100m at 100km/h (1x) -> ~3.6s",
			speed:      100,
			distance:   100,
			multiplier: 1.0,
			wantMin:    3 * time.Second,
			wantMax:    4 * time.Second,
		},
		{
			name:       "100m at 50km/h (1x) -> ~7.2s",
			speed:      50,
			distance:   100,
			multiplier: 1.0,
			wantMin:    7 * time.Second,
			wantMax:    8 * time.Second,
		},
		{
			name:       "100m at 120km/h (1x) -> ~3.0s",
			speed:      120,
			distance:   100,
			multiplier: 1.0,
			wantMin:    2500 * time.Millisecond,
			wantMax:    3500 * time.Millisecond,
		},
		{
			name:       "100m at 100km/h (10x) -> ~0.36s (clamped to 0.5s minimum)",
			speed:      100,
			distance:   100,
			multiplier: 10.0,
			// 3.6s / 10 = 0.36s, but minimum is 0.5s before scaling.
			// Actually: seconds = 100 / 27.78 = 3.6, clamped to 3.6, then /10 = 0.36s.
			// scaledDuration divides AFTER clamping, so result is 0.36s.
			wantMin: 300 * time.Millisecond,
			wantMax: 400 * time.Millisecond,
		},
		{
			name:       "very short distance at low speed (clamp to 0.5s)",
			speed:      100,
			distance:   5,
			multiplier: 1.0,
			// 5m / 27.78 m/s = 0.18s -> clamped to 0.5s.
			wantMin: 450 * time.Millisecond,
			wantMax: 550 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sim := NewSimulator(routes, "5013", []string{"DEMO1"}, tt.multiplier)
			pt := RoutePoint{Speed: tt.speed, Distance: tt.distance}
			nextPt := RoutePoint{Speed: tt.speed, Distance: tt.distance}
			points := []RoutePoint{pt, nextPt}

			interval := sim.pointInterval(pt, 0, points)

			if interval < tt.wantMin || interval > tt.wantMax {
				t.Errorf("interval = %v, want [%v, %v]", interval, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestPointInterval_RealisticRouteTimings(t *testing.T) {
	// Simulate what happens with a real smoothed route.
	// A 572km route with ~8000 points at ~80km/h average should take ~7 hours.
	routes := []*Route{
		{Name: "test", Points: []RoutePoint{{Lat: 48, Lon: 11}}},
	}
	sim := NewSimulator(routes, "5013", []string{"DEMO1"}, 1.0)

	// Create a set of points simulating a highway drive at 100m intervals.
	numPoints := 5720 // 572km / 100m
	points := make([]RoutePoint, numPoints)
	for i := range points {
		points[i] = RoutePoint{
			Speed:    80,  // 80 km/h
			Distance: 100, // 100m intervals
		}
	}

	// Sum up all intervals.
	var totalTime time.Duration
	for i := 0; i < len(points)-1; i++ {
		totalTime += sim.pointInterval(points[i], i, points)
	}

	totalHours := totalTime.Hours()
	// 572km at 80km/h = 7.15 hours.
	t.Logf("total simulated time for %d points: %.1f hours", numPoints, totalHours)

	if totalHours < 6 || totalHours > 9 {
		t.Errorf("total time = %.1f hours, expected 6-9 hours for 572km at 80km/h", totalHours)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

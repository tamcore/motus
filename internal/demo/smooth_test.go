package demo

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestSmoothRoute_EmptyAndSingle(t *testing.T) {
	// Empty route.
	r := &Route{Name: "empty"}
	smoothed := SmoothRoute(r)
	if smoothed.Name != "empty" {
		t.Error("name changed")
	}
	if len(smoothed.Points) != 0 {
		t.Errorf("expected 0 points, got %d", len(smoothed.Points))
	}

	// Single point.
	r = &Route{
		Name:   "single",
		Points: []RoutePoint{{Lat: 48.0, Lon: 11.0}},
	}
	smoothed = SmoothRoute(r)
	if len(smoothed.Points) != 1 {
		t.Errorf("expected 1 point, got %d", len(smoothed.Points))
	}
}

func TestEstimateSpeeds_NoSpeedData(t *testing.T) {
	points := []RoutePoint{
		{Lat: 48.0, Lon: 11.0, Speed: 0, Distance: 0},
		{Lat: 48.001, Lon: 11.001, Speed: 0, Distance: 100, Course: 45},
		{Lat: 48.002, Lon: 11.002, Speed: 0, Distance: 150, Course: 45},
		{Lat: 48.010, Lon: 11.010, Speed: 0, Distance: 1000, Course: 45},
	}

	estimated := estimateSpeeds(points)

	// All points should now have non-zero speeds.
	for i, p := range estimated {
		if p.Speed <= 0 {
			t.Errorf("point %d: speed = %.1f, expected > 0", i, p.Speed)
		}
	}

	// First point should have minimum starting speed.
	if estimated[0].Speed != minMovingSpeed {
		t.Errorf("first point speed = %.1f, want %.1f", estimated[0].Speed, minMovingSpeed)
	}

	// Longer segment should have higher estimated speed than short one.
	if estimated[3].Speed <= estimated[1].Speed {
		t.Errorf("long segment speed (%.1f) should be > short segment speed (%.1f)",
			estimated[3].Speed, estimated[1].Speed)
	}

	// Original points should be unchanged.
	for _, p := range points {
		if p.Speed != 0 {
			t.Error("original points were modified")
		}
	}
}

func TestEstimateSpeeds_WithExistingSpeed(t *testing.T) {
	points := []RoutePoint{
		{Lat: 48.0, Lon: 11.0, Speed: 50, Distance: 0},
		{Lat: 48.001, Lon: 11.001, Speed: 80, Distance: 100, Course: 45},
	}

	estimated := estimateSpeeds(points)

	// Should preserve existing speeds.
	if estimated[0].Speed != 50 {
		t.Errorf("point 0 speed = %.1f, want 50", estimated[0].Speed)
	}
	if estimated[1].Speed != 80 {
		t.Errorf("point 1 speed = %.1f, want 80", estimated[1].Speed)
	}
}

func TestSpeedForSegment(t *testing.T) {
	tests := []struct {
		name      string
		dist      float64
		turn      float64
		minExpect float64
		maxExpect float64
	}{
		{name: "short city segment", dist: 20, turn: 0, minExpect: 20, maxExpect: 40},
		{name: "medium urban", dist: 60, turn: 0, minExpect: 40, maxExpect: 60},
		{name: "medium road", dist: 150, turn: 0, minExpect: 60, maxExpect: 80},
		{name: "long road", dist: 400, turn: 0, minExpect: 80, maxExpect: 100},
		{name: "highway", dist: 1000, turn: 0, minExpect: 100, maxExpect: 120},
		{name: "sharp turn", dist: 200, turn: 70, minExpect: 20, maxExpect: 40},
		{name: "moderate turn", dist: 200, turn: 40, minExpect: 35, maxExpect: 55},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			speed := speedForSegment(tt.dist, tt.turn)
			if speed < tt.minExpect || speed > tt.maxExpect {
				t.Errorf("speed = %.1f, want [%.0f, %.0f]", speed, tt.minExpect, tt.maxExpect)
			}
		})
	}
}

func TestAngleDiff(t *testing.T) {
	tests := []struct {
		name string
		a, b float64
		want float64
	}{
		{name: "same", a: 90, b: 90, want: 0},
		{name: "right turn", a: 0, b: 90, want: 90},
		{name: "left turn", a: 90, b: 0, want: -90},
		{name: "wrap around", a: 350, b: 10, want: 20},
		{name: "wrap around reverse", a: 10, b: 350, want: -20},
		{name: "opposite", a: 0, b: 180, want: 180},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := angleDiff(tt.a, tt.b)
			if math.Abs(got-tt.want) > 0.1 {
				t.Errorf("angleDiff(%.0f, %.0f) = %.1f, want %.1f", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestInterpolateRoute_NoLongSegments(t *testing.T) {
	// All segments are short -- no interpolation needed.
	points := []RoutePoint{
		{Lat: 48.0, Lon: 11.0, Speed: 50, Distance: 0},
		{Lat: 48.0005, Lon: 11.0005, Speed: 50, Distance: 50, Course: 45},
		{Lat: 48.001, Lon: 11.001, Speed: 50, Distance: 50, Course: 45},
	}

	result := interpolateRoute(points, defaultInterpolationInterval)

	if len(result) != len(points) {
		t.Errorf("expected %d points, got %d (no interpolation needed)", len(points), len(result))
	}
}

func TestInterpolateRoute_LongSegment(t *testing.T) {
	// One long segment that should be split.
	points := []RoutePoint{
		{Lat: 48.0, Lon: 11.0, Speed: 50, Distance: 0, Course: 0},
		{Lat: 48.01, Lon: 11.01, Speed: 80, Distance: 1000, Course: 45},
	}

	result := interpolateRoute(points, defaultInterpolationInterval)

	// 1000m / 100m interval = 10 sub-segments, so expect 11 points (start + 9 interp + end).
	expectedPoints := 11
	if len(result) != expectedPoints {
		t.Errorf("expected %d points for 1000m segment at 100m interval, got %d", expectedPoints, len(result))
	}

	// First and last points should match originals.
	if result[0].Lat != points[0].Lat || result[0].Lon != points[0].Lon {
		t.Error("first point changed")
	}
	last := result[len(result)-1]
	if math.Abs(last.Lat-points[1].Lat) > 0.0001 || math.Abs(last.Lon-points[1].Lon) > 0.0001 {
		t.Error("last point diverged from original")
	}

	// All intermediate points should be between start and end.
	for i, p := range result {
		if p.Lat < 47.99 || p.Lat > 48.02 {
			t.Errorf("point %d lat %.6f out of range", i, p.Lat)
		}
		if p.Lon < 10.99 || p.Lon > 11.02 {
			t.Errorf("point %d lon %.6f out of range", i, p.Lon)
		}
	}

	// Speeds should be interpolated between 50 and 80.
	for i := 1; i < len(result)-1; i++ {
		if result[i].Speed < 49 || result[i].Speed > 81 {
			t.Errorf("interpolated point %d speed = %.1f, expected between 50 and 80", i, result[i].Speed)
		}
	}
}

func TestInterpolateBearing(t *testing.T) {
	tests := []struct {
		name     string
		from, to float64
		ratio    float64
		wantMin  float64
		wantMax  float64
	}{
		{name: "midpoint same", from: 90, to: 90, ratio: 0.5, wantMin: 89, wantMax: 91},
		{name: "midpoint 0-90", from: 0, to: 90, ratio: 0.5, wantMin: 44, wantMax: 46},
		{name: "wrap midpoint", from: 350, to: 10, ratio: 0.5, wantMin: 359, wantMax: 361},
		{name: "start", from: 0, to: 90, ratio: 0, wantMin: -1, wantMax: 1},
		{name: "end", from: 0, to: 90, ratio: 1, wantMin: 89, wantMax: 91},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := interpolateBearing(tt.from, tt.to, tt.ratio)
			// Normalize to [0, 360) for comparison.
			got = math.Mod(got+360, 360)
			wantMin := math.Mod(tt.wantMin+360, 360)
			wantMax := math.Mod(tt.wantMax+360, 360)

			if wantMin > wantMax {
				// Range wraps around 360.
				if got < wantMin && got > wantMax {
					t.Errorf("bearing = %.1f, want [%.0f, %.0f]", got, tt.wantMin, tt.wantMax)
				}
			} else {
				if got < wantMin || got > wantMax {
					t.Errorf("bearing = %.1f, want [%.0f, %.0f]", got, tt.wantMin, tt.wantMax)
				}
			}
		})
	}
}

func TestSmoothSpeeds(t *testing.T) {
	// Create a speed profile with an abrupt spike.
	points := make([]RoutePoint, 20)
	for i := range points {
		points[i].Speed = 80
		points[i].Distance = 100
	}
	// Add a spike at point 10.
	points[10].Speed = 200

	smoothed := smoothSpeeds(points)

	// The spike should be dampened.
	if smoothed[10].Speed >= 200 {
		t.Errorf("spike not smoothed: speed = %.1f", smoothed[10].Speed)
	}

	// Neighbours should be slightly elevated.
	if smoothed[9].Speed <= 80 {
		t.Logf("point 9 speed = %.1f (slightly elevated as expected)", smoothed[9].Speed)
	}

	// First and last points should be unchanged.
	if smoothed[0].Speed != points[0].Speed {
		t.Errorf("first point changed: %.1f -> %.1f", points[0].Speed, smoothed[0].Speed)
	}
	if smoothed[len(smoothed)-1].Speed != points[len(points)-1].Speed {
		t.Errorf("last point changed: %.1f -> %.1f",
			points[len(points)-1].Speed, smoothed[len(smoothed)-1].Speed)
	}
}

func TestEnforceAccelerationLimits(t *testing.T) {
	// Abrupt acceleration from 20 to 120.
	points := []RoutePoint{
		{Speed: 20, Distance: 100},
		{Speed: 120, Distance: 100},
		{Speed: 120, Distance: 100},
		{Speed: 120, Distance: 100},
	}

	result := enforceAccelerationLimits(points)

	// First point should stay at 20 (but min clamp might raise it).
	if result[0].Speed < 20 {
		t.Errorf("point 0 speed = %.1f, expected >= 20", result[0].Speed)
	}

	// Second point should not jump more than maxAcceleration from first.
	maxAllowed := result[0].Speed + maxAcceleration
	if result[1].Speed > maxAllowed+0.01 {
		t.Errorf("point 1 speed = %.1f, max allowed = %.1f", result[1].Speed, maxAllowed)
	}

	// All speeds should be at least minMovingSpeed.
	for i, p := range result {
		if p.Speed < minMovingSpeed {
			t.Errorf("point %d speed = %.1f, below minimum %.1f", i, p.Speed, minMovingSpeed)
		}
	}
}

func TestEnforceAccelerationLimits_Deceleration(t *testing.T) {
	// Abrupt deceleration from 120 to 20.
	points := []RoutePoint{
		{Speed: 120, Distance: 100},
		{Speed: 120, Distance: 100},
		{Speed: 120, Distance: 100},
		{Speed: 20, Distance: 100},
	}

	result := enforceAccelerationLimits(points)

	// The backward pass should reduce speeds before the deceleration point.
	// Point 2 (before deceleration) should be limited.
	if result[2].Speed > result[3].Speed+maxAcceleration+0.01 {
		t.Errorf("deceleration not enforced: point 2 = %.1f, point 3 = %.1f",
			result[2].Speed, result[3].Speed)
	}
}

func TestSmoothRoute_Integration(t *testing.T) {
	// Build a realistic route segment.
	route := &Route{
		Name: "test route",
		Points: []RoutePoint{
			{Lat: 48.0, Lon: 11.0, Speed: 0, Distance: 0, Course: 0},
			{Lat: 48.001, Lon: 11.001, Speed: 0, Distance: 100, Course: 45},
			{Lat: 48.005, Lon: 11.005, Speed: 0, Distance: 500, Course: 45},
			{Lat: 48.010, Lon: 11.010, Speed: 0, Distance: 600, Course: 45},
			{Lat: 48.012, Lon: 11.012, Speed: 0, Distance: 250, Course: 45},
			{Lat: 48.013, Lon: 11.013, Speed: 0, Distance: 100, Course: 45},
		},
	}

	smoothed := SmoothRoute(route)

	// Should have more points due to interpolation of long segments.
	if len(smoothed.Points) <= len(route.Points) {
		t.Errorf("expected more points after interpolation: %d -> %d",
			len(route.Points), len(smoothed.Points))
	}

	// All points should have non-zero speed.
	for i, p := range smoothed.Points {
		if p.Speed <= 0 {
			t.Errorf("point %d has zero speed after smoothing", i)
		}
	}

	// Speed changes between consecutive points should be bounded.
	for i := 1; i < len(smoothed.Points); i++ {
		diff := math.Abs(smoothed.Points[i].Speed - smoothed.Points[i-1].Speed)
		if diff > maxAcceleration+0.01 {
			t.Errorf("points %d-%d: speed change %.1f exceeds max %.1f (%.1f -> %.1f)",
				i-1, i, diff, maxAcceleration,
				smoothed.Points[i-1].Speed, smoothed.Points[i].Speed)
		}
	}

	t.Logf("route: %d points -> %d points", len(route.Points), len(smoothed.Points))
	for i, p := range smoothed.Points {
		if i < 5 || i >= len(smoothed.Points)-3 {
			t.Logf("  [%d] lat=%.4f lon=%.4f speed=%.1f course=%.1f dist=%.1f",
				i, p.Lat, p.Lon, p.Speed, p.Course, p.Distance)
		}
	}
}

func TestSmoothRoute_RealRoutes(t *testing.T) {
	dir := filepath.Join(findProjectRoot(t), "data", "demo")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skip("demo data directory not found")
	}

	routes, err := LoadRoutes(dir)
	if err != nil {
		t.Fatalf("LoadRoutes: %v", err)
	}

	for _, r := range routes {
		t.Run(r.Name, func(t *testing.T) {
			smoothed := SmoothRoute(r)

			t.Logf("original: %d points, %.0f km", len(r.Points), r.TotalDistance())
			t.Logf("smoothed: %d points, %.0f km", len(smoothed.Points), smoothed.TotalDistance())

			// Smoothed route should have at least as many points.
			if len(smoothed.Points) < len(r.Points) {
				t.Errorf("smoothed has fewer points: %d < %d", len(smoothed.Points), len(r.Points))
			}

			// Total distance should be approximately preserved.
			origDist := r.TotalDistance()
			smoothDist := smoothed.TotalDistance()
			diffPct := math.Abs(smoothDist-origDist) / origDist * 100
			if diffPct > 5 {
				t.Errorf("total distance changed by %.1f%% (%.0f -> %.0f km)",
					diffPct, origDist, smoothDist)
			}

			// All speeds should be positive.
			zeroSpeed := 0
			for _, p := range smoothed.Points {
				if p.Speed <= 0 {
					zeroSpeed++
				}
			}
			if zeroSpeed > 0 {
				t.Errorf("%d points still have zero speed", zeroSpeed)
			}

			// Speed transitions should be bounded.
			maxDelta := 0.0
			for i := 1; i < len(smoothed.Points); i++ {
				delta := math.Abs(smoothed.Points[i].Speed - smoothed.Points[i-1].Speed)
				if delta > maxDelta {
					maxDelta = delta
				}
			}
			t.Logf("max speed delta between consecutive points: %.1f km/h", maxDelta)
			if maxDelta > maxAcceleration+0.1 {
				t.Errorf("max speed delta %.1f exceeds maxAcceleration %.1f", maxDelta, maxAcceleration)
			}

			// Sample a few points for sanity.
			sample := []int{0, len(smoothed.Points) / 4, len(smoothed.Points) / 2, len(smoothed.Points) - 1}
			for _, idx := range sample {
				p := smoothed.Points[idx]
				t.Logf("  [%d/%d] lat=%.4f lon=%.4f speed=%.1f course=%.0f dist=%.0f",
					idx, len(smoothed.Points), p.Lat, p.Lon, p.Speed, p.Course, p.Distance)
			}
		})
	}
}

func TestSmoothRouteWithInterval(t *testing.T) {
	route := &Route{
		Name: "interval test",
		Points: []RoutePoint{
			{Lat: 48.0, Lon: 11.0, Speed: 0, Distance: 0, Course: 0},
			{Lat: 48.010, Lon: 11.010, Speed: 0, Distance: 1000, Course: 45},
		},
	}

	tests := []struct {
		name     string
		interval float64
		wantMin  int // minimum expected points
		wantMax  int // maximum expected points
	}{
		{
			name:     "50m interval produces many points",
			interval: 50.0,
			wantMin:  18, // 1000/50 = 20 sub-segments + smoothing effects
			wantMax:  25,
		},
		{
			name:     "100m interval (default)",
			interval: 100.0,
			wantMin:  9, // 1000/100 = 10 sub-segments + smoothing effects
			wantMax:  15,
		},
		{
			name:     "200m interval produces fewer points",
			interval: 200.0,
			wantMin:  4, // 1000/200 = 5 sub-segments + smoothing effects
			wantMax:  10,
		},
		{
			name:     "zero interval uses default (100m)",
			interval: 0,
			wantMin:  9,
			wantMax:  15,
		},
		{
			name:     "negative interval uses default (100m)",
			interval: -50,
			wantMin:  9,
			wantMax:  15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			smoothed := SmoothRouteWithInterval(route, tt.interval)
			count := len(smoothed.Points)
			t.Logf("interval=%.0fm -> %d points", tt.interval, count)
			if count < tt.wantMin || count > tt.wantMax {
				t.Errorf("point count = %d, want [%d, %d]", count, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestSmoothRouteWithInterval_PreservesDistance(t *testing.T) {
	// Verify that total distance is preserved regardless of interval.
	route := &Route{
		Name: "distance preservation test",
		Points: []RoutePoint{
			{Lat: 48.0, Lon: 11.0, Speed: 80, Distance: 0, Course: 0},
			{Lat: 48.005, Lon: 11.005, Speed: 80, Distance: 500, Course: 45},
			{Lat: 48.015, Lon: 11.015, Speed: 100, Distance: 1200, Course: 45},
			{Lat: 48.020, Lon: 11.020, Speed: 80, Distance: 600, Course: 45},
		},
	}

	origDist := route.TotalDistance()

	intervals := []float64{50, 100, 200, 500}
	for _, interval := range intervals {
		smoothed := SmoothRouteWithInterval(route, interval)
		smoothDist := smoothed.TotalDistance()
		diffPct := math.Abs(smoothDist-origDist) / origDist * 100
		t.Logf("interval=%.0fm: %.1f km -> %.1f km (diff %.1f%%)", interval, origDist, smoothDist, diffPct)
		if diffPct > 5 {
			t.Errorf("interval %.0fm: distance changed by %.1f%%", interval, diffPct)
		}
	}
}

func TestInterpolateRoute_CustomInterval(t *testing.T) {
	points := []RoutePoint{
		{Lat: 48.0, Lon: 11.0, Speed: 80, Distance: 0, Course: 0},
		{Lat: 48.01, Lon: 11.01, Speed: 100, Distance: 1000, Course: 45},
	}

	tests := []struct {
		name       string
		interval   float64
		wantPoints int
	}{
		{name: "50m", interval: 50, wantPoints: 21},    // 1000/50=20 segments + start
		{name: "100m", interval: 100, wantPoints: 11},  // 1000/100=10 segments + start
		{name: "200m", interval: 200, wantPoints: 6},   // 1000/200=5 segments + start
		{name: "500m", interval: 500, wantPoints: 3},   // 1000/500=2 segments + start
		{name: "1000m", interval: 1000, wantPoints: 2}, // 1000/1000=1 segment + start (no split)
		{name: "2000m", interval: 2000, wantPoints: 2}, // segment shorter than interval
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := interpolateRoute(points, tt.interval)
			if len(result) != tt.wantPoints {
				t.Errorf("interval=%.0f: got %d points, want %d", tt.interval, len(result), tt.wantPoints)
			}
		})
	}
}

func TestInterpolateRoute_NegativeAndZeroInterval(t *testing.T) {
	points := []RoutePoint{
		{Lat: 48.0, Lon: 11.0, Speed: 80, Distance: 0, Course: 0},
		{Lat: 48.01, Lon: 11.01, Speed: 100, Distance: 500, Course: 45},
	}

	// Zero interval should use default (100m).
	result := interpolateRoute(points, 0)
	// 500m / 100m = 5 segments + start = 6 points.
	if len(result) != 6 {
		t.Errorf("zero interval: got %d points, want 6", len(result))
	}

	// Negative interval should use default (100m).
	result = interpolateRoute(points, -100)
	if len(result) != 6 {
		t.Errorf("negative interval: got %d points, want 6", len(result))
	}
}

func TestSmoothRouteWithInterval_RealRoutes(t *testing.T) {
	dir := filepath.Join(findProjectRoot(t), "data", "demo")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skip("demo data directory not found")
	}

	routes, err := LoadRoutes(dir)
	if err != nil {
		t.Fatalf("LoadRoutes: %v", err)
	}

	intervals := []float64{50, 100, 200}

	for _, r := range routes {
		for _, interval := range intervals {
			testName := fmt.Sprintf("%s/%.0fm", r.Name, interval)
			t.Run(testName, func(t *testing.T) {
				smoothed := SmoothRouteWithInterval(r, interval)
				totalKm := smoothed.TotalDistance()

				// Calculate estimated traversal time at average speed.
				avgSpeed := 0.0
				for _, p := range smoothed.Points {
					avgSpeed += p.Speed
				}
				avgSpeed /= float64(len(smoothed.Points))

				// Expected time = distance / speed.
				expectedHours := totalKm / avgSpeed

				t.Logf("interval=%.0fm: %d points, %.0f km, avg %.0f km/h, est %.1f hours",
					interval, len(smoothed.Points), totalKm, avgSpeed, expectedHours)

				// With 1x speed multiplier, a 572km route at ~80km/h should take ~7 hours.
				if totalKm > 500 && expectedHours < 4 {
					t.Errorf("route seems too fast: %.1f hours for %.0f km", expectedHours, totalKm)
				}
			})
		}
	}
}

func TestCopyPoints(t *testing.T) {
	original := []RoutePoint{
		{Lat: 1, Lon: 2, Speed: 50},
		{Lat: 3, Lon: 4, Speed: 80},
	}

	copied := copyPoints(original)

	// Modify copy.
	copied[0].Speed = 999

	// Original should be unchanged.
	if original[0].Speed != 50 {
		t.Error("copyPoints did not make a true copy")
	}
}

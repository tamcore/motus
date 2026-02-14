package demo

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestParseGPXFile(t *testing.T) {
	// Create a minimal GPX file for testing.
	gpxContent := `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="test" xmlns="http://www.topografix.com/GPX/1/1">
  <metadata><name>Test Route</name></metadata>
  <trk>
    <name>Test Track</name>
    <trkseg>
      <trkpt lat="48.1351" lon="11.5820"><ele>519</ele><speed>40</speed></trkpt>
      <trkpt lat="48.1540" lon="11.5830"><ele>510</ele><speed>50</speed></trkpt>
      <trkpt lat="52.5200" lon="13.4050"><ele>35</ele><speed>0</speed></trkpt>
    </trkseg>
  </trk>
</gpx>`

	dir := t.TempDir()
	path := filepath.Join(dir, "test.gpx")
	if err := os.WriteFile(path, []byte(gpxContent), 0644); err != nil {
		t.Fatalf("write test GPX file: %v", err)
	}

	gpx, err := ParseGPXFile(path)
	if err != nil {
		t.Fatalf("ParseGPXFile: %v", err)
	}

	if gpx.Metadata.Name != "Test Route" {
		t.Errorf("metadata name = %q, want %q", gpx.Metadata.Name, "Test Route")
	}

	if len(gpx.Tracks) != 1 {
		t.Fatalf("tracks = %d, want 1", len(gpx.Tracks))
	}

	track := gpx.Tracks[0]
	if track.Name != "Test Track" {
		t.Errorf("track name = %q, want %q", track.Name, "Test Track")
	}

	if len(track.Segments) != 1 {
		t.Fatalf("segments = %d, want 1", len(track.Segments))
	}

	points := track.Segments[0].Points
	if len(points) != 3 {
		t.Fatalf("points = %d, want 3", len(points))
	}

	// Verify first point.
	if points[0].Lat != 48.1351 || points[0].Lon != 11.5820 {
		t.Errorf("point[0] = (%.4f, %.4f), want (48.1351, 11.5820)", points[0].Lat, points[0].Lon)
	}
	if points[0].Speed != 40 {
		t.Errorf("point[0].Speed = %.0f, want 40", points[0].Speed)
	}
}

func TestParseGPXFile_NotFound(t *testing.T) {
	_, err := ParseGPXFile("/nonexistent/file.gpx")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestParseGPXFile_InvalidXML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.gpx")
	if err := os.WriteFile(path, []byte("not xml"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseGPXFile(path)
	if err == nil {
		t.Error("expected error for invalid XML")
	}
}

func TestLoadRoutes(t *testing.T) {
	dir := t.TempDir()

	gpxContent := `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="test" xmlns="http://www.topografix.com/GPX/1/1">
  <trk>
    <name>Route A</name>
    <trkseg>
      <trkpt lat="48.1351" lon="11.5820"><speed>50</speed></trkpt>
      <trkpt lat="49.4521" lon="11.0767"><speed>120</speed></trkpt>
      <trkpt lat="52.5200" lon="13.4050"><speed>0</speed></trkpt>
    </trkseg>
  </trk>
</gpx>`

	if err := os.WriteFile(filepath.Join(dir, "route1.gpx"), []byte(gpxContent), 0644); err != nil {
		t.Fatal(err)
	}

	routes, err := LoadRoutes(dir)
	if err != nil {
		t.Fatalf("LoadRoutes: %v", err)
	}

	if len(routes) != 1 {
		t.Fatalf("routes = %d, want 1", len(routes))
	}

	route := routes[0]
	if route.Name != "Route A" {
		t.Errorf("route name = %q, want %q", route.Name, "Route A")
	}

	if len(route.Points) != 3 {
		t.Fatalf("points = %d, want 3", len(route.Points))
	}

	// First point should have zero distance.
	if route.Points[0].Distance != 0 {
		t.Errorf("first point distance = %f, want 0", route.Points[0].Distance)
	}

	// Second point should have non-zero distance (Munich to Nuremberg ~150km).
	dist := route.Points[1].Distance
	if dist < 100000 || dist > 200000 {
		t.Errorf("Munich-Nuremberg distance = %.0fm, expected 100-200km", dist)
	}

	// Total distance should be positive.
	total := route.TotalDistance()
	if total < 400 || total > 700 {
		t.Errorf("total distance = %.0f km, expected 400-700km", total)
	}
}

func TestLoadRoutes_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadRoutes(dir)
	if err == nil {
		t.Error("expected error for empty directory")
	}
}

func TestLoadRoutes_NonexistentDir(t *testing.T) {
	_, err := LoadRoutes("/nonexistent/dir")
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestHaversineDistance(t *testing.T) {
	tests := []struct {
		name    string
		lat1    float64
		lon1    float64
		lat2    float64
		lon2    float64
		wantMin float64 // meters
		wantMax float64
	}{
		{
			name: "same point",
			lat1: 48.1351, lon1: 11.5820,
			lat2: 48.1351, lon2: 11.5820,
			wantMin: 0, wantMax: 1,
		},
		{
			name: "Munich to Berlin (~500km)",
			lat1: 48.1351, lon1: 11.5820,
			lat2: 52.5200, lon2: 13.4050,
			wantMin: 480000, wantMax: 520000,
		},
		{
			name: "Frankfurt to Hamburg (~450km)",
			lat1: 50.1109, lon1: 8.6821,
			lat2: 53.5511, lon2: 9.9937,
			wantMin: 380000, wantMax: 420000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := haversineDistance(tt.lat1, tt.lon1, tt.lat2, tt.lon2)
			if d < tt.wantMin || d > tt.wantMax {
				t.Errorf("distance = %.0fm, want [%.0f, %.0f]", d, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestBearing(t *testing.T) {
	tests := []struct {
		name    string
		lat1    float64
		lon1    float64
		lat2    float64
		lon2    float64
		wantMin float64
		wantMax float64
	}{
		{
			name: "due north",
			lat1: 48.0, lon1: 11.0,
			lat2: 49.0, lon2: 11.0,
			wantMin: 0, wantMax: 1,
		},
		{
			name: "due east",
			lat1: 48.0, lon1: 11.0,
			lat2: 48.0, lon2: 12.0,
			wantMin: 89, wantMax: 91,
		},
		{
			name: "due south",
			lat1: 49.0, lon1: 11.0,
			lat2: 48.0, lon2: 11.0,
			wantMin: 179, wantMax: 181,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := bearing(tt.lat1, tt.lon1, tt.lat2, tt.lon2)
			if b < tt.wantMin || b > tt.wantMax {
				t.Errorf("bearing = %.1f, want [%.0f, %.0f]", b, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestLoadRealRoutes(t *testing.T) {
	// Test loading the actual demo routes from the repo.
	dir := filepath.Join(findProjectRoot(t), "data", "demo")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skip("demo data directory not found (not running from repo root)")
	}

	routes, err := LoadRoutes(dir)
	if err != nil {
		t.Fatalf("LoadRoutes: %v", err)
	}

	if len(routes) < 2 {
		t.Fatalf("expected at least 2 routes, got %d", len(routes))
	}

	for _, r := range routes {
		t.Logf("Route %q: %d points, %.0f km", r.Name, len(r.Points), r.TotalDistance())

		if len(r.Points) < 50 {
			t.Errorf("route %q has only %d points, want at least 50", r.Name, len(r.Points))
		}

		total := r.TotalDistance()
		if total < 300 {
			t.Errorf("route %q total distance %.0f km seems too short", r.Name, total)
		}

		// Verify all coordinates are in Germany (roughly).
		for i, p := range r.Points {
			if p.Lat < 47 || p.Lat > 55 || p.Lon < 5 || p.Lon > 16 {
				t.Errorf("route %q point %d (%.4f, %.4f) outside Germany bounds", r.Name, i, p.Lat, p.Lon)
			}
		}
	}
}

// findProjectRoot walks up from the test file to find the project root.
func findProjectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root")
		}
		dir = parent
	}
}

func almostEqual(a, b, tolerance float64) bool {
	return math.Abs(a-b) <= tolerance
}

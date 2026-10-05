// Package demo provides GPS simulation for demo mode.
//
// It loads GPX route files and simulates device movement by injecting
// H02 protocol messages into the local GPS server.
package demo

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tamcore/motus/internal/geo"
)

// GPXFile represents a parsed GPX file structure.
type GPXFile struct {
	XMLName xml.Name   `xml:"gpx"`
	Tracks  []GPXTrack `xml:"trk"`
}

// GPXTrack represents a track in a GPX file.
type GPXTrack struct {
	Name     string       `xml:"name"`
	Segments []GPXSegment `xml:"trkseg"`
}

// GPXSegment represents a track segment.
type GPXSegment struct {
	Points []GPXPoint `xml:"trkpt"`
}

// GPXPoint represents a single trackpoint in a GPX file.
type GPXPoint struct {
	Lat   float64   `xml:"lat,attr"`
	Lon   float64   `xml:"lon,attr"`
	Ele   float64   `xml:"ele"`
	Speed float64   `xml:"speed"` // km/h (custom extension)
	Time  time.Time `xml:"time"`  // RFC 3339; zero value if absent
}

// RoutePoint is a processed point with computed distance and bearing.
type RoutePoint struct {
	Lat      float64 // decimal degrees
	Lon      float64 // decimal degrees
	Ele      float64 // meters
	Speed    float64 // km/h
	Course   float64 // bearing in degrees (0-360)
	Distance float64 // distance from previous point in meters
}

// Route is a complete route ready for simulation.
type Route struct {
	Name   string
	Points []RoutePoint
}

// TotalDistance returns the total route distance in kilometers.
func (r *Route) TotalDistance() float64 {
	var total float64
	for _, p := range r.Points {
		total += p.Distance
	}
	return total / 1000.0
}

// ParseGPXFile reads and parses a single GPX file.
func ParseGPXFile(path string) (*GPXFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read GPX file %s: %w", path, err)
	}

	var gpx GPXFile
	if err := xml.Unmarshal(data, &gpx); err != nil {
		return nil, fmt.Errorf("parse GPX file %s: %w", path, err)
	}

	return &gpx, nil
}

// LoadRoutes loads all GPX files from a directory and returns processed routes.
func LoadRoutes(dir string) ([]*Route, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read GPX directory %s: %w", dir, err)
	}

	var gpxFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".gpx") {
			gpxFiles = append(gpxFiles, filepath.Join(dir, e.Name()))
		}
	}

	if len(gpxFiles) == 0 {
		return nil, fmt.Errorf("no GPX files found in %s", dir)
	}

	var routes []*Route
	for _, path := range gpxFiles {
		gpx, err := ParseGPXFile(path)
		if err != nil {
			return nil, err
		}

		for _, track := range gpx.Tracks {
			route := gpxToRoute(track)
			if len(route.Points) > 0 {
				routes = append(routes, route)
			}
		}
	}

	if len(routes) == 0 {
		return nil, fmt.Errorf("no valid routes found in GPX files")
	}

	return routes, nil
}

// gpxToRoute converts a GPX track to a processed Route with bearings and distances.
func gpxToRoute(track GPXTrack) *Route {
	route := &Route{
		Name: track.Name,
	}

	for _, seg := range track.Segments {
		for i, pt := range seg.Points {
			rp := RoutePoint{
				Lat:   pt.Lat,
				Lon:   pt.Lon,
				Ele:   pt.Ele,
				Speed: pt.Speed,
			}

			if i > 0 {
				prev := seg.Points[i-1]
				rp.Distance = geo.HaversineDistance(prev.Lat, prev.Lon, pt.Lat, pt.Lon) * 1000
				rp.Course = geo.Bearing(prev.Lat, prev.Lon, pt.Lat, pt.Lon)
			}

			route.Points = append(route.Points, rp)
		}
	}

	return route
}

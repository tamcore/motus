package demo

import (
	"math"
	"slices"
)

// Default smoothing parameters.
const (
	// defaultHighwaySpeed is the base speed (km/h) used for long straight
	// segments where the GPX file has no speed data.
	defaultHighwaySpeed = 110.0

	// defaultUrbanSpeed is the base speed (km/h) for short segments with
	// frequent direction changes, typical of city driving.
	defaultUrbanSpeed = 50.0

	// minMovingSpeed is the minimum speed (km/h) assigned to a moving point.
	// Prevents the simulator from treating estimated points as rest stops.
	minMovingSpeed = 20.0

	// speedSmoothingWindow is the number of neighbouring points used on each
	// side when applying a moving-average filter to the speed profile.
	speedSmoothingWindow = 5

	// maxAcceleration is the maximum speed change (km/h) allowed between
	// consecutive points. Larger deltas are clamped to produce gradual
	// acceleration and deceleration.
	maxAcceleration = 8.0
)

// SmoothRouteWithInterval produces a refined route with estimated speeds,
// interpolated points every interval meters and smoothed speed transitions.
// A smaller interval produces more points and smoother visual movement on the map. For example, 100m with a 100 km/h speed yields
// a position update roughly every 3.6 seconds.
//
// The original route is not modified; a new Route is returned.
func SmoothRouteWithInterval(route *Route, interval float64) *Route {
	if len(route.Points) < 2 {
		return route
	}

	// Step 1: estimate speeds from geometry if the GPX has no speed data.
	points := estimateSpeeds(route.Points)

	// Step 2: interpolate long segments to keep map movement smooth.
	points = interpolateRoute(points, interval)

	// Step 3: smooth the speed profile with a moving average.
	points = smoothSpeeds(points)

	// Step 4: enforce gradual acceleration / deceleration.
	points = enforceAccelerationLimits(points)

	return &Route{
		Name:   route.Name,
		Points: points,
	}
}

// estimateSpeeds assigns realistic speeds to route points that have no speed
// data (speed == 0). It uses the inter-point distance and bearing change to
// distinguish highway segments from urban manoeuvring.
func estimateSpeeds(points []RoutePoint) []RoutePoint {
	if slices.ContainsFunc(points, func(p RoutePoint) bool { return p.Speed > 0 }) {
		// GPX has speed data -- keep it as-is.
		return slices.Clone(points)
	}

	out := slices.Clone(points)

	for i := range out {
		if i == 0 {
			// First point: use a gentle start speed.
			out[i].Speed = minMovingSpeed
			continue
		}

		dist := out[i].Distance // meters to previous point
		bearingChange := 0.0
		if i >= 2 {
			bearingChange = angleDiff(out[i-1].Course, out[i].Course)
		}

		out[i].Speed = speedForSegment(dist, bearingChange)
	}

	return out
}

// speedForSegment estimates a realistic driving speed for a road segment
// based on its length and the bearing change from the previous segment.
//
// Longer, straighter segments get higher speeds (highway); short segments
// with sharp turns get lower speeds (city / curves).
func speedForSegment(distMeters, bearingChangeDeg float64) float64 {
	// Base speed from segment length.
	// Short segments (< 50m) are likely city streets.
	// Long segments (> 500m) are likely highways or major roads.
	var base float64
	switch {
	case distMeters < 30:
		base = 30
	case distMeters < 80:
		base = defaultUrbanSpeed
	case distMeters < 200:
		base = 70
	case distMeters < 500:
		base = 90
	default:
		base = defaultHighwaySpeed
	}

	// Reduce speed for sharp turns.
	// A straight road has ~0 degree change; a right-angle turn has ~90.
	absTurn := math.Abs(bearingChangeDeg)
	if absTurn > 60 {
		base *= 0.4
	} else if absTurn > 30 {
		base *= 0.6
	} else if absTurn > 15 {
		base *= 0.8
	}

	return max(base, minMovingSpeed)
}

// angleDiff returns the signed difference between two bearings in degrees,
// normalised to [-180, 180].
func angleDiff(a, b float64) float64 {
	d := b - a
	for d > 180 {
		d -= 360
	}
	for d < -180 {
		d += 360
	}
	return d
}

// interpolateRoute adds intermediate points to segments that exceed the given
// maxInterval (in meters) so the device appears to move smoothly on the map.
// Every segment longer than maxInterval is subdivided into equal sub-segments
// of approximately maxInterval length.
func interpolateRoute(points []RoutePoint, maxInterval float64) []RoutePoint {
	if len(points) < 2 {
		return points
	}

	out := []RoutePoint{points[0]}

	for i := 1; i < len(points); i++ {
		prev := points[i-1]
		cur := points[i]

		dist := cur.Distance
		if dist <= maxInterval {
			out = append(out, cur)
			continue
		}

		// Number of sub-segments needed.
		steps := int(math.Ceil(dist / maxInterval))
		segDist := dist / float64(steps)

		for s := 1; s < steps; s++ {
			ratio := float64(s) / float64(steps)
			interp := RoutePoint{
				Lat:      prev.Lat + (cur.Lat-prev.Lat)*ratio,
				Lon:      prev.Lon + (cur.Lon-prev.Lon)*ratio,
				Ele:      prev.Ele + (cur.Ele-prev.Ele)*ratio,
				Speed:    prev.Speed + (cur.Speed-prev.Speed)*ratio,
				Course:   interpolateBearing(prev.Course, cur.Course, ratio),
				Distance: segDist,
			}
			out = append(out, interp)
		}

		// The final original point with adjusted segment distance.
		final := cur
		final.Distance = segDist
		out = append(out, final)
	}

	return out
}

// interpolateBearing smoothly interpolates between two compass bearings,
// taking the shortest arc around the circle.
func interpolateBearing(from, to, ratio float64) float64 {
	diff := angleDiff(from, to)
	result := from + diff*ratio
	return math.Mod(result+360, 360)
}

// smoothSpeeds applies a moving-average filter to the speed profile so that
// speed doesn't jump abruptly from one point to the next.
func smoothSpeeds(points []RoutePoint) []RoutePoint {
	if len(points) < 3 {
		return points
	}

	out := slices.Clone(points)
	window := speedSmoothingWindow

	for i := 1; i < len(out)-1; i++ {
		lo := max(i-window, 0)
		hi := min(i+window, len(out)-1)

		sum := 0.0
		for j := lo; j <= hi; j++ {
			sum += points[j].Speed
		}

		out[i].Speed = max(sum/float64(hi-lo+1), minMovingSpeed)
	}

	return out
}

// enforceAccelerationLimits walks the points forwards and backwards,
// clamping speed changes to maxAcceleration km/h per step. This creates
// realistic ramp-up and ramp-down curves.
func enforceAccelerationLimits(points []RoutePoint) []RoutePoint {
	if len(points) < 2 {
		return points
	}

	out := slices.Clone(points)

	// Forward pass: limit acceleration.
	for i := 1; i < len(out); i++ {
		diff := out[i].Speed - out[i-1].Speed
		if diff > maxAcceleration {
			out[i].Speed = out[i-1].Speed + maxAcceleration
		}
	}

	// Backward pass: limit deceleration (so a car slows down before a curve).
	for i := len(out) - 2; i >= 0; i-- {
		diff := out[i].Speed - out[i+1].Speed
		if diff > maxAcceleration {
			out[i].Speed = out[i+1].Speed + maxAcceleration
		}
	}

	// Ensure minimum speed.
	for i := range out {
		out[i].Speed = max(out[i].Speed, minMovingSpeed)
	}

	return out
}

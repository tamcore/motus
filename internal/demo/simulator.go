package demo

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"strings"
	"time"
)

// writeDeadlineTimeout is the per-write TCP deadline. If a single write takes
// longer than this, the connection is considered broken.
const writeDeadlineTimeout = 5 * time.Second

// resolveTarget normalises the h02Port field into a "host:port" string.
func resolveTarget(h02Port string) string {
	if !strings.Contains(h02Port, ":") {
		return "localhost:" + h02Port
	}
	return h02Port
}

// Simulator drives GPS simulation for demo mode devices.
// It connects to the local H02 TCP server and injects position messages
// that follow pre-loaded GPX routes.
type Simulator struct {
	routes          []*Route
	h02Port         string
	deviceIMEIs     []string
	speedMultiplier float64
}

// NewSimulator creates a GPS simulator.
//
// Each device IMEI is assigned a route (cycling through available routes).
// speedMultiplier controls playback speed: 1.0 = real time, 10.0 = 10x faster.
func NewSimulator(routes []*Route, h02Port string, deviceIMEIs []string, speedMultiplier float64) *Simulator {
	if speedMultiplier <= 0 {
		speedMultiplier = 1.0
	}
	return &Simulator{
		routes:          routes,
		h02Port:         h02Port,
		deviceIMEIs:     deviceIMEIs,
		speedMultiplier: speedMultiplier,
	}
}

// Start begins simulating all configured devices. It blocks until the context
// is cancelled. Each device runs in its own goroutine.
func (s *Simulator) Start(ctx context.Context) {
	for i, imei := range s.deviceIMEIs {
		route := s.routes[i%len(s.routes)]
		go s.simulateDevice(ctx, imei, route)
	}

	// Block until context is done.
	<-ctx.Done()
}

// simulateDevice runs the simulation loop for a single device.
// It connects to the H02 server with exponential backoff, sends position
// messages following the route, and loops back to the start when the route
// ends. If the connection drops, it reconnects automatically with backoff
// and resumes from the approximate position where it left off.
func (s *Simulator) simulateDevice(ctx context.Context, imei string, route *Route) {
	slog.Info("starting simulation",
		slog.String("device", imei),
		slog.String("route", route.Name),
		slog.Float64("distanceKm", route.TotalDistance()),
		slog.Int("points", len(route.Points)))

	target := resolveTarget(s.h02Port)
	b := newBackoff()
	progress := newRouteProgress()
	reversed := reversePoints(route.Points)

	for {
		select {
		case <-ctx.Done():
			slog.Info("context cancelled, stopping simulation", slog.String("device", imei))
			return
		default:
		}

		// Establish TCP connection with retry + exponential backoff.
		conn, err := connectWithBackoff(ctx, target, &b)
		if err != nil {
			// Context was cancelled during connection retry.
			slog.Info("connection aborted", slog.String("device", imei), slog.Any("error", err))
			return
		}

		// Enable OS-level TCP keepalive to detect dead peers faster.
		if kaErr := enableTCPKeepAlive(conn, tcpKeepAlivePeriod); kaErr != nil {
			slog.Warn("TCP keepalive failed", slog.String("device", imei), slog.Any("error", kaErr))
		}

		w := newConnWriter(conn, writeDeadlineTimeout)
		slog.Info("connected to GPS server",
			slog.String("device", imei),
			slog.String("target", target),
			slog.String("direction", progress.direction.String()),
			slog.Int("pointIndex", progress.pointIndex),
			slog.Int("loopCount", progress.loopCount))

		// Run route traversal with a watchdog that detects stale connections.
		err = s.runWithWatchdog(ctx, w, imei, route, reversed, progress)
		_ = w.Close()

		if err != nil {
			if ctx.Err() != nil {
				slog.Info("stopping simulation (context done)", slog.String("device", imei))
				return
			}
			slog.Warn("connection lost, will reconnect",
				slog.String("device", imei),
				slog.String("direction", progress.direction.String()),
				slog.Int("pointIndex", progress.pointIndex),
				slog.Any("error", err))
			// The backoff state carries over -- connectWithBackoff will use
			// the current backoff delay on the next attempt and reset it
			// once a connection succeeds.
		}
	}
}

// runWithWatchdog runs the route loop alongside a watchdog goroutine.
// If the watchdog detects a stale connection (no successful write within
// the threshold), it cancels the traversal and forces a reconnection.
func (s *Simulator) runWithWatchdog(
	ctx context.Context,
	w *connWriter,
	imei string,
	route *Route,
	reversed []RoutePoint,
	progress *routeProgress,
) error {
	// Create a cancellable context for this connection's lifetime.
	connCtx, connCancel := context.WithCancel(ctx)
	defer connCancel()

	// Start the watchdog in a goroutine.
	watchdogErr := make(chan error, 1)
	go func() {
		watchdogErr <- runWatchdog(connCtx, w, defaultStaleThreshold, defaultWatchdogInterval)
	}()

	// Start the command reader — it owns all reads from w.conn.
	go runCommandReader(connCtx, w, imei)

	// Run the route loop.
	routeErr := make(chan error, 1)
	go func() {
		routeErr <- s.runRouteLoop(connCtx, w, imei, route, reversed, progress)
	}()

	// Wait for either the route loop to finish or the watchdog to fire.
	select {
	case err := <-routeErr:
		connCancel() // Stop the watchdog.
		return err
	case err := <-watchdogErr:
		connCancel() // Stop the route loop.
		if err != nil {
			slog.Warn("watchdog triggered", slog.String("device", imei), slog.Any("error", err))
			return err
		}
		// Watchdog exited cleanly (context cancelled). Wait for route to finish.
		return <-routeErr
	}
}

// runRouteLoop sends route traversals (forward then reverse) on the provided
// connection. It uses routeProgress to resume from where it left off after
// a reconnection, so the device does not teleport back to the start.
func (s *Simulator) runRouteLoop(
	ctx context.Context,
	w *connWriter,
	imei string,
	route *Route,
	reversed []RoutePoint,
	progress *routeProgress,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if progress.direction == directionForward {
			points := route.Points
			startIdx := progress.pointIndex
			slog.Debug("starting traversal",
				slog.String("device", imei),
				slog.String("direction", progress.direction.String()),
				slog.Int("startIdx", startIdx),
				slog.Int("totalPoints", len(points)))

			err := s.traverseRoute(ctx, w, imei, points, startIdx, progress)
			if err != nil {
				return err
			}

			slog.Debug("forward traversal complete, pausing at destination", slog.String("device", imei))
			progress.FinishDirection()

			// Send ignition-off at the destination before pausing.
			if len(route.Points) > 0 {
				dest := route.Points[len(route.Points)-1]
				now := time.Now().UTC()
				parked := BuildH02Message(imei, dest.Lat, dest.Lon, 0, dest.Course, dest.Ele, false, now)
				_ = w.WriteString(parked) // best-effort; connection errors caught by watchdog
			}

			// Pause at destination.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(scaledDuration(30*time.Second, s.speedMultiplier)):
			}
		}

		if progress.direction == directionReverse {
			startIdx := progress.pointIndex
			slog.Debug("starting traversal",
				slog.String("device", imei),
				slog.String("direction", progress.direction.String()),
				slog.Int("startIdx", startIdx),
				slog.Int("totalPoints", len(reversed)))

			err := s.traverseRoute(ctx, w, imei, reversed, startIdx, progress)
			if err != nil {
				return err
			}

			slog.Debug("return traversal complete, pausing at origin", slog.String("device", imei))
			progress.FinishDirection()

			// Send ignition-off at the origin before pausing.
			if len(reversed) > 0 {
				origin := reversed[len(reversed)-1]
				now := time.Now().UTC()
				parked := BuildH02Message(imei, origin.Lat, origin.Lon, 0, origin.Course, origin.Ele, false, now)
				_ = w.WriteString(parked) // best-effort; connection errors caught by watchdog
			}

			// Pause at origin before next loop.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(scaledDuration(60*time.Second, s.speedMultiplier)):
			}
		}
	}
}

// traverseRoute sends H02 messages for each point in the route segment,
// starting at the given index. It updates progress.pointIndex as it goes,
// so a reconnection can resume from the current position.
func (s *Simulator) traverseRoute(
	ctx context.Context,
	w *connWriter,
	imei string,
	points []RoutePoint,
	startIndex int,
	progress *routeProgress,
) error {
	currentSpeed := 0.0
	totalPoints := len(points)

	// If resuming mid-route, estimate the current speed from the resume point
	// to avoid starting from 0 km/h in the middle of a highway.
	if startIndex > 0 && startIndex < totalPoints {
		currentSpeed = points[startIndex].Speed * 0.8 // Slightly below target.
	}

	for i := startIndex; i < totalPoints; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		pt := points[i]

		// Update progress so reconnections know where we are.
		progress.pointIndex = i

		// Skip rest stops (speed == 0) with a pause.
		if pt.Speed == 0 {
			slog.Debug("rest stop",
				slog.String("device", imei),
				slog.Float64("lat", pt.Lat),
				slog.Float64("lon", pt.Lon))
			currentSpeed = 0
			// Send an ignition-off position so clients see the device parked.
			now := time.Now().UTC()
			stoppedMsg := BuildH02Message(imei, pt.Lat, pt.Lon, 0, pt.Course, pt.Ele, false, now)
			if err := w.WriteString(stoppedMsg); err != nil {
				return fmt.Errorf("write parked H02 message at rest stop %d/%d: %w", i+1, totalPoints, err)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(scaledDuration(2*time.Minute, s.speedMultiplier)):
			}
			continue
		}

		// Smoothly transition toward the target speed and add jitter.
		targetSpeed := pt.Speed
		currentSpeed = smoothAcceleration(currentSpeed, targetSpeed, maxAcceleration)
		reportedSpeed := addSpeedVariation(currentSpeed)

		now := time.Now().UTC()
		msg := BuildH02Message(imei, pt.Lat, pt.Lon, reportedSpeed, pt.Course, pt.Ele, true, now)

		// Write the message using the tracked writer (updates lastWriteAt).
		if err := w.WriteString(msg); err != nil {
			return fmt.Errorf("write H02 message at point %d/%d: %w", i+1, totalPoints, err)
		}

		// Log progress periodically (first point, every 50th, and last point).
		if i == startIndex || (i+1)%50 == 0 || i == totalPoints-1 {
			slog.Debug("sent GPS point",
				slog.String("device", imei),
				slog.Int("point", i+1),
				slog.Int("total", totalPoints),
				slog.Float64("lat", pt.Lat),
				slog.Float64("lon", pt.Lon),
				slog.Float64("speedKmh", reportedSpeed))
		}

		// Calculate interval based on distance and speed.
		interval := s.pointInterval(pt, i, points)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}

	return nil
}

// addSpeedVariation adds a small random perturbation (+-5%) to a speed value,
// making the simulation look more natural. The result is always positive.
func addSpeedVariation(speed float64) float64 {
	if speed <= 0 {
		return 0
	}
	// +-5% variation.
	variation := (rand.Float64() - 0.5) * 0.1
	result := speed * (1.0 + variation)
	if result < 1 {
		result = 1
	}
	return result
}

// smoothAcceleration gradually moves currentSpeed toward targetSpeed, clamping
// the change per step to maxChange km/h.
func smoothAcceleration(currentSpeed, targetSpeed, maxChange float64) float64 {
	diff := targetSpeed - currentSpeed
	if math.Abs(diff) <= maxChange {
		return targetSpeed
	}
	if diff > 0 {
		return currentSpeed + maxChange
	}
	return currentSpeed - maxChange
}

// pointInterval calculates how long to wait between sending two consecutive
// points based on real physics: time = distance / speed.
//
// With 100m interpolation intervals and realistic speeds (70-120 km/h), this
// produces natural update intervals:
//   - 100m at 100 km/h = 3.6 seconds
//   - 100m at  50 km/h = 7.2 seconds
//   - 100m at 120 km/h = 3.0 seconds
//
// The speedMultiplier can still compress time for faster demos if needed.
func (s *Simulator) pointInterval(pt RoutePoint, idx int, points []RoutePoint) time.Duration {
	// Use the distance to the next point if available.
	var distMeters float64
	if idx+1 < len(points) {
		distMeters = points[idx+1].Distance
	} else {
		distMeters = pt.Distance
	}

	// Minimum speed for interval calculation (avoid division by zero).
	speed := pt.Speed
	if speed < 5 {
		speed = 5
	}

	// time = distance / speed
	speedMS := speed / 3.6 // km/h to m/s
	seconds := distMeters / speedMS

	// Clamp to reasonable bounds: at least 0.5s, at most 30s between updates.
	// With fine-grained interpolation (100m), intervals are naturally shorter,
	// so we lower the minimum to allow smooth movement at high speeds.
	if seconds < 0.5 {
		seconds = 0.5
	}
	if seconds > 30 {
		seconds = 30
	}

	return scaledDuration(time.Duration(seconds*float64(time.Second)), s.speedMultiplier)
}

// BuildH02Message constructs an H02 V1 protocol message.
//
// Format: *HQ,{imei},V1,{HHMMSS},A,{lat},{N/S},{lon},{E/W},{speed_knots},{course},{DDMMYY},{flags},{altitude_m}#
//
// flags encodes ignition state in bit 10: FFFFFFEF = ignition ON, FFFFFBEF = ignition OFF.
// altitude is appended as a demo-simulator extension (meters, one decimal place).
func BuildH02Message(imei string, lat, lon, speedKmh, course, altitude float64, ignition bool, ts time.Time) string {
	timeStr := ts.Format("150405")
	dateStr := ts.Format("020106")

	// Convert lat/lon to NMEA format (DDMM.MMMM / DDDMM.MMMM).
	latNMEA, latDir := decimalToNMEA(lat, true)
	lonNMEA, lonDir := decimalToNMEA(lon, false)

	// Convert km/h to knots.
	speedKnots := speedKmh / 1.852

	// Set flags based on ignition state.
	// Bit 10 of the status word: 1 = ignition ON, 0 = ignition OFF.
	flags := "FFFFFFEF" // bit 10 = 1 (ignition ON)
	if !ignition {
		flags = "FFFFFBEF" // bit 10 = 0 (ignition OFF)
	}

	return fmt.Sprintf("*HQ,%s,V1,%s,A,%s,%s,%s,%s,%.2f,%.0f,%s,%s,%.1f#",
		imei, timeStr, latNMEA, latDir, lonNMEA, lonDir, speedKnots, course, dateStr, flags, altitude)
}

// decimalToNMEA converts decimal degrees to NMEA format.
// For latitude: DDMM.MMMM with N/S direction.
// For longitude: DDDMM.MMMM with E/W direction.
func decimalToNMEA(decimal float64, isLat bool) (string, string) {
	var dir string

	if isLat {
		if decimal >= 0 {
			dir = "N"
		} else {
			dir = "S"
			decimal = -decimal
		}
	} else {
		if decimal >= 0 {
			dir = "E"
		} else {
			dir = "W"
			decimal = -decimal
		}
	}

	degrees := math.Floor(decimal)
	minutes := (decimal - degrees) * 60.0

	if isLat {
		return fmt.Sprintf("%02.0f%07.4f", degrees, minutes), dir
	}
	return fmt.Sprintf("%03.0f%07.4f", degrees, minutes), dir
}

// reversePoints returns a copy of points in reverse order with recalculated
// bearings (180 degrees opposite).
func reversePoints(points []RoutePoint) []RoutePoint {
	n := len(points)
	reversed := make([]RoutePoint, n)
	for i, pt := range points {
		reversed[n-1-i] = RoutePoint{
			Lat:      pt.Lat,
			Lon:      pt.Lon,
			Ele:      pt.Ele,
			Speed:    pt.Speed,
			Course:   math.Mod(pt.Course+180, 360),
			Distance: pt.Distance,
		}
	}
	// First point in reversed list has no predecessor distance.
	if n > 0 {
		reversed[0].Distance = 0
	}
	return reversed
}

// scaledDuration applies the speed multiplier to a duration.
// Higher multiplier means shorter real-time duration.
func scaledDuration(d time.Duration, multiplier float64) time.Duration {
	if multiplier <= 0 {
		multiplier = 1.0
	}
	return time.Duration(float64(d) / multiplier)
}

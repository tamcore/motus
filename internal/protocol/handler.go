package protocol

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/tamcore/motus/internal/geo"
	"github.com/tamcore/motus/internal/geocoding"
	"github.com/tamcore/motus/internal/metrics"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/websocket"
)

// motionSpeedThreshold is the minimum speed in km/h to classify a device as
// moving. This mirrors services.MotionThreshold but is defined locally to
// avoid a circular import between protocol and services packages.
const motionSpeedThreshold = 5.0

// GeofenceChecker is the interface for checking geofence containment on new positions.
type GeofenceChecker interface {
	CheckGeofences(ctx context.Context, position *model.Position) error
}

type namedCheck struct {
	name string
	run  func(ctx context.Context, position *model.Position) error
}

// MileageChecker is the interface for tracking device mileage from positions.
type MileageChecker interface {
	ProcessPosition(ctx context.Context, position *model.Position, device *model.Device) error
}

// AddressLookup provides cached reverse geocoding for positions without
// blocking ingest. The address is set on the position's Address field for API
// responses and WebSocket broadcasts. It is NOT stored in the database -- that
// is handled separately by the idle service for stopped positions.
type AddressLookup interface {
	Peek(lat, lon float64) (string, bool)
	Prefetch(key int64, lat, lon float64)
	Resolved(key int64) (geocoding.Resolved, bool)
}

const (
	// addressReuseMeters is the distance within which a device reuses its
	// last address without a new lookup.
	addressReuseMeters = 50.0
	// addressStaleMeters bounds how far a device may be from its last address
	// while a fresher lookup is pending.
	addressStaleMeters = 1000.0
)

type deviceAddress struct {
	lat, lon float64
	address  string
	at       time.Time
}

// PositionHandler processes incoming GPS positions from protocol decoders.
type PositionHandler struct {
	positions      repository.PositionRepo
	devices        repository.DeviceRepo
	hub            *websocket.Hub
	geofenceEvents GeofenceChecker
	checks         []namedCheck
	mileage        MileageChecker
	addressLookup  AddressLookup
	addressMu      sync.Mutex
	addresses      map[int64]deviceAddress
	logger         *slog.Logger
}

// NewPositionHandler creates a handler that stores positions and broadcasts updates.
func NewPositionHandler(
	positions repository.PositionRepo,
	devices repository.DeviceRepo,
	hub *websocket.Hub,
	geofenceEvents GeofenceChecker,
) *PositionHandler {
	return &PositionHandler{
		positions:      positions,
		devices:        devices,
		hub:            hub,
		geofenceEvents: geofenceEvents,
		addresses:      make(map[int64]deviceAddress),
		logger:         slog.Default(),
	}
}

// AddCheck registers a check (e.g. motion, ignition, alarm) run on every stored
// position in registration order. Failures are logged as "<name> check failed".
func (h *PositionHandler) AddCheck(name string, check func(ctx context.Context, position *model.Position) error) {
	h.checks = append(h.checks, namedCheck{name, check})
}

// SetMileageChecker sets the mileage tracking service on the handler.
func (h *PositionHandler) SetMileageChecker(checker MileageChecker) {
	h.mileage = checker
}

// SetAddressLookup sets the geocoding service for enriching positions with
// cached addresses. When set, each incoming position gets the device's last
// address if it moved less than addressReuseMeters, else a cached address; on
// a cache miss the Address stays nil and the lookup runs in the background.
func (h *PositionHandler) SetAddressLookup(lookup AddressLookup) {
	h.addressLookup = lookup
}

// SetLogger configures the structured logger for this handler.
func (h *PositionHandler) SetLogger(l *slog.Logger) {
	if l != nil {
		h.logger = l
	}
}

// log returns the handler's logger, falling back to slog.Default() if nil.
func (h *PositionHandler) log() *slog.Logger {
	if h.logger != nil {
		return h.logger
	}
	return slog.Default()
}

// MarkOnline records activity from a device that sent a message without a
// position (e.g. a heartbeat), so it is shown as online and does not time out.
// Safe to call on a nil handler.
func (h *PositionHandler) MarkOnline(ctx context.Context, device *model.Device) {
	if h == nil || h.devices == nil || device == nil {
		return
	}
	now := time.Now().UTC()
	device.Status = "online"
	device.LastUpdate = &now
	if err := h.devices.Update(ctx, device); err != nil {
		h.log().Error("failed to mark device online",
			slog.Int64("deviceID", device.ID),
			slog.Any("error", err),
		)
		return
	}
	if h.hub != nil {
		h.hub.BroadcastDeviceStatus(device)
	}
}

// LastPosition returns the most recent stored position of a device, or nil
// if there is none. Safe to call on a nil handler.
func (h *PositionHandler) LastPosition(ctx context.Context, deviceID int64) *model.Position {
	if h == nil || h.positions == nil {
		return nil
	}
	pos, err := h.positions.GetLatestByDevice(ctx, deviceID)
	if err != nil {
		return nil
	}
	return pos
}

// address returns the live address for pos without blocking: the device's
// last address when close enough, else a cached one. A miss requests a
// background lookup for the device, whose result a later fix picks up.
func (h *PositionHandler) address(pos *model.Position) *string {
	h.addressMu.Lock()
	defer h.addressMu.Unlock()
	d, known := h.addresses[pos.DeviceID]
	if r, ok := h.addressLookup.Resolved(pos.DeviceID); ok && (!known || r.RequestedAt.After(d.at)) {
		d, known = deviceAddress{r.Lat, r.Lon, r.Address, r.RequestedAt}, true
		h.addresses[pos.DeviceID] = d
	}
	distance := func() float64 {
		return geo.HaversineDistance(d.lat, d.lon, pos.Latitude, pos.Longitude) * 1000
	}
	if known && distance() < addressReuseMeters {
		return new(d.address)
	}
	if addr, ok := h.addressLookup.Peek(pos.Latitude, pos.Longitude); ok {
		h.addresses[pos.DeviceID] = deviceAddress{pos.Latitude, pos.Longitude, addr, time.Now()}
		return new(addr)
	}
	h.addressLookup.Prefetch(pos.DeviceID, pos.Latitude, pos.Longitude)
	if known && distance() < addressStaleMeters {
		return new(d.address)
	}
	return nil
}

// HandlePosition stores a position and broadcasts it via WebSocket.
func (h *PositionHandler) HandlePosition(ctx context.Context, pos *model.Position) error {
	// Determine motion state from position speed.
	isMoving := pos.Speed != nil && *pos.Speed >= motionSpeedThreshold

	// Set the Traccar-compatible "motion" attribute on the position BEFORE
	// storing it so the attribute is persisted in the database. Home Assistant
	// and other Traccar clients read this to derive binary_sensor.motion.
	if pos.Attributes == nil {
		pos.Attributes = make(map[string]any)
	}
	pos.Attributes["motion"] = isMoving

	if err := h.positions.Create(ctx, pos); err != nil {
		metrics.PositionStorageErrors.Inc()
		return fmt.Errorf("store position: %w", err)
	}
	metrics.PositionsStored.Inc()

	// Enrich the position with a geocoded address for live API responses and
	// WebSocket broadcasts. This is set AFTER the DB write so the cached
	// address is not persisted (stored addresses are handled by the idle
	// service for stopped positions only).
	if h.addressLookup != nil && pos.Address == nil {
		pos.Address = h.address(pos)
	}

	// Update device status and record last_update.
	device, err := h.devices.GetByID(ctx, pos.DeviceID)
	if err != nil {
		h.log().Error("failed to get device for status update",
			slog.Int64("deviceID", pos.DeviceID),
			slog.Any("error", err),
		)
	} else {
		now := time.Now().UTC()
		// Always set status to "online" for Home Assistant compatibility.
		// HA binary_sensor.status only recognizes "online" as active (True).
		// Actual motion state is communicated via position.attributes.motion.
		device.Status = "online"
		device.LastUpdate = &now
		device.PositionID = &pos.ID
		// Clear the disabled flag when a device sends its first position.
		// This re-enables devices that were auto-excluded as "unknown".
		if device.Disabled {
			device.Disabled = false
		}
		if err := h.devices.Update(ctx, device); err != nil {
			h.log().Error("failed to update device status",
				slog.Int64("deviceID", pos.DeviceID),
				slog.Any("error", err),
			)
		}
		h.hub.BroadcastDeviceStatus(device)
	}

	// Check geofences before broadcasting so the position carries GeofenceIDs
	// when it reaches Home Assistant (and other WebSocket clients). The check
	// must happen after Create so the position has an ID for event records.
	if h.geofenceEvents != nil && !(pos.Latitude == 0 && pos.Longitude == 0) {
		if err := h.geofenceEvents.CheckGeofences(ctx, pos); err != nil {
			h.log().Error("geofence check failed",
				slog.Int64("deviceID", pos.DeviceID),
				slog.Any("error", err),
			)
		}
		// Persist computed GeofenceIDs back into the stored row so that REST
		// API reads (e.g. GET /api/positions polled by Home Assistant) also
		// return the correct geofence memberships.
		if len(pos.GeofenceIDs) > 0 {
			if err := h.positions.UpdateGeofenceIDs(ctx, pos.ID, pos.GeofenceIDs); err != nil {
				h.log().Error("failed to persist geofence ids",
					slog.Int64("positionID", pos.ID),
					slog.Any("error", err),
				)
			}
		}
	}

	// Broadcast after geofence check so the position includes GeofenceIDs.
	h.hub.BroadcastPosition(pos)

	for _, c := range h.checks {
		if err := c.run(ctx, pos); err != nil {
			h.log().Error(c.name+" check failed",
				slog.Int64("deviceID", pos.DeviceID),
				slog.Any("error", err),
			)
		}
	}

	// Track mileage: accumulate distance during motion, commit on trip completion.
	if h.mileage != nil && device != nil {
		if err := h.mileage.ProcessPosition(ctx, pos, device); err != nil {
			h.log().Error("mileage check failed",
				slog.Int64("deviceID", pos.DeviceID),
				slog.Any("error", err),
			)
		}
	}

	return nil
}

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/services"
)

func registerTools(s *server.MCPServer, deps Deps) {
	s.AddTool(mcp.NewTool("get_server_time",
		mcp.WithDescription("Returns the current server time. Call this first so you can resolve relative dates like 'last year' or 'this month' correctly."),
	), handleGetServerTime)

	s.AddTool(mcp.NewTool("list_devices",
		mcp.WithDescription("Lists all GPS devices accessible to the current user."),
		mcp.WithString("name_contains",
			mcp.Description("Optional substring filter on device name (case-insensitive)."),
		),
	), withDeps(deps, handleListDevices))

	s.AddTool(mcp.NewTool("get_latest_position",
		mcp.WithDescription("Returns the latest GPS position for a device."),
		mcp.WithString("device_id", mcp.Description("Numeric device ID.")),
		mcp.WithString("device_name", mcp.Description("Device name (alternative to device_id).")),
	), withDeps(deps, handleGetLatestPosition))

	s.AddTool(mcp.NewTool("get_distance_traveled",
		mcp.WithDescription("Returns total trip distance (km) per device and grand total for the given time window."),
		mcp.WithString("from", mcp.Required(), mcp.Description("Start of time window (RFC3339, e.g. 2024-01-01T00:00:00Z).")),
		mcp.WithString("to", mcp.Required(), mcp.Description("End of time window (RFC3339, exclusive).")),
		mcp.WithString("device_id", mcp.Description("Limit to a single device ID.")),
		mcp.WithString("device_name", mcp.Description("Limit to a device by name (alternative to device_id).")),
	), withDeps(deps, handleGetDistanceTraveled))

	s.AddTool(mcp.NewTool("list_geofences",
		mcp.WithDescription("Lists all geofences accessible to the current user."),
	), withDeps(deps, handleListGeofences))

	s.AddTool(mcp.NewTool("create_geofence",
		mcp.WithDescription("Creates a circular geofence around an address or coordinates. Validates name, emits an audit entry."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Geofence display name.")),
		mcp.WithString("address", mcp.Description("Address to geocode (alternative to latitude/longitude).")),
		mcp.WithString("latitude", mcp.Description("Latitude of the centre (decimal degrees). Required if address not given.")),
		mcp.WithString("longitude", mcp.Description("Longitude of the centre (decimal degrees). Required if address not given.")),
		mcp.WithNumber("radius_m", mcp.Description("Radius in metres. Default: 200.")),
		mcp.WithString("calendar_id", mcp.Description("Optional calendar ID for time-based activation.")),
	), withDeps(deps, handleCreateGeofence))

	s.AddTool(mcp.NewTool("geocode_address",
		mcp.WithDescription("Converts a free-text address into latitude/longitude coordinates. Read-only."),
		mcp.WithString("address", mcp.Required(), mcp.Description("Address to geocode.")),
	), withDeps(deps, handleGeocodeAddress))
}

// withDeps closes over Deps so handlers can be pure functions.
func withDeps(deps Deps, fn func(context.Context, mcp.CallToolRequest, Deps) (*mcp.CallToolResult, error)) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return fn(ctx, req, deps)
	}
}

// requireUser extracts the authenticated user or returns an error.
func requireUser(ctx context.Context) (*model.User, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return nil, errors.New("unauthenticated: no user in context")
	}
	return user, nil
}

// requireWriteAccess returns an error for readonly API keys.
// Cookie/session users always have write access.
func requireWriteAccess(ctx context.Context) error {
	if key := api.ApiKeyFromContext(ctx); key != nil && key.IsReadonly() {
		return errors.New("write access denied: readonly API key")
	}
	return nil
}

func jsonResult(v any) *mcp.CallToolResult {
	b, err := json.Marshal(v)
	if err != nil {
		return mcp.NewToolResultError("failed to encode result: " + err.Error())
	}
	return mcp.NewToolResultText(string(b))
}

// ---- handlers ---------------------------------------------------------------

func handleGetServerTime(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	now := time.Now().UTC()
	return jsonResult(map[string]string{
		"now":   now.Format(time.RFC3339),
		"year":  fmt.Sprintf("%d", now.Year()),
		"today": now.Format("2006-01-02"),
	}), nil
}

func handleListDevices(ctx context.Context, req mcp.CallToolRequest, deps Deps) (*mcp.CallToolResult, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	filter := strings.ToLower(req.GetString("name_contains", ""))

	var devices []*model.Device
	if user.IsAdmin() {
		all, err := deps.Devices.GetAllWithOwners(ctx)
		if err != nil {
			return mcp.NewToolResultError("failed to list devices: " + err.Error()), nil
		}
		for i := range all {
			devices = append(devices, &all[i])
		}
	} else {
		devices, err = deps.Devices.GetByUser(ctx, user.ID)
		if err != nil {
			return mcp.NewToolResultError("failed to list devices: " + err.Error()), nil
		}
	}

	type entry struct {
		ID         int64      `json:"id"`
		UniqueID   string     `json:"uniqueId"`
		Name       string     `json:"name"`
		Model      string     `json:"model,omitempty"`
		LastUpdate *time.Time `json:"lastUpdate,omitempty"`
		IgnitionOn bool       `json:"ignitionOn"`
	}
	var out []entry
	for _, d := range devices {
		if filter != "" && !strings.Contains(strings.ToLower(d.Name), filter) {
			continue
		}
		model := ""
		if d.Model != nil {
			model = *d.Model
		}
		out = append(out, entry{
			ID:         d.ID,
			UniqueID:   d.UniqueID,
			Name:       d.Name,
			Model:      model,
			LastUpdate: d.LastUpdate,
			IgnitionOn: d.IgnitionOn,
		})
	}
	return jsonResult(out), nil
}

func handleGetLatestPosition(ctx context.Context, req mcp.CallToolRequest, deps Deps) (*mcp.CallToolResult, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	deviceID, err := resolveDeviceID(ctx, req, user, deps)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if deviceID == 0 {
		return mcp.NewToolResultError("device_id or device_name is required"), nil
	}
	if !deps.Devices.UserHasAccess(ctx, user, deviceID) {
		return mcp.NewToolResultError("access denied"), nil
	}

	pos, err := deps.Positions.GetLatestByDevice(ctx, deviceID)
	if err != nil || pos == nil {
		return mcp.NewToolResultError("no position found"), nil
	}

	return jsonResult(map[string]any{
		"deviceId":  pos.DeviceID,
		"latitude":  pos.Latitude,
		"longitude": pos.Longitude,
		"address":   pos.Address,
		"speed":     pos.Speed,
		"timestamp": pos.Timestamp.Format(time.RFC3339),
	}), nil
}

func handleGetDistanceTraveled(ctx context.Context, req mcp.CallToolRequest, deps Deps) (*mcp.CallToolResult, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	fromStr, err := req.RequireString("from")
	if err != nil {
		return mcp.NewToolResultError("from is required"), nil
	}
	toStr, err := req.RequireString("to")
	if err != nil {
		return mcp.NewToolResultError("to is required"), nil
	}
	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		return mcp.NewToolResultError("invalid from: " + err.Error()), nil
	}
	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		return mcp.NewToolResultError("invalid to: " + err.Error()), nil
	}

	// Resolve device scope.
	var deviceIDs []int64
	specificID, _ := resolveDeviceID(ctx, req, user, deps)
	if specificID != 0 {
		if !deps.Devices.UserHasAccess(ctx, user, specificID) {
			return mcp.NewToolResultError("access denied"), nil
		}
		deviceIDs = []int64{specificID}
	} else {
		all, err := deps.Devices.GetByUser(ctx, user.ID)
		if err != nil {
			return mcp.NewToolResultError("failed to list devices: " + err.Error()), nil
		}
		for _, d := range all {
			deviceIDs = append(deviceIDs, d.ID)
		}
	}

	totals, grandTotal, err := deps.Events.SumTripDistance(ctx, deviceIDs, from, to)
	if err != nil {
		return mcp.NewToolResultError("failed to sum distances: " + err.Error()), nil
	}

	type row struct {
		ID         int64   `json:"id"`
		Name       string  `json:"name"`
		DistanceKm float64 `json:"distanceKm"`
		TripCount  int     `json:"tripCount"`
	}
	rows := make([]row, 0, len(totals))
	for _, t := range totals {
		name := ""
		if d, err := deps.Devices.GetByID(ctx, t.DeviceID); err == nil && d != nil {
			name = d.Name
		}
		rows = append(rows, row{
			ID:         t.DeviceID,
			Name:       name,
			DistanceKm: math.Round(t.DistanceKm*10) / 10,
			TripCount:  t.TripCount,
		})
	}

	return jsonResult(map[string]any{
		"devices": rows,
		"totalKm": math.Round(grandTotal*10) / 10,
	}), nil
}

func handleListGeofences(ctx context.Context, _ mcp.CallToolRequest, deps Deps) (*mcp.CallToolResult, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	geofences, err := deps.Geofences.GetByUser(ctx, user.ID)
	if err != nil {
		return mcp.NewToolResultError("failed to list geofences: " + err.Error()), nil
	}

	type entry struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		Area       string `json:"area,omitempty"`
		CalendarID *int64 `json:"calendarId,omitempty"`
	}
	out := make([]entry, 0, len(geofences))
	for _, g := range geofences {
		out = append(out, entry{
			ID:         g.ID,
			Name:       g.Name,
			Area:       g.Area,
			CalendarID: g.CalendarID,
		})
	}
	return jsonResult(out), nil
}

func handleCreateGeofence(ctx context.Context, req mcp.CallToolRequest, deps Deps) (*mcp.CallToolResult, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := requireWriteAccess(ctx); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	name, err := req.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError("name is required"), nil
	}

	radiusM := req.GetFloat("radius_m", 200)
	if radiusM <= 0 {
		radiusM = 200
	}

	lat, lon, err := resolveCoords(ctx, req, deps)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var calID *int64
	if v := req.GetString("calendar_id", ""); v != "" {
		var id int64
		if _, err := fmt.Sscanf(v, "%d", &id); err == nil {
			calID = &id
		}
	}

	geometry := circleGeoJSON(lat, lon, radiusM)

	g, err := deps.GeofenceService.CreateForUser(ctx, user, services.CreateGeofenceInput{
		Name:       name,
		Geometry:   geometry,
		CalendarID: calID,
	})
	if err != nil {
		return mcp.NewToolResultError("failed to create geofence: " + err.Error()), nil
	}

	return jsonResult(map[string]any{
		"id":        g.ID,
		"name":      g.Name,
		"latitude":  lat,
		"longitude": lon,
		"radiusM":   radiusM,
	}), nil
}

func handleGeocodeAddress(ctx context.Context, req mcp.CallToolRequest, deps Deps) (*mcp.CallToolResult, error) {
	if _, err := requireUser(ctx); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	address, err := req.RequireString("address")
	if err != nil {
		return mcp.NewToolResultError("address is required"), nil
	}
	if deps.ForwardGeocoder == nil {
		return mcp.NewToolResultError("geocoding not available"), nil
	}
	lat, lon, displayName, err := deps.ForwardGeocoder.ForwardGeocode(ctx, address)
	if err != nil {
		return mcp.NewToolResultError("geocoding failed: " + err.Error()), nil
	}
	return jsonResult(map[string]any{
		"latitude":    lat,
		"longitude":   lon,
		"displayName": displayName,
	}), nil
}

// ---- helpers ----------------------------------------------------------------

// resolveDeviceID reads device_id (string int) or device_name from the request.
func resolveDeviceID(ctx context.Context, req mcp.CallToolRequest, user *model.User, deps Deps) (int64, error) {
	if idStr := req.GetString("device_id", ""); idStr != "" {
		var id int64
		if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
			return 0, fmt.Errorf("invalid device_id: %s", idStr)
		}
		return id, nil
	}
	if name := req.GetString("device_name", ""); name != "" {
		var devices []*model.Device
		if user.IsAdmin() {
			all, err := deps.Devices.GetAllWithOwners(ctx)
			if err != nil {
				return 0, err
			}
			for i := range all {
				devices = append(devices, &all[i])
			}
		} else {
			var err error
			devices, err = deps.Devices.GetByUser(ctx, user.ID)
			if err != nil {
				return 0, err
			}
		}
		lower := strings.ToLower(name)
		for _, d := range devices {
			if strings.ToLower(d.Name) == lower {
				return d.ID, nil
			}
		}
		return 0, fmt.Errorf("device not found: %s", name)
	}
	return 0, nil
}

// resolveCoords returns lat/lon from the request, geocoding address if needed.
func resolveCoords(ctx context.Context, req mcp.CallToolRequest, deps Deps) (lat, lon float64, err error) {
	if addr := req.GetString("address", ""); addr != "" {
		if deps.ForwardGeocoder == nil {
			return 0, 0, errors.New("geocoding not available")
		}
		lat, lon, _, err = deps.ForwardGeocoder.ForwardGeocode(ctx, addr)
		return
	}
	latStr := req.GetString("latitude", "")
	lonStr := req.GetString("longitude", "")
	if latStr == "" || lonStr == "" {
		return 0, 0, errors.New("address or latitude/longitude is required")
	}
	if _, err = fmt.Sscanf(latStr, "%f", &lat); err != nil {
		return 0, 0, fmt.Errorf("invalid latitude: %s", latStr)
	}
	if _, err = fmt.Sscanf(lonStr, "%f", &lon); err != nil {
		return 0, 0, fmt.Errorf("invalid longitude: %s", lonStr)
	}
	return
}

// circleGeoJSON returns a GeoJSON Polygon approximating a circle of radius
// metres around (lat, lon). Uses 32 segments.
func circleGeoJSON(lat, lon, radiusM float64) string {
	const segments = 32
	// Degrees per metre (rough spherical approximation, <0.5% error under 10 km).
	dLat := radiusM / 111320.0
	dLon := radiusM / (111320.0 * math.Cos(lat*math.Pi/180.0))

	coords := make([][2]float64, segments+1)
	for i := 0; i < segments; i++ {
		angle := 2 * math.Pi * float64(i) / float64(segments)
		coords[i] = [2]float64{
			lon + dLon*math.Cos(angle),
			lat + dLat*math.Sin(angle),
		}
	}
	coords[segments] = coords[0] // close the ring

	b, _ := json.Marshal(map[string]any{
		"type":        "Polygon",
		"coordinates": [][][2]float64{coords},
	})
	return string(b)
}

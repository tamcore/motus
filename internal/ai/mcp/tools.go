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
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/notification"
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

	// ---- calendars ---------------------------------------------------------------

	s.AddTool(mcp.NewTool("list_calendars",
		mcp.WithDescription("Lists all time-window calendars accessible to the current user. Calendars can be attached to geofences to make them time-conditional."),
	), withDeps(deps, handleListCalendars))

	s.AddTool(mcp.NewTool("create_calendar",
		mcp.WithDescription(`Creates a new calendar that can be attached to geofences for time-based activation.
Two modes:
- One-shot: provide start_time and end_time (RFC3339).
- Weekly recurring: provide weekdays (comma-separated MO/TU/WE/TH/FR/SA/SU) and daily_start_time/daily_end_time (HH:MM UTC).`),
		mcp.WithString("name", mcp.Required(), mcp.Description("Calendar display name.")),
		mcp.WithString("start_time", mcp.Description("One-shot mode: start datetime (RFC3339, e.g. 2026-06-06T00:00:00Z).")),
		mcp.WithString("end_time", mcp.Description("One-shot mode: end datetime (RFC3339).")),
		mcp.WithString("weekdays", mcp.Description("Recurring mode: comma-separated weekday codes, e.g. MO,WE,FR.")),
		mcp.WithString("daily_start_time", mcp.Description("Recurring mode: daily start time in HH:MM UTC, e.g. 08:00.")),
		mcp.WithString("daily_end_time", mcp.Description("Recurring mode: daily end time in HH:MM UTC, e.g. 18:00.")),
	), withDeps(deps, handleCreateCalendar))

	// ---- geofences (update / delete) -------------------------------------------

	s.AddTool(mcp.NewTool("update_geofence",
		mcp.WithDescription("Updates a geofence's name and/or calendar attachment. Use calendar_id to attach a calendar, or 'clear' to detach one."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Geofence ID.")),
		mcp.WithString("name", mcp.Description("New display name (optional).")),
		mcp.WithString("calendar_id", mcp.Description("Calendar ID to attach, or 'clear' to detach any existing calendar.")),
	), withDeps(deps, handleUpdateGeofence))

	s.AddTool(mcp.NewTool("delete_geofence",
		mcp.WithDescription("Permanently deletes a geofence. This cannot be undone."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Geofence ID.")),
	), withDeps(deps, handleDeleteGeofence))

	// ---- notifications -----------------------------------------------------------

	s.AddTool(mcp.NewTool("list_notification_rules",
		mcp.WithDescription("Lists all notification rules for the current user."),
	), withDeps(deps, handleListNotificationRules))

	s.AddTool(mcp.NewTool("create_notification_rule",
		mcp.WithDescription(`Creates a notification rule that sends an alert when a GPS event occurs.
Supported event types: geofenceEnter, geofenceExit, deviceOnline, deviceOffline, motion, deviceIdle, ignitionOn, ignitionOff, alarm, tripCompleted.
Supported channels: webhook.
For webhook channel: provide webhook_url (must be http/https; private IPs are blocked).`),
		mcp.WithString("name", mcp.Required(), mcp.Description("Rule display name.")),
		mcp.WithString("event_types", mcp.Required(), mcp.Description("Comma-separated event types, e.g. deviceOffline,alarm.")),
		mcp.WithString("channel", mcp.Required(), mcp.Description("Delivery channel. Currently: webhook.")),
		mcp.WithString("webhook_url", mcp.Description("Webhook URL (required for webhook channel).")),
		mcp.WithString("template", mcp.Description("Optional JSON payload template. Supports {{device.name}}, {{geofence.name}}, {{position.latitude}}, etc.")),
		mcp.WithString("enabled", mcp.Description("true or false. Default: true.")),
	), withDeps(deps, handleCreateNotificationRule))

	s.AddTool(mcp.NewTool("update_notification_rule",
		mcp.WithDescription("Updates an existing notification rule. Only the provided fields are changed."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Rule ID.")),
		mcp.WithString("name", mcp.Description("New display name.")),
		mcp.WithString("event_types", mcp.Description("New comma-separated event types.")),
		mcp.WithString("webhook_url", mcp.Description("New webhook URL.")),
		mcp.WithString("template", mcp.Description("New payload template.")),
		mcp.WithString("enabled", mcp.Description("true to enable, false to disable.")),
	), withDeps(deps, handleUpdateNotificationRule))

	s.AddTool(mcp.NewTool("delete_notification_rule",
		mcp.WithDescription("Permanently deletes a notification rule."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Rule ID.")),
	), withDeps(deps, handleDeleteNotificationRule))

	// ---- events ------------------------------------------------------------------

	s.AddTool(mcp.NewTool("list_events",
		mcp.WithDescription(`Lists GPS events (geofence transitions, alarms, trip completions, etc.) for the given time range.
Supported event_types: geofenceEnter, geofenceExit, deviceOnline, deviceOffline, motion, deviceIdle, ignitionOn, ignitionOff, alarm, tripCompleted.`),
		mcp.WithString("from", mcp.Required(), mcp.Description("Start of time window (RFC3339).")),
		mcp.WithString("to", mcp.Required(), mcp.Description("End of time window (RFC3339).")),
		mcp.WithString("device_id", mcp.Description("Filter to a specific device ID.")),
		mcp.WithString("device_name", mcp.Description("Filter by device name (alternative to device_id).")),
		mcp.WithString("event_types", mcp.Description("Comma-separated event types to filter by. Omit to include all types.")),
		mcp.WithNumber("limit", mcp.Description("Maximum results to return (1–500, default 100).")),
	), withDeps(deps, handleListEvents))
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

// ---- calendar handlers ------------------------------------------------------

func handleListCalendars(ctx context.Context, _ mcp.CallToolRequest, deps Deps) (*mcp.CallToolResult, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if deps.Calendars == nil {
		return mcp.NewToolResultError("calendar service not available"), nil
	}

	cals, err := deps.Calendars.GetByUser(ctx, user.ID)
	if err != nil {
		return mcp.NewToolResultError("failed to list calendars: " + err.Error()), nil
	}

	type entry struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	out := make([]entry, 0, len(cals))
	for _, c := range cals {
		out = append(out, entry{ID: c.ID, Name: c.Name})
	}
	return jsonResult(out), nil
}

func handleCreateCalendar(ctx context.Context, req mcp.CallToolRequest, deps Deps) (*mcp.CallToolResult, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := requireWriteAccess(ctx); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if deps.CalendarService == nil {
		return mcp.NewToolResultError("calendar service not available"), nil
	}

	name, err := req.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError("name is required"), nil
	}

	spec := CalendarSpec{Name: name}

	startStr := req.GetString("start_time", "")
	endStr := req.GetString("end_time", "")
	weekdaysStr := req.GetString("weekdays", "")
	dailyStart := req.GetString("daily_start_time", "")
	dailyEnd := req.GetString("daily_end_time", "")

	switch {
	case startStr != "" && endStr != "":
		start, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			return mcp.NewToolResultError("invalid start_time: " + err.Error()), nil
		}
		end, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			return mcp.NewToolResultError("invalid end_time: " + err.Error()), nil
		}
		spec.StartTime = &start
		spec.EndTime = &end
	case weekdaysStr != "" && dailyStart != "" && dailyEnd != "":
		days := splitTrim(weekdaysStr)
		spec.Weekdays = days
		spec.DailyStartTime = &dailyStart
		spec.DailyEndTime = &dailyEnd
	default:
		return mcp.NewToolResultError("provide either (start_time + end_time) or (weekdays + daily_start_time + daily_end_time)"), nil
	}

	ical, err := BuildICalendar(spec)
	if err != nil {
		return mcp.NewToolResultError("invalid calendar spec: " + err.Error()), nil
	}

	c, err := deps.CalendarService.CreateForUser(ctx, user, services.CreateCalendarInput{
		Name: name,
		Data: ical,
	})
	if err != nil {
		return mcp.NewToolResultError("failed to create calendar: " + err.Error()), nil
	}
	return jsonResult(map[string]any{"id": c.ID, "name": c.Name}), nil
}

// ---- geofence update / delete handlers --------------------------------------

func handleUpdateGeofence(ctx context.Context, req mcp.CallToolRequest, deps Deps) (*mcp.CallToolResult, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := requireWriteAccess(ctx); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	idStr, err := req.RequireString("id")
	if err != nil {
		return mcp.NewToolResultError("id is required"), nil
	}
	var geoID int64
	if _, err := fmt.Sscanf(idStr, "%d", &geoID); err != nil {
		return mcp.NewToolResultError("invalid id"), nil
	}

	in := services.UpdateGeofenceInput{}

	if n := req.GetString("name", ""); n != "" {
		in.Name = &n
	}
	if calStr := req.GetString("calendar_id", ""); calStr != "" {
		in.CalendarIDSet = true
		if calStr == "clear" || calStr == "0" || calStr == "none" {
			in.CalendarID = nil
		} else {
			var calID int64
			if _, err := fmt.Sscanf(calStr, "%d", &calID); err != nil {
				return mcp.NewToolResultError("invalid calendar_id"), nil
			}
			in.CalendarID = &calID
		}
	}

	g, err := deps.GeofenceService.UpdateForUser(ctx, user, geoID, in)
	if err != nil {
		return mcp.NewToolResultError("failed to update geofence: " + err.Error()), nil
	}
	return jsonResult(map[string]any{
		"id":         g.ID,
		"name":       g.Name,
		"calendarId": g.CalendarID,
	}), nil
}

func handleDeleteGeofence(ctx context.Context, req mcp.CallToolRequest, deps Deps) (*mcp.CallToolResult, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := requireWriteAccess(ctx); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	idStr, err := req.RequireString("id")
	if err != nil {
		return mcp.NewToolResultError("id is required"), nil
	}
	var geoID int64
	if _, err := fmt.Sscanf(idStr, "%d", &geoID); err != nil {
		return mcp.NewToolResultError("invalid id"), nil
	}

	if err := deps.GeofenceService.DeleteForUser(ctx, user, geoID); err != nil {
		return mcp.NewToolResultError("failed to delete geofence: " + err.Error()), nil
	}
	return jsonResult(map[string]any{"deleted": geoID}), nil
}

// ---- notification handlers --------------------------------------------------

func handleListNotificationRules(ctx context.Context, _ mcp.CallToolRequest, deps Deps) (*mcp.CallToolResult, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if deps.Notifications == nil {
		return mcp.NewToolResultError("notification service not available"), nil
	}

	rules, err := deps.Notifications.GetByUser(ctx, user.ID)
	if err != nil {
		return mcp.NewToolResultError("failed to list rules: " + err.Error()), nil
	}

	type entry struct {
		ID         int64    `json:"id"`
		Name       string   `json:"name"`
		EventTypes []string `json:"eventTypes"`
		Channel    string   `json:"channel"`
		Enabled    bool     `json:"enabled"`
	}
	out := make([]entry, 0, len(rules))
	for _, r := range rules {
		out = append(out, entry{
			ID:         r.ID,
			Name:       r.Name,
			EventTypes: r.EventTypes,
			Channel:    r.Channel,
			Enabled:    r.Enabled,
		})
	}
	return jsonResult(out), nil
}

func handleCreateNotificationRule(ctx context.Context, req mcp.CallToolRequest, deps Deps) (*mcp.CallToolResult, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := requireWriteAccess(ctx); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if deps.Notifications == nil {
		return mcp.NewToolResultError("notification service not available"), nil
	}

	name, err := req.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError("name is required"), nil
	}
	eventTypesStr, err := req.RequireString("event_types")
	if err != nil {
		return mcp.NewToolResultError("event_types is required"), nil
	}
	channel, err := req.RequireString("channel")
	if err != nil {
		return mcp.NewToolResultError("channel is required"), nil
	}

	eventTypes := splitTrim(eventTypesStr)
	if err := validateEventTypes(eventTypes); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := validateChannel(channel); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	cfg, err := buildNotificationConfig(req, channel)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	enabled := true
	if v := req.GetString("enabled", ""); v == "false" {
		enabled = false
	}

	rule := &model.NotificationRule{
		UserID:     user.ID,
		Name:       name,
		EventTypes: eventTypes,
		Channel:    channel,
		Config:     cfg,
		Template:   req.GetString("template", ""),
		Enabled:    enabled,
	}
	if err := deps.Notifications.Create(ctx, rule); err != nil {
		return mcp.NewToolResultError("failed to create rule: " + err.Error()), nil
	}

	if deps.AuditLogger != nil {
		deps.AuditLogger.Log(ctx, &user.ID, audit.ActionNotifCreate, audit.ResourceNotification, &rule.ID,
			map[string]any{"name": rule.Name, "channel": rule.Channel}, "", "")
	}

	return jsonResult(map[string]any{"id": rule.ID, "name": rule.Name}), nil
}

func handleUpdateNotificationRule(ctx context.Context, req mcp.CallToolRequest, deps Deps) (*mcp.CallToolResult, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := requireWriteAccess(ctx); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if deps.Notifications == nil {
		return mcp.NewToolResultError("notification service not available"), nil
	}

	idStr, err := req.RequireString("id")
	if err != nil {
		return mcp.NewToolResultError("id is required"), nil
	}
	var ruleID int64
	if _, err := fmt.Sscanf(idStr, "%d", &ruleID); err != nil {
		return mcp.NewToolResultError("invalid id"), nil
	}

	existing, err := deps.Notifications.GetByID(ctx, ruleID)
	if err != nil || existing == nil {
		return mcp.NewToolResultError("rule not found"), nil
	}
	if existing.UserID != user.ID && !user.IsAdmin() {
		return mcp.NewToolResultError("access denied"), nil
	}

	updated := *existing
	if n := req.GetString("name", ""); n != "" {
		updated.Name = n
	}
	if et := req.GetString("event_types", ""); et != "" {
		types := splitTrim(et)
		if err := validateEventTypes(types); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		updated.EventTypes = types
	}
	if wu := req.GetString("webhook_url", ""); wu != "" {
		if err := notification.ValidateWebhookURL(wu); err != nil {
			return mcp.NewToolResultError("invalid webhook_url: " + err.Error()), nil
		}
		if updated.Config == nil {
			updated.Config = make(map[string]any)
		}
		updated.Config["webhookUrl"] = wu
	}
	if t := req.GetString("template", ""); t != "" {
		updated.Template = t
	}
	if v := req.GetString("enabled", ""); v != "" {
		updated.Enabled = v != "false"
	}

	if err := deps.Notifications.Update(ctx, &updated); err != nil {
		return mcp.NewToolResultError("failed to update rule: " + err.Error()), nil
	}
	return jsonResult(map[string]any{"id": updated.ID, "name": updated.Name, "enabled": updated.Enabled}), nil
}

func handleDeleteNotificationRule(ctx context.Context, req mcp.CallToolRequest, deps Deps) (*mcp.CallToolResult, error) {
	user, err := requireUser(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := requireWriteAccess(ctx); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if deps.Notifications == nil {
		return mcp.NewToolResultError("notification service not available"), nil
	}

	idStr, err := req.RequireString("id")
	if err != nil {
		return mcp.NewToolResultError("id is required"), nil
	}
	var ruleID int64
	if _, err := fmt.Sscanf(idStr, "%d", &ruleID); err != nil {
		return mcp.NewToolResultError("invalid id"), nil
	}

	existing, err := deps.Notifications.GetByID(ctx, ruleID)
	if err != nil || existing == nil {
		return mcp.NewToolResultError("rule not found"), nil
	}
	if existing.UserID != user.ID && !user.IsAdmin() {
		return mcp.NewToolResultError("access denied"), nil
	}

	if err := deps.Notifications.Delete(ctx, ruleID); err != nil {
		return mcp.NewToolResultError("failed to delete rule: " + err.Error()), nil
	}
	return jsonResult(map[string]any{"deleted": ruleID}), nil
}

// ---- event handler ----------------------------------------------------------

func handleListEvents(ctx context.Context, req mcp.CallToolRequest, deps Deps) (*mcp.CallToolResult, error) {
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

	var deviceIDs []int64
	specificID, _ := resolveDeviceID(ctx, req, user, deps)
	if specificID != 0 {
		if !deps.Devices.UserHasAccess(ctx, user, specificID) {
			return mcp.NewToolResultError("access denied"), nil
		}
		deviceIDs = []int64{specificID}
	}

	var eventTypes []string
	if et := req.GetString("event_types", ""); et != "" {
		eventTypes = splitTrim(et)
	}

	limit := int(req.GetFloat("limit", 100))
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	events, err := deps.Events.GetByFilters(ctx, user.ID, deviceIDs, eventTypes, from, to)
	if err != nil {
		return mcp.NewToolResultError("failed to list events: " + err.Error()), nil
	}

	if len(events) > limit {
		events = events[:limit]
	}

	type entry struct {
		ID         int64          `json:"id"`
		DeviceID   int64          `json:"deviceId"`
		Type       string         `json:"type"`
		Timestamp  string         `json:"timestamp"`
		GeofenceID *int64         `json:"geofenceId,omitempty"`
		Attributes map[string]any `json:"attributes,omitempty"`
	}
	out := make([]entry, 0, len(events))
	for _, e := range events {
		out = append(out, entry{
			ID:         e.ID,
			DeviceID:   e.DeviceID,
			Type:       e.Type,
			Timestamp:  e.Timestamp.Format(time.RFC3339),
			GeofenceID: e.GeofenceID,
			Attributes: e.Attributes,
		})
	}
	return jsonResult(out), nil
}

// ---- notification helpers ---------------------------------------------------

var mcpValidEventTypes = map[string]bool{
	"geofenceEnter": true, "geofenceExit": true,
	"deviceOnline": true, "deviceOffline": true,
	"motion": true, "deviceIdle": true,
	"ignitionOn": true, "ignitionOff": true,
	"alarm": true, "tripCompleted": true,
}

var mcpValidChannels = map[string]bool{
	"webhook": true,
}

func splitTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func validateEventTypes(types []string) error {
	for _, t := range types {
		if !mcpValidEventTypes[t] {
			return fmt.Errorf("unsupported event type %q", t)
		}
	}
	return nil
}

func validateChannel(ch string) error {
	if !mcpValidChannels[ch] {
		return fmt.Errorf("unsupported channel %q (supported: webhook)", ch)
	}
	return nil
}

func buildNotificationConfig(req mcp.CallToolRequest, channel string) (map[string]any, error) {
	cfg := make(map[string]any)
	switch channel {
	case "webhook":
		wu := req.GetString("webhook_url", "")
		if wu == "" {
			return nil, fmt.Errorf("webhook_url is required for webhook channel")
		}
		if err := notification.ValidateWebhookURL(wu); err != nil {
			return nil, fmt.Errorf("invalid webhook_url: %w", err)
		}
		cfg["webhookUrl"] = wu
		if h := req.GetString("headers", ""); h != "" {
			cfg["headers"] = h
		}
	default:
		return nil, fmt.Errorf("unsupported channel %q", channel)
	}
	return cfg, nil
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
	for i := range segments {
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

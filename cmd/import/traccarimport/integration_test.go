package traccarimport

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

// connStrToConfig parses a postgres:// URL into the Config target fields.
// Example: postgres://postgres:test@localhost:32768/motus_test?sslmode=disable
func connStrToConfig(connStr string) (host string, port int, dbname, user, password string, err error) {
	u, err := url.Parse(connStr)
	if err != nil {
		return "", 0, "", "", "", fmt.Errorf("parse conn str: %w", err)
	}
	host = u.Hostname()
	portStr := u.Port()
	if portStr == "" {
		portStr = "5432"
	}
	p, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, "", "", "", fmt.Errorf("parse port: %w", err)
	}
	port = p
	dbname = strings.TrimPrefix(u.Path, "/")
	user = u.User.Username()
	password, _ = u.User.Password()
	return host, port, dbname, user, password, nil
}

// TestImportDevices_Integration verifies that importDevices inserts rows into
// the devices and user_devices tables and returns the correct ID mapping.
func TestImportDevices_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	// Seed admin user.
	var adminID int64
	err := pool.QueryRow(ctx,
		"INSERT INTO users (email, password_hash, name, role) VALUES ($1, 'hash', 'Admin', 'admin') RETURNING id",
		"admin@test.local",
	).Scan(&adminID)
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	devices := []TraccarDevice{
		{ID: 1, Name: "Car A", UniqueID: "ANON-001", Phone: "+4900000001", Model: "GPS-4G Tracker", Status: "offline"},
		{ID: 2, Name: "Watch B", UniqueID: "ANON-002", Phone: "", Model: "Q50 Watch", Status: "online"},
		// Placeholder: name == uniqueID → renamed to "Device ANON-003"
		{ID: 3, Name: "ANON-003", UniqueID: "ANON-003", Status: "offline"},
		// Empty status → defaults to "offline"
		{ID: 4, Name: "Car D", UniqueID: "ANON-004", Status: ""},
	}

	config := &Config{Verbose: true}
	deviceMap, err := importDevices(ctx, pool, devices, adminID, config)
	if err != nil {
		t.Fatalf("importDevices: %v", err)
	}

	if len(deviceMap) != 4 {
		t.Errorf("deviceMap len = %d, want 4", len(deviceMap))
	}
	for traccarID, motusID := range deviceMap {
		if motusID <= 0 {
			t.Errorf("deviceMap[%d] = %d (invalid Motus ID)", traccarID, motusID)
		}
	}

	// Verify device row in DB.
	var name, protocol string
	err = pool.QueryRow(ctx, "SELECT name, protocol FROM devices WHERE unique_id = $1", "ANON-001").Scan(&name, &protocol)
	if err != nil {
		t.Fatalf("query device ANON-001: %v", err)
	}
	if name != "Car A" {
		t.Errorf("name = %q, want Car A", name)
	}
	if protocol != "h02" {
		t.Errorf("protocol = %q, want h02", protocol)
	}

	// Watch device must use "watch" protocol.
	err = pool.QueryRow(ctx, "SELECT protocol FROM devices WHERE unique_id = $1", "ANON-002").Scan(&protocol)
	if err != nil {
		t.Fatalf("query device ANON-002: %v", err)
	}
	if protocol != "watch" {
		t.Errorf("watch protocol = %q, want watch", protocol)
	}

	// Placeholder device should be renamed.
	err = pool.QueryRow(ctx, "SELECT name FROM devices WHERE unique_id = $1", "ANON-003").Scan(&name)
	if err != nil {
		t.Fatalf("query device ANON-003: %v", err)
	}
	if !strings.HasPrefix(name, "Device ") {
		t.Errorf("placeholder device name = %q, want prefix 'Device '", name)
	}

	// All devices should be associated with admin in user_devices.
	var count int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM user_devices WHERE user_id = $1", adminID).Scan(&count)
	if err != nil {
		t.Fatalf("count user_devices: %v", err)
	}
	if count != 4 {
		t.Errorf("user_devices count = %d, want 4", count)
	}

	// Re-importing the same devices (upsert) must not error.
	_, err = importDevices(ctx, pool, devices[:1], adminID, config)
	if err != nil {
		t.Errorf("upsert importDevices: %v", err)
	}
}

// TestImportPositions_Integration verifies batch insertion, device mapping,
// coordinate validation (skip 0,0), and invalid device skipping.
func TestImportPositions_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	var adminID int64
	if err := pool.QueryRow(ctx,
		"INSERT INTO users (email, password_hash, name, role) VALUES ($1, 'hash', 'Admin', 'admin') RETURNING id",
		"admin@positions.local",
	).Scan(&adminID); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	devices := []TraccarDevice{
		{ID: 10, Name: "TestCar", UniqueID: "POS-001", Status: "offline"},
	}
	config := &Config{Verbose: true}
	deviceMap, err := importDevices(ctx, pool, devices, adminID, config)
	if err != nil {
		t.Fatalf("importDevices: %v", err)
	}

	now := time.Now().UTC()
	positions := []TraccarPosition{
		// Valid positions.
		{ID: 1, DeviceID: 10, Valid: true, Latitude: 52.001, Longitude: 10.001, Speed: 5.0, FixTime: now.Add(-2 * time.Hour)},
		{ID: 2, DeviceID: 10, Valid: true, Latitude: 52.002, Longitude: 10.002, Speed: 10.0, FixTime: now.Add(-1 * time.Hour)},
		// Invalid: coordinates (0,0) — skipped.
		{ID: 3, DeviceID: 10, Valid: true, Latitude: 0, Longitude: 0, FixTime: now.Add(-30 * time.Minute)},
		// Invalid: valid=false — skipped.
		{ID: 4, DeviceID: 10, Valid: false, Latitude: 52.003, Longitude: 10.003, FixTime: now.Add(-20 * time.Minute)},
		// Unknown device ID — skipped.
		{ID: 5, DeviceID: 999, Valid: true, Latitude: 52.004, Longitude: 10.004, FixTime: now},
	}

	if err := importPositions(ctx, pool, positions, deviceMap, config); err != nil {
		t.Fatalf("importPositions: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM positions").Scan(&count); err != nil {
		t.Fatalf("count positions: %v", err)
	}
	if count != 2 {
		t.Errorf("positions count = %d, want 2 (3 skipped)", count)
	}

	// Empty positions slice → no-op.
	if err := importPositions(ctx, pool, nil, deviceMap, config); err != nil {
		t.Errorf("empty importPositions: %v", err)
	}
}

// TestUpdateDeviceLastUpdate_Integration verifies that last_update is set
// correctly from the maximum position timestamp.
func TestUpdateDeviceLastUpdate_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	var adminID int64
	if err := pool.QueryRow(ctx,
		"INSERT INTO users (email, password_hash, name, role) VALUES ($1, 'hash', 'Admin', 'admin') RETURNING id",
		"admin@lastupdate.local",
	).Scan(&adminID); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	config := &Config{}
	deviceMap, err := importDevices(ctx, pool, []TraccarDevice{
		{ID: 20, Name: "LU Car", UniqueID: "LU-001", Status: "offline"},
	}, adminID, config)
	if err != nil {
		t.Fatalf("importDevices: %v", err)
	}

	oldest := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	newest := time.Date(2026, 1, 2, 9, 0, 0, 0, time.UTC)
	positions := []TraccarPosition{
		{ID: 1, DeviceID: 20, Valid: true, Latitude: 52.0, Longitude: 10.0, FixTime: oldest},
		{ID: 2, DeviceID: 20, Valid: true, Latitude: 52.1, Longitude: 10.1, FixTime: newest},
	}
	if err := importPositions(ctx, pool, positions, deviceMap, config); err != nil {
		t.Fatalf("importPositions: %v", err)
	}

	if err := updateDeviceLastUpdate(ctx, pool, deviceMap); err != nil {
		t.Fatalf("updateDeviceLastUpdate: %v", err)
	}

	motusID := deviceMap[20]
	var lastUpdate *time.Time
	if err := pool.QueryRow(ctx, "SELECT last_update FROM devices WHERE id = $1", motusID).Scan(&lastUpdate); err != nil {
		t.Fatalf("query last_update: %v", err)
	}
	if lastUpdate == nil {
		t.Fatal("last_update is nil, want non-nil")
	}
	if !lastUpdate.Truncate(time.Second).Equal(newest.Truncate(time.Second)) {
		t.Errorf("last_update = %v, want %v", lastUpdate, newest)
	}
}

// TestImportCalendars_Integration verifies calendar insertion, base64 decoding,
// normalisation, and user_calendars association.
func TestImportCalendars_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	var adminID int64
	if err := pool.QueryRow(ctx,
		"INSERT INTO users (email, password_hash, name, role) VALUES ($1, 'hash', 'Admin', 'admin') RETURNING id",
		"admin@calendars.local",
	).Scan(&adminID); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	// Calendar 1: raw (not base64) iCal text.
	rawICal := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\n" +
		"DTSTART;TZID=Europe/Berlin:20251103T090000\r\n" +
		"DTEND;TZID=Europe/Berlin:20251103T170000\r\n" +
		"RRULE:FREQ=DAILY\r\n" +
		"SUMMARY:Shift\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

	// Calendar 2: base64-encoded (Traccar's storage format).
	b64ICal := base64.StdEncoding.EncodeToString([]byte(rawICal))

	// Calendar 3: multi-day DTEND without UNTIL — normalisation adds UNTIL.
	needsNorm := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\n" +
		"DTSTART;TZID=Europe/Berlin:20251105T200000\r\n" +
		"DTEND;TZID=Europe/Berlin:20251110T200000\r\n" +
		"RRULE:FREQ=DAILY\r\n" +
		"SUMMARY:Maintenance\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

	calendars := []TraccarCalendar{
		{ID: 1, Name: "Shift", Data: rawICal},
		{ID: 2, Name: "ShiftB64", Data: b64ICal},
		{ID: 3, Name: "Maintenance", Data: needsNorm},
	}

	config := &Config{Verbose: true}
	calMap, err := importCalendars(ctx, pool, calendars, adminID, config)
	if err != nil {
		t.Fatalf("importCalendars: %v", err)
	}

	if len(calMap) != 3 {
		t.Fatalf("calMap len = %d, want 3", len(calMap))
	}
	for traccarID, motusID := range calMap {
		if motusID <= 0 {
			t.Errorf("calMap[%d] = %d (invalid)", traccarID, motusID)
		}
	}

	// Verify normalisation was applied to calendar 3.
	var data string
	if err := pool.QueryRow(ctx, "SELECT data FROM calendars WHERE id = $1", calMap[3]).Scan(&data); err != nil {
		t.Fatalf("query calendar data: %v", err)
	}
	if !strings.Contains(data, "UNTIL=") {
		t.Errorf("normalised calendar should contain UNTIL, got %q", data[:min(100, len(data))])
	}

	// user_calendars should have 3 rows.
	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM user_calendars WHERE user_id = $1", adminID).Scan(&count); err != nil {
		t.Fatalf("count user_calendars: %v", err)
	}
	if count != 3 {
		t.Errorf("user_calendars count = %d, want 3", count)
	}
}

// TestImportGeofences_Integration verifies POLYGON and CIRCLE import,
// user_geofences association, and calendar linking.
func TestImportGeofences_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	var adminID int64
	if err := pool.QueryRow(ctx,
		"INSERT INTO users (email, password_hash, name, role) VALUES ($1, 'hash', 'Admin', 'admin') RETURNING id",
		"admin@geofences.local",
	).Scan(&adminID); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	// Seed a calendar to link to a geofence.
	var calMotusID int64
	if err := pool.QueryRow(ctx,
		"INSERT INTO calendars (user_id, name, data, created_at, updated_at) VALUES ($1, 'Cal', 'data', NOW(), NOW()) RETURNING id",
		adminID,
	).Scan(&calMotusID); err != nil {
		t.Fatalf("seed calendar: %v", err)
	}
	calID := int64(99) // Traccar calendar ID
	calMap := map[int64]int64{calID: calMotusID}

	geofences := []TraccarGeofence{
		// POLYGON in Traccar lat,lon order (will be swapped for PostGIS).
		{
			ID:   1,
			Name: "Home Base",
			Area: "POLYGON ((52.01 10.01, 52.00 10.01, 52.00 10.02, 52.01 10.02, 52.01 10.01))",
		},
		// CIRCLE format with calendar link.
		{
			ID:         2,
			Name:       "Workshop",
			Area:       "CIRCLE (52.50 13.40, 500)",
			CalendarID: &calID,
		},
	}

	config := &Config{Verbose: true}
	if err := importGeofences(ctx, pool, geofences, adminID, calMap, config); err != nil {
		t.Fatalf("importGeofences: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM geofences").Scan(&count); err != nil {
		t.Fatalf("count geofences: %v", err)
	}
	if count != 2 {
		t.Errorf("geofences count = %d, want 2", count)
	}

	// Verify calendar link on Workshop geofence.
	var linkedCalID *int64
	if err := pool.QueryRow(ctx, "SELECT calendar_id FROM geofences WHERE name = 'Workshop'").Scan(&linkedCalID); err != nil {
		t.Fatalf("query workshop calendar_id: %v", err)
	}
	if linkedCalID == nil || *linkedCalID != calMotusID {
		t.Errorf("Workshop calendar_id = %v, want %d", linkedCalID, calMotusID)
	}

	// user_geofences should have 2 rows.
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM user_geofences WHERE user_id = $1", adminID).Scan(&count); err != nil {
		t.Fatalf("count user_geofences: %v", err)
	}
	if count != 2 {
		t.Errorf("user_geofences count = %d, want 2", count)
	}

	// DryRun=true: geofences are not inserted.
	testutil.CleanTables(t, pool)
	if err := pool.QueryRow(ctx,
		"INSERT INTO users (email, password_hash, name, role) VALUES ($1, 'hash', 'Admin2', 'admin') RETURNING id",
		"admin2@geofences.local",
	).Scan(&adminID); err != nil {
		t.Fatalf("re-seed admin: %v", err)
	}
	dryConfig := &Config{DryRun: true, Verbose: true}
	if err := importGeofences(ctx, pool, geofences, adminID, nil, dryConfig); err != nil {
		t.Errorf("dry-run importGeofences: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM geofences").Scan(&count); err != nil {
		t.Fatalf("count geofences after dry-run: %v", err)
	}
	if count != 0 {
		t.Errorf("dry-run should not insert geofences, got %d", count)
	}
}

// TestGeocodeRecentPositions_Integration tests the geocoding path. We use
// GeocodeLastN=0 to exercise only the no-positions branch (no external HTTP call).
func TestGeocodeRecentPositions_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	// Insert a device and a position with an address already set (→ no geocoding needed).
	var adminID int64
	if err := pool.QueryRow(ctx,
		"INSERT INTO users (email, password_hash, name, role) VALUES ($1, 'hash', 'Admin', 'admin') RETURNING id",
		"admin@geocode.local",
	).Scan(&adminID); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	var devID int64
	if err := pool.QueryRow(ctx,
		"INSERT INTO devices (unique_id, name, protocol, status, created_at, updated_at) VALUES ('GEO-001','GeoTest','h02','offline',NOW(),NOW()) RETURNING id",
	).Scan(&devID); err != nil {
		t.Fatalf("seed device: %v", err)
	}
	if _, err := pool.Exec(ctx,
		"INSERT INTO positions (device_id, latitude, longitude, altitude, speed, course, timestamp, address) VALUES ($1, 52.0, 10.0, 0, 0, 0, NOW(), 'Test Street 1')",
		devID,
	); err != nil {
		t.Fatalf("seed position with address: %v", err)
	}

	// With GeocodeLastN=5 but all positions already have addresses → no-op.
	config := &Config{GeocodeLastN: 5}
	if err := geocodeRecentPositions(ctx, pool, config); err != nil {
		t.Errorf("geocodeRecentPositions (all addressed): %v", err)
	}

	// With RecentDays filter that excludes the only position → no-op.
	config2 := &Config{GeocodeLastN: 5, RecentDays: 1}
	if err := geocodeRecentPositions(ctx, pool, config2); err != nil {
		t.Errorf("geocodeRecentPositions (recent filter): %v", err)
	}
}

// TestGeocodeRecentPositions_WithNullAddress exercises the geocoding loop by
// inserting a position without an address. The Nominatim HTTP call may fail in
// CI; the function uses a coordinate-based fallback so it must always succeed.
func TestGeocodeRecentPositions_WithNullAddress(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	var devID int64
	if err := pool.QueryRow(ctx,
		"INSERT INTO devices (unique_id, name, protocol, status, created_at, updated_at) VALUES ('GEO-NULL-001','NullAddrDev','h02','offline',NOW(),NOW()) RETURNING id",
	).Scan(&devID); err != nil {
		t.Fatalf("seed device: %v", err)
	}
	// Insert a position with a NULL address.
	if _, err := pool.Exec(ctx,
		"INSERT INTO positions (device_id, latitude, longitude, altitude, speed, course, timestamp) VALUES ($1, 52.52, 13.405, 0, 0, 0, NOW())",
		devID,
	); err != nil {
		t.Fatalf("seed position without address: %v", err)
	}

	// Run geocoding with Verbose=true to also cover the progress logging branch.
	// The geocoder may fail (no network), but falls back to coordinates.
	config := &Config{GeocodeLastN: 1, Verbose: true}
	if err := geocodeRecentPositions(ctx, pool, config); err != nil {
		t.Errorf("geocodeRecentPositions with null address: %v", err)
	}

	// The position must now have a non-empty address (either Nominatim or fallback).
	var addr string
	if err := pool.QueryRow(ctx,
		"SELECT COALESCE(address,'') FROM positions WHERE device_id = $1",
		devID,
	).Scan(&addr); err != nil {
		t.Fatalf("query address: %v", err)
	}
	if addr == "" {
		t.Error("expected position to have an address after geocodeRecentPositions")
	}
}

// TestGeocodeRecentPositions_CancelledContext exercises the pool.Query error
// path when the context is cancelled before the query runs.
func TestGeocodeRecentPositions_CancelledContext(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel to force query failure

	config := &Config{GeocodeLastN: 5}
	err := geocodeRecentPositions(ctx, pool, config)
	if err == nil {
		t.Error("expected error when context is pre-cancelled")
	}
}

// TestRunImport_DryRun exercises runImport with DryRun=true.
// No database connection is required; it parses the dump and logs a summary.
func TestRunImport_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	dumpPath := tmpDir + "/dryrun.sql"

	b64Cal := base64.StdEncoding.EncodeToString([]byte("BEGIN:VCALENDAR\nVERSION:2.0\nEND:VCALENDAR"))
	content := "COPY public.tc_devices (id, name, uniqueid, lastupdate, positionid, groupid, attributes, phone, model, contact, category, disabled, status) FROM stdin;\n" +
		"1\tDryDevice\tDRY-001\t2026-01-01 00:00:00\t\\N\t\\N\t{}\t\\N\t\\N\t\\N\t\\N\tf\toffline\n" +
		"\\.\n" +
		"COPY public.tc_calendars (id, name, data, attributes) FROM stdin;\n" +
		"1\tDryCal\t" + b64Cal + "\t{}\n" +
		"\\.\n" +
		"COPY public.tc_geofences (id, name, description, area, attributes, calendarid) FROM stdin;\n" +
		"1\tDryFence\t\\N\tPOLYGON ((52.0 10.0, 52.1 10.0, 52.1 10.1, 52.0 10.1, 52.0 10.0))\t{}\t\\N\n" +
		"\\.\n"

	if err := writeTestFile(dumpPath, content); err != nil {
		t.Fatalf("write dump: %v", err)
	}

	config := &Config{
		SourceDump:      dumpPath,
		DryRun:          true,
		Verbose:         true,
		ImportDevices:   true,
		ImportPositions: true,
		ImportGeofences: true,
		ImportCalendars: true,
	}

	if err := runImport(config); err != nil {
		t.Errorf("runImport dry-run: %v", err)
	}
}

// TestRunImport_WithDB exercises the full runImport pipeline against a real
// PostGIS testcontainer: parse dump → connect → import devices + calendars +
// geofences → verify DB state.
func TestRunImport_WithDB(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	connStr := testutil.ConnStr(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	// Seed admin user.
	if _, err := pool.Exec(ctx,
		"INSERT INTO users (email, password_hash, name, role) VALUES ($1, 'hash', 'Admin', 'admin')",
		"admin@runimport.local",
	); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	host, port, dbname, user, password, err := connStrToConfig(connStr)
	if err != nil {
		t.Fatalf("parse connStr: %v", err)
	}

	tmpDir := t.TempDir()
	dumpPath := tmpDir + "/full.sql"

	b64Cal := base64.StdEncoding.EncodeToString([]byte(
		"BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\n" +
			"DTSTART;TZID=Europe/Berlin:20251103T090000\r\n" +
			"DTEND;TZID=Europe/Berlin:20251103T170000\r\n" +
			"RRULE:FREQ=DAILY\r\n" +
			"SUMMARY:Shift\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
	))

	dumpContent := "COPY public.tc_devices (id, name, uniqueid, lastupdate, positionid, groupid, attributes, phone, model, contact, category, disabled, status) FROM stdin;\n" +
		"1\tTestCar\tRI-001\t2026-01-15 10:00:00\t\\N\t\\N\t{}\t+4900000001\tGPS-4G\t\\N\t\\N\tf\toffline\n" +
		"\\.\n" +
		"COPY public.tc_positions (id, protocol, deviceid, servertime, devicetime, fixtime, valid, latitude, longitude, altitude, speed, course, address, attributes) FROM stdin;\n" +
		"1\th02\t1\t2026-01-10 08:00:00\t2026-01-10 08:00:00\t2026-01-10 08:00:00\tt\t52.001\t10.001\t0\t5.0\t90\t\\N\t{}\n" +
		"2\th02\t1\t2026-01-11 08:00:00\t2026-01-11 08:00:00\t2026-01-11 08:00:00\tt\t52.002\t10.002\t0\t0.0\t0\t\\N\t{}\n" +
		"\\.\n" +
		"COPY public.tc_calendars (id, name, data, attributes) FROM stdin;\n" +
		"1\tShift\t" + b64Cal + "\t{}\n" +
		"\\.\n" +
		"COPY public.tc_geofences (id, name, description, area, attributes, calendarid) FROM stdin;\n" +
		"1\tHome Base\t\\N\tPOLYGON ((52.01 10.01, 52.00 10.01, 52.00 10.02, 52.01 10.02, 52.01 10.01))\t{}\t\\N\n" +
		"\\.\n"

	if err := writeTestFile(dumpPath, dumpContent); err != nil {
		t.Fatalf("write dump: %v", err)
	}

	config := &Config{
		SourceDump:      dumpPath,
		TargetHost:      host,
		TargetPort:      port,
		TargetDB:        dbname,
		TargetUser:      user,
		TargetPassword:  password,
		AdminEmail:      "admin@runimport.local",
		RecentDays:      0,
		MaxPositions:    0,
		GeocodeLastN:    1, // exercise the GeocodeLastN > 0 branch
		Verbose:         true,
		ImportDevices:   true,
		ImportPositions: true,
		ImportGeofences: true,
		ImportCalendars: true,
	}

	if err := runImport(config); err != nil {
		t.Fatalf("runImport: %v", err)
	}

	// Verify device was imported.
	var devCount int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM devices WHERE unique_id = 'RI-001'").Scan(&devCount); err != nil {
		t.Fatalf("count devices: %v", err)
	}
	if devCount != 1 {
		t.Errorf("devices count = %d, want 1", devCount)
	}

	// Verify positions were imported (2 valid positions).
	var posCount int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM positions").Scan(&posCount); err != nil {
		t.Fatalf("count positions: %v", err)
	}
	if posCount != 2 {
		t.Errorf("positions count = %d, want 2", posCount)
	}

	// Verify calendar was imported.
	var calCount int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM calendars").Scan(&calCount); err != nil {
		t.Fatalf("count calendars: %v", err)
	}
	if calCount != 1 {
		t.Errorf("calendars count = %d, want 1", calCount)
	}

	// Verify geofence was imported.
	var geoCount int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM geofences").Scan(&geoCount); err != nil {
		t.Fatalf("count geofences: %v", err)
	}
	if geoCount != 1 {
		t.Errorf("geofences count = %d, want 1", geoCount)
	}

	// Running again with same data must succeed (upsert semantics).
	if err := runImport(config); err != nil {
		t.Errorf("runImport second pass: %v", err)
	}
}

// TestExtractFromDB_Integration tests the extractFromDB code path by creating
// Traccar-like tables in the test container and calling extractFromDB against it.
func TestExtractFromDB_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()
	connStr := testutil.ConnStr(t)

	host, port, dbname, user, password, err := connStrToConfig(connStr)
	if err != nil {
		t.Fatalf("parse connStr: %v", err)
	}

	// Create Traccar-like tables in the test DB.
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS tc_devices (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			uniqueid TEXT NOT NULL,
			phone TEXT,
			model TEXT,
			category TEXT,
			disabled BOOLEAN NOT NULL DEFAULT FALSE,
			status TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS tc_positions (
			id BIGSERIAL PRIMARY KEY,
			protocol TEXT,
			deviceid BIGINT,
			servertime TIMESTAMP,
			devicetime TIMESTAMP,
			fixtime TIMESTAMP,
			valid BOOLEAN,
			latitude DOUBLE PRECISION,
			longitude DOUBLE PRECISION,
			altitude DOUBLE PRECISION,
			speed DOUBLE PRECISION,
			course DOUBLE PRECISION,
			address TEXT,
			attributes TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS tc_geofences (
			id BIGSERIAL PRIMARY KEY,
			name TEXT,
			description TEXT,
			area TEXT,
			calendarid BIGINT
		)`,
		`CREATE TABLE IF NOT EXISTS tc_calendars (
			id BIGSERIAL PRIMARY KEY,
			name TEXT,
			data BYTEA
		)`,
	} {
		if _, err := pool.Exec(ctx, ddl); err != nil {
			t.Fatalf("create traccar table: %v", err)
		}
	}

	// Clean up Traccar tables after the test.
	t.Cleanup(func() {
		for _, tbl := range []string{"tc_positions", "tc_geofences", "tc_calendars", "tc_devices"} {
			_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS "+tbl)
		}
	})

	// Insert test data.
	var devID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO tc_devices (name, uniqueid, phone, model, category, disabled, status)
		 VALUES ('Test Car', 'TC-EDB-001', '+49123', 'GPS-4G', 'car', false, 'online')
		 RETURNING id`,
	).Scan(&devID); err != nil {
		t.Fatalf("insert tc_device: %v", err)
	}
	// Also insert an 'unknown' device to test ExcludeUnknown filter.
	if _, err := pool.Exec(ctx,
		`INSERT INTO tc_devices (name, uniqueid, disabled, status) VALUES ('Unknown', 'TC-UNK-001', false, 'unknown')`,
	); err != nil {
		t.Fatalf("insert unknown device: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx,
		`INSERT INTO tc_positions (protocol, deviceid, servertime, devicetime, fixtime, valid, latitude, longitude, altitude, speed, course, address, attributes)
		 VALUES ('h02', $1, $2, $2, $2, true, 52.5, 13.4, 100, 10, 90, null, '{}')`,
		devID, now,
	); err != nil {
		t.Fatalf("insert tc_position: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO tc_geofences (name, description, area, calendarid)
		 VALUES ('Home', 'Home base', 'POLYGON ((52.0 13.0, 52.1 13.0, 52.1 13.1, 52.0 13.1, 52.0 13.0))', null)`,
	); err != nil {
		t.Fatalf("insert tc_geofence: %v", err)
	}

	icalData := []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nEND:VCALENDAR\r\n")
	var calID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO tc_calendars (name, data) VALUES ('TestCal', $1) RETURNING id`,
		icalData,
	).Scan(&calID); err != nil {
		t.Fatalf("insert tc_calendar: %v", err)
	}

	// Basic extraction: all scopes enabled.
	config := &Config{
		SourceDBHost:    host,
		SourceDBPort:    port,
		SourceDBName:    dbname,
		SourceDBUser:    user,
		SourceDBPass:    password,
		ImportDevices:   true,
		ImportPositions: true,
		ImportGeofences: true,
		ImportCalendars: true,
		Verbose:         true,
	}

	devices, positions, geofences, calendars, err := extractFromDB(ctx, config)
	if err != nil {
		t.Fatalf("extractFromDB: %v", err)
	}
	// Both devices (known + unknown) should be returned without ExcludeUnknown.
	if len(devices) != 2 {
		t.Errorf("expected 2 devices, got %d", len(devices))
	}
	if len(positions) != 1 {
		t.Errorf("expected 1 position, got %d", len(positions))
	}
	if len(geofences) != 1 {
		t.Errorf("expected 1 geofence, got %d", len(geofences))
	}
	if len(calendars) != 1 {
		t.Errorf("expected 1 calendar, got %d", len(calendars))
	}

	// With ExcludeUnknown: only the known device should appear.
	config2 := *config
	config2.ExcludeUnknown = true
	devs2, _, _, _, err := extractFromDB(ctx, &config2)
	if err != nil {
		t.Fatalf("extractFromDB (exclude-unknown): %v", err)
	}
	if len(devs2) != 1 {
		t.Errorf("expected 1 device with exclude-unknown, got %d", len(devs2))
	}

	// With DeviceFilter: only the matching device.
	config3 := *config
	config3.DeviceFilter = "TC-EDB-001"
	devs3, _, _, _, err := extractFromDB(ctx, &config3)
	if err != nil {
		t.Fatalf("extractFromDB (device-filter): %v", err)
	}
	if len(devs3) != 1 {
		t.Errorf("expected 1 device with device-filter, got %d", len(devs3))
	}

	// With RecentDays=1 and MaxPositions=10: positions within 1 day, max 10.
	config4 := *config
	config4.RecentDays = 1
	config4.MaxPositions = 10
	_, pos4, _, _, err := extractFromDB(ctx, &config4)
	if err != nil {
		t.Fatalf("extractFromDB (recent-days + max-positions): %v", err)
	}
	if len(pos4) != 1 {
		t.Errorf("expected 1 position with recent-days=1, got %d", len(pos4))
	}
}

// TestRunImport_ParseDumpError verifies that runImport returns an error when the
// dump file cannot be opened (non-existent path).
func TestRunImport_ParseDumpError(t *testing.T) {
	config := &Config{
		SourceDump:    "/nonexistent/path/that/does/not/exist.sql",
		DryRun:        true,
		ImportDevices: true,
	}
	if err := runImport(config); err == nil {
		t.Error("expected error for non-existent dump file, got nil")
	}
}

// TestNewCmd_RunE_ValidateConfigError verifies that the RunE closure returns an
// error (without calling os.Exit) when validateConfig detects an invalid config.
func TestNewCmd_RunE_ValidateConfigError(t *testing.T) {
	cmd := NewCmd()
	// Default flags: no source-dump and no source-db → validateConfig returns error.
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Error("expected error from validateConfig, got nil")
	}
}

// TestImportGeofences_CircleParseError verifies that importGeofences logs a
// warning and skips a geofence whose CIRCLE WKT cannot be parsed.
func TestImportGeofences_CircleParseError(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	var adminID int64
	if err := pool.QueryRow(ctx,
		"INSERT INTO users (email, password_hash, name, role) VALUES ($1, 'hash', 'Admin', 'admin') RETURNING id",
		"admin@circleerr.local",
	).Scan(&adminID); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	// A CIRCLE geofence with no comma — parseTraccarCircle returns error.
	geofences := []TraccarGeofence{
		{ID: 1, Name: "Bad Circle", Area: "CIRCLE (no comma here)"},
	}
	config := &Config{Verbose: true}
	if err := importGeofences(ctx, pool, geofences, adminID, nil, config); err != nil {
		t.Errorf("importGeofences with bad CIRCLE should not return error, got: %v", err)
	}

	// The geofence should NOT have been inserted.
	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM geofences").Scan(&count); err != nil {
		t.Fatalf("count geofences: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 geofences (bad CIRCLE skipped), got %d", count)
	}
}

// TestImportGeofences_InvalidWKT verifies that importGeofences logs a warning
// and skips a geofence whose WKT geometry is invalid (ST_GeomFromText fails).
func TestImportGeofences_InvalidWKT(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	var adminID int64
	if err := pool.QueryRow(ctx,
		"INSERT INTO users (email, password_hash, name, role) VALUES ($1, 'hash', 'Admin', 'admin') RETURNING id",
		"admin@invalidwkt.local",
	).Scan(&adminID); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	// Area is not CIRCLE and is invalid WKT → ST_GeomFromText fails.
	geofences := []TraccarGeofence{
		{ID: 1, Name: "Bad WKT Fence", Area: "NOTVALID_WKT"},
	}
	config := &Config{Verbose: true}
	if err := importGeofences(ctx, pool, geofences, adminID, nil, config); err != nil {
		t.Errorf("importGeofences with invalid WKT should not return error, got: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM geofences").Scan(&count); err != nil {
		t.Fatalf("count geofences: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 geofences (invalid WKT skipped), got %d", count)
	}
}

// min returns the smaller of two ints (needed for string truncation below go1.21).
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

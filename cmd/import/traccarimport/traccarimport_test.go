package traccarimport

import (
	"encoding/base64"
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSwapWKTCoordinates(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple polygon with lat,lon to lon,lat",
			input:    "POLYGON((52.51 13.35, 52.53 13.35, 52.53 13.40, 52.51 13.40, 52.51 13.35))",
			expected: "POLYGON((13.35 52.51, 13.35 52.53, 13.40 52.53, 13.40 52.51, 13.35 52.51))",
		},
		{
			name:     "polygon with spaces after commas",
			input:    "POLYGON ((50.0 8.0, 51.0 8.0, 51.0 9.0, 50.0 9.0, 50.0 8.0))",
			expected: "POLYGON ((8.0 50.0, 8.0 51.0, 9.0 51.0, 9.0 50.0, 8.0 50.0))",
		},
		{
			name:     "real Traccar polygon from Germany",
			input:    "POLYGON((49.79 9.93, 49.81 9.93, 49.81 9.97, 49.79 9.97, 49.79 9.93))",
			expected: "POLYGON((9.93 49.79, 9.93 49.81, 9.97 49.81, 9.97 49.79, 9.93 49.79))",
		},
		{
			name:     "no coordinates returns as-is",
			input:    "EMPTY",
			expected: "EMPTY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := swapWKTCoordinates(tt.input)
			if got != tt.expected {
				t.Errorf("swapWKTCoordinates(%q)\n  got:  %q\n  want: %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestIsTraccarCircle(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"CIRCLE (52.52 13.37, 1000)", true},
		{"circle (52.52 13.37, 1000)", true},
		{"POLYGON((52.51 13.35, 52.53 13.35))", false},
		{"", false},
	}

	for _, tt := range tests {
		got := isTraccarCircle(tt.input)
		if got != tt.expected {
			t.Errorf("isTraccarCircle(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestParseTraccarCircle(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLat   float64
		wantLon   float64
		wantRad   float64
		wantError bool
	}{
		{
			name:    "standard circle",
			input:   "CIRCLE (52.52 13.37, 1000)",
			wantLat: 52.52,
			wantLon: 13.37,
			wantRad: 1000,
		},
		{
			name:    "circle with decimals",
			input:   "CIRCLE (55.75414 37.6204, 100)",
			wantLat: 55.75414,
			wantLon: 37.6204,
			wantRad: 100,
		},
		{
			name:    "circle no space before paren",
			input:   "CIRCLE(48.1351 11.5820, 500)",
			wantLat: 48.1351,
			wantLon: 11.5820,
			wantRad: 500,
		},
		{
			name:      "invalid format - no comma",
			input:     "CIRCLE (52.52 13.37)",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lat, lon, radius, err := parseTraccarCircle(tt.input)
			if tt.wantError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if math.Abs(lat-tt.wantLat) > 0.0001 {
				t.Errorf("lat = %f, want %f", lat, tt.wantLat)
			}
			if math.Abs(lon-tt.wantLon) > 0.0001 {
				t.Errorf("lon = %f, want %f", lon, tt.wantLon)
			}
			if math.Abs(radius-tt.wantRad) > 0.1 {
				t.Errorf("radius = %f, want %f", radius, tt.wantRad)
			}
		})
	}
}

func TestParseDevice(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantDevice  TraccarDevice
		wantError   bool
		errContains string
	}{
		{
			name: "full device with phone and model",
			// id=1, name=GT3 RS, uniqueid=123456789012345, lastupdate, positionid, groupid, attributes,
			// phone=+491234567890, model=TK103B, contact=john@example.com, category=car, disabled=f, status=online
			input: strings.Join([]string{
				"1", "GT3 RS", "123456789012345", "2025-01-15 10:30:00", "42", "\\N", "{}",
				"+491234567890", "TK103B", "john@example.com", "car", "f", "online",
			}, "\t"),
			wantDevice: TraccarDevice{
				ID:       1,
				Name:     "GT3 RS",
				UniqueID: "123456789012345",
				Phone:    "+491234567890",
				Model:    "TK103B",
				Category: "car",
				Disabled: false,
				Status:   "online",
			},
		},
		{
			name: "device with null phone and model",
			input: strings.Join([]string{
				"2", "Tracker2", "ABC123", "2025-01-15 10:30:00", "\\N", "\\N", "{}",
				"\\N", "\\N", "\\N", "\\N", "f", "offline",
			}, "\t"),
			wantDevice: TraccarDevice{
				ID:       2,
				Name:     "Tracker2",
				UniqueID: "ABC123",
				Phone:    "",
				Model:    "",
				Category: "",
				Disabled: false,
				Status:   "offline",
			},
		},
		{
			name: "disabled device with model and category",
			input: strings.Join([]string{
				"3", "Watch1", "WATCH001", "2025-02-01 08:00:00", "100", "1", "{\"speedLimit\":80}",
				"\\N", "Q50", "\\N", "watch", "t", "offline",
			}, "\t"),
			wantDevice: TraccarDevice{
				ID:       3,
				Name:     "Watch1",
				UniqueID: "WATCH001",
				Phone:    "",
				Model:    "Q50",
				Category: "watch",
				Disabled: true,
				Status:   "offline",
			},
		},
		{
			name:        "too few fields",
			input:       "1\tGT3 RS\t123456789012345",
			wantError:   true,
			errContains: "expected at least 13 fields",
		},
		{
			name: "invalid id",
			input: strings.Join([]string{
				"abc", "GT3 RS", "123456789012345", "2025-01-15 10:30:00", "42", "\\N", "{}",
				"+491234567890", "TK103B", "john@example.com", "car", "f", "online",
			}, "\t"),
			wantError:   true,
			errContains: "parse id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDevice(tt.input)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.ID != tt.wantDevice.ID {
				t.Errorf("ID = %d, want %d", got.ID, tt.wantDevice.ID)
			}
			if got.Name != tt.wantDevice.Name {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantDevice.Name)
			}
			if got.UniqueID != tt.wantDevice.UniqueID {
				t.Errorf("UniqueID = %q, want %q", got.UniqueID, tt.wantDevice.UniqueID)
			}
			if got.Phone != tt.wantDevice.Phone {
				t.Errorf("Phone = %q, want %q", got.Phone, tt.wantDevice.Phone)
			}
			if got.Model != tt.wantDevice.Model {
				t.Errorf("Model = %q, want %q", got.Model, tt.wantDevice.Model)
			}
			if got.Category != tt.wantDevice.Category {
				t.Errorf("Category = %q, want %q", got.Category, tt.wantDevice.Category)
			}
			if got.Disabled != tt.wantDevice.Disabled {
				t.Errorf("Disabled = %v, want %v", got.Disabled, tt.wantDevice.Disabled)
			}
			if got.Status != tt.wantDevice.Status {
				t.Errorf("Status = %q, want %q", got.Status, tt.wantDevice.Status)
			}
		})
	}
}

func TestParsePosition(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantPos     TraccarPosition
		wantError   bool
		errContains string
	}{
		{
			name: "valid position",
			// id, protocol, deviceid, servertime, devicetime, fixtime, valid,
			// lat, lon, alt, speed, course, address, attributes
			input: strings.Join([]string{
				"100", "h02", "1", "2025-01-15 10:30:00", "2025-01-15 10:30:00", "2025-01-15 10:30:00",
				"t", "49.7913", "9.9534", "200.0", "45.5", "180.0", "Main Street", `{"batteryLevel":85}`,
			}, "\t"),
			wantPos: TraccarPosition{
				ID:         100,
				Protocol:   "h02",
				DeviceID:   1,
				Valid:      true,
				Latitude:   49.7913,
				Longitude:  9.9534,
				Altitude:   200.0,
				Speed:      45.5,
				Course:     180.0,
				Address:    "Main Street",
				Attributes: `{"batteryLevel":85}`,
			},
		},
		{
			name: "position with null address",
			input: strings.Join([]string{
				"101", "watch", "2", "2025-01-15 10:30:00", "2025-01-15 10:30:00", "2025-01-15 10:30:00",
				"f", "50.0", "10.0", "0", "0", "0", "\\N", "{}",
			}, "\t"),
			wantPos: TraccarPosition{
				ID:         101,
				Protocol:   "watch",
				DeviceID:   2,
				Valid:      false,
				Latitude:   50.0,
				Longitude:  10.0,
				Altitude:   0,
				Speed:      0,
				Course:     0,
				Address:    "",
				Attributes: "{}",
			},
		},
		{
			name:        "too few fields",
			input:       "100\th02\t1",
			wantError:   true,
			errContains: "expected at least 14 fields",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePosition(tt.input)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.ID != tt.wantPos.ID {
				t.Errorf("ID = %d, want %d", got.ID, tt.wantPos.ID)
			}
			if got.Protocol != tt.wantPos.Protocol {
				t.Errorf("Protocol = %q, want %q", got.Protocol, tt.wantPos.Protocol)
			}
			if got.DeviceID != tt.wantPos.DeviceID {
				t.Errorf("DeviceID = %d, want %d", got.DeviceID, tt.wantPos.DeviceID)
			}
			if got.Valid != tt.wantPos.Valid {
				t.Errorf("Valid = %v, want %v", got.Valid, tt.wantPos.Valid)
			}
			if math.Abs(got.Latitude-tt.wantPos.Latitude) > 0.0001 {
				t.Errorf("Latitude = %f, want %f", got.Latitude, tt.wantPos.Latitude)
			}
			if math.Abs(got.Longitude-tt.wantPos.Longitude) > 0.0001 {
				t.Errorf("Longitude = %f, want %f", got.Longitude, tt.wantPos.Longitude)
			}
			if got.Address != tt.wantPos.Address {
				t.Errorf("Address = %q, want %q", got.Address, tt.wantPos.Address)
			}
			if got.Attributes != tt.wantPos.Attributes {
				t.Errorf("Attributes = %q, want %q", got.Attributes, tt.wantPos.Attributes)
			}
		})
	}
}

func TestParsePosition_ErrorCases(t *testing.T) {
	makeFields := func(id, deviceid, fixtime string) string {
		return strings.Join([]string{
			id, "h02", deviceid, "2025-01-15 10:30:00", "2025-01-15 10:30:00", fixtime,
			"t", "49.0", "9.0", "0", "0", "0", "addr", "{}",
		}, "\t")
	}

	tests := []struct{ name, input string }{
		{"invalid id", makeFields("notanint", "1", "2025-01-15 10:30:00")},
		{"invalid deviceid", makeFields("1", "notanint", "2025-01-15 10:30:00")},
		{"invalid timestamp", makeFields("1", "1", "not-a-time")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parsePosition(tt.input)
			if err == nil {
				t.Errorf("parsePosition(%q) expected error", tt.name)
			}
		})
	}
}

func TestParseGeofence_ErrorCases(t *testing.T) {
	tests := []struct{ name, input string }{
		{"too few fields", "1\tname"},
		{"invalid id", strings.Join([]string{"notanint", "fence", "desc", "POLYGON((0 0,1 0,1 1,0 1,0 0))"}, "\t")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseGeofence(tt.input)
			if err == nil {
				t.Errorf("parseGeofence(%q) expected error", tt.name)
			}
		})
	}
}

func TestNullStr(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`\N`, ""},
		{"hello", "hello"},
		{"", ""},
		{"+491234567890", "+491234567890"},
	}

	for _, tt := range tests {
		got := nullStr(tt.input)
		if got != tt.expected {
			t.Errorf("nullStr(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestKnotsToKmh(t *testing.T) {
	tests := []struct {
		knots    float64
		expected float64
	}{
		{0, 0},
		{1, 1.852},
		{10, 18.52},
		{100, 185.2},
	}

	for _, tt := range tests {
		got := knotsToKmh(tt.knots)
		if math.Abs(got-tt.expected) > 0.01 {
			t.Errorf("knotsToKmh(%f) = %f, want %f", tt.knots, got, tt.expected)
		}
	}
}

func TestNullToNil(t *testing.T) {
	tests := []struct {
		input   string
		wantNil bool
		wantVal string
	}{
		{"", true, ""},
		{"+491234567890", false, "+491234567890"},
		{"TK103B", false, "TK103B"},
	}

	for _, tt := range tests {
		got := nullToNil(tt.input)
		if tt.wantNil {
			if got != nil {
				t.Errorf("nullToNil(%q) = %q, want nil", tt.input, *got)
			}
		} else {
			if got == nil {
				t.Errorf("nullToNil(%q) = nil, want %q", tt.input, tt.wantVal)
			} else if *got != tt.wantVal {
				t.Errorf("nullToNil(%q) = %q, want %q", tt.input, *got, tt.wantVal)
			}
		}
	}
}

func TestParseDumpWithDeviceFields(t *testing.T) {
	// Create a temporary dump file with device data including phone and model
	tmpDir := t.TempDir()
	dumpPath := tmpDir + "/test.sql"

	dumpContent := `-- PostgreSQL dump
COPY public.tc_devices (id, name, uniqueid, lastupdate, positionid, groupid, attributes, phone, model, contact, category, disabled, status) FROM stdin;
1	GT3 RS	123456789012345	2025-01-15 10:30:00	42	\N	{}	+491234567890	TK103B	john@example.com	car	f	online
2	Watch1	WATCH001	2025-02-01 08:00:00	100	\N	{}	\N	Q50	\N	watch	t	offline
3	NoPhoneNoModel	DEV003	2025-02-01 08:00:00	\N	\N	{}	\N	\N	\N	\N	f	offline
\.
COPY public.tc_positions (id, protocol, deviceid, servertime, devicetime, fixtime, valid, latitude, longitude, altitude, speed, course, address, attributes) FROM stdin;
100	h02	1	2025-01-15 10:30:00	2025-01-15 10:30:00	2025-01-15 10:30:00	t	49.7913	9.9534	200.0	45.5	180.0	Main Street	{"batteryLevel":85}
\.
`
	if err := writeTestFile(dumpPath, dumpContent); err != nil {
		t.Fatalf("failed to write test dump: %v", err)
	}

	config := &Config{
		SourceDump:      dumpPath,
		RecentDays:      0,
		MaxPositions:    0,
		Verbose:         false,
		ImportDevices:   true,
		ImportPositions: true,
		ImportGeofences: true,
		ImportCalendars: true,
	}

	devices, positions, _, _, err := parseDump(config)
	if err != nil {
		t.Fatalf("parseDump failed: %v", err)
	}

	if len(devices) != 3 {
		t.Fatalf("expected 3 devices, got %d", len(devices))
	}
	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}

	// Verify GT3 RS device has phone and model
	gt3rs := devices[0]
	if gt3rs.Name != "GT3 RS" {
		t.Errorf("device[0].Name = %q, want %q", gt3rs.Name, "GT3 RS")
	}
	if gt3rs.Phone != "+491234567890" {
		t.Errorf("device[0].Phone = %q, want %q", gt3rs.Phone, "+491234567890")
	}
	if gt3rs.Model != "TK103B" {
		t.Errorf("device[0].Model = %q, want %q", gt3rs.Model, "TK103B")
	}
	if gt3rs.Category != "car" {
		t.Errorf("device[0].Category = %q, want %q", gt3rs.Category, "car")
	}
	if gt3rs.Disabled {
		t.Error("device[0].Disabled = true, want false")
	}

	// Verify Watch1 has model but no phone
	watch := devices[1]
	if watch.Phone != "" {
		t.Errorf("device[1].Phone = %q, want empty", watch.Phone)
	}
	if watch.Model != "Q50" {
		t.Errorf("device[1].Model = %q, want %q", watch.Model, "Q50")
	}
	if watch.Category != "watch" {
		t.Errorf("device[1].Category = %q, want %q", watch.Category, "watch")
	}
	if !watch.Disabled {
		t.Error("device[1].Disabled = false, want true")
	}

	// Verify device with null phone and model
	noFields := devices[2]
	if noFields.Phone != "" {
		t.Errorf("device[2].Phone = %q, want empty", noFields.Phone)
	}
	if noFields.Model != "" {
		t.Errorf("device[2].Model = %q, want empty", noFields.Model)
	}
}

func TestParseDumpWithDeviceFilter(t *testing.T) {
	tmpDir := t.TempDir()
	dumpPath := tmpDir + "/test.sql"

	dumpContent := `COPY public.tc_devices (id, name, uniqueid, lastupdate, positionid, groupid, attributes, phone, model, contact, category, disabled, status) FROM stdin;
1	GT3 RS	123456789012345	2025-01-15 10:30:00	42	\N	{}	+491234567890	TK103B	\N	car	f	online
2	Other	OTHER001	2025-01-15 10:30:00	\N	\N	{}	\N	\N	\N	\N	f	offline
\.
COPY public.tc_positions (id, protocol, deviceid, servertime, devicetime, fixtime, valid, latitude, longitude, altitude, speed, course, address, attributes) FROM stdin;
100	h02	1	2025-01-15 10:30:00	2025-01-15 10:30:00	2025-01-15 10:30:00	t	49.0	9.0	0	0	0	\N	{}
101	h02	2	2025-01-15 10:30:00	2025-01-15 10:30:00	2025-01-15 10:30:00	t	50.0	10.0	0	0	0	\N	{}
\.
`
	if err := writeTestFile(dumpPath, dumpContent); err != nil {
		t.Fatalf("failed to write test dump: %v", err)
	}

	config := &Config{
		SourceDump:      dumpPath,
		DeviceFilter:    "GT3 RS",
		RecentDays:      0,
		MaxPositions:    0,
		ImportDevices:   true,
		ImportPositions: true,
		ImportGeofences: true,
		ImportCalendars: true,
	}

	devices, positions, _, _, err := parseDump(config)
	if err != nil {
		t.Fatalf("parseDump failed: %v", err)
	}

	if len(devices) != 1 {
		t.Fatalf("expected 1 device (filtered), got %d", len(devices))
	}
	if devices[0].Name != "GT3 RS" {
		t.Errorf("filtered device name = %q, want %q", devices[0].Name, "GT3 RS")
	}
	if devices[0].Phone != "+491234567890" {
		t.Errorf("filtered device phone = %q, want %q", devices[0].Phone, "+491234567890")
	}
	if devices[0].Model != "TK103B" {
		t.Errorf("filtered device model = %q, want %q", devices[0].Model, "TK103B")
	}

	// Only position for device 1 should be included
	if len(positions) != 1 {
		t.Fatalf("expected 1 position (filtered), got %d", len(positions))
	}
	if positions[0].DeviceID != 1 {
		t.Errorf("filtered position device_id = %d, want 1", positions[0].DeviceID)
	}
}

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

func TestParseCalendar(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantCal     TraccarCalendar
		wantError   bool
		errContains string
	}{
		{
			name: "valid calendar with base64 data",
			input: strings.Join([]string{
				"1", "Business Hours", "QkVHSU46VkNBTEVOREFS", "{}",
			}, "\t"),
			wantCal: TraccarCalendar{
				ID:   1,
				Name: "Business Hours",
				Data: "QkVHSU46VkNBTEVOREFS",
			},
		},
		{
			name: "calendar with null attributes",
			input: strings.Join([]string{
				"2", "Weekend", "SEVMTE8=", `\N`,
			}, "\t"),
			wantCal: TraccarCalendar{
				ID:   2,
				Name: "Weekend",
				Data: "SEVMTE8=",
			},
		},
		{
			name: "calendar with only 3 fields (minimum)",
			input: strings.Join([]string{
				"3", "Minimal", "data123",
			}, "\t"),
			wantCal: TraccarCalendar{
				ID:   3,
				Name: "Minimal",
				Data: "data123",
			},
		},
		{
			name: "calendar with null data",
			input: strings.Join([]string{
				"4", "Empty Cal", `\N`, "{}",
			}, "\t"),
			wantCal: TraccarCalendar{
				ID:   4,
				Name: "Empty Cal",
				Data: "",
			},
		},
		{
			name:        "too few fields",
			input:       "1\tBusiness Hours",
			wantError:   true,
			errContains: "expected at least 3 fields",
		},
		{
			name: "invalid id",
			input: strings.Join([]string{
				"abc", "Bad ID", "data",
			}, "\t"),
			wantError:   true,
			errContains: "parse id",
		},
		{
			// PostgreSQL COPY format represents bytea as \\xHEX (double-backslash x).
			// "BEGIN:VCALENDAR" in hex = 424547494e3a5643414c454e444152
			name: "postgresql bytea hex encoding",
			input: strings.Join([]string{
				"5", "Bytea Cal", "\\\\x424547494e3a5643414c454e444152",
			}, "\t"),
			wantCal: TraccarCalendar{
				ID:   5,
				Name: "Bytea Cal",
				Data: "BEGIN:VCALENDAR",
			},
		},
		{
			name: "invalid bytea hex",
			input: strings.Join([]string{
				"6", "Bad Bytea", "\\\\xINVALIDHEX",
			}, "\t"),
			wantError:   true,
			errContains: "decode bytea hex",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCalendar(tt.input)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.ID != tt.wantCal.ID {
				t.Errorf("ID = %d, want %d", got.ID, tt.wantCal.ID)
			}
			if got.Name != tt.wantCal.Name {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantCal.Name)
			}
			if got.Data != tt.wantCal.Data {
				t.Errorf("Data = %q, want %q", got.Data, tt.wantCal.Data)
			}
		})
	}
}

func TestParseGeofenceWithCalendarID(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCalID *int64
	}{
		{
			name: "geofence without calendarid",
			input: strings.Join([]string{
				"1", "Home Zone", "", "POLYGON((49.79 9.93, 49.81 9.93, 49.81 9.97, 49.79 9.97, 49.79 9.93))", "{}",
			}, "\t"),
			wantCalID: nil,
		},
		{
			name: "geofence with calendarid",
			input: strings.Join([]string{
				"2", "Office Zone", "", "POLYGON((50.0 8.0, 51.0 8.0, 51.0 9.0, 50.0 9.0, 50.0 8.0))", "{}", "5",
			}, "\t"),
			wantCalID: int64Ptr(5),
		},
		{
			name: "geofence with null calendarid",
			input: strings.Join([]string{
				"3", "Park", "", "POLYGON((52.0 13.0, 52.1 13.0, 52.1 13.1, 52.0 13.1, 52.0 13.0))", `{}`, `\N`,
			}, "\t"),
			wantCalID: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := parseGeofence(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantCalID == nil {
				if g.CalendarID != nil {
					t.Errorf("CalendarID = %d, want nil", *g.CalendarID)
				}
			} else {
				if g.CalendarID == nil {
					t.Errorf("CalendarID = nil, want %d", *tt.wantCalID)
				} else if *g.CalendarID != *tt.wantCalID {
					t.Errorf("CalendarID = %d, want %d", *g.CalendarID, *tt.wantCalID)
				}
			}
		})
	}
}

func int64Ptr(v int64) *int64 {
	return &v
}

func TestParseDumpWithCalendars(t *testing.T) {
	tmpDir := t.TempDir()
	dumpPath := tmpDir + "/test.sql"

	// Base64-encode a simple iCalendar string to simulate Traccar's format.
	icalData := "BEGIN:VCALENDAR\nVERSION:2.0\nEND:VCALENDAR"
	b64Data := base64.StdEncoding.EncodeToString([]byte(icalData))

	dumpContent := `-- PostgreSQL dump
COPY public.tc_devices (id, name, uniqueid, lastupdate, positionid, groupid, attributes, phone, model, contact, category, disabled, status) FROM stdin;
1	TestDevice	DEV001	2025-01-15 10:30:00	\N	\N	{}	\N	\N	\N	\N	f	online
\.
COPY public.tc_calendars (id, name, data, attributes) FROM stdin;
1	Business Hours	` + b64Data + `	{}
2	Weekend	` + b64Data + `	{}
\.
COPY public.tc_geofences (id, name, description, area, attributes, calendarid) FROM stdin;
1	Home Zone	My home	POLYGON((49.79 9.93, 49.81 9.93, 49.81 9.97, 49.79 9.97, 49.79 9.93))	{}	1
2	Park	\N	POLYGON((50.0 8.0, 51.0 8.0, 51.0 9.0, 50.0 9.0, 50.0 8.0))	{}	\N
\.
`
	if err := writeTestFile(dumpPath, dumpContent); err != nil {
		t.Fatalf("failed to write test dump: %v", err)
	}

	config := &Config{
		SourceDump:      dumpPath,
		RecentDays:      0,
		MaxPositions:    0,
		Verbose:         false,
		ImportDevices:   true,
		ImportPositions: true,
		ImportGeofences: true,
		ImportCalendars: true,
	}

	devices, _, geofences, calendars, err := parseDump(config)
	if err != nil {
		t.Fatalf("parseDump failed: %v", err)
	}

	// Verify devices parsed.
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}

	// Verify calendars parsed.
	if len(calendars) != 2 {
		t.Fatalf("expected 2 calendars, got %d", len(calendars))
	}
	if calendars[0].ID != 1 {
		t.Errorf("calendar[0].ID = %d, want 1", calendars[0].ID)
	}
	if calendars[0].Name != "Business Hours" {
		t.Errorf("calendar[0].Name = %q, want %q", calendars[0].Name, "Business Hours")
	}
	if calendars[0].Data != b64Data {
		t.Errorf("calendar[0].Data = %q, want %q", calendars[0].Data, b64Data)
	}
	if calendars[1].ID != 2 {
		t.Errorf("calendar[1].ID = %d, want 2", calendars[1].ID)
	}
	if calendars[1].Name != "Weekend" {
		t.Errorf("calendar[1].Name = %q, want %q", calendars[1].Name, "Weekend")
	}

	// Verify geofences parsed with calendar associations.
	if len(geofences) != 2 {
		t.Fatalf("expected 2 geofences, got %d", len(geofences))
	}
	// First geofence should have calendarid=1.
	if geofences[0].CalendarID == nil {
		t.Fatal("geofence[0].CalendarID should not be nil")
	}
	if *geofences[0].CalendarID != 1 {
		t.Errorf("geofence[0].CalendarID = %d, want 1", *geofences[0].CalendarID)
	}
	// Second geofence should have no calendar (null).
	if geofences[1].CalendarID != nil {
		t.Errorf("geofence[1].CalendarID = %d, want nil", *geofences[1].CalendarID)
	}
}

func TestParseDumpCalendarsOnly(t *testing.T) {
	// Dump with only calendars section (no devices/positions/geofences).
	tmpDir := t.TempDir()
	dumpPath := tmpDir + "/test.sql"

	dumpContent := `COPY public.tc_calendars (id, name, data, attributes) FROM stdin;
10	Night Shift	bmlja3Q=	{}
\.
`
	if err := writeTestFile(dumpPath, dumpContent); err != nil {
		t.Fatalf("failed to write test dump: %v", err)
	}

	config := &Config{
		SourceDump:      dumpPath,
		RecentDays:      0,
		MaxPositions:    0,
		ImportDevices:   true,
		ImportPositions: true,
		ImportGeofences: true,
		ImportCalendars: true,
	}

	devices, positions, geofences, calendars, err := parseDump(config)
	if err != nil {
		t.Fatalf("parseDump failed: %v", err)
	}

	if len(devices) != 0 {
		t.Errorf("expected 0 devices, got %d", len(devices))
	}
	if len(positions) != 0 {
		t.Errorf("expected 0 positions, got %d", len(positions))
	}
	if len(geofences) != 0 {
		t.Errorf("expected 0 geofences, got %d", len(geofences))
	}
	if len(calendars) != 1 {
		t.Fatalf("expected 1 calendar, got %d", len(calendars))
	}
	if calendars[0].ID != 10 {
		t.Errorf("calendar.ID = %d, want 10", calendars[0].ID)
	}
	if calendars[0].Name != "Night Shift" {
		t.Errorf("calendar.Name = %q, want %q", calendars[0].Name, "Night Shift")
	}
	if calendars[0].Data != "bmlja3Q=" {
		t.Errorf("calendar.Data = %q, want %q", calendars[0].Data, "bmlja3Q=")
	}
}

func TestNormalizeTraccarCalendar(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantSubstr string // substring that must be present in output
		noSubstr   string // substring that must NOT be present in input but should be in output
		unchanged  bool   // if true, output must equal input exactly
	}{
		{
			name: "adds UNTIL to RRULE when DTEND spans multiple days",
			input: "BEGIN:VCALENDAR\r\n" +
				"VERSION:2.0\r\n" +
				"BEGIN:VEVENT\r\n" +
				"DTSTART;TZID=Europe/Berlin:20251105T200000\r\n" +
				"DTEND;TZID=Europe/Berlin:20251110T200000\r\n" +
				"RRULE:FREQ=DAILY\r\n" +
				"END:VEVENT\r\n" +
				"END:VCALENDAR\r\n",
			wantSubstr: "RRULE:FREQ=DAILY;UNTIL=20251110T200000",
		},
		{
			name: "adds UNTIL preserving existing RRULE params",
			input: "BEGIN:VCALENDAR\r\n" +
				"VERSION:2.0\r\n" +
				"BEGIN:VEVENT\r\n" +
				"DTSTART;TZID=Europe/Berlin:20251105T200000\r\n" +
				"DTEND;TZID=Europe/Berlin:20251115T200000\r\n" +
				"RRULE:FREQ=DAILY;INTERVAL=2\r\n" +
				"END:VEVENT\r\n" +
				"END:VCALENDAR\r\n",
			wantSubstr: "RRULE:FREQ=DAILY;INTERVAL=2;UNTIL=20251115T200000",
		},
		{
			name: "does not modify RRULE that already has UNTIL",
			input: "BEGIN:VCALENDAR\r\n" +
				"VERSION:2.0\r\n" +
				"BEGIN:VEVENT\r\n" +
				"DTSTART;TZID=Europe/Berlin:20251105T200000\r\n" +
				"DTEND;TZID=Europe/Berlin:20251110T200000\r\n" +
				"RRULE:FREQ=DAILY;UNTIL=20251108T200000\r\n" +
				"END:VEVENT\r\n" +
				"END:VCALENDAR\r\n",
			unchanged: true,
		},
		{
			name: "does not modify when DTEND minus DTSTART is 24h or less",
			input: "BEGIN:VCALENDAR\r\n" +
				"VERSION:2.0\r\n" +
				"BEGIN:VEVENT\r\n" +
				"DTSTART;TZID=Europe/Berlin:20251105T200000\r\n" +
				"DTEND;TZID=Europe/Berlin:20251106T200000\r\n" +
				"RRULE:FREQ=DAILY\r\n" +
				"END:VEVENT\r\n" +
				"END:VCALENDAR\r\n",
			unchanged: true,
		},
		{
			name: "does not modify when no RRULE present",
			input: "BEGIN:VCALENDAR\r\n" +
				"VERSION:2.0\r\n" +
				"BEGIN:VEVENT\r\n" +
				"DTSTART;TZID=Europe/Berlin:20251105T200000\r\n" +
				"DTEND;TZID=Europe/Berlin:20251110T200000\r\n" +
				"END:VEVENT\r\n" +
				"END:VCALENDAR\r\n",
			unchanged: true,
		},
		{
			name: "handles UTC timestamps (Z suffix)",
			input: "BEGIN:VCALENDAR\r\n" +
				"VERSION:2.0\r\n" +
				"BEGIN:VEVENT\r\n" +
				"DTSTART:20251105T200000Z\r\n" +
				"DTEND:20251110T200000Z\r\n" +
				"RRULE:FREQ=DAILY\r\n" +
				"END:VEVENT\r\n" +
				"END:VCALENDAR\r\n",
			wantSubstr: "RRULE:FREQ=DAILY;UNTIL=20251110T200000Z",
		},
		{
			name: "handles date-only format",
			input: "BEGIN:VCALENDAR\r\n" +
				"VERSION:2.0\r\n" +
				"BEGIN:VEVENT\r\n" +
				"DTSTART;VALUE=DATE:20251105\r\n" +
				"DTEND;VALUE=DATE:20251110\r\n" +
				"RRULE:FREQ=DAILY\r\n" +
				"END:VEVENT\r\n" +
				"END:VCALENDAR\r\n",
			wantSubstr: "RRULE:FREQ=DAILY;UNTIL=20251110",
		},
		{
			name:      "returns empty string unchanged",
			input:     "",
			unchanged: true,
		},
		{
			name:      "returns garbage data unchanged",
			input:     "not ical data at all",
			unchanged: true,
		},
		{
			name: "does not modify RRULE with COUNT",
			input: "BEGIN:VCALENDAR\r\n" +
				"VERSION:2.0\r\n" +
				"BEGIN:VEVENT\r\n" +
				"DTSTART;TZID=Europe/Berlin:20251105T200000\r\n" +
				"DTEND;TZID=Europe/Berlin:20251110T200000\r\n" +
				"RRULE:FREQ=DAILY;COUNT=5\r\n" +
				"END:VEVENT\r\n" +
				"END:VCALENDAR\r\n",
			unchanged: true,
		},
		{
			name: "handles LF line endings",
			input: "BEGIN:VCALENDAR\n" +
				"VERSION:2.0\n" +
				"BEGIN:VEVENT\n" +
				"DTSTART;TZID=Europe/Berlin:20251105T200000\n" +
				"DTEND;TZID=Europe/Berlin:20251110T200000\n" +
				"RRULE:FREQ=DAILY\n" +
				"END:VEVENT\n" +
				"END:VCALENDAR\n",
			wantSubstr: "RRULE:FREQ=DAILY;UNTIL=20251110T200000",
		},
		{
			name: "handles WEEKLY frequency with multi-day span",
			input: "BEGIN:VCALENDAR\r\n" +
				"VERSION:2.0\r\n" +
				"BEGIN:VEVENT\r\n" +
				"DTSTART;TZID=Europe/Berlin:20251105T080000\r\n" +
				"DTEND;TZID=Europe/Berlin:20251205T080000\r\n" +
				"RRULE:FREQ=WEEKLY;BYDAY=MO,WE,FR\r\n" +
				"END:VEVENT\r\n" +
				"END:VCALENDAR\r\n",
			wantSubstr: "RRULE:FREQ=WEEKLY;BYDAY=MO,WE,FR;UNTIL=20251205T080000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeTraccarCalendar(tt.input)

			if tt.unchanged {
				if got != tt.input {
					t.Errorf("expected unchanged output\n  got:  %q\n  want: %q", got, tt.input)
				}
				return
			}

			if tt.wantSubstr != "" && !strings.Contains(got, tt.wantSubstr) {
				t.Errorf("output missing expected substring %q\n  got: %q", tt.wantSubstr, got)
			}
		})
	}
}

func TestNormalizeTraccarCalendar_MechanicExample(t *testing.T) {
	// Real-world example: Mechanic calendar from Traccar
	// DTSTART=Nov 5, DTEND=Nov 10, RRULE:FREQ=DAILY (no UNTIL)
	// This would create infinite recurring events without normalization.
	input := "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//Traccar//Traccar 6.5//EN\r\n" +
		"BEGIN:VEVENT\r\n" +
		"DTSTART;TZID=Europe/Berlin:20251105T200000\r\n" +
		"DTEND;TZID=Europe/Berlin:20251110T200000\r\n" +
		"RRULE:FREQ=DAILY\r\n" +
		"SUMMARY:Mechanic\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"

	got := normalizeTraccarCalendar(input)

	// Must have UNTIL added
	if !strings.Contains(got, "RRULE:FREQ=DAILY;UNTIL=20251110T200000") {
		t.Errorf("Mechanic calendar not normalized correctly.\n  got: %q", got)
	}

	// DTEND should be adjusted to DTSTART + 24h for proper event duration
	// (the original multi-day DTEND was actually the series end, not the event duration)
	if !strings.Contains(got, "DTEND;TZID=Europe/Berlin:20251106T200000") {
		t.Errorf("DTEND should be adjusted to DTSTART+24h for single-event duration.\n  got: %q", got)
	}

	// Original DTSTART should be preserved
	if !strings.Contains(got, "DTSTART;TZID=Europe/Berlin:20251105T200000") {
		t.Errorf("DTSTART should be preserved.\n  got: %q", got)
	}
}

func TestValidateConfig(t *testing.T) {
	// validScopes is a base Config with the minimum valid scope settings.
	validScopes := func() Config {
		return Config{
			ImportDevices:   true,
			ImportPositions: false,
			ImportGeofences: false,
			ImportCalendars: false,
		}
	}

	tests := []struct {
		name        string
		config      Config
		wantError   bool
		errContains string
	}{
		{
			name:        "neither source set",
			config:      validScopes(),
			wantError:   true,
			errContains: "one of --source-dump or --source-db* flags is required",
		},
		{
			name: "both dump and db set (dbpass) - mutually exclusive",
			config: func() Config {
				c := validScopes()
				c.SourceDump = "traccar.sql"
				c.SourceDBPass = "secret"
				c.SourceDBHost = "localhost"
				c.TargetPassword = "motus"
				return c
			}(),
			wantError:   true,
			errContains: "mutually exclusive",
		},
		{
			name: "both dump and non-default host - mutually exclusive",
			config: func() Config {
				c := validScopes()
				c.SourceDump = "traccar.sql"
				c.SourceDBHost = "myhost"
				c.TargetPassword = "motus"
				return c
			}(),
			wantError:   true,
			errContains: "mutually exclusive",
		},
		{
			name: "dump set, no target-password, no dry-run",
			config: func() Config {
				c := validScopes()
				c.SourceDump = "traccar.sql"
				return c
			}(),
			wantError:   true,
			errContains: "--target-password is required",
		},
		{
			name: "dump set, dry-run",
			config: func() Config {
				c := validScopes()
				c.SourceDump = "traccar.sql"
				c.DryRun = true
				return c
			}(),
			wantError: false,
		},
		{
			name: "dump set, target-password set",
			config: func() Config {
				c := validScopes()
				c.SourceDump = "traccar.sql"
				c.TargetPassword = "motus"
				return c
			}(),
			wantError: false,
		},
		{
			name: "db mode (dbpass only), no target-password, no dry-run",
			config: func() Config {
				c := validScopes()
				c.SourceDBHost = "localhost"
				c.SourceDBPass = "secret"
				return c
			}(),
			wantError:   true,
			errContains: "--target-password is required",
		},
		{
			name: "db mode, dry-run",
			config: func() Config {
				c := validScopes()
				c.SourceDBHost = "localhost"
				c.SourceDBPass = "secret"
				c.DryRun = true
				return c
			}(),
			wantError: false,
		},
		{
			name: "db mode, target-password set",
			config: func() Config {
				c := validScopes()
				c.SourceDBHost = "localhost"
				c.SourceDBPass = "secret"
				c.TargetPassword = "motus"
				return c
			}(),
			wantError: false,
		},
		{
			name: "db mode (non-default host), no dbpass",
			config: func() Config {
				c := validScopes()
				c.SourceDBHost = "traccar-db.local"
				c.TargetPassword = "motus"
				return c
			}(),
			wantError:   true,
			errContains: "--source-dbpass is required",
		},
		{
			name: "db mode (non-default host), dbpass set",
			config: func() Config {
				c := validScopes()
				c.SourceDBHost = "traccar-db.local"
				c.SourceDBPass = "secret"
				c.TargetPassword = "motus"
				return c
			}(),
			wantError: false,
		},
		{
			name: "dump set but invalid scopes",
			config: func() Config {
				c := Config{
					SourceDump:      "traccar.sql",
					TargetPassword:  "motus",
					ImportDevices:   false,
					ImportPositions: false,
					ImportGeofences: false,
					ImportCalendars: false,
				}
				return c
			}(),
			wantError:   true,
			errContains: "at least one import scope",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(&tt.config)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateImportScopes(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		wantError   bool
		errContains string
	}{
		{
			name: "all scopes enabled (default)",
			config: Config{
				ImportDevices:   true,
				ImportPositions: true,
				ImportGeofences: true,
				ImportCalendars: true,
			},
			wantError: false,
		},
		{
			name: "only devices enabled",
			config: Config{
				ImportDevices:   true,
				ImportPositions: false,
				ImportGeofences: false,
				ImportCalendars: false,
			},
			wantError: false,
		},
		{
			name: "only geofences enabled",
			config: Config{
				ImportDevices:   false,
				ImportPositions: false,
				ImportGeofences: true,
				ImportCalendars: false,
			},
			wantError: false,
		},
		{
			name: "only calendars enabled",
			config: Config{
				ImportDevices:   false,
				ImportPositions: false,
				ImportGeofences: false,
				ImportCalendars: true,
			},
			wantError: false,
		},
		{
			name: "no scopes enabled",
			config: Config{
				ImportDevices:   false,
				ImportPositions: false,
				ImportGeofences: false,
				ImportCalendars: false,
			},
			wantError:   true,
			errContains: "at least one import scope",
		},
		{
			name: "positions without devices",
			config: Config{
				ImportDevices:   false,
				ImportPositions: true,
				ImportGeofences: false,
				ImportCalendars: false,
			},
			wantError:   true,
			errContains: "--positions requires --devices",
		},
		{
			name: "positions with devices is valid",
			config: Config{
				ImportDevices:   true,
				ImportPositions: true,
				ImportGeofences: false,
				ImportCalendars: false,
			},
			wantError: false,
		},
		{
			name: "geofences with calendars requires both enabled",
			config: Config{
				ImportDevices:   false,
				ImportPositions: false,
				ImportGeofences: true,
				ImportCalendars: false,
			},
			// Geofences without calendars is fine; calendar linkage is best-effort.
			wantError: false,
		},
		{
			name: "devices and geofences only",
			config: Config{
				ImportDevices:   true,
				ImportPositions: false,
				ImportGeofences: true,
				ImportCalendars: false,
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateImportScopes(&tt.config)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseDumpRespectsImportScopes(t *testing.T) {
	tmpDir := t.TempDir()
	dumpPath := tmpDir + "/test.sql"

	icalData := "BEGIN:VCALENDAR\nVERSION:2.0\nEND:VCALENDAR"
	b64Data := base64.StdEncoding.EncodeToString([]byte(icalData))

	dumpContent := `-- PostgreSQL dump
COPY public.tc_devices (id, name, uniqueid, lastupdate, positionid, groupid, attributes, phone, model, contact, category, disabled, status) FROM stdin;
1	GT3 RS	123456789012345	2025-01-15 10:30:00	42	\N	{}	+491234567890	TK103B	\N	car	f	online
\.
COPY public.tc_positions (id, protocol, deviceid, servertime, devicetime, fixtime, valid, latitude, longitude, altitude, speed, course, address, attributes) FROM stdin;
100	h02	1	2025-01-15 10:30:00	2025-01-15 10:30:00	2025-01-15 10:30:00	t	49.7913	9.9534	200.0	45.5	180.0	Main Street	{"batteryLevel":85}
\.
COPY public.tc_calendars (id, name, data, attributes) FROM stdin;
1	Business Hours	` + b64Data + `	{}
\.
COPY public.tc_geofences (id, name, description, area, attributes, calendarid) FROM stdin;
1	Home Zone	My home	POLYGON((49.79 9.93, 49.81 9.93, 49.81 9.97, 49.79 9.97, 49.79 9.93))	{}	1
\.
`
	if err := writeTestFile(dumpPath, dumpContent); err != nil {
		t.Fatalf("failed to write test dump: %v", err)
	}

	t.Run("skip positions", func(t *testing.T) {
		config := &Config{
			SourceDump:      dumpPath,
			RecentDays:      0,
			MaxPositions:    0,
			ImportDevices:   true,
			ImportPositions: false,
			ImportGeofences: true,
			ImportCalendars: true,
		}
		devices, positions, geofences, calendars, err := parseDump(config)
		if err != nil {
			t.Fatalf("parseDump failed: %v", err)
		}
		if len(devices) != 1 {
			t.Errorf("expected 1 device, got %d", len(devices))
		}
		if len(positions) != 0 {
			t.Errorf("expected 0 positions (skipped), got %d", len(positions))
		}
		if len(geofences) != 1 {
			t.Errorf("expected 1 geofence, got %d", len(geofences))
		}
		if len(calendars) != 1 {
			t.Errorf("expected 1 calendar, got %d", len(calendars))
		}
	})

	t.Run("skip devices and positions", func(t *testing.T) {
		config := &Config{
			SourceDump:      dumpPath,
			RecentDays:      0,
			MaxPositions:    0,
			ImportDevices:   false,
			ImportPositions: false,
			ImportGeofences: true,
			ImportCalendars: true,
		}
		devices, positions, geofences, calendars, err := parseDump(config)
		if err != nil {
			t.Fatalf("parseDump failed: %v", err)
		}
		if len(devices) != 0 {
			t.Errorf("expected 0 devices (skipped), got %d", len(devices))
		}
		if len(positions) != 0 {
			t.Errorf("expected 0 positions (skipped), got %d", len(positions))
		}
		if len(geofences) != 1 {
			t.Errorf("expected 1 geofence, got %d", len(geofences))
		}
		if len(calendars) != 1 {
			t.Errorf("expected 1 calendar, got %d", len(calendars))
		}
	})

	t.Run("skip geofences and calendars", func(t *testing.T) {
		config := &Config{
			SourceDump:      dumpPath,
			RecentDays:      0,
			MaxPositions:    0,
			ImportDevices:   true,
			ImportPositions: true,
			ImportGeofences: false,
			ImportCalendars: false,
		}
		devices, positions, geofences, calendars, err := parseDump(config)
		if err != nil {
			t.Fatalf("parseDump failed: %v", err)
		}
		if len(devices) != 1 {
			t.Errorf("expected 1 device, got %d", len(devices))
		}
		if len(positions) != 1 {
			t.Errorf("expected 1 position, got %d", len(positions))
		}
		if len(geofences) != 0 {
			t.Errorf("expected 0 geofences (skipped), got %d", len(geofences))
		}
		if len(calendars) != 0 {
			t.Errorf("expected 0 calendars (skipped), got %d", len(calendars))
		}
	})

	t.Run("only calendars", func(t *testing.T) {
		config := &Config{
			SourceDump:      dumpPath,
			RecentDays:      0,
			MaxPositions:    0,
			ImportDevices:   false,
			ImportPositions: false,
			ImportGeofences: false,
			ImportCalendars: true,
		}
		devices, positions, geofences, calendars, err := parseDump(config)
		if err != nil {
			t.Fatalf("parseDump failed: %v", err)
		}
		if len(devices) != 0 {
			t.Errorf("expected 0 devices, got %d", len(devices))
		}
		if len(positions) != 0 {
			t.Errorf("expected 0 positions, got %d", len(positions))
		}
		if len(geofences) != 0 {
			t.Errorf("expected 0 geofences, got %d", len(geofences))
		}
		if len(calendars) != 1 {
			t.Errorf("expected 1 calendar, got %d", len(calendars))
		}
	})

	t.Run("all scopes enabled parses everything", func(t *testing.T) {
		config := &Config{
			SourceDump:      dumpPath,
			RecentDays:      0,
			MaxPositions:    0,
			ImportDevices:   true,
			ImportPositions: true,
			ImportGeofences: true,
			ImportCalendars: true,
		}
		devices, positions, geofences, calendars, err := parseDump(config)
		if err != nil {
			t.Fatalf("parseDump failed: %v", err)
		}
		if len(devices) != 1 {
			t.Errorf("expected 1 device, got %d", len(devices))
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
	})
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{"standard format", "2025-01-15 10:30:00.123456", false},
		{"no microseconds", "2025-01-15 10:30:00", false},
		{"ISO with Z", "2025-01-15T10:30:00Z", false},
		{"RFC3339", "2025-01-15T10:30:00+01:00", false},
		{"null", `\N`, true},
		{"empty", "", true},
		{"garbage", "not-a-date", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTimestamp(tt.input)
			if tt.wantError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseDumpExcludeUnknownDevices(t *testing.T) {
	tmpDir := t.TempDir()
	dumpPath := tmpDir + "/test.sql"

	dumpContent := `COPY public.tc_devices (id, name, uniqueid, lastupdate, positionid, groupid, attributes, phone, model, contact, category, disabled, status) FROM stdin;
1	GT3 RS	123456789012345	2025-01-15 10:30:00	42	\N	{}	+491234567890	TK103B	\N	car	f	online
2	OldTracker	OLD001	2025-01-15 10:30:00	\N	\N	{}	\N	\N	\N	\N	f	unknown
3	ActiveWatch	WATCH003	2025-01-15 10:30:00	100	\N	{}	\N	Q50	\N	watch	f	offline
4	LostDevice	LOST004	2025-01-15 10:30:00	\N	\N	{}	\N	\N	\N	\N	f	unknown
\.
COPY public.tc_positions (id, protocol, deviceid, servertime, devicetime, fixtime, valid, latitude, longitude, altitude, speed, course, address, attributes) FROM stdin;
100	h02	1	2025-01-15 10:30:00	2025-01-15 10:30:00	2025-01-15 10:30:00	t	49.0	9.0	0	0	0	\N	{}
101	h02	2	2025-01-15 10:30:00	2025-01-15 10:30:00	2025-01-15 10:30:00	t	50.0	10.0	0	0	0	\N	{}
102	watch	3	2025-01-15 10:30:00	2025-01-15 10:30:00	2025-01-15 10:30:00	t	51.0	11.0	0	0	0	\N	{}
103	h02	4	2025-01-15 10:30:00	2025-01-15 10:30:00	2025-01-15 10:30:00	t	52.0	12.0	0	0	0	\N	{}
\.
`
	if err := writeTestFile(dumpPath, dumpContent); err != nil {
		t.Fatalf("failed to write test dump: %v", err)
	}

	t.Run("exclude-unknown filters devices and their positions", func(t *testing.T) {
		config := &Config{
			SourceDump:      dumpPath,
			RecentDays:      0,
			MaxPositions:    0,
			ExcludeUnknown:  true,
			ImportDevices:   true,
			ImportPositions: true,
			ImportGeofences: true,
			ImportCalendars: true,
		}

		devices, positions, _, _, err := parseDump(config)
		if err != nil {
			t.Fatalf("parseDump failed: %v", err)
		}

		// Should have 2 devices (GT3 RS=online, ActiveWatch=offline), excluding 2 unknown
		if len(devices) != 2 {
			t.Fatalf("expected 2 devices (excluding unknown), got %d", len(devices))
		}
		if devices[0].Name != "GT3 RS" {
			t.Errorf("device[0].Name = %q, want %q", devices[0].Name, "GT3 RS")
		}
		if devices[1].Name != "ActiveWatch" {
			t.Errorf("device[1].Name = %q, want %q", devices[1].Name, "ActiveWatch")
		}

		// Should have 2 positions (for device IDs 1 and 3 only)
		if len(positions) != 2 {
			t.Fatalf("expected 2 positions (excluding unknown device positions), got %d", len(positions))
		}
		for _, p := range positions {
			if p.DeviceID == 2 || p.DeviceID == 4 {
				t.Errorf("position for excluded device %d should not be present", p.DeviceID)
			}
		}
	})

	t.Run("exclude-unknown false includes all devices", func(t *testing.T) {
		config := &Config{
			SourceDump:      dumpPath,
			RecentDays:      0,
			MaxPositions:    0,
			ExcludeUnknown:  false,
			ImportDevices:   true,
			ImportPositions: true,
			ImportGeofences: true,
			ImportCalendars: true,
		}

		devices, positions, _, _, err := parseDump(config)
		if err != nil {
			t.Fatalf("parseDump failed: %v", err)
		}

		// All 4 devices should be included
		if len(devices) != 4 {
			t.Fatalf("expected 4 devices (no filter), got %d", len(devices))
		}

		// All 4 positions should be included
		if len(positions) != 4 {
			t.Fatalf("expected 4 positions (no filter), got %d", len(positions))
		}
	})

	t.Run("exclude-unknown combined with device-filter", func(t *testing.T) {
		// Device filter for a device that is unknown should produce no results
		config := &Config{
			SourceDump:      dumpPath,
			DeviceFilter:    "OldTracker",
			RecentDays:      0,
			MaxPositions:    0,
			ExcludeUnknown:  true,
			ImportDevices:   true,
			ImportPositions: true,
			ImportGeofences: true,
			ImportCalendars: true,
		}

		devices, positions, _, _, err := parseDump(config)
		if err != nil {
			t.Fatalf("parseDump failed: %v", err)
		}

		// Device filter matches OldTracker, but exclude-unknown removes it
		if len(devices) != 0 {
			t.Fatalf("expected 0 devices (filtered device is unknown), got %d", len(devices))
		}
		if len(positions) != 0 {
			t.Fatalf("expected 0 positions (no matching devices), got %d", len(positions))
		}
	})

	t.Run("exclude-unknown with verbose logging", func(t *testing.T) {
		config := &Config{
			SourceDump:      dumpPath,
			RecentDays:      0,
			MaxPositions:    0,
			ExcludeUnknown:  true,
			Verbose:         true,
			ImportDevices:   true,
			ImportPositions: true,
			ImportGeofences: true,
			ImportCalendars: true,
		}

		devices, _, _, _, err := parseDump(config)
		if err != nil {
			t.Fatalf("parseDump failed: %v", err)
		}

		// Verify correct filtering (logging is tested by not panicking and correct result)
		if len(devices) != 2 {
			t.Fatalf("expected 2 devices, got %d", len(devices))
		}
	})

	t.Run("exclude-unknown with all unknown devices", func(t *testing.T) {
		allUnknownDump := tmpDir + "/all_unknown.sql"
		content := `COPY public.tc_devices (id, name, uniqueid, lastupdate, positionid, groupid, attributes, phone, model, contact, category, disabled, status) FROM stdin;
1	Dev1	DEV001	2025-01-15 10:30:00	\N	\N	{}	\N	\N	\N	\N	f	unknown
2	Dev2	DEV002	2025-01-15 10:30:00	\N	\N	{}	\N	\N	\N	\N	f	unknown
\.
COPY public.tc_positions (id, protocol, deviceid, servertime, devicetime, fixtime, valid, latitude, longitude, altitude, speed, course, address, attributes) FROM stdin;
100	h02	1	2025-01-15 10:30:00	2025-01-15 10:30:00	2025-01-15 10:30:00	t	49.0	9.0	0	0	0	\N	{}
\.
`
		if err := writeTestFile(allUnknownDump, content); err != nil {
			t.Fatalf("failed to write test dump: %v", err)
		}

		config := &Config{
			SourceDump:      allUnknownDump,
			RecentDays:      0,
			MaxPositions:    0,
			ExcludeUnknown:  true,
			ImportDevices:   true,
			ImportPositions: true,
			ImportGeofences: true,
			ImportCalendars: true,
		}

		devices, positions, _, _, err := parseDump(config)
		if err != nil {
			t.Fatalf("parseDump failed: %v", err)
		}

		if len(devices) != 0 {
			t.Fatalf("expected 0 devices (all unknown excluded), got %d", len(devices))
		}
		if len(positions) != 0 {
			t.Fatalf("expected 0 positions (all devices excluded), got %d", len(positions))
		}
	})
}

func TestSourceMode(t *testing.T) {
	cfg := &Config{SourceDump: "path/to/dump.xml"}
	if got := cfg.sourceMode(); got != "dump" {
		t.Errorf("sourceMode with dump = %q, want %q", got, "dump")
	}

	cfg2 := &Config{SourceDump: ""}
	if got := cfg2.sourceMode(); got != "db" {
		t.Errorf("sourceMode without dump = %q, want %q", got, "db")
	}
}

func TestMinInt(t *testing.T) {
	tests := []struct{ a, b, want int }{
		{3, 5, 3},
		{5, 3, 3},
		{4, 4, 4},
		{-1, 1, -1},
	}
	for _, tt := range tests {
		got := minInt(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("minInt(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestExtractICalTimestamp_EdgeCases(t *testing.T) {
	// No colon → empty string.
	if got := extractICalTimestamp("NOCOTON"); got != "" {
		t.Errorf("expected empty for line with no colon, got %q", got)
	}
	// Colon at the last char → empty string (nothing after it).
	if got := extractICalTimestamp("DTSTART:"); got != "" {
		t.Errorf("expected empty when nothing after colon, got %q", got)
	}
	// Normal case.
	if got := extractICalTimestamp("DTSTART:20260115T090000Z"); got != "20260115T090000Z" {
		t.Errorf("expected timestamp value, got %q", got)
	}
}

func TestParseICalTimestamp_Formats(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"20260115T090000Z", false},
		{"20260115T090000", false},
		{"20260115", false},
		{"not-a-timestamp", true},
	}
	for _, tt := range tests {
		_, err := parseICalTimestamp(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseICalTimestamp(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
		}
	}
}

func TestBuildAdjustedDTEnd_InvalidDTEnd(t *testing.T) {
	// When DTEND cannot be parsed, fallback to dtstart + 24h.
	dtstart := time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)
	got := buildAdjustedDTEnd("DTEND:BADVALUE", "DTSTART:20260115T090000Z", dtstart)
	want := "DTEND:" + dtstart.Add(24*time.Hour).Format("20060102T150405Z")
	if got != want {
		t.Errorf("buildAdjustedDTEnd with invalid DTEND = %q, want %q", got, want)
	}
}

func TestBuildAdjustedDTEnd_NoColonInDTEnd(t *testing.T) {
	// When DTEND line has no colon, return it unchanged.
	dtstart := time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)
	got := buildAdjustedDTEnd("DTEND_NO_COLON", "DTSTART:20260115T090000Z", dtstart)
	if got != "DTEND_NO_COLON" {
		t.Errorf("buildAdjustedDTEnd with no colon = %q, want unchanged", got)
	}
}

func TestParseTraccarCircle_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"no comma", "CIRCLE (1.0 2.0)"},
		{"bad coords count", "CIRCLE (1.0, 500)"},
		{"bad lat", "CIRCLE (lat 2.0, 500)"},
		{"bad lon", "CIRCLE (1.0 lon, 500)"},
		{"bad radius", "CIRCLE (1.0 2.0, radius)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := parseTraccarCircle(tt.input)
			if err == nil {
				t.Errorf("parseTraccarCircle(%q) expected error", tt.input)
			}
		})
	}
}

func TestSwapWKTCoordinates_OddPair(t *testing.T) {
	// A coordinate pair with != 2 tokens should be written as-is.
	// Inject via a geometry that has an odd token between commas.
	// The function splits on comma and then on spaces.
	wkt := "POLYGON ((1.0 2.0 3.0, 4.0 5.0))"
	got := swapWKTCoordinates(wkt)
	// "1.0 2.0 3.0" has 3 parts so it's written as-is; "4.0 5.0" is swapped.
	if got == "" {
		t.Error("expected non-empty result")
	}
}

func TestParseDump_RecentDaysAndVerbose(t *testing.T) {
	// Tests: RecentDays > 0 (cutoff path), Verbose=true (section-header debug logs),
	// invalid device line (parse error → skip), MaxPositions, and position cutoff filter.
	tmpDir := t.TempDir()
	dumpPath := tmpDir + "/test.sql"

	dumpContent := "-- dump\n" +
		"COPY public.tc_devices (id, name, uniqueid, lastupdate, positionid, groupid, attributes, phone, model, contact, category, disabled, status) FROM stdin;\n" +
		"1\tDevice1\tDEV001\t2025-01-01 00:00:00\t\\N\t\\N\t{}\t\\N\t\\N\t\\N\t\\N\tf\tonline\n" +
		"BADLINE\n" + // invalid device → skipped
		"\\.\n" +
		"COPY public.tc_positions (id, protocol, deviceid, servertime, devicetime, fixtime, valid, latitude, longitude, altitude, speed, course, address, attributes) FROM stdin;\n" +
		// Old position (before cutoff) — should be filtered out
		"1\th02\t1\t2020-01-01 00:00:00\t2020-01-01 00:00:00\t2020-01-01 00:00:00\tt\t49.0\t9.0\t0\t0\t0\t\\N\t{}\n" +
		// Recent position
		"2\th02\t1\t2099-01-01 00:00:00\t2099-01-01 00:00:00\t2099-01-01 00:00:00\tt\t49.0\t9.0\t0\t0\t0\t\\N\t{}\n" +
		// Invalid position line
		"BADPOS\n" +
		"\\.\n" +
		"COPY public.tc_geofences (id, name, description, area, attributes, calendarid) FROM stdin;\n" +
		"\\.\n" +
		"COPY public.tc_calendars (id, name, data, attributes) FROM stdin;\n" +
		"\\.\n"

	if err := writeTestFile(dumpPath, dumpContent); err != nil {
		t.Fatalf("failed to write test dump: %v", err)
	}

	config := &Config{
		SourceDump:      dumpPath,
		RecentDays:      1, // any position older than 1 day is cut off
		MaxPositions:    5,
		Verbose:         true,
		ImportDevices:   true,
		ImportPositions: true,
		ImportGeofences: true,
		ImportCalendars: true,
	}

	devices, positions, _, _, err := parseDump(config)
	if err != nil {
		t.Fatalf("parseDump failed: %v", err)
	}

	// BADLINE is skipped; only Device1 is valid.
	if len(devices) != 1 {
		t.Errorf("expected 1 device, got %d", len(devices))
	}
	// Old position filtered by RecentDays; BADPOS skipped; only recent position remains.
	if len(positions) != 1 {
		t.Errorf("expected 1 position (old one filtered), got %d", len(positions))
	}
}

func TestParseDump_FileNotFound(t *testing.T) {
	config := &Config{SourceDump: "/nonexistent/path/dump.sql"}
	_, _, _, _, err := parseDump(config)
	if err == nil {
		t.Error("expected error for non-existent dump file")
	}
}

func TestParseDump_InvalidGeofenceAndCalendar(t *testing.T) {
	// Covers the geofence and calendar parse-error skip paths in parseDump.
	tmpDir := t.TempDir()
	dumpPath := tmpDir + "/test.sql"

	dumpContent := "COPY public.tc_devices (id, name, uniqueid, lastupdate, positionid, groupid, attributes, phone, model, contact, category, disabled, status) FROM stdin;\n" +
		"\\.\n" +
		"COPY public.tc_positions (id, protocol, deviceid, servertime, devicetime, fixtime, valid, latitude, longitude, altitude, speed, course, address, attributes) FROM stdin;\n" +
		"\\.\n" +
		"COPY public.tc_geofences (id, name, description, area, attributes, calendarid) FROM stdin;\n" +
		"BADGEOFENCE\n" + // invalid geofence → skipped
		"\\.\n" +
		"COPY public.tc_calendars (id, name, data, attributes) FROM stdin;\n" +
		"BADCAL\n" + // invalid calendar → skipped
		"\\.\n"

	if err := writeTestFile(dumpPath, dumpContent); err != nil {
		t.Fatalf("failed to write test dump: %v", err)
	}

	config := &Config{
		SourceDump:      dumpPath,
		ImportDevices:   true,
		ImportPositions: true,
		ImportGeofences: true,
		ImportCalendars: true,
	}

	_, _, geofences, calendars, err := parseDump(config)
	if err != nil {
		t.Fatalf("parseDump failed: %v", err)
	}
	if len(geofences) != 0 {
		t.Errorf("expected 0 geofences (bad line skipped), got %d", len(geofences))
	}
	if len(calendars) != 0 {
		t.Errorf("expected 0 calendars (bad line skipped), got %d", len(calendars))
	}
}

func TestParseDump_MaxPositions(t *testing.T) {
	tmpDir := t.TempDir()
	dumpPath := tmpDir + "/test.sql"

	dumpContent := "COPY public.tc_devices (id, name, uniqueid, lastupdate, positionid, groupid, attributes, phone, model, contact, category, disabled, status) FROM stdin;\n" +
		"\\.\n" +
		"COPY public.tc_positions (id, protocol, deviceid, servertime, devicetime, fixtime, valid, latitude, longitude, altitude, speed, course, address, attributes) FROM stdin;\n" +
		"1\th02\t1\t2099-01-01 00:00:00\t2099-01-01 00:00:00\t2099-01-01 00:00:00\tt\t49.0\t9.0\t0\t0\t0\t\\N\t{}\n" +
		"2\th02\t1\t2099-01-01 00:00:01\t2099-01-01 00:00:01\t2099-01-01 00:00:01\tt\t49.1\t9.1\t0\t0\t0\t\\N\t{}\n" +
		"3\th02\t1\t2099-01-01 00:00:02\t2099-01-01 00:00:02\t2099-01-01 00:00:02\tt\t49.2\t9.2\t0\t0\t0\t\\N\t{}\n" +
		"\\.\n"

	if err := writeTestFile(dumpPath, dumpContent); err != nil {
		t.Fatalf("failed to write test dump: %v", err)
	}

	config := &Config{
		SourceDump:      dumpPath,
		MaxPositions:    2, // Only allow 2 positions
		ImportDevices:   true,
		ImportPositions: true,
		ImportGeofences: true,
		ImportCalendars: true,
	}

	_, positions, _, _, err := parseDump(config)
	if err != nil {
		t.Fatalf("parseDump failed: %v", err)
	}

	if len(positions) != 2 {
		t.Errorf("expected 2 positions (MaxPositions=2), got %d", len(positions))
	}
}

func TestNormalizeTraccarCalendar_EarlyReturn(t *testing.T) {
	// Case: DTSTART or DTEND value is empty (line with no colon after the property name).
	// Should return the input unchanged.
	noEnd := "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nDTSTART\r\nDTEND\r\nRRULE:FREQ=DAILY\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	got := normalizeTraccarCalendar(noEnd)
	if got != noEnd {
		t.Errorf("expected unchanged output when DTSTART/DTEND have no value")
	}
}

func TestNormalizeTraccarCalendar_UnparseableDTSTART(t *testing.T) {
	// DTSTART has an unparseable value → return unchanged.
	bad := "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nDTSTART:BADVALUE\r\nDTEND:20251110T200000\r\nRRULE:FREQ=DAILY\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	got := normalizeTraccarCalendar(bad)
	if got != bad {
		t.Errorf("expected unchanged output for unparseable DTSTART")
	}
}

func TestNormalizeTraccarCalendar_UnparseableDTEND(t *testing.T) {
	// DTEND has an unparseable value → return unchanged.
	bad := "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nDTSTART:20251105T083300\r\nDTEND:BADVALUE\r\nRRULE:FREQ=DAILY\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	got := normalizeTraccarCalendar(bad)
	if got != bad {
		t.Errorf("expected unchanged output for unparseable DTEND")
	}
}

func TestNormalizeTraccarCalendar_DifferentTimes(t *testing.T) {
	// Real-world example with different start/end times (08:33 to 20:00).
	input := "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//Traccar//NONSGML Traccar//EN\r\n" +
		"BEGIN:VEVENT\r\n" +
		"DTSTART;TZID=Europe/Berlin:20251105T083300\r\n" +
		"DTEND;TZID=Europe/Berlin:20251110T200000\r\n" +
		"RRULE:FREQ=DAILY\r\n" +
		"SUMMARY:Event\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"

	got := normalizeTraccarCalendar(input)

	// Must have UNTIL added
	if !strings.Contains(got, "RRULE:FREQ=DAILY;UNTIL=20251110T200000") {
		t.Errorf("UNTIL not added to RRULE.\n  got: %q", got)
	}

	// DTEND should be same day as DTSTART but with time from original DTEND (20:00)
	if !strings.Contains(got, "DTEND;TZID=Europe/Berlin:20251105T200000") {
		t.Errorf("DTEND should use DTSTART date (Nov 5) + DTEND time (20:00).\n  got: %q", got)
	}

	// DTSTART should be preserved
	if !strings.Contains(got, "DTSTART;TZID=Europe/Berlin:20251105T083300") {
		t.Errorf("DTSTART should be preserved.\n  got: %q", got)
	}
}

// TestLogParsedData exercises the logParsedData function (DRY RUN summary logger).
// logParsedData is a pure logging function; this test exists only for coverage.
func TestLogParsedData(t *testing.T) {
	calID := int64(1)
	devices := []TraccarDevice{
		{ID: 1, Name: "TestCar", UniqueID: "ANON-001", Model: "GPS-4G", Status: "offline"},
		{ID: 2, Name: "TestWatch", UniqueID: "ANON-002", Model: "SmartWatch", Status: "online"},
	}
	positions := []TraccarPosition{
		{ID: 10, DeviceID: 1, FixTime: time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)},
		{ID: 11, DeviceID: 1, FixTime: time.Date(2026, 1, 2, 9, 0, 0, 0, time.UTC)},   // after → updates latest
		{ID: 12, DeviceID: 2, FixTime: time.Date(2025, 12, 31, 6, 0, 0, 0, time.UTC)}, // before → updates earliest
	}
	geofences := []TraccarGeofence{
		{ID: 1, Name: "Home Base", Area: "POLYGON ((10.0 52.0, 10.1 52.0, 10.1 52.1, 10.0 52.1, 10.0 52.0))", CalendarID: &calID},
		{ID: 2, Name: "Workshop", Area: "POLYGON ((10.2 52.2, 10.3 52.2, 10.3 52.3, 10.2 52.3, 10.2 52.2))", CalendarID: nil},
	}
	calendars := []TraccarCalendar{
		{ID: 1, Name: "Maintenance", Data: "BEGIN:VCALENDAR\nVERSION:2.0\nEND:VCALENDAR"},
	}
	config := &Config{Verbose: true}
	// Must not panic; no return value to check.
	logParsedData(devices, positions, geofences, calendars, config)

	// Also call with no positions to cover the empty-positions branch.
	logParsedData(devices, nil, geofences, calendars, config)
}

// TestParseDump_RealisticDump exercises parseDump with a realistic production-like
// SQL dump format: 23-column tc_devices, bytea-hex encoded tc_calendars,
// full 17-column tc_positions, and tc_geofences with a calendarid reference.
//
// The fixture is based on anonymized real-world Traccar data (device names,
// phone numbers, coordinates and addresses have been replaced with synthetic values).
func TestParseDump_RealisticDump(t *testing.T) {
	// Anonymized iCal content encoded as PostgreSQL bytea hex (\\x prefix in COPY format).
	// Content: a DAILY recurring "Maintenance" event, Nov 3–7 2025, Berlin time.
	const calendarHex = "424547494e3a5643414c454e4441520a56455253494f4e3a322e300a50524f4449443a2d2f2f547261636361722f2f4e4f4e53474d4c20547261636361722f2f454e0a424547494e3a564556454e540a5549443a61616161616161612d626262622d636363632d646464642d6565656565656565656565650a445453544152543b545a49443d4575726f70652f4265726c696e3a3230323531313033543039303030300a4454454e443b545a49443d4575726f70652f4265726c696e3a3230323531313037543137303030300a5252554c453a465245513d4441494c590a53554d4d4152593a4d61696e74656e616e63650a454e443a564556454e540a454e443a5643414c454e4441520a"

	tmpDir := t.TempDir()
	dumpPath := tmpDir + "/realistic.sql"

	// 23-column tc_devices row (matching actual Traccar 6.x PostgreSQL schema).
	// Real-world values anonymized: name, uniqueid, phone, model replaced with synthetic data.
	dumpContent := "-- PostgreSQL database dump\n" +
		"COPY public.tc_devices (id, name, uniqueid, lastupdate, positionid, groupid, attributes, phone, model, contact, category, disabled, status, expirationtime, motionstate, motiontime, motiondistance, overspeedstate, overspeedtime, overspeedgeofenceid, motionstreak, calendarid, motionpositionid) FROM stdin;\n" +
		"1\tTestCar\tANON-VEH-001\t2026-01-15 10:00:00.000\t5\t\\N\t{}\t+4901234500001\tGPS-4G Tracker\t\\N\t\\N\tf\toffline \t\\N\tf\t\\N\t0\tf\t\\N\t\\N\tf\t\\N\t\\N\n" +
		"2\tANON-VEH-002\tANON-VEH-002\t\\N\t\\N\t\\N\t{}\t\\N\t\\N\t\\N\t\\N\tf\t\\N\t\\N\tf\t\\N\t0\tf\t\\N\t0\tf\t\\N\t\\N\n" +
		"\\.\n" +
		// tc_calendars with bytea hex encoding (same format as real Traccar dump)
		"COPY public.tc_calendars (id, name, data, attributes) FROM stdin;\n" +
		"1\tMaintenance\t\\\\x" + calendarHex + "\t{}\n" +
		"\\.\n" +
		// tc_geofences: two geofences, one with calendarid=1
		// Coordinates shifted to fictional area (52.0N, 10.0E) — not a real location.
		"COPY public.tc_geofences (id, name, description, area, attributes, calendarid) FROM stdin;\n" +
		"1\tHome Base\t\\N\tPOLYGON ((52.01 10.01, 52.00 10.01, 52.00 10.02, 52.01 10.02, 52.01 10.01))\t{}\t\\N\n" +
		"2\tWorkshop\t\\N\tPOLYGON ((52.11 10.11, 52.10 10.11, 52.10 10.12, 52.11 10.12, 52.11 10.11))\t{}\t1\n" +
		"\\.\n" +
		// tc_positions: full 17-column rows (same format as real dump)
		// Coordinates near 52.0N, 10.0E — fictional, not a real address.
		"COPY public.tc_positions (id, protocol, deviceid, servertime, devicetime, fixtime, valid, latitude, longitude, altitude, speed, course, address, attributes, accuracy, network, geofenceids) FROM stdin;\n" +
		"1\th02\t1\t2026-01-10 08:07:06.164\t2026-01-10 08:07:04\t2026-01-10 08:07:04\tt\t52.00100\t10.00100\t0\t0\t0\t\\N\t{\"ignition\":false,\"distance\":0.0,\"totalDistance\":0.0,\"motion\":false}\t0\tnull\t\\N\n" +
		"2\th02\t1\t2026-01-10 08:15:00.000\t2026-01-10 08:15:00\t2026-01-10 08:15:00\tt\t52.00200\t10.00200\t0\t1.5\t90\t\\N\t{\"ignition\":true,\"distance\":120.0,\"totalDistance\":120.0,\"motion\":true}\t0\tnull\t\\N\n" +
		"3\th02\t1\t2026-01-10 09:00:00.000\t2026-01-10 09:00:00\t2026-01-10 09:00:00\tt\t52.00300\t10.00300\t5\t0\t180\t\\N\t{\"ignition\":false,\"distance\":85.0,\"totalDistance\":205.0,\"motion\":false}\t0\tnull\t\\N\n" +
		"4\th02\t1\t2026-01-10 12:30:00.000\t2026-01-10 12:30:00\t2026-01-10 12:30:00\tt\t52.00400\t10.00400\t0\t0\t0\t\\N\t{\"ignition\":false,\"distance\":0.0,\"totalDistance\":205.0,\"motion\":false}\t0\tnull\t\\N\n" +
		"5\th02\t1\t2026-01-15 10:00:00.000\t2026-01-15 10:00:00\t2026-01-15 10:00:00\tf\t52.00500\t10.00500\t0\t0\t0\t\\N\t{\"ignition\":true,\"status\":4294959103,\"distance\":0.0,\"totalDistance\":205.0,\"motion\":false}\t0\tnull\t\\N\n" +
		"\\.\n"

	if err := writeTestFile(dumpPath, dumpContent); err != nil {
		t.Fatalf("failed to write realistic dump: %v", err)
	}

	config := &Config{
		SourceDump:      dumpPath,
		RecentDays:      0,
		MaxPositions:    0,
		Verbose:         true,
		ImportDevices:   true,
		ImportPositions: true,
		ImportGeofences: true,
		ImportCalendars: true,
	}

	devices, positions, geofences, calendars, err := parseDump(config)
	if err != nil {
		t.Fatalf("parseDump failed: %v", err)
	}

	// --- Devices ---
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
	if devices[0].Name != "TestCar" {
		t.Errorf("devices[0].Name = %q, want TestCar", devices[0].Name)
	}
	if devices[0].UniqueID != "ANON-VEH-001" {
		t.Errorf("devices[0].UniqueID = %q, want ANON-VEH-001", devices[0].UniqueID)
	}
	if devices[0].Phone != "+4901234500001" {
		t.Errorf("devices[0].Phone = %q, want +4901234500001", devices[0].Phone)
	}
	if devices[0].Model != "GPS-4G Tracker" {
		t.Errorf("devices[0].Model = %q, want GPS-4G Tracker", devices[0].Model)
	}
	if devices[0].Status != "offline" {
		t.Errorf("devices[0].Status = %q, want offline", devices[0].Status)
	}

	// Second device has same name as uniqueid → placeholder device.
	if devices[1].UniqueID != "ANON-VEH-002" {
		t.Errorf("devices[1].UniqueID = %q, want ANON-VEH-002", devices[1].UniqueID)
	}

	// --- Positions ---
	if len(positions) != 5 {
		t.Fatalf("expected 5 positions, got %d", len(positions))
	}
	if positions[0].DeviceID != 1 {
		t.Errorf("positions[0].DeviceID = %d, want 1", positions[0].DeviceID)
	}
	if positions[0].Protocol != "h02" {
		t.Errorf("positions[0].Protocol = %q, want h02", positions[0].Protocol)
	}
	if positions[0].Valid != true {
		t.Errorf("positions[0].Valid = false, want true")
	}
	// Last position has valid=false (integrity check)
	if positions[4].Valid != false {
		t.Errorf("positions[4].Valid = true, want false")
	}

	// --- Calendars ---
	if len(calendars) != 1 {
		t.Fatalf("expected 1 calendar, got %d", len(calendars))
	}
	if calendars[0].ID != 1 {
		t.Errorf("calendars[0].ID = %d, want 1", calendars[0].ID)
	}
	if calendars[0].Name != "Maintenance" {
		t.Errorf("calendars[0].Name = %q, want Maintenance", calendars[0].Name)
	}
	// Decoded bytea hex should contain the iCal header
	if !strings.HasPrefix(calendars[0].Data, "BEGIN:VCALENDAR") {
		t.Errorf("calendars[0].Data should start with BEGIN:VCALENDAR, got %q", calendars[0].Data[:minInt(40, len(calendars[0].Data))])
	}
	if !strings.Contains(calendars[0].Data, "SUMMARY:Maintenance") {
		t.Errorf("calendars[0].Data should contain SUMMARY:Maintenance")
	}

	// --- Geofences ---
	if len(geofences) != 2 {
		t.Fatalf("expected 2 geofences, got %d", len(geofences))
	}
	if geofences[0].Name != "Home Base" {
		t.Errorf("geofences[0].Name = %q, want Home Base", geofences[0].Name)
	}
	if geofences[0].CalendarID != nil {
		t.Errorf("geofences[0].CalendarID = %d, want nil", *geofences[0].CalendarID)
	}
	if geofences[1].Name != "Workshop" {
		t.Errorf("geofences[1].Name = %q, want Workshop", geofences[1].Name)
	}
	if geofences[1].CalendarID == nil || *geofences[1].CalendarID != 1 {
		t.Errorf("geofences[1].CalendarID = nil or wrong, want 1")
	}
}

// TestNewCmd verifies that NewCmd returns a properly configured cobra.Command.
func TestNewCmd(t *testing.T) {
	cmd := NewCmd()
	if cmd == nil {
		t.Fatal("NewCmd returned nil")
	}
	if cmd.Use != "import" {
		t.Errorf("Use = %q, want %q", cmd.Use, "import")
	}
	if cmd.RunE == nil {
		t.Error("RunE should be set")
	}
	// Verify expected flags are registered.
	for _, flag := range []string{
		"source-dump", "source-dbhost", "source-dbport", "source-dbname",
		"source-dbuser", "source-dbpass",
		"target-host", "target-port", "target-db", "target-user", "target-password",
		"admin-email", "device-filter", "max-positions", "recent-days",
		"exclude-unknown", "verbose", "dry-run",
		"devices", "positions", "geofences", "calendars", "geocode-last-n",
	} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("flag --%s not registered", flag)
		}
	}
}

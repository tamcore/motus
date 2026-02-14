package h02

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestDecodeV1_RealMessage(t *testing.T) {
	raw := "*HQ,123456789012345,V1,212250,A,4948.8999,N,00958.2106,E,000.00,000,110226,FFFFFBFF,262,03,49032,46083637#"

	msg, err := Decode(raw)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if msg.DeviceID != "123456789012345" {
		t.Errorf("DeviceID: got %q, want %q", msg.DeviceID, "123456789012345")
	}
	if msg.Type != "V1" {
		t.Errorf("Type: got %q, want %q", msg.Type, "V1")
	}
	if !msg.Valid {
		t.Error("expected valid position (A flag)")
	}

	// Latitude: 4948.8999 N = 49 + 48.8999/60 = 49.814998...
	wantLat := 49.814998
	if math.Abs(msg.Latitude-wantLat) > 0.001 {
		t.Errorf("Latitude: got %f, want ~%f", msg.Latitude, wantLat)
	}

	// Longitude: 00958.2106 E = 9 + 58.2106/60 = 9.970177...
	wantLon := 9.970177
	if math.Abs(msg.Longitude-wantLon) > 0.001 {
		t.Errorf("Longitude: got %f, want ~%f", msg.Longitude, wantLon)
	}

	// Speed: 000.00 knots = 0.0 km/h
	if msg.Speed != 0.0 {
		t.Errorf("Speed: got %f, want 0.0", msg.Speed)
	}

	// Course: 000
	if msg.Course != 0.0 {
		t.Errorf("Course: got %f, want 0.0", msg.Course)
	}

	// Timestamp: 21:22:50 UTC, 2026-02-11
	wantTime := time.Date(2026, 2, 11, 21, 22, 50, 0, time.UTC)
	if !msg.Timestamp.Equal(wantTime) {
		t.Errorf("Timestamp: got %v, want %v", msg.Timestamp, wantTime)
	}

	if msg.Flags != "FFFFFBFF" {
		t.Errorf("Flags: got %q, want %q", msg.Flags, "FFFFFBFF")
	}

	// FFFFFBFF: bit 10 = 0 → ignition OFF (vehicle parked).
	if msg.Ignition {
		t.Error("Ignition: expected false for FFFFFBFF (bit 10 = 0)")
	}

	// Cell tower info.
	if msg.MCC != 262 {
		t.Errorf("MCC: got %d, want 262", msg.MCC)
	}
	if msg.MNC != 3 {
		t.Errorf("MNC: got %d, want 3", msg.MNC)
	}
	if msg.LAC != 49032 {
		t.Errorf("LAC: got %d, want 49032", msg.LAC)
	}
	if msg.CellID != 46083637 {
		t.Errorf("CellID: got %d, want 46083637", msg.CellID)
	}
}

func TestDecodeV1_MovingVehicle(t *testing.T) {
	// Real message from h02.log showing the vehicle in motion.
	raw := "*HQ,123456789012345,V1,064713,A,4948.6549,N,00958.3511,E,015.70,323,130226,FFFFDFFF,262,03,49032,46083627#"

	msg, err := Decode(raw)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	// Speed: 15.70 knots = 15.70 * 1.852 = 29.0764 km/h
	wantSpeed := 15.70 * 1.852
	if math.Abs(msg.Speed-wantSpeed) > 0.01 {
		t.Errorf("Speed: got %f, want %f", msg.Speed, wantSpeed)
	}

	if msg.Course != 323.0 {
		t.Errorf("Course: got %f, want 323.0", msg.Course)
	}

	wantTime := time.Date(2026, 2, 13, 6, 47, 13, 0, time.UTC)
	if !msg.Timestamp.Equal(wantTime) {
		t.Errorf("Timestamp: got %v, want %v", msg.Timestamp, wantTime)
	}

	// FFFFDFFF: bit 10 = 1 → ignition ON (vehicle in motion).
	if !msg.Ignition {
		t.Error("Ignition: expected true for FFFFDFFF (bit 10 = 1)")
	}
}

func TestDecodeV1_InvalidGPSFix(t *testing.T) {
	// Real message with V (invalid) GPS fix from h02.log.
	raw := "*HQ,123456789012345,V1,061221,V,4948.8999,N,00958.2106,E,000.00,000,130226,FBFFDFFF,262,03,49032,46083627#"

	msg, err := Decode(raw)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if msg.Valid {
		t.Error("expected invalid position (V flag)")
	}

	// Position data is still parsed even when invalid.
	if msg.Latitude == 0 {
		t.Error("expected non-zero latitude even for invalid fix")
	}
}

func TestDecodeV6_RealMessage(t *testing.T) {
	raw := "*HQ,123456789012345,V6,211755,A,4948.8999,N,00958.2106,E,000.00,000,110226,FFFFFBFF,262,03,49032,46083637,8949227221106570251F#"

	msg, err := Decode(raw)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if msg.Type != "V6" {
		t.Errorf("Type: got %q, want %q", msg.Type, "V6")
	}

	if msg.ICCID != "8949227221106570251F" {
		t.Errorf("ICCID: got %q, want %q", msg.ICCID, "8949227221106570251F")
	}

	// V6 should have all the same fields as V1.
	if !msg.Valid {
		t.Error("expected valid position")
	}

	wantLat := 49.814998
	if math.Abs(msg.Latitude-wantLat) > 0.001 {
		t.Errorf("Latitude: got %f, want ~%f", msg.Latitude, wantLat)
	}
}

func TestDecodeV4_Heartbeat(t *testing.T) {
	raw := "*HQ,123456789012345,V4,V1,20260211212008#"

	msg, err := Decode(raw)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if msg.Type != "V4" {
		t.Errorf("Type: got %q, want %q", msg.Type, "V4")
	}

	if msg.DeviceID != "123456789012345" {
		t.Errorf("DeviceID: got %q, want %q", msg.DeviceID, "123456789012345")
	}

	if msg.Valid {
		t.Error("heartbeat should not have valid position")
	}

	// Heartbeat should parse embedded timestamp.
	wantTime := time.Date(2026, 2, 11, 21, 20, 8, 0, time.UTC)
	if !msg.Timestamp.Equal(wantTime) {
		t.Errorf("Timestamp: got %v, want %v", msg.Timestamp, wantTime)
	}
}

func TestDecodeV4_ShortHeartbeat(t *testing.T) {
	// Some devices send shorter V4 heartbeats.
	raw := "*HQ,123456789012345,V4,V1#"

	msg, err := Decode(raw)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if msg.Type != "V4" {
		t.Errorf("Type: got %q, want %q", msg.Type, "V4")
	}

	// Should still decode, just without a parsed timestamp.
	if msg.Valid {
		t.Error("heartbeat should not have valid position")
	}
}

func TestDecode_InvalidFormat(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"empty", ""},
		{"no prefix", "HQ,123,V1,data#"},
		{"no suffix", "*HQ,123,V1,data"},
		{"wrong prefix", "*XX,123,V1,data#"},
		{"too few fields", "*HQ,123#"},
		{"unknown type", "*HQ,123,ZZ,data#"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decode(tt.raw)
			if err == nil {
				t.Errorf("expected error for %q", tt.raw)
			}
		})
	}
}

func TestDecode_InsufficientPositionFields(t *testing.T) {
	// V1 with too few fields for position parsing.
	raw := "*HQ,123,V1,212250,A,4948.8999,N#"

	_, err := Decode(raw)
	if err == nil {
		t.Error("expected error for V1 with insufficient fields")
	}
}

func TestParseCoordinate(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"4948.8999", 49.814998}, // Latitude
		{"00958.2106", 9.970177}, // Longitude
		{"0000.0000", 0.0},       // Zero
		{"9000.0000", 90.0},      // Max latitude
		{"18000.0000", 180.0},    // Max longitude
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseCoordinate(tt.input)
			if err != nil {
				t.Fatalf("parseCoordinate(%q) error: %v", tt.input, err)
			}
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("parseCoordinate(%q) = %f, want ~%f", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseCoordinate_Error(t *testing.T) {
	tests := []string{
		"",
		"abc",
		"not_a_number",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := parseCoordinate(input)
			if err == nil {
				t.Errorf("expected error for %q", input)
			}
		})
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		timeStr string
		dateStr string
		want    time.Time
	}{
		{"212250", "110226", time.Date(2026, 2, 11, 21, 22, 50, 0, time.UTC)},
		{"060921", "130226", time.Date(2026, 2, 13, 6, 9, 21, 0, time.UTC)},
		{"000000", "010100", time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"235959", "311299", time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC)},
	}

	for _, tt := range tests {
		t.Run(tt.timeStr+"/"+tt.dateStr, func(t *testing.T) {
			got, err := parseTimestamp(tt.timeStr, tt.dateStr)
			if err != nil {
				t.Fatalf("parseTimestamp(%q, %q) error: %v", tt.timeStr, tt.dateStr, err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("parseTimestamp(%q, %q) = %v, want %v", tt.timeStr, tt.dateStr, got, tt.want)
			}
		})
	}
}

func TestParseTimestamp_Error(t *testing.T) {
	tests := []struct {
		name    string
		timeStr string
		dateStr string
	}{
		{"short time", "1234", "110226"},
		{"short date", "212250", "1102"},
		{"long time", "21225099", "110226"},
		{"long date", "212250", "11022699"},
		// Non-numeric hour.
		{"non-numeric hour", "ab1234", "110226"},
		// Non-numeric minute.
		{"non-numeric minute", "12ab34", "110226"},
		// Non-numeric second.
		{"non-numeric second", "1234ab", "110226"},
		// Non-numeric day.
		{"non-numeric day", "212250", "ab0226"},
		// Non-numeric month.
		{"non-numeric month", "212250", "11ab26"},
		// Non-numeric year.
		{"non-numeric year", "212250", "1102ab"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTimestamp(tt.timeStr, tt.dateStr)
			if err == nil {
				t.Errorf("expected error for timeStr=%q dateStr=%q", tt.timeStr, tt.dateStr)
			}
		})
	}
}

func TestEncodeResponse(t *testing.T) {
	resp := EncodeResponse("123456789012345", "V1")

	if !strings.HasPrefix(resp, "*HQ,123456789012345,V4,V1,") {
		t.Errorf("unexpected response prefix: %q", resp)
	}
	if !strings.HasSuffix(resp, "#") {
		t.Errorf("response should end with #: %q", resp)
	}

	// The timestamp portion should be 14 digits.
	parts := strings.Split(resp, ",")
	if len(parts) != 5 {
		t.Fatalf("expected 5 comma-separated parts, got %d", len(parts))
	}
	ts := strings.TrimSuffix(parts[4], "#")
	if len(ts) != 14 {
		t.Errorf("timestamp should be 14 digits, got %d: %q", len(ts), ts)
	}
}

func TestDecode_SouthWestCoordinates(t *testing.T) {
	// Synthetic message with south/west coordinates.
	raw := "*HQ,1234567890,V1,120000,A,3412.3456,S,05834.5678,W,010.50,180,150226,FFFFFFFF,0,0,0,0#"

	msg, err := Decode(raw)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if msg.Latitude >= 0 {
		t.Errorf("south latitude should be negative, got %f", msg.Latitude)
	}
	if msg.Longitude >= 0 {
		t.Errorf("west longitude should be negative, got %f", msg.Longitude)
	}

	// 3412.3456 S = -(34 + 12.3456/60) = -34.205760
	wantLat := -34.205760
	if math.Abs(msg.Latitude-wantLat) > 0.001 {
		t.Errorf("Latitude: got %f, want ~%f", msg.Latitude, wantLat)
	}

	// 05834.5678 W = -(58 + 34.5678/60) = -58.576130
	wantLon := -58.576130
	if math.Abs(msg.Longitude-wantLon) > 0.001 {
		t.Errorf("Longitude: got %f, want ~%f", msg.Longitude, wantLon)
	}
}

func TestDecode_WhitespaceHandling(t *testing.T) {
	// Messages may have trailing whitespace from network reads.
	raw := "  *HQ,123456789012345,V4,V1,20260211212008#  \n"

	msg, err := Decode(raw)
	if err != nil {
		t.Fatalf("decode with whitespace failed: %v", err)
	}

	if msg.Type != "V4" {
		t.Errorf("Type: got %q, want %q", msg.Type, "V4")
	}
}

func TestDecodeV1_BadLatitude(t *testing.T) {
	// Invalid latitude value.
	raw := "*HQ,123,V1,120000,A,BADLAT,N,00958.2106,E,010.50,180,150226,FFFFFFFF,0,0,0,0#"
	_, err := Decode(raw)
	if err == nil {
		t.Error("expected error for bad latitude")
	}
}

func TestDecodeV1_BadLongitude(t *testing.T) {
	// Invalid longitude value.
	raw := "*HQ,123,V1,120000,A,4948.8999,N,BADLON,E,010.50,180,150226,FFFFFFFF,0,0,0,0#"
	_, err := Decode(raw)
	if err == nil {
		t.Error("expected error for bad longitude")
	}
}

func TestDecodeV1_BadSpeed(t *testing.T) {
	// Invalid speed value.
	raw := "*HQ,123,V1,120000,A,4948.8999,N,00958.2106,E,FAST,180,150226,FFFFFFFF,0,0,0,0#"
	_, err := Decode(raw)
	if err == nil {
		t.Error("expected error for bad speed")
	}
}

func TestDecodeV1_BadCourse(t *testing.T) {
	// Invalid course value.
	raw := "*HQ,123,V1,120000,A,4948.8999,N,00958.2106,E,010.50,NORTH,150226,FFFFFFFF,0,0,0,0#"
	_, err := Decode(raw)
	if err == nil {
		t.Error("expected error for bad course")
	}
}

func TestDecodeV1_BadTimestamp(t *testing.T) {
	// Invalid time field (letters instead of digits).
	raw := "*HQ,123,V1,BADTIM,A,4948.8999,N,00958.2106,E,010.50,180,150226,FFFFFFFF,0,0,0,0#"
	_, err := Decode(raw)
	if err == nil {
		t.Error("expected error for bad timestamp")
	}
}

func TestDecodeV6_InvalidPosition(t *testing.T) {
	// V6 message with an invalid latitude field causes decodePosition to fail,
	// which propagates through decodePositionV6.
	raw := "*HQ,123,V6,120000,A,BADLAT,N,00958.2106,E,010.50,180,150226,FFFFFFFF,262,03,49032,46083637#"
	_, err := Decode(raw)
	if err == nil {
		t.Error("expected error for V6 message with invalid latitude")
	}
}

func TestDecodeV6_WithoutICCID(t *testing.T) {
	// V6 message without the ICCID field (only 16 fields).
	raw := "*HQ,123,V6,120000,A,4948.8999,N,00958.2106,E,010.50,180,150226,FFFFFFFF,262,03,49032,46083637#"
	msg, err := Decode(raw)
	if err != nil {
		t.Fatalf("decode V6 without ICCID failed: %v", err)
	}
	if msg.Type != "V6" {
		t.Errorf("Type: got %q, want %q", msg.Type, "V6")
	}
	if msg.ICCID != "" {
		t.Errorf("ICCID should be empty, got %q", msg.ICCID)
	}
}

func TestDecodeV1_NoFlags(t *testing.T) {
	// V1 message with exactly 11 fields (no flags, no cell tower).
	// Fields: imei, V1, time, validity, lat, N/S, lon, E/W, speed, course, date
	raw := "*HQ,123,V1,120000,A,4948.8999,N,00958.2106,E,010.50,180,150226#"
	// This has 11 fields after split (0..10), but decodePosition requires >= 12.
	_, err := Decode(raw)
	if err == nil {
		t.Error("expected error for V1 with only 11 fields")
	}
}

func TestDecodeIgnition(t *testing.T) {
	tests := []struct {
		name    string
		flags   string
		wantIgn bool
	}{
		// Real values from h02.log:
		// Vehicle parked/ignition off — bit 10 is 0.
		{"FFFFFBFF ignition off", "FFFFFBFF", false},
		// Vehicle driving/ignition on — bit 10 is 1.
		{"FFFFDFFF ignition on", "FFFFDFFF", true},
		// All bits set — ignition on.
		{"FFFFFFFF all set", "FFFFFFFF", true},
		// All bits clear — ignition off.
		{"00000000 all clear", "00000000", false},
		// Only bit 10 set (0x400 = 1024).
		{"00000400 only bit10", "00000400", true},
		// Bit 10 clear, neighbours set.
		{"FFFFFBFF same as parked", "FFFFFBFF", false},
		// Edge cases.
		{"empty flags", "", false},
		{"invalid hex", "ZZZZZZZZ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeIgnition(tt.flags)
			if got != tt.wantIgn {
				t.Errorf("decodeIgnition(%q) = %v, want %v", tt.flags, got, tt.wantIgn)
			}
		})
	}
}

func TestDecodeIgnition_ViaFullDecode(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantIgn bool
	}{
		{
			name:    "parked FFFFFBFF → ignition off",
			raw:     "*HQ,123,V1,164701,A,4948.8984,N,00958.2110,E,000.00,067,220226,FFFFFBFF,262,03,49030,55255#",
			wantIgn: false,
		},
		{
			name:    "driving FFFFDFFF → ignition on",
			raw:     "*HQ,123,V1,164558,A,4948.9164,N,00958.2028,E,004.03,198,220226,FFFFDFFF,262,03,49032,46083627#",
			wantIgn: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := Decode(tt.raw)
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}
			if msg.Ignition != tt.wantIgn {
				t.Errorf("Ignition = %v, want %v (flags=%q)", msg.Ignition, tt.wantIgn, msg.Flags)
			}
		})
	}
}

func TestDecodeV1_OptionalAltitude(t *testing.T) {
	tests := []struct {
		name         string
		raw          string
		wantAlt      float64
		wantIgnition bool
	}{
		{
			name:         "altitude present, ignition on",
			raw:          "*HQ,9000000000001,V1,143045,A,4808.1060,N,01134.9200,E,64.80,45,150226,FFFFFFEF,523.4#",
			wantAlt:      523.4,
			wantIgnition: true,
		},
		{
			name:         "altitude present, ignition off (parked)",
			raw:          "*HQ,9000000000001,V1,143045,A,4808.1060,N,01134.9200,E,00.00,0,150226,FFFFFBEF,520.0#",
			wantAlt:      520.0,
			wantIgnition: false,
		},
		{
			name:         "no altitude field (real device format)",
			raw:          "*HQ,123,V1,164701,A,4948.8984,N,00958.2110,E,000.00,067,220226,FFFFFBFF#",
			wantAlt:      0,
			wantIgnition: false,
		},
		{
			name:         "altitude zero",
			raw:          "*HQ,9000000000001,V1,143045,A,4808.1060,N,01134.9200,E,64.80,45,150226,FFFFFFEF,0.0#",
			wantAlt:      0,
			wantIgnition: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := Decode(tt.raw)
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}
			if math.Abs(msg.Altitude-tt.wantAlt) > 0.05 {
				t.Errorf("Altitude = %.2f, want %.2f", msg.Altitude, tt.wantAlt)
			}
			if msg.Ignition != tt.wantIgnition {
				t.Errorf("Ignition = %v, want %v", msg.Ignition, tt.wantIgnition)
			}
		})
	}
}

func TestDecodeSMS(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantDevice string
		wantResult string
		wantErr    bool
	}{
		{
			name:       "simple rconf response",
			raw:        "*HQ,123456789012345,SMS,IP:1.2.3.4,PORT:8080#",
			wantDevice: "123456789012345",
			wantResult: "IP:1.2.3.4,PORT:8080",
		},
		{
			name:       "multi-word result",
			raw:        "*HQ,123456789012345,SMS,OK#",
			wantDevice: "123456789012345",
			wantResult: "OK",
		},
		{
			// A bare *HQ,imei,SMS# has only 2 fields, which the decoder rejects.
			// Real devices always include at least one result field.
			name:    "bare SMS no result — rejected",
			raw:     "*HQ,123456789012345,SMS#",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := Decode(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if msg.DeviceID != tt.wantDevice {
				t.Errorf("DeviceID: got %q, want %q", msg.DeviceID, tt.wantDevice)
			}
			if msg.Type != "SMS" {
				t.Errorf("Type: got %q, want SMS", msg.Type)
			}
			if msg.Result != tt.wantResult {
				t.Errorf("Result: got %q, want %q", msg.Result, tt.wantResult)
			}
			if msg.Valid {
				t.Error("SMS message should not be valid (no position)")
			}
		})
	}
}

func TestDecodeAlarm(t *testing.T) {
	tests := []struct {
		name      string
		flags     string
		wantAlarm string
	}{
		{"no alarm (all 1s)", "FFFFFFFF", ""},
		{"SOS bit 1", "FFFFFFFD", "sos"},
		{"SOS bit 18", "FFFBFFFF", "sos"},
		{"power cut bit 19", "FFF7FFFF", "powerCut"},
		{"vibration bit 0", "FFFFFFFE", "vibration"},
		{"overspeed bit 2", "FFFFFFFB", "overspeed"},
		{"SOS takes priority over vibration", "FFFFFFFC", "sos"}, // bits 1 and 0 both 0
		{"empty flags", "", ""},
		{"invalid hex", "ZZZZZZZZ", ""},
		// Real device flags — no alarm bits set
		{"real parked FFFFFBFF", "FFFFFBFF", ""},
		{"real driving FFFFDFFF", "FFFFDFFF", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeAlarm(tt.flags)
			if got != tt.wantAlarm {
				t.Errorf("decodeAlarm(%q) = %q, want %q", tt.flags, got, tt.wantAlarm)
			}
		})
	}
}

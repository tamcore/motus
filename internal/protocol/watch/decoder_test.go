package watch

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestDecodeUD_Position(t *testing.T) {
	raw := "[3G*1234567890*0078*UD,14022026,153045,A,49.814998,N,9.970177,E,15.50,270.0,0.0,8,100,460,0,9527,3661]"

	msg, err := Decode(raw)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if msg.Manufacturer != "3G" {
		t.Errorf("Manufacturer: got %q, want %q", msg.Manufacturer, "3G")
	}
	if msg.DeviceID != "1234567890" {
		t.Errorf("DeviceID: got %q, want %q", msg.DeviceID, "1234567890")
	}
	if msg.Type != "UD" {
		t.Errorf("Type: got %q, want %q", msg.Type, "UD")
	}
	if !msg.Valid {
		t.Error("expected valid position")
	}

	wantLat := 49.814998
	if math.Abs(msg.Latitude-wantLat) > 0.001 {
		t.Errorf("Latitude: got %f, want ~%f", msg.Latitude, wantLat)
	}

	wantLon := 9.970177
	if math.Abs(msg.Longitude-wantLon) > 0.001 {
		t.Errorf("Longitude: got %f, want ~%f", msg.Longitude, wantLon)
	}

	if msg.Speed != 15.50 {
		t.Errorf("Speed: got %f, want 15.50", msg.Speed)
	}

	if msg.Course != 270.0 {
		t.Errorf("Course: got %f, want 270.0", msg.Course)
	}

	wantTime := time.Date(2026, 2, 14, 15, 30, 45, 0, time.UTC)
	if !msg.Timestamp.Equal(wantTime) {
		t.Errorf("Timestamp: got %v, want %v", msg.Timestamp, wantTime)
	}
}

func TestDecodeUD2_ExtendedPosition(t *testing.T) {
	raw := "[3G*9876543210*0080*UD2,14022026,120000,A,34.052235,N,118.243683,W,25.00,180.0,100.0,12,95,310,260,1234,5678]"

	msg, err := Decode(raw)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if msg.Type != "UD2" {
		t.Errorf("Type: got %q, want %q", msg.Type, "UD2")
	}

	// West longitude should be negative.
	if msg.Longitude >= 0 {
		t.Errorf("west longitude should be negative, got %f", msg.Longitude)
	}

	wantLon := -118.243683
	if math.Abs(msg.Longitude-wantLon) > 0.001 {
		t.Errorf("Longitude: got %f, want ~%f", msg.Longitude, wantLon)
	}
}

func TestDecodeLK_Heartbeat(t *testing.T) {
	raw := "[3G*1234567890*0005*LK,85]"

	msg, err := Decode(raw)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if msg.Type != "LK" {
		t.Errorf("Type: got %q, want %q", msg.Type, "LK")
	}

	if msg.Valid {
		t.Error("heartbeat should not have valid position")
	}

	if msg.Battery != 85 {
		t.Errorf("Battery: got %d, want 85", msg.Battery)
	}
}

func TestDecodeLK_NoPayload(t *testing.T) {
	raw := "[3G*1234567890*0002*LK]"

	msg, err := Decode(raw)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if msg.Type != "LK" {
		t.Errorf("Type: got %q, want %q", msg.Type, "LK")
	}
}

func TestDecodeAL_Alarm(t *testing.T) {
	raw := "[3G*1234567890*0078*AL,14022026,153045,A,49.814998,N,9.970177,E,0.00,0.0,0.0,8,100,460,0,9527,3661]"

	msg, err := Decode(raw)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if msg.Type != "AL" {
		t.Errorf("Type: got %q, want %q", msg.Type, "AL")
	}

	if !msg.Valid {
		t.Error("alarm with A flag should be valid")
	}
}

func TestDecodeUnknownType(t *testing.T) {
	raw := "[3G*1234567890*0004*PING]"

	msg, err := Decode(raw)
	if err != nil {
		t.Fatalf("unexpected error for unknown type: %v", err)
	}

	if msg.Type != "PING" {
		t.Errorf("Type: got %q, want %q", msg.Type, "PING")
	}

	if msg.Valid {
		t.Error("unknown type should not be valid")
	}
}

func TestDecode_InvalidFormat(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"empty", ""},
		{"no brackets", "3G*123*0004*LK"},
		{"no opening bracket", "3G*123*0004*LK]"},
		{"no closing bracket", "[3G*123*0004*LK"},
		{"too few header parts", "[3G*123]"},
		{"insufficient UD fields", "[3G*123*000A*UD,14022026,120000,A]"},
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

func TestDecode_SouthCoordinates(t *testing.T) {
	raw := "[3G*1234567890*0078*UD,14022026,120000,A,33.868820,S,151.209290,E,0.00,0.0,0.0,10,90,505,2,12345,6789]"

	msg, err := Decode(raw)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if msg.Latitude >= 0 {
		t.Errorf("south latitude should be negative, got %f", msg.Latitude)
	}

	wantLat := -33.868820
	if math.Abs(msg.Latitude-wantLat) > 0.001 {
		t.Errorf("Latitude: got %f, want ~%f", msg.Latitude, wantLat)
	}
}

func TestDecode_InvalidPosition(t *testing.T) {
	raw := "[3G*1234567890*0078*UD,14022026,120000,V,0.000000,N,0.000000,E,0.00,0.0,0.0,0,100,460,0,0,0]"

	msg, err := Decode(raw)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if msg.Valid {
		t.Error("expected invalid position (V flag)")
	}
}

func TestEncodeResponse_Heartbeat(t *testing.T) {
	resp := EncodeResponse("3G", "1234567890", "LK")

	if !strings.HasPrefix(resp, "[3G*1234567890*") {
		t.Errorf("unexpected response prefix: %q", resp)
	}
	if !strings.HasSuffix(resp, "*LK]") {
		t.Errorf("unexpected response suffix: %q", resp)
	}
}

func TestDecode_Whitespace(t *testing.T) {
	raw := "  [3G*1234567890*0002*LK]  \n"

	msg, err := Decode(raw)
	if err != nil {
		t.Fatalf("decode with whitespace failed: %v", err)
	}

	if msg.Type != "LK" {
		t.Errorf("Type: got %q, want %q", msg.Type, "LK")
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		dateStr string
		timeStr string
		want    time.Time
	}{
		{"14022026", "153045", time.Date(2026, 2, 14, 15, 30, 45, 0, time.UTC)},
		{"01012000", "000000", time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"31122099", "235959", time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC)},
	}

	for _, tt := range tests {
		t.Run(tt.dateStr+"/"+tt.timeStr, func(t *testing.T) {
			got, err := parseTimestamp(tt.dateStr, tt.timeStr)
			if err != nil {
				t.Fatalf("parseTimestamp error: %v", err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseTimestamp_Error(t *testing.T) {
	tests := []struct {
		name    string
		dateStr string
		timeStr string
	}{
		{"short date", "140220", "153045"},
		{"short time", "14022026", "1530"},
		// Non-numeric day.
		{"non-numeric day", "AB022026", "153045"},
		// Non-numeric month.
		{"non-numeric month", "14AB2026", "153045"},
		// Non-numeric year.
		{"non-numeric year", "1402ABCD", "153045"},
		// Non-numeric hour.
		{"non-numeric hour", "14022026", "AB3045"},
		// Non-numeric minute.
		{"non-numeric minute", "14022026", "15AB45"},
		// Non-numeric second.
		{"non-numeric second", "14022026", "1530AB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTimestamp(tt.dateStr, tt.timeStr)
			if err == nil {
				t.Errorf("expected error for dateStr=%q timeStr=%q", tt.dateStr, tt.timeStr)
			}
		})
	}
}

func TestDecodeUD_BadLatitude(t *testing.T) {
	raw := "[3G*123*0078*UD,14022026,153045,A,BADLAT,N,9.970177,E,15.50,270.0,0.0,8,100,460,0,9527,3661]"
	_, err := Decode(raw)
	if err == nil {
		t.Error("expected error for bad latitude")
	}
}

func TestDecodeUD_BadLongitude(t *testing.T) {
	raw := "[3G*123*0078*UD,14022026,153045,A,49.814998,N,BADLON,E,15.50,270.0,0.0,8,100,460,0,9527,3661]"
	_, err := Decode(raw)
	if err == nil {
		t.Error("expected error for bad longitude")
	}
}

func TestDecodeUD_BadTimestamp(t *testing.T) {
	raw := "[3G*123*0078*UD,BADDATE!,153045,A,49.814998,N,9.970177,E,15.50,270.0,0.0,8,100,460,0,9527,3661]"
	_, err := Decode(raw)
	if err == nil {
		t.Error("expected error for bad timestamp")
	}
}

func TestDecodeUD_NoSatellites(t *testing.T) {
	// UD message with exactly 9 fields (no altitude, no satellites).
	raw := "[3G*123*0040*UD,14022026,153045,A,49.814998,N,9.970177,E,15.50,270.0]"
	msg, err := Decode(raw)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if msg.Satellites != 0 {
		t.Errorf("satellites: got %d, want 0", msg.Satellites)
	}
}

func TestEncodeResponse_NonHeartbeat(t *testing.T) {
	// Non-LK type should still produce a valid response.
	resp := EncodeResponse("3G", "1234567890", "UD")
	if !strings.HasPrefix(resp, "[3G*1234567890*") {
		t.Errorf("unexpected response prefix: %q", resp)
	}
	if !strings.HasSuffix(resp, "*UD]") {
		t.Errorf("expected response to end with *UD], got %q", resp)
	}
}

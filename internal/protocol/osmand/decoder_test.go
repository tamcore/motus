package osmand

import (
	"math"
	"net/url"
	"testing"
	"time"
)

var now = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func query(t *testing.T, raw string) url.Values {
	t.Helper()
	v, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("bad query %q: %v", raw, err)
	}
	return v
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// Query vectors ported from Traccar's OsmAndProtocolDecoderTest.testDecodeQuery.
// Every one with an id must decode.
func TestDecodeQuery_TraccarVectors(t *testing.T) {
	for _, raw := range []string{
		"id=123456&timestamp=1377177267&cell=257,02,16,2224&cell=257,02,16,2223,-90&wifi=00-14-22-01-23-45,-80&wifi=00-1C-B3-09-85-15,-70",
		"id=902064&lat=42.06288&lon=-88.23412&timestamp=2016-01-27T18%3A55%3A47Z&hdop=6.0&altitude=224.0&speed=0.0",
		"id=902064&lat=42.06288&lon=-88.23412&timestamp=1442068686579&hdop=6.0&altitude=224.0&speed=0.0",
		"lat=49.60688&lon=6.15788&timestamp=2014-06-04+09%3A10%3A11&altitude=384.7&speed=0.0&id=353861053849681",
		"id=123456&timestamp=1377177267&lat=60.0&lon=30.0&speed=0.0&bearing=0.0&altitude=0&hdop=0.0",
		"id=123456&timestamp=1377177267&lat=60.0&lon=30.0",
		"lat=60.0&lon=30.0&speed=0.0&heading=0.0&vacc=0&hacc=0&altitude=0&deviceid=123456",
		"id=861001000719969&lat=41.666667&lon=-0.883333&altitude=350.059479&speed=0.000000&batt=87",
		"id=123456&timestamp=1377177267&location=60.0,30.0",
		"id=123456789012345&timestamp=1504763810&lat=40.7232948571&lon=-74.0061408571&bearing=7.19889788244&speed=40&ignition=true&rpm=933&fuel=24",
	} {
		if _, err := DecodeQuery(query(t, raw), now); err != nil {
			t.Errorf("DecodeQuery(%q): %v", raw, err)
		}
	}
}

func TestDecodeQuery_MissingDeviceID(t *testing.T) {
	// Traccar vector without id: rejected (HTTP 400).
	if _, err := DecodeQuery(query(t, "timestamp=1377177267&lat=60.0&lon=30.0"), now); err != ErrMissingDeviceID {
		t.Errorf("got %v, want ErrMissingDeviceID", err)
	}
}

func TestDecodeQuery_Fields(t *testing.T) {
	m, err := DecodeQuery(query(t, "id=123456789012345&timestamp=1504763810&lat=40.7232948571&lon=-74.0061408571&bearing=7.19889788244&speed=40&altitude=12.5&accuracy=8&hdop=1.5&batt=87&charge=true&driverUniqueId=D42&ignition=true&rpm=933&fuel=24.5&note=hello&notificationToken=abc"), now)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m.DeviceID != "123456789012345" || !m.HasLocation || !m.Valid {
		t.Errorf("header: %+v", m)
	}
	// Traccar expects 2017-09-07T05:56:50Z, 40.7232948571, -74.0061408571.
	if want := time.Date(2017, 9, 7, 5, 56, 50, 0, time.UTC); !m.Timestamp.Equal(want) {
		t.Errorf("timestamp: got %v, want %v", m.Timestamp, want)
	}
	if !near(m.Latitude, 40.7232948571) || !near(m.Longitude, -74.0061408571) {
		t.Errorf("location: %f,%f", m.Latitude, m.Longitude)
	}
	// Speed is sent in knots and stored in km/h.
	if !near(m.Speed, 40*1.852) || !near(m.Course, 7.19889788244) || m.Altitude != 12.5 || m.Accuracy != 8 {
		t.Errorf("speed/course/alt/acc: %v/%v/%v/%v", m.Speed, m.Course, m.Altitude, m.Accuracy)
	}
	want := map[string]any{
		"hdop": 1.5, "batteryLevel": 87.0, "charge": true, "driverUniqueId": "D42",
		"ignition": true, "rpm": 933.0, "fuel": 24.5, "note": "hello",
	}
	for k, v := range want {
		if m.Attributes[k] != v {
			t.Errorf("attribute %s: got %v (%T), want %v (%T)", k, m.Attributes[k], m.Attributes[k], v, v)
		}
	}
	if len(m.Attributes) != len(want) {
		t.Errorf("unexpected attributes: %v", m.Attributes)
	}
}

func TestDecodeQuery_Timestamps(t *testing.T) {
	tests := []struct {
		value string
		want  time.Time
	}{
		{"1377177267", time.Unix(1377177267, 0)},                                  // seconds
		{"1442068686579", time.UnixMilli(1442068686579)},                          // milliseconds
		{"2016-01-27T18:55:47Z", time.Date(2016, 1, 27, 18, 55, 47, 0, time.UTC)}, // ISO 8601
		{"2025-06-15T13:45:12.862Z", time.Date(2025, 6, 15, 13, 45, 12, 862e6, time.UTC)},
		{"2016-01-27T20:55:47+02:00", time.Date(2016, 1, 27, 18, 55, 47, 0, time.UTC)},
		{"2014-06-04 09:10:11", time.Date(2014, 6, 4, 9, 10, 11, 0, time.Local)}, // server local time
	}
	for _, tt := range tests {
		m, err := DecodeQuery(url.Values{"id": {"1"}, "timestamp": {tt.value}}, now)
		if err != nil {
			t.Fatalf("%q: %v", tt.value, err)
		}
		if !m.Timestamp.Equal(tt.want) {
			t.Errorf("%q: got %v, want %v", tt.value, m.Timestamp, tt.want)
		}
	}

	// Without a timestamp the receive time is used.
	m, err := DecodeQuery(url.Values{"id": {"1"}, "lat": {"1"}, "lon": {"2"}}, now)
	if err != nil || !m.Timestamp.Equal(now) {
		t.Errorf("no timestamp: got %v / %v", m, err)
	}
}

func TestDecodeQuery_Location(t *testing.T) {
	m, err := DecodeQuery(url.Values{"id": {"1"}, "location": {"60.0,30.0"}}, now)
	if err != nil || !m.HasLocation || m.Latitude != 60 || m.Longitude != 30 {
		t.Errorf("location param: %+v / %v", m, err)
	}

	// Only one coordinate, or none: no location (last known location is used).
	for _, v := range []url.Values{{"id": {"1"}}, {"id": {"1"}, "lat": {"60.0"}}} {
		m, err := DecodeQuery(v, now)
		if err != nil || m.HasLocation {
			t.Errorf("%v: expected no location, got %+v / %v", v, m, err)
		}
	}
}

func TestDecodeQuery_Valid(t *testing.T) {
	for value, want := range map[string]bool{"true": true, "TRUE": true, "1": true, "false": false, "0": false, "yes": false} {
		m, err := DecodeQuery(url.Values{"id": {"1"}, "valid": {value}}, now)
		if err != nil || m.Valid != want {
			t.Errorf("valid=%q: got %v / %v, want %v", value, m.Valid, err, want)
		}
	}
	if m, _ := DecodeQuery(url.Values{"id": {"1"}}, now); !m.Valid {
		t.Error("positions are valid by default")
	}
}

func TestDecodeQuery_Network(t *testing.T) {
	m, err := DecodeQuery(query(t, "id=123456&timestamp=1377177267&cell=257,02,16,2224&cell=257,02,16,2223,-90&wifi=00-14-22-01-23-45,-80&wifi=00-1C-B3-09-85-15,-70"), now)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	cells, _ := m.Network["cellTowers"].([]map[string]any)
	wifis, _ := m.Network["wifiAccessPoints"].([]map[string]any)
	if len(cells) != 2 || len(wifis) != 2 {
		t.Fatalf("network: %v", m.Network)
	}
	if cells[0]["mobileCountryCode"] != 257 || cells[0]["mobileNetworkCode"] != 2 || cells[0]["locationAreaCode"] != 16 || cells[0]["cellId"] != 2224 {
		t.Errorf("cell[0]: %v", cells[0])
	}
	if _, ok := cells[0]["signalStrength"]; ok {
		t.Errorf("cell[0] has no signal: %v", cells[0])
	}
	if cells[1]["signalStrength"] != -90 {
		t.Errorf("cell[1]: %v", cells[1])
	}
	if wifis[0]["macAddress"] != "00:14:22:01:23:45" || wifis[0]["signalStrength"] != -80 ||
		wifis[1]["macAddress"] != "00:1C:B3:09:85:15" || wifis[1]["signalStrength"] != -70 {
		t.Errorf("wifi: %v", wifis)
	}
	if m.Attributes["mcc"] != 257 || m.Attributes["cellId"] != 2224 {
		t.Errorf("first cell attributes: %v", m.Attributes)
	}
}

func TestDecodeQuery_Errors(t *testing.T) {
	for _, raw := range []string{
		"id=1&lat=abc&lon=1",
		"id=1&location=60.0",
		"id=1&timestamp=yesterday",
		"id=1&speed=fast",
		"id=1&cell=257,02,16",
		"id=1&wifi=00-14-22-01-23-45",
		"id=1&batt=full",
		"id=",
	} {
		if _, err := DecodeQuery(query(t, raw), now); err == nil {
			t.Errorf("DecodeQuery(%q): expected error", raw)
		}
	}
}

func TestDecodeQuery_NonFiniteNumbersStayStrings(t *testing.T) {
	// JSON cannot encode NaN/Inf, so such values must not become numbers.
	m, err := DecodeQuery(url.Values{"id": {"1"}, "x": {"NaN"}, "y": {"Inf"}}, now)
	if err != nil || m.Attributes["x"] != "NaN" || m.Attributes["y"] != "Inf" {
		t.Errorf("got %v / %v", m.Attributes, err)
	}
}

// JSON vectors ported from Traccar's OsmAndProtocolDecoderTest.testDecodeJson.
func TestDecodeJSON_TraccarVectors(t *testing.T) {
	for _, body := range []string{
		`{"location":{"timestamp":"2025-06-15T13:45:12.862Z","coords":{"latitude":37.4219983,"longitude":-122.084,"accuracy":5,"speed":0,"heading":-1,"altitude":5},"is_moving":false,"odometer":0,"event":"motionchange","battery":{"level":1,"is_charging":false},"activity":{"type":"still"},"extras":{},"_":"&id=48241179&lat=37.4219983&lon=-122.084&timestamp=2025-06-15T13:45:12.862Z&"},"device_id":"48241179"}`,
		`{"location":{"timestamp":"2025-06-15T13:45:12.862Z","coords":{"latitude":37.4219983,"longitude":-122.084,"accuracy":5,"speed":0,"heading":-1,"altitude":5},"is_moving":false,"odometer":0,"event":"motionchange","battery":{"level":1,"is_charging":false},"activity":{"type":"still"},"extras":{"device_string_test":"long_device_id","device_bool_test":true,"device_bool_false_test":false,"device_integer_test":1,"device_integer_twelve_test":12,"device_double_test":1.345,"device_very_long_double_test":1763636.3442},"_":"&id=48241179&lat=37.4219983&lon=-122.084&timestamp=2025-06-15T13:45:12.862Z&"},"device_id":"48241179"}`,
		`{"location":{"extras":{},"mock":true,"coords":{"speed_accuracy":-1,"speed":-1,"longitude":-122.406417,"ellipsoidal_altitude":0,"floor":null,"heading_accuracy":-1,"latitude":37.785834000000001,"accuracy":5,"altitude_accuracy":-1,"altitude":0,"heading":-1},"is_moving":false,"age":188,"odometer":0,"uuid":"2FB04C65-99CF-42AB-8DD3-EBCB4B108BF8","event":"motionchange","battery":{"level":-1,"is_charging":false},"activity":{"type":"unknown","confidence":100},"timestamp":"2025-05-09T04:11:30.579Z"},"device_id":"658765"}`,
		`{"location":{"event":"motionchange","is_moving":false,"uuid":"0e9a2473-a9a7-4c00-997b-fb97d2154e75","timestamp":"2021-07-21T08:06:34.444Z","odometer":0,"coords":{"latitude":-6.1148096,"longitude":106.6837015,"accuracy":3.8,"speed":18.67,"speed_accuracy":0.26,"heading":63,"heading_accuracy":0.28,"altitude":35.7,"altitude_accuracy":3.8},"activity":{"type":"still","confidence":100},"battery":{"is_charging":false,"level":0.79},"extras":{}},"device_id":"8737767034"}`,
	} {
		if _, err := DecodeJSON([]byte(body)); err != nil {
			t.Errorf("DecodeJSON(%s): %v", body, err)
		}
	}
}

func TestDecodeJSON_Fields(t *testing.T) {
	m, err := DecodeJSON([]byte(`{"location":{"event":"motionchange","is_moving":true,"timestamp":"2021-07-21T08:06:34.444Z","odometer":1234,"mock":false,"alarm":"sos","coords":{"latitude":-6.1148096,"longitude":106.6837015,"accuracy":3.8,"speed":18.67,"heading":63,"altitude":35.7},"activity":{"type":"in_vehicle","confidence":100},"battery":{"is_charging":true,"level":0.79},"extras":{"alarm":"ignored","trip":"a","count":12,"ratio":1.345,"flag":true,"obj":{"x":1}}},"device_id":"8737767034"}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m.DeviceID != "8737767034" || !m.HasLocation || !m.Valid {
		t.Errorf("header: %+v", m)
	}
	if want := time.Date(2021, 7, 21, 8, 6, 34, 444e6, time.UTC); !m.Timestamp.Equal(want) {
		t.Errorf("timestamp: got %v, want %v", m.Timestamp, want)
	}
	// Speed is sent in m/s and stored in km/h.
	if !near(m.Latitude, -6.1148096) || !near(m.Longitude, 106.6837015) || !near(m.Speed, 18.67*3.6) ||
		m.Course != 63 || m.Altitude != 35.7 || m.Accuracy != 3.8 {
		t.Errorf("fix: %+v", m)
	}
	want := map[string]any{
		"event": "motionchange", "odometer": int64(1234), "mock": false, "activity": "in_vehicle",
		"batteryLevel": 79, "charge": true, "alarm": "sos",
		"trip": "a", "count": int64(12), "ratio": 1.345, "flag": true,
	}
	for k, v := range want {
		if m.Attributes[k] != v {
			t.Errorf("attribute %s: got %v (%T), want %v (%T)", k, m.Attributes[k], m.Attributes[k], v, v)
		}
	}
	if len(m.Attributes) != len(want) {
		t.Errorf("unexpected attributes: %v", m.Attributes)
	}
}

func TestDecodeJSON_UnknownValues(t *testing.T) {
	// -1 means unknown for speed, heading and battery level.
	m, err := DecodeJSON([]byte(`{"location":{"timestamp":"2025-05-09T04:11:30.579Z","coords":{"speed":-1,"longitude":-122.406417,"latitude":37.785834,"accuracy":5,"altitude":0,"heading":-1,"floor":null},"battery":{"level":-1,"is_charging":false}},"device_id":"658765"}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m.Speed != 0 || m.Course != 0 {
		t.Errorf("speed/course: %v/%v", m.Speed, m.Course)
	}
	if _, ok := m.Attributes["batteryLevel"]; ok {
		t.Errorf("battery level -1 must be ignored: %v", m.Attributes)
	}
	if _, ok := m.Attributes["charge"]; ok {
		t.Errorf("charge only set when charging: %v", m.Attributes)
	}
}

func TestDecodeJSON_WithoutCoords(t *testing.T) {
	m, err := DecodeJSON([]byte(`{"location":{"timestamp":"2025-05-09T04:11:30.579Z","event":"heartbeat","battery":{"level":0.5,"is_charging":false}},"device_id":"658765"}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m.HasLocation || m.Attributes["event"] != "heartbeat" || m.Attributes["batteryLevel"] != 50 {
		t.Errorf("got %+v", m)
	}
}

func TestDecodeJSON_Errors(t *testing.T) {
	for _, body := range []string{
		``,
		`not json`,
		`[]`,
		`{"location":{"timestamp":"2025-05-09T04:11:30Z"}}`,
		`{"device_id":"","location":{"timestamp":"2025-05-09T04:11:30Z"}}`,
		`{"device_id":"1"}`,
		`{"device_id":"1","location":{}}`,
		`{"device_id":"1","location":{"timestamp":"yesterday"}}`,
		`{"device_id":"1","location":{"timestamp":"2025-05-09T04:11:30Z","coords":{"latitude":1}}}`,
		`{"device_id":1,"location":{"timestamp":"2025-05-09T04:11:30Z"}}`,
	} {
		_, err := DecodeJSON([]byte(body))
		if err == nil {
			t.Errorf("DecodeJSON(%s): expected error", body)
		}
	}
	if _, err := DecodeJSON([]byte(`{"location":{"timestamp":"2025-05-09T04:11:30Z"}}`)); err != ErrMissingDeviceID {
		t.Errorf("missing device_id: got %v, want ErrMissingDeviceID", err)
	}
}

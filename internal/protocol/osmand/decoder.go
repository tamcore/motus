// Package osmand implements the OsmAnd / Traccar Client HTTP protocol
// decoder, mirroring Traccar's OsmAndProtocolDecoder.
//
// Traccar Client (Android/iOS), OsmAnd and many other apps report positions
// with plain HTTP requests (Traccar's default port is 5055) in one of two
// formats:
//
//   - Query: GET or POST with URL query or form parameters, e.g.
//     /?id=123456&timestamp=1377177267&lat=60.0&lon=30.0&speed=0.0&batt=87
//   - JSON: POST with Content-Type application/json, as sent by current
//     Traccar Client versions:
//     {"device_id":"123456","location":{"timestamp":"…","coords":{…},"battery":{…}}}
//
// Speeds are converted to km/h: the query format uses knots, JSON uses m/s.
package osmand

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrMissingDeviceID is returned when a report does not identify the device.
var ErrMissingDeviceID = errors.New("missing device id")

// Message is a decoded position report.
type Message struct {
	DeviceID string

	// HasLocation is false when the report carries no coordinates; the
	// position should then be placed at the device's last known location.
	HasLocation bool
	Timestamp   time.Time
	Valid       bool
	Latitude    float64
	Longitude   float64
	Speed       float64 // km/h
	Course      float64
	Altitude    float64
	Accuracy    float64

	// Attributes holds protocol extras such as batteryLevel, charge, hdop,
	// event, activity and any unrecognised query parameters.
	Attributes map[string]any
	// Network holds cellTowers and wifiAccessPoints in Traccar's JSON layout,
	// or nil when none were reported.
	Network map[string]any
}

const knotsToKmh = 1.852
const mpsToKmh = 3.6

// DecodeQuery decodes a report sent as URL query or form parameters. now is
// used as the fix time when the report has no timestamp.
func DecodeQuery(params url.Values, now time.Time) (*Message, error) {
	m := &Message{Valid: true, Attributes: map[string]any{}}

	var lat, lon *float64
	var cells, wifis []map[string]any
	hasTime := false

	for key, values := range params {
		for _, value := range values {
			var err error
			switch key {
			case "id", "deviceid":
				m.DeviceID = value
			case "notificationToken":
				// Push notification token for Traccar's command delivery; not used.
			case "valid":
				m.Valid = strings.EqualFold(value, "true") || value == "1"
			case "timestamp":
				m.Timestamp, err = parseQueryTime(value)
				hasTime = true
			case "lat":
				lat, err = parseFloatPtr(value)
			case "lon":
				lon, err = parseFloatPtr(value)
			case "location":
				la, lo, ok := strings.Cut(value, ",")
				if !ok {
					err = fmt.Errorf("expected lat,lon")
					break
				}
				if lat, err = parseFloatPtr(la); err == nil {
					lon, err = parseFloatPtr(lo)
				}
			case "cell":
				var cell map[string]any
				if cell, err = parseCell(value); err == nil {
					cells = append(cells, cell)
				}
			case "wifi":
				var wifi map[string]any
				if wifi, err = parseWifi(value); err == nil {
					wifis = append(wifis, wifi)
				}
			case "speed":
				var knots float64
				if knots, err = strconv.ParseFloat(value, 64); err == nil {
					m.Speed = knots * knotsToKmh
				}
			case "bearing", "heading":
				m.Course, err = strconv.ParseFloat(value, 64)
			case "altitude":
				m.Altitude, err = strconv.ParseFloat(value, 64)
			case "accuracy":
				m.Accuracy, err = strconv.ParseFloat(value, 64)
			case "hdop":
				err = setFloat(m.Attributes, "hdop", value)
			case "batt":
				err = setFloat(m.Attributes, "batteryLevel", value)
			case "driverUniqueId":
				m.Attributes["driverUniqueId"] = value
			case "charge":
				m.Attributes["charge"] = strings.EqualFold(value, "true")
			default:
				m.Attributes[key] = queryValue(value)
			}
			if err != nil {
				return nil, fmt.Errorf("parse %s %q: %w", key, value, err)
			}
		}
	}

	if m.DeviceID == "" {
		return nil, ErrMissingDeviceID
	}
	if !hasTime {
		m.Timestamp = now
	}
	if lat != nil && lon != nil {
		m.HasLocation = true
		m.Latitude, m.Longitude = *lat, *lon
	}
	m.setNetwork(cells, wifis)
	return m, nil
}

// jsonReport is the Traccar Client JSON format (transistorsoft background
// geolocation layout).
type jsonReport struct {
	DeviceID *string `json:"device_id"`
	Location *struct {
		Timestamp string `json:"timestamp"`
		Coords    *struct {
			Latitude  *float64 `json:"latitude"`
			Longitude *float64 `json:"longitude"`
			Speed     *float64 `json:"speed"`
			Heading   *float64 `json:"heading"`
			Accuracy  *float64 `json:"accuracy"`
			Altitude  *float64 `json:"altitude"`
		} `json:"coords"`
		Event    *string `json:"event"`
		Odometer *int64  `json:"odometer"`
		Mock     *bool   `json:"mock"`
		Activity *struct {
			Type string `json:"type"`
		} `json:"activity"`
		Battery *struct {
			Level      *float64 `json:"level"`
			IsCharging bool     `json:"is_charging"`
		} `json:"battery"`
		Alarm  *string                    `json:"alarm"`
		Extras map[string]json.RawMessage `json:"extras"`
	} `json:"location"`
}

// DecodeJSON decodes a report sent as a JSON body.
func DecodeJSON(body []byte) (*Message, error) {
	var r jsonReport
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}
	if r.DeviceID == nil || *r.DeviceID == "" {
		return nil, ErrMissingDeviceID
	}
	if r.Location == nil {
		return nil, fmt.Errorf("missing location")
	}
	loc := r.Location

	m := &Message{DeviceID: *r.DeviceID, Attributes: map[string]any{}}
	ts, err := time.Parse(time.RFC3339Nano, loc.Timestamp)
	if err != nil {
		return nil, fmt.Errorf("parse timestamp %q: %w", loc.Timestamp, err)
	}
	m.Timestamp = ts

	if c := loc.Coords; c != nil {
		if c.Latitude == nil || c.Longitude == nil {
			return nil, fmt.Errorf("coords without latitude/longitude")
		}
		m.HasLocation, m.Valid = true, true
		m.Latitude, m.Longitude = *c.Latitude, *c.Longitude
		// Negative speed and heading mean "unknown".
		if c.Speed != nil && *c.Speed >= 0 {
			m.Speed = *c.Speed * mpsToKmh
		}
		if c.Heading != nil && *c.Heading >= 0 {
			m.Course = *c.Heading
		}
		if c.Accuracy != nil && *c.Accuracy >= 0 {
			m.Accuracy = *c.Accuracy
		}
		if c.Altitude != nil {
			m.Altitude = *c.Altitude
		}
	}

	// is_moving is not stored: motus derives the motion attribute from speed.
	if loc.Event != nil {
		m.Attributes["event"] = *loc.Event
	}
	if loc.Odometer != nil {
		m.Attributes["odometer"] = *loc.Odometer
	}
	if loc.Mock != nil {
		m.Attributes["mock"] = *loc.Mock
	}
	if loc.Activity != nil {
		m.Attributes["activity"] = loc.Activity.Type
	}
	if b := loc.Battery; b != nil {
		if b.Level != nil && *b.Level >= 0 {
			m.Attributes["batteryLevel"] = int(*b.Level * 100)
		}
		if b.IsCharging {
			m.Attributes["charge"] = true
		}
	}
	if loc.Alarm != nil {
		m.Attributes["alarm"] = *loc.Alarm
	}
	for key, raw := range loc.Extras {
		if key == "alarm" && loc.Alarm != nil {
			continue
		}
		if v, ok := extraValue(raw); ok {
			m.Attributes[key] = v
		}
	}
	return m, nil
}

// extraValue converts a JSON extras value: integral numbers become int64,
// other numbers float64; booleans and strings are kept; anything else is
// skipped.
func extraValue(raw json.RawMessage) (any, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	switch x := v.(type) {
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i, true
		}
		if f, err := x.Float64(); err == nil {
			return f, true
		}
	case bool, string:
		return x, true
	}
	return nil, false
}

// parseQueryTime parses a query timestamp: Unix seconds or milliseconds,
// ISO 8601, or "yyyy-MM-dd HH:mm:ss" in server local time.
func parseQueryTime(value string) (time.Time, error) {
	if n, err := strconv.ParseInt(value, 10, 64); err == nil {
		if n < math.MaxInt32 {
			return time.Unix(n, 0), nil
		}
		return time.UnixMilli(n), nil
	}
	if strings.Contains(value, "T") {
		return time.Parse(time.RFC3339Nano, value)
	}
	return time.ParseInLocation("2006-01-02 15:04:05", value, time.Local)
}

func parseFloatPtr(value string) (*float64, error) {
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func setFloat(attrs map[string]any, key, value string) error {
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return err
	}
	attrs[key] = f
	return nil
}

// queryValue converts an unrecognised parameter to a number or boolean when
// possible, and keeps it as a string otherwise.
func queryValue(value string) any {
	if f, err := strconv.ParseFloat(value, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
		return f
	}
	switch value {
	case "true":
		return true
	case "false":
		return false
	}
	return value
}

// parseCell parses "mcc,mnc,lac,cid[,rssi]".
func parseCell(value string) (map[string]any, error) {
	parts := strings.Split(value, ",")
	if len(parts) < 4 {
		return nil, fmt.Errorf("expected mcc,mnc,lac,cid[,rssi]")
	}
	n := make([]int, len(parts))
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil {
			return nil, err
		}
		n[i] = v
	}
	cell := map[string]any{
		"mobileCountryCode": n[0],
		"mobileNetworkCode": n[1],
		"locationAreaCode":  n[2],
		"cellId":            n[3],
	}
	if len(n) > 4 {
		cell["signalStrength"] = n[4]
	}
	return cell, nil
}

// parseWifi parses "mac,rssi"; dashes in the MAC address become colons.
func parseWifi(value string) (map[string]any, error) {
	mac, rssi, ok := strings.Cut(value, ",")
	if !ok {
		return nil, fmt.Errorf("expected mac,rssi")
	}
	signal, err := strconv.Atoi(rssi)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"macAddress":     strings.ReplaceAll(mac, "-", ":"),
		"signalStrength": signal,
	}, nil
}

// setNetwork stores cell towers and Wi-Fi access points, and copies the
// first cell tower to the mcc/mnc/lac/cellId attributes like other protocols.
func (m *Message) setNetwork(cells, wifis []map[string]any) {
	if len(cells) == 0 && len(wifis) == 0 {
		return
	}
	m.Network = map[string]any{"radioType": "gsm", "considerIp": false}
	if len(cells) > 0 {
		m.Network["cellTowers"] = cells
		m.Attributes["mcc"] = cells[0]["mobileCountryCode"]
		m.Attributes["mnc"] = cells[0]["mobileNetworkCode"]
		m.Attributes["lac"] = cells[0]["locationAreaCode"]
		m.Attributes["cellId"] = cells[0]["cellId"]
	}
	if len(wifis) > 0 {
		m.Network["wifiAccessPoints"] = wifis
	}
}

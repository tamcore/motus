// Package h02 implements the H02 GPS tracker protocol decoder.
//
// The H02 protocol is used by Sinotrack and similar GPS trackers.
// Messages are ASCII, delimited by * prefix and # suffix.
//
// Supported message types:
//   - V1: Standard position report
//   - V6: Extended position report (V1 + ICCID)
//   - V4: Heartbeat / keep-alive
package h02

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Message represents a decoded H02 protocol message.
type Message struct {
	DeviceID  string
	Type      string // V1, V4, V6, SMS
	Timestamp time.Time
	Valid     bool
	Latitude  float64
	Longitude float64
	Altitude  float64 // meters above sea level (optional, 0 if not present)
	Speed     float64 // km/h
	Course    float64 // degrees
	Flags     string
	Ignition  bool   // true = ACC/ignition on (flags bit 10)
	Alarm     string // non-empty when an alarm bit is set (e.g. "sos", "powerCut", "vibration", "overspeed")
	MCC       int    // Mobile Country Code
	MNC       int    // Mobile Network Code
	LAC       int    // Location Area Code
	CellID    int    // Cell Tower ID
	ICCID     string // SIM card ID (V6 only)
	Result    string // command response text (SMS type only)
}

// Decode parses an H02 protocol message string.
//
// Format: *HQ,<imei>,<type>,...#
//
// Returns the decoded message or an error if the format is invalid.
func Decode(raw string) (*Message, error) {
	raw = strings.TrimSpace(raw)

	if !strings.HasPrefix(raw, "*HQ,") || !strings.HasSuffix(raw, "#") {
		return nil, fmt.Errorf("invalid H02 message: must start with *HQ, and end with #")
	}

	// Strip prefix and suffix.
	data := raw[4 : len(raw)-1] // remove "*HQ," and "#"
	fields := strings.Split(data, ",")

	if len(fields) < 3 {
		return nil, fmt.Errorf("insufficient fields in H02 message: got %d, need at least 3", len(fields))
	}

	msg := &Message{
		DeviceID: fields[0],
		Type:     fields[1],
	}

	switch msg.Type {
	case "V1":
		return decodePosition(msg, fields)
	case "V6":
		return decodePositionV6(msg, fields)
	case "V4":
		return decodeHeartbeat(msg, fields)
	case "SMS":
		return decodeSMS(msg, fields)
	default:
		return nil, fmt.Errorf("unknown H02 message type: %s", msg.Type)
	}
}

// decodePosition parses a V1 position message.
//
// Field layout (0-indexed from after *HQ,):
//
//	0: IMEI
//	1: V1
//	2: HHMMSS (time)
//	3: A/V (validity: A=valid, V=invalid)
//	4: DDMM.MMMM (latitude)
//	5: N/S
//	6: DDDMM.MMMM (longitude)
//	7: E/W
//	8: speed (knots)
//	9: course (degrees)
//	10: DDMMYY (date)
//	11: flags (hex status word)
//	12: MCC
//	13: MNC
//	14: LAC
//	15: CellID
func decodePosition(msg *Message, fields []string) (*Message, error) {
	if len(fields) < 12 {
		return nil, fmt.Errorf("insufficient V1 fields: got %d, need at least 12", len(fields))
	}

	// Parse validity.
	msg.Valid = fields[3] == "A"

	// Parse latitude (DDMM.MMMM).
	lat, err := parseCoordinate(fields[4])
	if err != nil {
		return nil, fmt.Errorf("parse latitude %q: %w", fields[4], err)
	}
	if fields[5] == "S" {
		lat = -lat
	}
	msg.Latitude = lat

	// Parse longitude (DDDMM.MMMM).
	lon, err := parseCoordinate(fields[6])
	if err != nil {
		return nil, fmt.Errorf("parse longitude %q: %w", fields[6], err)
	}
	if fields[7] == "W" {
		lon = -lon
	}
	msg.Longitude = lon

	// Parse speed (knots -> km/h).
	speed, err := strconv.ParseFloat(fields[8], 64)
	if err != nil {
		return nil, fmt.Errorf("parse speed %q: %w", fields[8], err)
	}
	msg.Speed = speed * 1.852

	// Parse course (degrees).
	course, err := strconv.ParseFloat(fields[9], 64)
	if err != nil {
		return nil, fmt.Errorf("parse course %q: %w", fields[9], err)
	}
	msg.Course = course

	// Parse timestamp.
	ts, err := parseTimestamp(fields[2], fields[10])
	if err != nil {
		return nil, fmt.Errorf("parse timestamp: %w", err)
	}
	msg.Timestamp = ts

	// Parse flags, ignition state, and alarm bits.
	if len(fields) > 11 {
		msg.Flags = fields[11]
		msg.Ignition = decodeIgnition(fields[11])
		msg.Alarm = decodeAlarm(fields[11])
	}

	// Parse optional altitude (meters) at field 12.
	// This is a demo-simulator extension: the simulator appends altitude after
	// the flags field. Only present when field count is 13-15 (i.e. no cell
	// tower data, which occupies fields 12-15 and requires len > 15).
	if len(fields) >= 13 && len(fields) < 16 {
		if alt, err := strconv.ParseFloat(fields[12], 64); err == nil {
			msg.Altitude = alt
		}
	}

	// Parse cell tower info if present.
	if len(fields) > 15 {
		msg.MCC, _ = strconv.Atoi(fields[12])
		msg.MNC, _ = strconv.Atoi(fields[13])
		msg.LAC, _ = strconv.Atoi(fields[14])
		msg.CellID, _ = strconv.Atoi(fields[15])
	}

	return msg, nil
}

// decodePositionV6 parses a V6 extended position message.
// Same as V1 but with an additional ICCID field at the end.
func decodePositionV6(msg *Message, fields []string) (*Message, error) {
	msg, err := decodePosition(msg, fields)
	if err != nil {
		return nil, err
	}

	// V6 has ICCID as the last field (after cell tower info).
	if len(fields) > 16 {
		msg.ICCID = fields[16]
	}

	return msg, nil
}

// decodeHeartbeat parses a V4 heartbeat message.
//
// Format: *HQ,<imei>,V4,<sub_type>,<timestamp>#
// The device sends these to maintain the connection.
func decodeHeartbeat(msg *Message, fields []string) (*Message, error) {
	msg.Valid = false
	msg.Timestamp = time.Now().UTC()

	// Try to parse the embedded timestamp if present.
	// Format: *HQ,imei,V4,V1,YYYYMMDDHHmmSS#
	// Fields: [0]=imei, [1]=V4, [2]=sub_type, [3]=timestamp
	if len(fields) >= 4 && len(fields[3]) == 14 {
		ts, err := time.Parse("20060102150405", fields[3])
		if err == nil {
			msg.Timestamp = ts
		}
	}

	return msg, nil
}

// decodeSMS parses an SMS (command response) message.
//
// Format: *HQ,<imei>,SMS,<result_text...>#
// The result text may contain commas; everything after the type field is the response.
func decodeSMS(msg *Message, fields []string) (*Message, error) {
	msg.Valid = false
	msg.Timestamp = time.Now().UTC()
	if len(fields) > 2 {
		msg.Result = strings.Join(fields[2:], ",")
	}
	return msg, nil
}

// decodeIgnition extracts the ACC/ignition state from the H02 flags hex word.
// Bit 10 of the status word is the ignition bit: 1 = on, 0 = off.
// This matches the Traccar H02 decoder (BitUtil.check(status, 10)).
func decodeIgnition(flags string) bool {
	if flags == "" {
		return false
	}
	val, err := strconv.ParseUint(flags, 16, 32)
	if err != nil {
		return false
	}
	return (val>>10)&1 == 1
}

// decodeAlarm extracts the highest-priority active alarm from the H02 flags
// word. Alarm bits are active-low (0 = alarm triggered). Priority order
// matches Traccar: SOS > power cut > vibration > overspeed.
//
// Bit mapping:
//
//	1 or 18: SOS / panic button
//	19:      power cut (external power disconnected)
//	0:       vibration / movement alarm
//	2:       overspeed (hardware-level, distinct from the software check)
func decodeAlarm(flags string) string {
	if flags == "" {
		return ""
	}
	val, err := strconv.ParseUint(flags, 16, 32)
	if err != nil {
		return ""
	}
	check := func(bit uint) bool { return (val>>bit)&1 == 0 }

	switch {
	case check(1) || check(18):
		return "sos"
	case check(19):
		return "powerCut"
	case check(0):
		return "vibration"
	case check(2):
		return "overspeed"
	default:
		return ""
	}
}

// parseCoordinate converts NMEA coordinate format (DDMM.MMMM or DDDMM.MMMM)
// to decimal degrees.
//
// Examples:
//
//	4948.8999 -> 49 degrees, 48.8999 minutes -> 49.814998 degrees
//	00958.2106 -> 9 degrees, 58.2106 minutes -> 9.970177 degrees
func parseCoordinate(s string) (float64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty coordinate")
	}

	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("parse float: %w", err)
	}

	// Integer division by 100 gives the degrees portion.
	// The remainder is the minutes portion.
	degrees := float64(int(val / 100))
	minutes := val - (degrees * 100)

	return degrees + (minutes / 60.0), nil
}

// parseTimestamp combines HHMMSS time and DDMMYY date strings into a UTC time.
func parseTimestamp(timeStr, dateStr string) (time.Time, error) {
	if len(timeStr) != 6 {
		return time.Time{}, fmt.Errorf("invalid time format %q: expected 6 digits (HHMMSS)", timeStr)
	}
	if len(dateStr) != 6 {
		return time.Time{}, fmt.Errorf("invalid date format %q: expected 6 digits (DDMMYY)", dateStr)
	}

	hour, err := strconv.Atoi(timeStr[0:2])
	if err != nil {
		return time.Time{}, fmt.Errorf("parse hour: %w", err)
	}
	min, err := strconv.Atoi(timeStr[2:4])
	if err != nil {
		return time.Time{}, fmt.Errorf("parse minute: %w", err)
	}
	sec, err := strconv.Atoi(timeStr[4:6])
	if err != nil {
		return time.Time{}, fmt.Errorf("parse second: %w", err)
	}

	day, err := strconv.Atoi(dateStr[0:2])
	if err != nil {
		return time.Time{}, fmt.Errorf("parse day: %w", err)
	}
	month, err := strconv.Atoi(dateStr[2:4])
	if err != nil {
		return time.Time{}, fmt.Errorf("parse month: %w", err)
	}
	year, err := strconv.Atoi(dateStr[4:6])
	if err != nil {
		return time.Time{}, fmt.Errorf("parse year: %w", err)
	}

	year += 2000

	return time.Date(year, time.Month(month), day, hour, min, sec, 0, time.UTC), nil
}

// EncodeResponse creates an H02 acknowledgment response.
//
// Traccar sends this format after receiving a V1/V6 position:
//
//	*HQ,<imei>,V4,<msg_type>,<YYYYMMDDHHmmSS>#
func EncodeResponse(deviceID, msgType string) string {
	ts := time.Now().UTC().Format("20060102150405")
	return fmt.Sprintf("*HQ,%s,V4,%s,%s#", deviceID, msgType, ts)
}

// Package watch implements the WATCH GPS protocol decoder.
//
// The WATCH protocol is used by children's GPS watch trackers (e.g., Q50, Q90).
// Messages are enclosed in square brackets with comma-separated fields.
//
// Message format: [<manufacturer>*<imei>*<length>*<type>,<data>]
//
// Supported message types:
//   - UD:  Position report (GPS + LBS data)
//   - UD2: Extended position report
//   - LK:  Heartbeat / keep-alive
//   - AL:  Alarm
package watch

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Message represents a decoded WATCH protocol message.
type Message struct {
	Manufacturer string
	DeviceID     string
	Type         string // UD, UD2, LK, AL
	Timestamp    time.Time
	Valid        bool
	Latitude     float64
	Longitude    float64
	Speed        float64
	Course       float64
	Satellites   int
	Battery      int
}

// Decode parses a WATCH protocol message string.
//
// Format: [<manufacturer>*<imei>*<hex_length>*<type>,<data>]
//
// Returns the decoded message or an error if the format is invalid.
func Decode(raw string) (*Message, error) {
	raw = strings.TrimSpace(raw)

	if !strings.HasPrefix(raw, "[") || !strings.HasSuffix(raw, "]") {
		return nil, fmt.Errorf("invalid WATCH message: must be enclosed in brackets")
	}

	// Remove brackets.
	data := raw[1 : len(raw)-1]

	// Split header: manufacturer*imei*hex_length*content
	parts := strings.SplitN(data, "*", 4)
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid WATCH header: expected manufacturer*imei*length*content, got %d parts", len(parts))
	}

	msg := &Message{
		Manufacturer: parts[0],
		DeviceID:     parts[1],
	}

	// The content is type,data (comma separated).
	content := parts[3]
	comma := strings.IndexByte(content, ',')
	if comma == -1 {
		// Some messages (like LK responses) have no comma.
		msg.Type = content
		return decodeByType(msg, nil)
	}

	msg.Type = content[:comma]
	payload := content[comma+1:]
	fields := strings.Split(payload, ",")

	return decodeByType(msg, fields)
}

func decodeByType(msg *Message, fields []string) (*Message, error) {
	switch msg.Type {
	case "UD", "UD2":
		return decodePosition(msg, fields)
	case "LK":
		return decodeHeartbeat(msg, fields)
	case "AL":
		return decodeAlarm(msg, fields)
	default:
		// Unknown types are still returned for logging.
		msg.Valid = false
		msg.Timestamp = time.Now().UTC()
		return msg, nil
	}
}

// decodePosition parses a UD/UD2 position message.
//
// UD payload fields:
//
//	0: DDMMYYYY (date)
//	1: HHMMSS (time)
//	2: A/V (validity)
//	3: latitude (DD.DDDDDD decimal degrees)
//	4: N/S
//	5: longitude (DDD.DDDDDD decimal degrees)
//	6: E/W
//	7: speed (km/h)
//	8: course (degrees)
//	9: altitude (meters)
//	10: satellites
//	11+: GSM/LBS data
func decodePosition(msg *Message, fields []string) (*Message, error) {
	if len(fields) < 9 {
		return nil, fmt.Errorf("insufficient UD fields: got %d, need at least 9", len(fields))
	}

	// Parse validity.
	msg.Valid = fields[2] == "A"

	// Parse latitude (already in decimal degrees for WATCH protocol).
	lat, err := strconv.ParseFloat(fields[3], 64)
	if err != nil {
		return nil, fmt.Errorf("parse latitude %q: %w", fields[3], err)
	}
	if fields[4] == "S" {
		lat = -lat
	}
	msg.Latitude = lat

	// Parse longitude.
	lon, err := strconv.ParseFloat(fields[5], 64)
	if err != nil {
		return nil, fmt.Errorf("parse longitude %q: %w", fields[5], err)
	}
	if fields[6] == "W" {
		lon = -lon
	}
	msg.Longitude = lon

	// Parse speed (km/h).
	speed, _ := strconv.ParseFloat(fields[7], 64)
	msg.Speed = speed

	// Parse course.
	course, _ := strconv.ParseFloat(fields[8], 64)
	msg.Course = course

	// Parse satellites if present.
	if len(fields) > 10 {
		msg.Satellites, _ = strconv.Atoi(fields[10])
	}

	// Parse timestamp.
	ts, err := parseTimestamp(fields[0], fields[1])
	if err != nil {
		return nil, fmt.Errorf("parse timestamp: %w", err)
	}
	msg.Timestamp = ts

	return msg, nil
}

// decodeHeartbeat parses an LK heartbeat message.
//
// LK payload may contain battery level and other status.
func decodeHeartbeat(msg *Message, fields []string) (*Message, error) {
	msg.Valid = false
	msg.Timestamp = time.Now().UTC()

	if len(fields) > 0 {
		msg.Battery, _ = strconv.Atoi(fields[0])
	}

	return msg, nil
}

// decodeAlarm parses an AL alarm message (same position format as UD).
func decodeAlarm(msg *Message, fields []string) (*Message, error) {
	return decodePosition(msg, fields)
}

// parseTimestamp combines DDMMYYYY date and HHMMSS time strings.
func parseTimestamp(dateStr, timeStr string) (time.Time, error) {
	if len(dateStr) != 8 {
		return time.Time{}, fmt.Errorf("invalid date format %q: expected 8 digits (DDMMYYYY)", dateStr)
	}
	if len(timeStr) != 6 {
		return time.Time{}, fmt.Errorf("invalid time format %q: expected 6 digits (HHMMSS)", timeStr)
	}

	day, err := strconv.Atoi(dateStr[0:2])
	if err != nil {
		return time.Time{}, fmt.Errorf("parse day: %w", err)
	}
	month, err := strconv.Atoi(dateStr[2:4])
	if err != nil {
		return time.Time{}, fmt.Errorf("parse month: %w", err)
	}
	year, err := strconv.Atoi(dateStr[4:8])
	if err != nil {
		return time.Time{}, fmt.Errorf("parse year: %w", err)
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

	return time.Date(year, time.Month(month), day, hour, min, sec, 0, time.UTC), nil
}

// EncodeResponse creates a WATCH protocol response.
//
// Format: [<manufacturer>*<imei>*<hex_length>*<type>,<data>]
func EncodeResponse(manufacturer, deviceID, msgType string) string {
	var content string
	switch msgType {
	case "LK":
		// Heartbeat response with empty payload.
		content = "LK"
	default:
		content = msgType
	}
	length := fmt.Sprintf("%04X", len(content))
	return fmt.Sprintf("[%s*%s*%s*%s]", manufacturer, deviceID, length, content)
}

// Package watch implements the WATCH GPS protocol decoder.
//
// The WATCH protocol (also known as 3G/SG/CS/ZJ) is used by children's GPS
// watch trackers (e.g., Q50, Q90). The implementation mirrors Traccar's
// WatchFrameDecoder and WatchProtocolDecoder, which serve as the protocol
// specification.
//
// Frame format:
//
//	[<manufacturer>*<id>*<hex_length>*<type>[,<content>]]
//	[<manufacturer>*<id>*<index>*<hex_length>*<type>[,<content>]]   (indexed)
//
// Frames are not newline terminated; see SplitFunc for stream framing and
// Unescape for binary payload escaping.
//
// Supported message types:
//   - INIT:        registration, acknowledged with INIT,1
//   - LK:          heartbeat (optionally steps, tumbles, battery), acknowledged
//   - UD*, WT*:    position report (UD, UD2, UD3, UD_LTE, UD_WCDMA, WT, ...)
//   - AL*:         alarm with position (AL, AL_LTE, ALCUSTOMER1, ...), acknowledged
//   - TKQ, TKQ2:   acknowledged by echoing the type
package watch

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Message represents a decoded WATCH protocol frame.
type Message struct {
	Manufacturer string
	DeviceID     string
	// Index is the optional frame index of indexed frames ("" if absent).
	// Responses must echo it back.
	Index string
	// Type is the message type, e.g. LK, UD, UD_LTE, AL, TKQ.
	Type string
	// Content is the payload following "<type>,", or "" if there is none.
	Content string

	// Position holds the decoded position for UD*/AL*/WT* messages. It is
	// nil when the content could not be parsed; PositionErr then holds the
	// reason.
	Position    *Position
	PositionErr error

	// Attributes holds non-position data (e.g. LK battery and steps). It is
	// nil when the message carries no such data.
	Attributes map[string]any

	// Response is the acknowledgement frame to send back to the device, or
	// "" if the message type is not acknowledged.
	Response string
}

// HasPosition reports whether the message type carries a position report.
func (m *Message) HasPosition() bool {
	return strings.HasPrefix(m.Type, "UD") || strings.HasPrefix(m.Type, "AL") || strings.HasPrefix(m.Type, "WT")
}

// Position is a position report decoded from a UD*/AL*/WT* message.
type Position struct {
	Timestamp time.Time
	Valid     bool
	Latitude  float64
	Longitude float64
	Speed     float64 // km/h
	Course    float64
	Altitude  float64
	// Attributes holds satellites, rssi, batteryLevel, steps, status, alarm
	// and the first cell tower (mcc, mnc, lac, cellId).
	Attributes map[string]any
	// Network holds cellTowers and wifiAccessPoints in Traccar's JSON layout,
	// or nil when none were reported.
	Network map[string]any
}

// Alarm returns the position's alarm, or "" if none is set.
func (p *Position) Alarm() string {
	a, _ := p.Attributes["alarm"].(string)
	return a
}

// Decode parses a single WATCH frame, including its enclosing brackets.
// Escaped bytes are decoded before parsing. An error is returned only when
// the frame header is malformed; position content errors are reported via
// Message.PositionErr so the frame can still be acknowledged.
func Decode(raw string) (*Message, error) {
	raw = strings.TrimSpace(raw)

	if !strings.HasPrefix(raw, "[") || !strings.HasSuffix(raw, "]") {
		return nil, fmt.Errorf("invalid WATCH message: must be enclosed in brackets")
	}

	unescaped, err := Unescape([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid WATCH message: %w", err)
	}
	data := string(unescaped[1 : len(unescaped)-1])

	msg := &Message{}

	manufacturer, rest, ok := strings.Cut(data, "*")
	if !ok || manufacturer == "" {
		return nil, fmt.Errorf("invalid WATCH header: missing manufacturer")
	}
	msg.Manufacturer = manufacturer

	id, rest, ok := strings.Cut(rest, "*")
	if !ok || id == "" {
		return nil, fmt.Errorf("invalid WATCH header: missing device id")
	}
	msg.DeviceID = id

	// Optional index: present when the field after the next '*' is four hex
	// digits followed by another '*' (i.e. that field is the length).
	if star := strings.IndexByte(rest, '*'); star >= 0 && star+5 < len(rest) &&
		rest[star+5] == '*' && isHex(rest[star+1:star+5]) {
		msg.Index = rest[:star]
		rest = rest[star+1:]
	}

	// Four hex digit length followed by '*'.
	if len(rest) < 5 || rest[4] != '*' {
		return nil, fmt.Errorf("invalid WATCH header: missing length")
	}
	body := rest[5:]

	msg.Type, msg.Content, _ = strings.Cut(body, ",")
	if msg.Type == "" {
		return nil, fmt.Errorf("invalid WATCH message: missing type")
	}

	decodeByType(msg)
	return msg, nil
}

func decodeByType(msg *Message) {
	switch {
	case msg.Type == "INIT":
		msg.Response = msg.response("INIT,1")

	case msg.Type == "LK":
		msg.Response = msg.response("LK")
		decodeHeartbeat(msg)

	case msg.HasPosition():
		msg.Position, msg.PositionErr = decodePosition(msg.Content)
		if strings.HasPrefix(msg.Type, "AL") {
			if msg.Position != nil && msg.Position.Alarm() == "" {
				msg.Position.Attributes["alarm"] = "general"
			}
			msg.Response = msg.response("AL")
		}

	case msg.Type == "TKQ" || msg.Type == "TKQ2":
		msg.Response = msg.response(msg.Type)
	}
}

func (m *Message) response(content string) string {
	return EncodeResponse(m.Manufacturer, m.DeviceID, m.Index, content)
}

// decodeHeartbeat parses the optional LK content: steps,tumbles,battery.
// Like Traccar, the values are only used when at least three are present.
func decodeHeartbeat(msg *Message) {
	if msg.Content == "" {
		return
	}
	values := javaSplit(msg.Content)
	if len(values) < 3 {
		return
	}
	steps, err1 := strconv.Atoi(values[0])
	battery, err2 := strconv.Atoi(values[2])
	if err1 != nil || err2 != nil {
		return
	}
	msg.Attributes = map[string]any{
		"steps":        steps,
		"batteryLevel": battery,
	}
}

// positionPattern mirrors Traccar's WatchProtocolDecoder.PATTERN_POSITION.
var positionPattern = regexp.MustCompile(`^` +
	`(\d\d)(\d\d)(\d\d),` + // date (ddmmyy)
	`(\d\d)(\d\d)(\d\d),` + // time (hhmmss)
	`([AV]),` + // validity
	` *(-?\d+\.?\d*),` + // latitude
	`([NS])?,` +
	` *(-?\d+\.?\d*),` + // longitude
	`([EW])?,` +
	`(\d+\.?\d*),` + // speed (km/h)
	`(\d+\.?\d*),` + // course
	`(-?\d+\.?\d*),` + // altitude
	`(\d+),` + // satellites
	`(\d+),` + // rssi
	`(\d+),` + // battery
	`(\d+),` + // steps
	`\d+,` + // tumbles
	`([0-9a-fA-F]+),` + // status
	`(.*)$`) // cell and wifi

// decodePosition parses UD*/AL*/WT* content (everything after "<type>,").
func decodePosition(content string) (*Position, error) {
	m := positionPattern.FindStringSubmatch(content)
	if m == nil {
		return nil, fmt.Errorf("position content does not match expected format: %q", content)
	}

	day, _ := strconv.Atoi(m[1])
	month, _ := strconv.Atoi(m[2])
	year, _ := strconv.Atoi(m[3])
	hour, _ := strconv.Atoi(m[4])
	minute, _ := strconv.Atoi(m[5])
	second, _ := strconv.Atoi(m[6])

	pos := &Position{
		Timestamp: time.Date(2000+year, time.Month(month), day, hour, minute, second, 0, time.UTC),
		Valid:     m[7] == "A",
		Latitude:  coordinate(m[8], m[9]),
		Longitude: coordinate(m[10], m[11]),
	}
	pos.Speed, _ = strconv.ParseFloat(m[12], 64)
	pos.Course, _ = strconv.ParseFloat(m[13], 64)
	pos.Altitude, _ = strconv.ParseFloat(m[14], 64)

	satellites, _ := strconv.Atoi(m[15])
	rssi, _ := strconv.Atoi(m[16])
	battery, _ := strconv.Atoi(m[17])
	steps, _ := strconv.Atoi(m[18])
	status, err := strconv.ParseInt(m[19], 16, 64)
	if err != nil {
		return nil, fmt.Errorf("parse status %q: %w", m[19], err)
	}

	pos.Attributes = map[string]any{
		"satellites":   satellites,
		"rssi":         rssi,
		"batteryLevel": battery,
		"steps":        steps,
		"status":       status,
	}
	if alarm := decodeAlarm(status); alarm != "" {
		pos.Attributes["alarm"] = alarm
	}

	decodeNetwork(pos, m[20])

	return pos, nil
}

// coordinate parses a decimal-degree value with an optional hemisphere.
// S/W force the value negative; values may also carry their own sign.
func coordinate(value, hemisphere string) float64 {
	v, _ := strconv.ParseFloat(value, 64)
	if hemisphere == "S" || hemisphere == "W" {
		v = -math.Abs(v)
	}
	return v
}

// decodeAlarm maps status bits to an alarm type, in Traccar's priority order.
func decodeAlarm(status int64) string {
	bit := func(n uint) bool { return status&(1<<n) != 0 }
	switch {
	case bit(0):
		return "lowBattery"
	case bit(1):
		return "geofenceExit"
	case bit(2):
		return "geofenceEnter"
	case bit(14):
		return "powerCut"
	case bit(16):
		return "sos"
	case bit(17):
		return "lowBattery"
	case bit(18):
		return "geofenceExit"
	case bit(19):
		return "geofenceEnter"
	case bit(20):
		return "removing"
	case bit(21), bit(22):
		return "fallDown"
	}
	return ""
}

// decodeNetwork parses the cell tower and Wi-Fi section following the
// status field:
//
//	cellCount[,ta,mcc,mnc,(lac,cid,rssi)*cellCount][,wifiCount,(name,mac,rssi)*wifiCount][,...]
//
// Like Traccar, the section is skipped when its fourth value contains hex
// letters (an alternative layout). Malformed data is ignored rather than
// failing the whole position.
func decodeNetwork(pos *Position, data string) {
	values := javaSplit(data)
	if len(values) == 0 || (len(values) >= 4 && containsHexLetter(values[3])) {
		return
	}

	var cells, wifis []map[string]any
	ok := func() bool {
		index := 0
		next := func() (string, bool) {
			if index >= len(values) {
				return "", false
			}
			index++
			return values[index-1], true
		}

		cellCount, err := strconv.Atoi(values[0])
		if err != nil {
			return false
		}
		index++

		if cellCount > 0 {
			index++ // timing advance
			var mcc, mnc int
			if v, ok := next(); !ok {
				return false
			} else if v != "" {
				if mcc, err = strconv.Atoi(v); err != nil {
					return false
				}
			}
			if v, ok := next(); !ok {
				return false
			} else if v != "" {
				if mnc, err = strconv.Atoi(v); err != nil {
					return false
				}
			}
			for range cellCount {
				lacStr, ok1 := next()
				cidStr, ok2 := next()
				rssiStr, ok3 := next()
				if !ok1 || !ok2 || !ok3 {
					return false
				}
				lac, err1 := parseCellNumber(lacStr)
				cid, err2 := parseCellNumber(cidStr)
				if err1 != nil || err2 != nil {
					return false
				}
				cell := map[string]any{
					"mobileCountryCode": mcc,
					"mobileNetworkCode": mnc,
					"locationAreaCode":  lac,
					"cellId":            cid,
				}
				if rssiStr != "" {
					rssi, err := strconv.Atoi(rssiStr)
					if err != nil {
						return false
					}
					cell["signalStrength"] = rssi
				}
				cells = append(cells, cell)
			}
		}

		if index < len(values) && values[index] != "" {
			wifiCount, err := strconv.Atoi(values[index])
			if err != nil {
				return false
			}
			index++
			for range wifiCount {
				_, ok1 := next() // wifi name
				mac, ok2 := next()
				rssiStr, ok3 := next()
				if !ok1 || !ok2 || !ok3 {
					return false
				}
				if mac == "" || mac == "0" || rssiStr == "" {
					continue
				}
				rssi, err := strconv.Atoi(rssiStr)
				if err != nil {
					return false
				}
				wifis = append(wifis, map[string]any{
					"macAddress":     mac,
					"signalStrength": rssi,
				})
			}
		}
		return true
	}()

	if !ok || (len(cells) == 0 && len(wifis) == 0) {
		return
	}

	pos.Network = map[string]any{"radioType": "gsm", "considerIp": false}
	if len(cells) > 0 {
		pos.Network["cellTowers"] = cells
		pos.Attributes["mcc"] = cells[0]["mobileCountryCode"]
		pos.Attributes["mnc"] = cells[0]["mobileNetworkCode"]
		pos.Attributes["lac"] = cells[0]["locationAreaCode"]
		pos.Attributes["cellId"] = cells[0]["cellId"]
	}
	if len(wifis) > 0 {
		pos.Network["wifiAccessPoints"] = wifis
	}
}

// parseCellNumber parses a LAC/CID, which is hexadecimal when it contains
// hex letters and decimal otherwise.
func parseCellNumber(s string) (int, error) {
	base := 10
	if containsHexLetter(s) {
		base = 16
	}
	n, err := strconv.ParseInt(s, base, 64)
	return int(n), err
}

// javaSplit splits on commas and drops trailing empty strings, matching
// Java's String.split semantics that Traccar's field indexing relies on.
func javaSplit(s string) []string {
	values := strings.Split(s, ",")
	for len(values) > 0 && values[len(values)-1] == "" {
		values = values[:len(values)-1]
	}
	return values
}

func containsHexLetter(s string) bool {
	return strings.ContainsAny(s, "abcdefABCDEF")
}

func isHex(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// EncodeResponse creates a WATCH protocol frame for the given content,
// echoing the index when the device used indexed frames. Like Traccar, the
// length is four lowercase hex digits and no line terminator is appended.
func EncodeResponse(manufacturer, deviceID, index, content string) string {
	if index != "" {
		return fmt.Sprintf("[%s*%s*%s*%04x*%s]", manufacturer, deviceID, index, len(content), content)
	}
	return fmt.Sprintf("[%s*%s*%04x*%s]", manufacturer, deviceID, len(content), content)
}

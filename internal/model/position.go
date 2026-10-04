package model

import "time"

// MotionThreshold is the minimum speed in km/h to consider a device in motion.
const MotionThreshold = 5.0

// Position represents a GPS position report from a device.
type Position struct {
	ID          int64          `json:"id"`
	DeviceID    int64          `json:"deviceId"`
	Protocol    string         `json:"protocol,omitempty"`
	ServerTime  *time.Time     `json:"serverTime,omitempty"`
	DeviceTime  *time.Time     `json:"deviceTime,omitempty"`
	Timestamp   time.Time      `json:"fixTime"`
	Valid       bool           `json:"valid"`
	Latitude    float64        `json:"latitude"`
	Longitude   float64        `json:"longitude"`
	Altitude    *float64       `json:"altitude"`
	Speed       *float64       `json:"speed"`
	Course      *float64       `json:"course"`
	Address     *string        `json:"address"`
	Accuracy    float64        `json:"accuracy"`
	Network     map[string]any `json:"network"`
	GeofenceIDs []int64        `json:"geofenceIds"`
	Outdated    bool           `json:"outdated"`
	Attributes  map[string]any `json:"attributes"`
}

// SpeedOrZero returns the speed in km/h, or 0 when it is unknown.
func (p *Position) SpeedOrZero() float64 {
	if p.Speed == nil {
		return 0
	}
	return *p.Speed
}

const kmhPerKnot = 1.852

// KnotsToKmh converts a speed from knots to km/h.
func KnotsToKmh(knots float64) float64 { return knots * kmhPerKnot }

// KmhToKnots converts a speed from km/h to knots.
func KmhToKnots(kmh float64) float64 { return kmh / kmhPerKnot }

// InKnots returns a copy of p with Speed converted from km/h to knots, the
// unit of the Traccar API.
func (p *Position) InKnots() *Position {
	cp := *p
	if p.Speed != nil {
		cp.Speed = new(KmhToKnots(*p.Speed))
	}
	return &cp
}

// PositionPoint is the subset of a position map views need. Speed is in km/h.
type PositionPoint struct {
	Lat      float64
	Lon      float64
	Speed    float64
	FixTime  time.Time
	Course   *float64
	Altitude *float64
}

// CellNetwork builds a Traccar-style network map from cell towers and Wi-Fi
// access points and copies the first cell tower to the mcc/mnc/lac/cellId
// attributes. It returns nil when both lists are empty.
func CellNetwork(cells, wifis []map[string]any, attrs map[string]any) map[string]any {
	if len(cells) == 0 && len(wifis) == 0 {
		return nil
	}
	network := map[string]any{"radioType": "gsm", "considerIp": false}
	if len(cells) > 0 {
		network["cellTowers"] = cells
		attrs["mcc"] = cells[0]["mobileCountryCode"]
		attrs["mnc"] = cells[0]["mobileNetworkCode"]
		attrs["lac"] = cells[0]["locationAreaCode"]
		attrs["cellId"] = cells[0]["cellId"]
	}
	if len(wifis) > 0 {
		network["wifiAccessPoints"] = wifis
	}
	return network
}

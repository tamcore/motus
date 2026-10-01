package model

import "time"

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

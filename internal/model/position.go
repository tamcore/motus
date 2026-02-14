package model

import "time"

// Position represents a GPS position report from a device.
type Position struct {
	ID          int64                  `json:"id"`
	DeviceID    int64                  `json:"deviceId"`
	Protocol    string                 `json:"protocol,omitempty"`
	ServerTime  *time.Time             `json:"serverTime,omitempty"`
	DeviceTime  *time.Time             `json:"deviceTime,omitempty"`
	Timestamp   time.Time              `json:"fixTime"`
	Valid       bool                   `json:"valid"`
	Latitude    float64                `json:"latitude"`
	Longitude   float64                `json:"longitude"`
	Altitude    *float64               `json:"altitude"`
	Speed       *float64               `json:"speed"`
	Course      *float64               `json:"course"`
	Address     *string                `json:"address"`
	Accuracy    float64                `json:"accuracy"`
	Network     map[string]interface{} `json:"network"`
	GeofenceIDs []int64                `json:"geofenceIds"`
	Outdated    bool                   `json:"outdated"`
	Attributes  map[string]interface{} `json:"attributes"`
}

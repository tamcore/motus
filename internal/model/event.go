package model

import "time"

// Event represents a system event (geofence enter/exit, alarm, etc).
type Event struct {
	ID         int64                  `json:"id"`
	DeviceID   int64                  `json:"deviceId"`
	GeofenceID *int64                 `json:"geofenceId,omitempty"`
	Type       string                 `json:"type"`
	PositionID *int64                 `json:"positionId,omitempty"`
	Timestamp  time.Time              `json:"eventTime"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

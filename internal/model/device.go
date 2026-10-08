package model

import "time"

// Device represents a GPS tracking device.
type Device struct {
	ID               int64          `json:"id"`
	UniqueID         string         `json:"uniqueId"`
	Name             string         `json:"name"`
	Protocol         string         `json:"protocol,omitempty"`
	Status           string         `json:"status"`
	SpeedLimit       *float64       `json:"speedLimit,omitempty"`
	LastUpdate       *time.Time     `json:"lastUpdate,omitempty"`
	PositionID       *int64         `json:"positionId"`
	GroupID          *int64         `json:"groupId"`
	Phone            *string        `json:"phone"`
	Model            *string        `json:"model"`
	Contact          *string        `json:"contact"`
	Category         *string        `json:"category"`
	CalendarID       *int64         `json:"calendarId"`
	ExpirationTime   *time.Time     `json:"expirationTime"`
	Disabled         bool           `json:"disabled"`
	Mileage          *float64       `json:"mileage"`
	BatteryLevel     *float64       `json:"batteryLevel"`
	PendingMileage   float64        `json:"-"`
	IgnitionOn       bool           `json:"-"`
	LastIgnitionTime *time.Time     `json:"-"`
	Attributes       map[string]any `json:"attributes"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`

	// OwnerName is populated only in admin list-all responses.
	OwnerName string `json:"ownerName,omitempty"`

	// GeofenceIDs are the attached geofences (device_geofences), loaded by the
	// API handlers only. Non-empty limits geofence event evaluation to them.
	GeofenceIDs []int64 `json:"geofenceIds,omitempty"`
}

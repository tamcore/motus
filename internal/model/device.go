package model

import "time"

// Device represents a GPS tracking device.
type Device struct {
	ID             int64                  `json:"id"`
	UniqueID       string                 `json:"uniqueId"`
	Name           string                 `json:"name"`
	Protocol       string                 `json:"protocol,omitempty"`
	Status         string                 `json:"status"`
	SpeedLimit     *float64               `json:"speedLimit,omitempty"`
	LastUpdate     *time.Time             `json:"lastUpdate,omitempty"`
	PositionID     *int64                 `json:"positionId"`
	GroupID        *int64                 `json:"groupId"`
	Phone          *string                `json:"phone"`
	Model          *string                `json:"model"`
	Contact        *string                `json:"contact"`
	Category       *string                `json:"category"`
	CalendarID     *int64                 `json:"calendarId"`
	ExpirationTime *time.Time             `json:"expirationTime"`
	Disabled       bool                   `json:"disabled"`
	Mileage        *float64               `json:"mileage"`
	PendingMileage float64                `json:"-"`
	Attributes     map[string]interface{} `json:"attributes"`
	CreatedAt      time.Time              `json:"createdAt"`
	UpdatedAt      time.Time              `json:"updatedAt"`

	// OwnerName is populated only in admin list-all responses.
	OwnerName string `json:"ownerName,omitempty"`
}

// ApplyUniqueIDPrefix prepends prefix to the device's UniqueID.
// Used in API responses to avoid ID collisions when running alongside
// another Traccar-compatible server (e.g. in Home Assistant).
func ApplyUniqueIDPrefix(devices []*Device, prefix string) {
	if prefix == "" {
		return
	}
	for _, d := range devices {
		d.UniqueID = prefix + d.UniqueID
	}
}

package model

import "time"

// TrailBookmark is a named time range of a device's trail owned by a user
// (e.g. a hike), so the trail can be reopened on the map later.
type TrailBookmark struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"userId"`
	DeviceID    int64     `json:"deviceId"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	From        time.Time `json:"from"`
	To          time.Time `json:"to"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`

	// DeviceName is populated on reads (joined from devices).
	DeviceName string `json:"deviceName,omitempty"`
}

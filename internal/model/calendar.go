package model

import "time"

// Calendar represents a time-based schedule stored in iCalendar (RFC 5545) format.
// Calendars can be associated with geofences to restrict when they trigger events.
type Calendar struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"userId,omitempty"`
	Name      string    `json:"name"`
	Data      string    `json:"data"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// OwnerName is populated only in admin list-all responses.
	OwnerName string `json:"ownerName,omitempty"`
}

package model

import "time"

// DeviceShare represents a shareable link for public device tracking.
type DeviceShare struct {
	ID        int64      `json:"id"`
	DeviceID  int64      `json:"deviceId"`
	Token     string     `json:"token"`
	CreatedBy int64      `json:"createdBy"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

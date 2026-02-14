package model

import "time"

// Geofence represents a geographic boundary with PostGIS geometry.
// The Geometry field is stored as GeoJSON when communicating with the API
// and converted to/from PostGIS geometry in the repository layer.
// The Area field is WKT format for Traccar compatibility.
type Geofence struct {
	ID          int64                  `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Area        string                 `json:"area"`
	Geometry    string                 `json:"geometry,omitempty"`
	CalendarID  *int64                 `json:"calendarId,omitempty"`
	Attributes  map[string]interface{} `json:"attributes,omitempty"`
	CreatedAt   time.Time              `json:"createdAt"`
	UpdatedAt   time.Time              `json:"updatedAt"`

	// OwnerName is populated only in admin list-all responses.
	OwnerName string `json:"ownerName,omitempty"`
}

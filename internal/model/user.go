package model

import "time"

// Valid user roles.
const (
	RoleAdmin    = "admin"
	RoleUser     = "user"
	RoleReadonly = "readonly"
)

// ValidRoles returns the set of supported user roles.
func ValidRoles() []string {
	return []string{RoleAdmin, RoleUser, RoleReadonly}
}

// IsValidRole reports whether role is a recognised user role.
func IsValidRole(role string) bool {
	for _, r := range ValidRoles() {
		if r == role {
			return true
		}
	}
	return false
}

// User represents a system user with optional API token.
type User struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Name         string    `json:"name"`
	Role         string    `json:"-"`
	Token        *string   `json:"-"`
	CreatedAt    time.Time `json:"createdAt"`

	// OIDCSubject is the "sub" claim from the OIDC provider.
	// Nil for users who have never authenticated via OIDC.
	OIDCSubject *string `json:"-"`
	// OIDCIssuer is the OIDC issuer URL.
	// Nil for users who have never authenticated via OIDC.
	OIDCIssuer *string `json:"-"`

	// Traccar-compatible fields (computed from Role).
	Administrator    bool                   `json:"administrator"`
	Readonly         bool                   `json:"readonly"`
	Disabled         bool                   `json:"disabled"`
	Map              *string                `json:"map,omitempty"`
	Latitude         *float64               `json:"latitude,omitempty"`
	Longitude        *float64               `json:"longitude,omitempty"`
	Zoom             *int                   `json:"zoom,omitempty"`
	CoordinateFormat *string                `json:"coordinateFormat,omitempty"`
	Attributes       map[string]interface{} `json:"attributes,omitempty"`
}

// IsAdmin reports whether the user has the admin role.
func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

// PopulateTraccarFields sets the Traccar-compatible boolean fields
// from the internal Role field.
func (u *User) PopulateTraccarFields() {
	u.Administrator = (u.Role == RoleAdmin)
	u.Readonly = (u.Role == RoleReadonly)
	u.Disabled = false
}

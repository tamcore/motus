package model

import (
	"slices"
	"time"
)

// Valid user roles.
const (
	RoleAdmin    = "admin"
	RoleUser     = "user"
	RoleReadonly = "readonly"
)

// IsValidRole reports whether role is a recognised user role.
func IsValidRole(role string) bool {
	return slices.Contains([]string{RoleAdmin, RoleUser, RoleReadonly}, role)
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
}

// IsAdmin reports whether the user has the admin role.
func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

// CanManage reports whether u may manage a resource owned by ownerID.
func (u *User) CanManage(ownerID int64) bool {
	return ownerID == u.ID || u.IsAdmin()
}

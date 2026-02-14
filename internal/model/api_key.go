package model

import "time"

// API key permission levels.
const (
	PermissionFull     = "full"
	PermissionReadonly = "readonly"
)

// ValidPermissions returns the set of supported API key permission levels.
func ValidPermissions() []string {
	return []string{PermissionFull, PermissionReadonly}
}

// IsValidPermission reports whether perm is a recognised permission level.
func IsValidPermission(perm string) bool {
	for _, p := range ValidPermissions() {
		if p == perm {
			return true
		}
	}
	return false
}

// ApiKey represents a named API key with permission level for a user.
type ApiKey struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"userId"`
	Token       string     `json:"token,omitempty"`
	Name        string     `json:"name"`
	Permissions string     `json:"permissions"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	LastUsedAt  *time.Time `json:"lastUsedAt,omitempty"`
}

// IsReadonly reports whether this key has read-only permissions.
func (k *ApiKey) IsReadonly() bool {
	return k.Permissions == PermissionReadonly
}

// IsExpired reports whether this key has passed its expiration time.
// Keys without an expiration (ExpiresAt == nil) never expire.
func (k *ApiKey) IsExpired() bool {
	if k.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*k.ExpiresAt)
}

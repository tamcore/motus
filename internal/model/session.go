package model

import "time"

// Session represents an authenticated user session.
type Session struct {
	ID             string    `json:"-"`
	UserID         int64     `json:"userId"`
	RememberMe     bool      `json:"rememberMe"`
	OriginalUserID *int64    `json:"originalUserId,omitempty"`
	IsSudo         bool      `json:"isSudo,omitempty"`
	ApiKeyID       *int64    `json:"apiKeyId,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	ExpiresAt      time.Time `json:"expiresAt"`

	// Read-only fields populated at query time (never stored directly).
	ApiKeyName        *string    `json:"apiKeyName,omitempty"`
	IsCurrent         bool       `json:"isCurrent,omitempty"`
	LastSeenAt        *time.Time `json:"lastSeenAt,omitempty"`
	LastSeenIP        *string    `json:"lastSeenIp,omitempty"`
	LastSeenUserAgent *string    `json:"lastSeenUserAgent,omitempty"`
}

// TruncatedID returns a shortened prefix of the session ID for display
// purposes. The full session ID is the session cookie value and must
// never be exposed in API responses.
func (s *Session) TruncatedID() string {
	if len(s.ID) > 12 {
		return s.ID[:12] + "…"
	}
	return s.ID
}

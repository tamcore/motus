package model

import (
	"encoding/json"
	"time"
)

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
	ApiKeyName *string `json:"apiKeyName,omitempty"`
	IsCurrent  bool    `json:"isCurrent,omitempty"`
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

// sessionJSON is the JSON-safe representation of a Session. It replaces
// the full session token with a truncated display ID.
type sessionJSON struct {
	ID             string    `json:"id"`
	UserID         int64     `json:"userId"`
	RememberMe     bool      `json:"rememberMe"`
	OriginalUserID *int64    `json:"originalUserId,omitempty"`
	IsSudo         bool      `json:"isSudo,omitempty"`
	ApiKeyID       *int64    `json:"apiKeyId,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	ExpiresAt      time.Time `json:"expiresAt"`
	ApiKeyName     *string   `json:"apiKeyName,omitempty"`
	IsCurrent      bool      `json:"isCurrent,omitempty"`
}

// MarshalJSON serialises the session with a truncated display ID instead
// of the full session cookie token.
func (s Session) MarshalJSON() ([]byte, error) {
	return json.Marshal(sessionJSON{
		ID:             s.TruncatedID(),
		UserID:         s.UserID,
		RememberMe:     s.RememberMe,
		OriginalUserID: s.OriginalUserID,
		IsSudo:         s.IsSudo,
		ApiKeyID:       s.ApiKeyID,
		CreatedAt:      s.CreatedAt,
		ExpiresAt:      s.ExpiresAt,
		ApiKeyName:     s.ApiKeyName,
		IsCurrent:      s.IsCurrent,
	})
}

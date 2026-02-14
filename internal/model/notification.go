package model

import "time"

// NotificationRule defines when and how to send a notification.
type NotificationRule struct {
	ID         int64                  `json:"id"`
	UserID     int64                  `json:"userId"`
	Name       string                 `json:"name"`
	EventTypes []string               `json:"eventTypes"`
	Channel    string                 `json:"channel"`
	Config     map[string]interface{} `json:"config"`
	Template   string                 `json:"template"`
	Enabled    bool                   `json:"enabled"`
	CreatedAt  time.Time              `json:"createdAt"`
	UpdatedAt  time.Time              `json:"updatedAt"`

	// OwnerName is populated only in admin list-all responses.
	OwnerName string `json:"ownerName,omitempty"`
}

// NotificationLog records the delivery status of a notification.
type NotificationLog struct {
	ID           int64      `json:"id"`
	RuleID       int64      `json:"ruleId"`
	EventID      *int64     `json:"eventId,omitempty"`
	Status       string     `json:"status"`
	SentAt       *time.Time `json:"sentAt,omitempty"`
	Error        string     `json:"error,omitempty"`
	ResponseCode int        `json:"responseCode,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
}

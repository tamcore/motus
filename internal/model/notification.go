package model

import (
	"fmt"
	"maps"
	"slices"
	"time"
)

// Notification delivery channels.
const (
	// NotificationChannelWebhook POSTs the rendered template to a URL.
	NotificationChannelWebhook = "webhook"
	// NotificationChannelCommand sends a device command to the device that
	// triggered the event.
	NotificationChannelCommand = "command"
)

// Geofence event types. A rule's geofence filter only restricts these.
const (
	EventTypeGeofenceEnter = "geofenceEnter"
	EventTypeGeofenceExit  = "geofenceExit"
)

// Connection event types.
const (
	EventTypeDeviceOnline  = "deviceOnline"
	EventTypeDeviceOffline = "deviceOffline"
)

// notificationEventTypes lists the event types notification rules support.
var notificationEventTypes = map[string]bool{
	EventTypeGeofenceEnter: true,
	EventTypeGeofenceExit:  true,
	EventTypeDeviceOnline:  true,
	EventTypeDeviceOffline: true,
	"motion":               true,
	"deviceIdle":           true,
	"ignitionOn":           true,
	"ignitionOff":          true,
	"alarm":                true,
	"tripCompleted":        true,
}

// IsNotificationEventType reports whether notification rules support eventType.
func IsNotificationEventType(eventType string) bool {
	return notificationEventTypes[eventType]
}

// IsGeofenceEventType reports whether eventType is a geofence transition.
func IsGeofenceEventType(eventType string) bool {
	return eventType == EventTypeGeofenceEnter || eventType == EventTypeGeofenceExit
}

// ValidateCommandEventTypes rejects command rules that would re-trigger
// themselves: a reboot (or a custom command, which may reboot the device)
// sent on deviceOnline/deviceOffline makes the device reconnect, which fires
// the rule again, without end.
func ValidateCommandEventTypes(cmdType string, eventTypes []string) error {
	if cmdType != CommandRebootDevice && cmdType != CommandCustom {
		return nil
	}
	for _, et := range eventTypes {
		if et == EventTypeDeviceOnline || et == EventTypeDeviceOffline {
			return fmt.Errorf("%s commands cannot be triggered by %s events: the device would reconnect and trigger the rule again without end", cmdType, et)
		}
	}
	return nil
}

// NotificationCommandTypes returns the command types a notification rule may
// send automatically. factoryReset is excluded: wiping a device as a side
// effect of a GPS event is never what the user wants.
func NotificationCommandTypes() []string {
	return slices.DeleteFunc(SupportedCommandTypes(), func(t string) bool {
		return t == CommandFactoryReset
	})
}

// NotificationRule defines when and how to send a notification.
type NotificationRule struct {
	ID         int64          `json:"id"`
	UserID     int64          `json:"userId"`
	Name       string         `json:"name"`
	EventTypes []string       `json:"eventTypes"`
	Channel    string         `json:"channel"`
	Config     map[string]any `json:"config"`
	Template   string         `json:"template"`
	Enabled    bool           `json:"enabled"`
	// GeofenceIDs restricts geofenceEnter/geofenceExit events to these
	// geofences. Empty means the rule applies to all geofences.
	GeofenceIDs []int64   `json:"geofenceIds"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`

	// OwnerName is populated only in admin list-all responses.
	OwnerName string `json:"ownerName,omitempty"`
}

// MatchesEvent reports whether the rule subscribes to the event's type and,
// for geofence events, whether the event's geofence passes the rule's
// geofence filter. It does not check Enabled.
func (r *NotificationRule) MatchesEvent(e *Event) bool {
	if !slices.Contains(r.EventTypes, e.Type) {
		return false
	}
	if !IsGeofenceEventType(e.Type) || len(r.GeofenceIDs) == 0 {
		return true
	}
	return e.GeofenceID != nil && slices.Contains(r.GeofenceIDs, *e.GeofenceID)
}

// DeviceCommand builds the pending command a command-channel rule sends to
// deviceID. The rule config holds "commandType" and optional "attributes".
func (r *NotificationRule) DeviceCommand(deviceID int64) (*Command, error) {
	if r.Channel != NotificationChannelCommand {
		return nil, fmt.Errorf("rule channel %q does not send commands", r.Channel)
	}
	cmdType, _ := r.Config["commandType"].(string)
	if cmdType == "" {
		return nil, fmt.Errorf("commandType not configured")
	}
	if !slices.Contains(NotificationCommandTypes(), cmdType) {
		return nil, fmt.Errorf("unsupported command type: %s", cmdType)
	}
	if err := ValidateCommandEventTypes(cmdType, r.EventTypes); err != nil {
		return nil, err
	}
	cmd := &Command{DeviceID: deviceID, Type: cmdType, Status: CommandStatusPending}
	if attrs, ok := r.Config["attributes"].(map[string]any); ok && len(attrs) > 0 {
		cmd.Attributes = maps.Clone(attrs)
	}
	return cmd, nil
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

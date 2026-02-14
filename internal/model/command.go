package model

import "time"

// Supported command types for device control.
const (
	CommandRebootDevice     = "rebootDevice"
	CommandPositionPeriodic = "positionPeriodic"
	CommandPositionSingle   = "positionSingle"
	CommandSosNumber        = "sosNumber"
	CommandCustom           = "custom"
	CommandSetSpeedAlarm    = "setSpeedAlarm"
	CommandFactoryReset     = "factoryReset"
)

// CommandStatusPending is the default status for new commands.
const CommandStatusPending = "pending"

// CommandStatusSent indicates the command has been delivered to the device connection.
const CommandStatusSent = "sent"

// Command represents a control command to be sent to a device.
type Command struct {
	ID         int64                  `json:"id"`
	DeviceID   int64                  `json:"deviceId"`
	Type       string                 `json:"type"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
	Status     string                 `json:"status"`
	Result     *string                `json:"result,omitempty"`
	CreatedAt  time.Time              `json:"createdAt"`
	ExecutedAt *time.Time             `json:"executedAt,omitempty"`
}

// SupportedCommandTypes returns the list of all supported command types.
func SupportedCommandTypes() []string {
	return []string{
		CommandRebootDevice,
		CommandPositionPeriodic,
		CommandPositionSingle,
		CommandSosNumber,
		CommandCustom,
		CommandSetSpeedAlarm,
		CommandFactoryReset,
	}
}

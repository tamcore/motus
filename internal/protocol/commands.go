package protocol

import (
	"fmt"

	"github.com/tamcore/motus/internal/model"
)

// CommandEncoder encodes commands into protocol-specific byte sequences.
// deviceID is the device's unique identifier (IMEI) required by some protocols.
type CommandEncoder interface {
	EncodeCommand(cmd *model.Command, deviceID string) ([]byte, error)
	Protocol() string
}

// H02CommandEncoder encodes commands for the H02 GPS protocol.
type H02CommandEncoder struct{}

// Protocol returns the protocol name.
func (e *H02CommandEncoder) Protocol() string { return "h02" }

// EncodeCommand converts a command to H02 protocol format.
// deviceID is the device IMEI, required for *HQ,...# framed commands.
func (e *H02CommandEncoder) EncodeCommand(cmd *model.Command, deviceID string) ([]byte, error) {
	switch cmd.Type {
	case model.CommandRebootDevice:
		return []byte(fmt.Sprintf("*HQ,%s,reset#", deviceID)), nil
	case model.CommandPositionSingle:
		return []byte(fmt.Sprintf("*HQ,%s,locate#", deviceID)), nil
	case model.CommandPositionPeriodic:
		interval, ok := cmd.Attributes["frequency"]
		if !ok {
			return nil, fmt.Errorf("frequency attribute required for positionPeriodic")
		}
		return []byte(fmt.Sprintf("*HQ,%s,time,%v#", deviceID, interval)), nil
	case model.CommandSosNumber:
		phone, ok := cmd.Attributes["phoneNumber"]
		if !ok {
			return nil, fmt.Errorf("phoneNumber attribute required for sosNumber")
		}
		return []byte(fmt.Sprintf("setphone,1,%v", phone)), nil
	case model.CommandSetSpeedAlarm:
		speed, ok := cmd.Attributes["speed"]
		if !ok {
			return nil, fmt.Errorf("speed attribute required for setSpeedAlarm")
		}
		return []byte(fmt.Sprintf("setspeed,%v", speed)), nil
	case model.CommandFactoryReset:
		return []byte("FACTORY"), nil
	default:
		return nil, fmt.Errorf("unsupported command type for H02: %s", cmd.Type)
	}
}

// EncoderRegistry maps protocol names to their command encoders.
type EncoderRegistry struct {
	encoders map[string]CommandEncoder
}

// NewEncoderRegistry creates a registry with default encoders.
func NewEncoderRegistry() *EncoderRegistry {
	r := &EncoderRegistry{encoders: make(map[string]CommandEncoder)}
	r.Register(&H02CommandEncoder{})
	return r
}

// Register adds an encoder for its protocol.
func (r *EncoderRegistry) Register(enc CommandEncoder) {
	r.encoders[enc.Protocol()] = enc
}

// Get returns the encoder for the given protocol, or nil if not found.
func (r *EncoderRegistry) Get(protocol string) CommandEncoder {
	return r.encoders[protocol]
}

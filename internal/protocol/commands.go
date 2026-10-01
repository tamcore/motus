package protocol

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/protocol/watch"
)

// CommandEncoder encodes commands into protocol-specific byte sequences.
// deviceID is the device's unique identifier (IMEI) required by some protocols.
type CommandEncoder interface {
	EncodeCommand(cmd *model.Command, deviceID string) ([]byte, error)
	Protocol() string
	// SupportedCommands lists the command types EncodeCommand accepts, in
	// model.SupportedCommandTypes order.
	SupportedCommands() []string
}

// H02CommandEncoder encodes commands for the H02 GPS protocol.
type H02CommandEncoder struct{}

// Protocol returns the protocol name.
func (e *H02CommandEncoder) Protocol() string { return "h02" }

// SupportedCommands lists the command types the H02 encoder accepts.
func (e *H02CommandEncoder) SupportedCommands() []string {
	return []string{
		model.CommandRebootDevice,
		model.CommandPositionPeriodic,
		model.CommandPositionSingle,
		model.CommandSosNumber,
		model.CommandCustom,
		model.CommandSetSpeedAlarm,
		model.CommandFactoryReset,
	}
}

// EncodeCommand converts a command to H02 protocol format.
// deviceID is the device IMEI, required for *HQ,...# framed commands.
func (e *H02CommandEncoder) EncodeCommand(cmd *model.Command, deviceID string) ([]byte, error) {
	switch cmd.Type {
	case model.CommandRebootDevice:
		return fmt.Appendf(nil, "*HQ,%s,reset#", deviceID), nil
	case model.CommandPositionSingle:
		return fmt.Appendf(nil, "*HQ,%s,locate#", deviceID), nil
	case model.CommandPositionPeriodic:
		interval, ok := cmd.Attributes["frequency"]
		if !ok {
			return nil, fmt.Errorf("frequency attribute required for positionPeriodic")
		}
		return fmt.Appendf(nil, "*HQ,%s,time,%v#", deviceID, interval), nil
	case model.CommandSosNumber:
		phone, ok := cmd.Attributes["phoneNumber"]
		if !ok {
			return nil, fmt.Errorf("phoneNumber attribute required for sosNumber")
		}
		return fmt.Appendf(nil, "setphone,1,%v", phone), nil
	case model.CommandSetSpeedAlarm:
		speed, ok := cmd.Attributes["speed"]
		if !ok {
			return nil, fmt.Errorf("speed attribute required for setSpeedAlarm")
		}
		return fmt.Appendf(nil, "setspeed,%v", speed), nil
	case model.CommandFactoryReset:
		return []byte("FACTORY"), nil
	case model.CommandCustom:
		// Custom commands are sent verbatim.
		text, _ := cmd.Attributes["text"].(string)
		return []byte(text), nil
	default:
		return nil, fmt.Errorf("unsupported command type for H02: %s", cmd.Type)
	}
}

// WatchCommandEncoder encodes commands for the WATCH GPS protocol, mirroring
// Traccar's WatchProtocolEncoder.
type WatchCommandEncoder struct {
	sessions *DeviceRegistry
}

// NewWatchCommandEncoder creates a WATCH encoder. sessions supplies the
// manufacturer code and frame indexing of each device's live connection; when
// nil or unknown, frames use the "CS" manufacturer without an index.
func NewWatchCommandEncoder(sessions *DeviceRegistry) *WatchCommandEncoder {
	return &WatchCommandEncoder{sessions: sessions}
}

// Protocol returns the protocol name.
func (e *WatchCommandEncoder) Protocol() string { return "watch" }

// SupportedCommands lists the command types the WATCH encoder accepts.
func (e *WatchCommandEncoder) SupportedCommands() []string {
	return []string{
		model.CommandRebootDevice,
		model.CommandPositionPeriodic,
		model.CommandPositionSingle,
		model.CommandSosNumber,
		model.CommandCustom,
	}
}

// watchCommandContent returns the frame content of a WATCH command, e.g.
// "UPLOAD,60" for positionPeriodic.
func watchCommandContent(cmd *model.Command) (string, error) {
	switch cmd.Type {
	case model.CommandCustom:
		text, _ := cmd.Attributes["text"].(string)
		if text == "" {
			return "", fmt.Errorf("text attribute required for custom")
		}
		return text, nil
	case model.CommandPositionSingle:
		return "CR", nil
	case model.CommandPositionPeriodic:
		frequency, ok := cmd.Attributes["frequency"]
		if !ok {
			return "", fmt.Errorf("frequency attribute required for positionPeriodic")
		}
		return fmt.Sprintf("UPLOAD,%v", frequency), nil
	case model.CommandSosNumber:
		phone, ok := cmd.Attributes["phoneNumber"]
		if !ok {
			return "", fmt.Errorf("phoneNumber attribute required for sosNumber")
		}
		index, ok := cmd.Attributes["index"]
		if !ok {
			index = 1
		}
		return fmt.Sprintf("SOS%v,%v", index, phone), nil
	case model.CommandRebootDevice:
		return "RESET", nil
	default:
		return "", fmt.Errorf("unsupported command type for WATCH: %s", cmd.Type)
	}
}

// watchCommandKeyword returns the keyword a WATCH device echoes when it
// replies to cmd (the content up to the first comma, e.g. "UPLOAD"), or ""
// when cmd cannot be encoded for WATCH.
func watchCommandKeyword(cmd *model.Command) string {
	content, err := watchCommandContent(cmd)
	if err != nil {
		return ""
	}
	keyword, _, _ := strings.Cut(content, ",")
	return keyword
}

// EncodeCommand converts a command to a WATCH protocol frame.
func (e *WatchCommandEncoder) EncodeCommand(cmd *model.Command, deviceID string) ([]byte, error) {
	content, err := watchCommandContent(cmd)
	if err != nil {
		return nil, err
	}

	manufacturer, index := "CS", ""
	if e.sessions != nil {
		if s, ok := e.sessions.Session(deviceID); ok && s.Manufacturer != "" {
			manufacturer = s.Manufacturer
			if manufacturer == "3G" {
				manufacturer = "SG"
			}
			if s.Indexed {
				index = "0001"
			}
		}
	}
	return []byte(watch.EncodeResponse(manufacturer, deviceID, index, content)), nil
}

// EncoderRegistry maps protocol names to their command encoders.
type EncoderRegistry struct {
	encoders map[string]CommandEncoder
}

// NewEncoderRegistry creates a registry with the H02 and WATCH encoders.
// sessions is passed to the WATCH encoder and may be nil.
func NewEncoderRegistry(sessions *DeviceRegistry) *EncoderRegistry {
	r := &EncoderRegistry{encoders: make(map[string]CommandEncoder)}
	for _, enc := range []CommandEncoder{&H02CommandEncoder{}, NewWatchCommandEncoder(sessions)} {
		r.encoders[enc.Protocol()] = enc
	}
	return r
}

// Get returns the encoder for the given protocol, or nil if not found.
func (r *EncoderRegistry) Get(protocol string) CommandEncoder {
	return r.encoders[protocol]
}

// SupportedCommands lists the command types a device speaking protocol can
// receive. A device whose protocol is not known yet ("") may receive any
// command; a protocol without an encoder (e.g. osmand) receives none. Safe to
// call on a nil registry, which allows every command.
func (r *EncoderRegistry) SupportedCommands(protocol string) []string {
	if r == nil || protocol == "" {
		return model.SupportedCommandTypes()
	}
	enc := r.Get(protocol)
	if enc == nil {
		return []string{}
	}
	return enc.SupportedCommands()
}

// ErrNoEncoder is returned by EncoderRegistry.Encode when the device protocol
// has no command encoder.
var ErrNoEncoder = errors.New("no command encoder for protocol")

// Encode returns the wire bytes for cmd sent to a device speaking protocol.
// Custom commands are passed through the protocol's encoder (so WATCH frames
// them) and sent verbatim when the protocol has no encoder. Safe to call on a
// nil registry.
func (r *EncoderRegistry) Encode(protocol string, cmd *model.Command, uniqueID string) ([]byte, error) {
	var enc CommandEncoder
	if r != nil {
		enc = r.Get(protocol)
	}
	if enc == nil {
		if cmd.Type == model.CommandCustom {
			text, _ := cmd.Attributes["text"].(string)
			return []byte(text), nil
		}
		return nil, fmt.Errorf("%w: %s", ErrNoEncoder, protocol)
	}
	return enc.EncodeCommand(cmd, uniqueID)
}

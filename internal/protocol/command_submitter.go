package protocol

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// Errors returned by CommandSubmitter.Submit (besides ErrNoEncoder).
var (
	// ErrCommandUnsupported means the device protocol cannot receive the
	// command type.
	ErrCommandUnsupported = errors.New("command type not supported by device protocol")
	// ErrCommandEncode wraps a protocol encoder error (e.g. a missing attribute).
	ErrCommandEncode = errors.New("encode command")
)

// CommandSubmitter is the single path for sending a command to a device: it
// validates and encodes the command, persists it as pending, and delivers it
// immediately when the device is connected to this process. Commands for
// offline devices (or devices connected to another replica) stay pending and
// are delivered by the CommandDispatcher once the device is online.
//
// Callers are responsible for authorization (device access, readonly keys).
type CommandSubmitter struct {
	Commands repository.CommandRepo
	// Encoders may be nil: every command type is then accepted and only
	// queued (custom commands are still encoded verbatim).
	Encoders *EncoderRegistry
	// Registry may be nil, in which case commands are only queued.
	Registry *DeviceRegistry
}

// Submit queues cmdType with attrs for device and attempts immediate delivery.
// The returned command's Status is "sent" when it was written to a live
// connection and "pending" otherwise.
func (s *CommandSubmitter) Submit(ctx context.Context, device *model.Device, cmdType string, attrs map[string]any) (*model.Command, error) {
	if !slices.Contains(s.Encoders.SupportedCommands(device.Protocol), cmdType) {
		return nil, fmt.Errorf("%w: command type %s is not supported by device protocol %s",
			ErrCommandUnsupported, cmdType, device.Protocol)
	}

	// Custom commands are framed by the protocol encoder when there is one
	// (WATCH) and sent verbatim otherwise.
	var payload []byte
	if cmdType == model.CommandCustom || s.Encoders != nil {
		var err error
		payload, err = s.Encoders.Encode(device.Protocol, &model.Command{Type: cmdType, Attributes: attrs}, device.UniqueID)
		if errors.Is(err, ErrNoEncoder) {
			return nil, err
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrCommandEncode, err)
		}
	}

	// Save command as pending before dispatching (device can respond within ms).
	cmd := &model.Command{
		DeviceID:   device.ID,
		Type:       cmdType,
		Attributes: attrs,
		Status:     model.CommandStatusPending,
	}
	if err := s.Commands.Create(ctx, cmd); err != nil {
		return nil, fmt.Errorf("store command: %w", err)
	}

	// Attempt immediate delivery if the device is connected here. The status
	// is marked "sent" before the write so a fast device reply finds it, and
	// reverted to "pending" if the write fails so the dispatcher retries.
	online := s.Registry != nil && s.Registry.IsOnline(device.UniqueID)
	if online && payload != nil {
		if updErr := s.Commands.UpdateStatus(ctx, cmd.ID, model.CommandStatusSent); updErr == nil {
			if s.Registry.Send(device.UniqueID, payload) {
				cmd.Status = model.CommandStatusSent
			} else {
				_ = s.Commands.UpdateStatus(ctx, cmd.ID, model.CommandStatusPending)
			}
		}
	}
	return cmd, nil
}

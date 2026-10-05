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
	Encoders *EncoderRegistry
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

	payload, err := s.Encoders.Encode(device.Protocol, &model.Command{Type: cmdType, Attributes: attrs}, device.UniqueID)
	if errors.Is(err, ErrNoEncoder) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCommandEncode, err)
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

	// Attempt immediate delivery if the device is connected here.
	if s.Registry.IsOnline(device.UniqueID) {
		if sent, _ := deliver(ctx, s.Commands, s.Registry, device.UniqueID, cmd.ID, payload); sent {
			cmd.Status = model.CommandStatusSent
		}
	}
	return cmd, nil
}

// deliver marks a command "sent" and writes payload to the device. The status
// is set before the write so a fast device reply (SMS) finds the command; if
// the write fails, it is reverted to "pending" so the dispatcher retries it.
func deliver(ctx context.Context, cmds repository.CommandRepo, reg *DeviceRegistry, uniqueID string, cmdID int64, payload []byte) (bool, error) {
	if err := cmds.UpdateStatus(ctx, cmdID, model.CommandStatusSent); err != nil {
		return false, fmt.Errorf("mark command sent: %w", err)
	}
	if reg.Send(uniqueID, payload) {
		return true, nil
	}
	if err := cmds.UpdateStatus(ctx, cmdID, model.CommandStatusPending); err != nil {
		return false, fmt.Errorf("revert command to pending: %w", err)
	}
	return false, nil
}

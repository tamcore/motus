package protocol

import (
	"context"
	"log/slog"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

const dispatchInterval = 1 * time.Second

// CommandDispatcher periodically dispatches pending commands to locally connected
// devices. This solves the multi-replica problem: when the HTTP request that saves
// a command lands on pod A but the device's TCP connection lives on pod B, pod B's
// dispatcher picks up the "pending" command on its next tick and delivers it.
type CommandDispatcher struct {
	registry *DeviceRegistry
	cmdRepo  repository.CommandRepo
	devRepo  repository.DeviceRepo
	encoders *EncoderRegistry
	interval time.Duration
	logger   *slog.Logger
}

// NewCommandDispatcher creates a dispatcher that polls at 1-second intervals.
func NewCommandDispatcher(
	registry *DeviceRegistry,
	cmdRepo repository.CommandRepo,
	devRepo repository.DeviceRepo,
	encoders *EncoderRegistry,
) *CommandDispatcher {
	return &CommandDispatcher{
		registry: registry,
		cmdRepo:  cmdRepo,
		devRepo:  devRepo,
		encoders: encoders,
		interval: dispatchInterval,
		logger:   slog.Default(),
	}
}

// SetLogger overrides the default logger.
func (d *CommandDispatcher) SetLogger(l *slog.Logger) { d.logger = l }

// Start runs the dispatch loop until ctx is cancelled.
func (d *CommandDispatcher) Start(ctx context.Context) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.dispatch(ctx)
		}
	}
}

// dispatch iterates over all locally online devices and delivers their pending commands.
func (d *CommandDispatcher) dispatch(ctx context.Context) {
	for _, uniqueID := range d.registry.OnlineDeviceIDs() {
		d.dispatchForDevice(ctx, uniqueID)
	}
}

// dispatchForDevice fetches and sends all pending commands for one device.
func (d *CommandDispatcher) dispatchForDevice(ctx context.Context, uniqueID string) {
	dev, err := d.devRepo.GetByUniqueID(ctx, uniqueID)
	if err != nil || dev == nil {
		return
	}

	cmds, err := d.cmdRepo.GetPendingByDevice(ctx, dev.ID)
	if err != nil {
		d.logger.Warn("dispatcher: failed to fetch pending commands",
			slog.String("device", uniqueID),
			slog.Any("error", err),
		)
		return
	}

	for _, cmd := range cmds {
		d.sendCommand(ctx, dev, uniqueID, cmd)
	}
}

// sendCommand encodes and delivers a single command, then marks it "sent".
//
// The DB status is updated to "sent" BEFORE the payload is written to the TCP
// connection. This prevents a race where the device responds (SMS) before the
// status update completes, causing GetLatestSentByDevice to miss the command.
// If registry.Send fails after the DB update, the status is reverted to
// "pending" so the next dispatcher tick retries it.
func (d *CommandDispatcher) sendCommand(ctx context.Context, dev *model.Device, uniqueID string, cmd *model.Command) {
	payload, err := d.encodePayload(dev, uniqueID, cmd)
	if err != nil {
		d.logger.Warn("dispatcher: encode error",
			slog.String("device", uniqueID),
			slog.Int64("commandId", cmd.ID),
			slog.Any("error", err),
		)
		return
	}
	if len(payload) == 0 {
		return
	}

	// Mark "sent" in DB first so that the SMS response (which can arrive
	// within milliseconds of the TCP write) finds the command.
	if err := d.cmdRepo.UpdateStatus(ctx, cmd.ID, model.CommandStatusSent); err != nil {
		d.logger.Warn("dispatcher: failed to update command status to sent",
			slog.Int64("commandId", cmd.ID),
			slog.Any("error", err),
		)
		return
	}

	if !d.registry.Send(uniqueID, payload) {
		// Device disconnected or channel full — revert to pending so the next
		// tick picks it up again.
		if revertErr := d.cmdRepo.UpdateStatus(ctx, cmd.ID, model.CommandStatusPending); revertErr != nil {
			d.logger.Warn("dispatcher: failed to revert command status to pending",
				slog.Int64("commandId", cmd.ID),
				slog.Any("error", revertErr),
			)
		}
		return
	}

	d.logger.Debug("dispatcher: sent command",
		slog.String("device", uniqueID),
		slog.Int64("commandId", cmd.ID),
		slog.String("type", cmd.Type),
	)
}

// encodePayload returns the wire bytes for cmd, handling custom (raw text) commands
// and protocol-encoded commands via the EncoderRegistry.
func (d *CommandDispatcher) encodePayload(dev *model.Device, uniqueID string, cmd *model.Command) ([]byte, error) {
	if cmd.Type == model.CommandCustom {
		text, _ := cmd.Attributes["text"].(string)
		return []byte(text), nil
	}
	enc := d.encoders.Get(dev.Protocol)
	if enc == nil {
		return nil, nil // no encoder — skip silently
	}
	return enc.EncodeCommand(cmd, uniqueID)
}

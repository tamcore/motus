package protocol

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/ticker"
)

const dispatchInterval = 1 * time.Second

// CommandDispatcher periodically dispatches pending commands to locally connected
// devices. This solves the multi-replica problem: when the HTTP request that saves
// a command lands on pod A but the device's TCP connection lives on pod B, pod B's
// dispatcher picks up the "pending" command on its next tick and delivers it.
type CommandDispatcher struct {
	registry *DeviceRegistry
	cmdRepo  repository.CommandRepo
	encoders *EncoderRegistry
	logger   *slog.Logger
}

// NewCommandDispatcher creates a dispatcher that polls at 1-second intervals.
func NewCommandDispatcher(
	registry *DeviceRegistry,
	cmdRepo repository.CommandRepo,
	encoders *EncoderRegistry,
) *CommandDispatcher {
	return &CommandDispatcher{
		registry: registry,
		cmdRepo:  cmdRepo,
		encoders: encoders,
		logger:   slog.Default(),
	}
}

// SetLogger overrides the default logger.
func (d *CommandDispatcher) SetLogger(l *slog.Logger) { d.logger = l }

// Start runs the dispatch loop until ctx is cancelled.
func (d *CommandDispatcher) Start(ctx context.Context) {
	ticker.Every(ctx, dispatchInterval, func() { d.dispatch(ctx) })
}

// dispatch delivers the pending commands of all locally online devices,
// fetched in one query.
func (d *CommandDispatcher) dispatch(ctx context.Context) {
	online := d.registry.OnlineDeviceIDs()
	if len(online) == 0 {
		return
	}
	pending, err := d.cmdRepo.GetPendingByUniqueIDs(ctx, online)
	if err != nil {
		d.logger.Warn("dispatcher: failed to fetch pending commands",
			slog.Int("devices", len(online)),
			slog.Any("error", err),
		)
		return
	}
	for _, pc := range pending {
		d.sendCommand(ctx, pc.Protocol, pc.UniqueID, pc.Command)
	}
}

// sendCommand encodes and delivers a single command.
func (d *CommandDispatcher) sendCommand(ctx context.Context, protocol, uniqueID string, cmd *model.Command) {
	// Protocols without an encoder skip non-custom commands silently.
	payload, err := d.encoders.Encode(protocol, cmd, uniqueID)
	if errors.Is(err, ErrNoEncoder) {
		return
	}
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

	sent, err := deliver(ctx, d.cmdRepo, d.registry, uniqueID, cmd.ID, payload)
	if err != nil {
		d.logger.Warn("dispatcher: command status update failed",
			slog.Int64("commandId", cmd.ID),
			slog.Any("error", err),
		)
	}
	if !sent {
		return
	}

	d.logger.Debug("dispatcher: sent command",
		slog.String("device", uniqueID),
		slog.Int64("commandId", cmd.ID),
		slog.String("type", cmd.Type),
	)
}

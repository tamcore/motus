package services

import (
	"cmp"
	"context"
	"log/slog"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/websocket"
)

type eventStore interface {
	Create(ctx context.Context, e *model.Event) error
	GetRecentByDeviceAndType(ctx context.Context, deviceID int64, eventType string, limit int) ([]*model.Event, error)
}

type eventEmitter struct {
	eventRepo           eventStore
	hub                 *websocket.Hub
	notificationService *NotificationService
	logger              *slog.Logger
}

func newEventEmitter(eventRepo eventStore, hub *websocket.Hub, notificationService *NotificationService, logger *slog.Logger) eventEmitter {
	return eventEmitter{
		eventRepo:           eventRepo,
		hub:                 hub,
		notificationService: notificationService,
		logger:              cmp.Or(logger, slog.Default()),
	}
}

// emit stores the event, then logs msg, broadcasts the event and triggers notifications.
func (e *eventEmitter) emit(ctx context.Context, event *model.Event, msg string, attrs ...slog.Attr) error {
	if err := e.eventRepo.Create(ctx, event); err != nil {
		return err
	}
	e.logger.LogAttrs(ctx, slog.LevelInfo, msg, attrs...)
	if e.hub != nil {
		e.hub.BroadcastEvent(event)
	}
	if e.notificationService != nil {
		if err := e.notificationService.ProcessEvent(ctx, event); err != nil {
			e.logger.Error("failed to process notifications for event",
				slog.String("eventType", event.Type),
				slog.Int64("eventID", event.ID),
				slog.Any("error", err),
			)
		}
	}
	return nil
}

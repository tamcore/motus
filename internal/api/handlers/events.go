package handlers

import (
	"context"
	"time"

	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
)

func (h *Handler) listEvents(ctx context.Context, deviceID oas.OptInt64, eventType oas.OptString, from, to oas.OptDateTime) ([]oas.Event, *oas.Error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return nil, &oas.Error{Error: "unauthorized"}
	}

	var deviceIDs []int64
	if id, ok := deviceID.Get(); ok && id > 0 {
		if !h.cfg.Devices.UserHasAccess(ctx, user, id) {
			return nil, &oas.Error{Error: "access denied"}
		}
		deviceIDs = []int64{id}
	}

	var eventTypes []string
	if t, ok := eventType.Get(); ok && t != "" {
		eventTypes = []string{t}
	}

	now := time.Now()
	events, err := h.cfg.Events.GetByFilters(ctx, user.ID, deviceIDs, eventTypes,
		from.Or(now.Add(-24*time.Hour)), to.Or(now))
	if err != nil {
		return nil, &oas.Error{Error: "failed to get events"}
	}
	return mapSlice[[]oas.Event](events, eventToOAS), nil
}

// ListEvents implements oas.Handler for GET /api/events.
func (h *Handler) ListEvents(ctx context.Context, params oas.ListEventsParams) (oas.ListEventsRes, error) {
	events, e := h.listEvents(ctx, params.DeviceId, params.Type, params.From, params.To)
	if e != nil {
		return e, nil
	}
	return new(oas.ListEventsOKApplicationJSON(events)), nil
}

// ReportEvents implements oas.Handler for GET /api/reports/events (Traccar-compat alias).
func (h *Handler) ReportEvents(ctx context.Context, params oas.ReportEventsParams) (oas.ReportEventsRes, error) {
	events, e := h.listEvents(ctx, params.DeviceId, params.Type, params.From, params.To)
	if e != nil {
		return e, nil
	}
	return new(oas.ReportEventsOKApplicationJSON(events)), nil
}

package handlers

import (
	"context"
	"log/slog"
	"time"

	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/model"
)

// positionQueryTimeout caps the server-side wall time for a single
// position range query. Unbounded queries (no limit) can be large; this
// prevents a slow or stalled client from holding a DB connection open
// indefinitely. WriteTimeout is intentionally 0 (WebSocket compat), so
// this is the only ceiling on this path.
const positionQueryTimeout = 120 * time.Second

// kmhToKnotsRatio converts a speed value from km/h to knots.
// Traccar's REST API contract specifies speed in knots; internal storage uses km/h.
const kmhToKnotsRatio = 1.0 / 1.852

func positionInKnots(p *model.Position) *model.Position {
	if p.Speed == nil {
		return p
	}
	cp := *p
	knots := *p.Speed * kmhToKnotsRatio
	cp.Speed = &knots
	return &cp
}

// GetPositions implements oas.Handler for GET /api/positions.
// Supports two modes based on the OAS params (DeviceId, From, To):
//   - No DeviceId: latest per user device; or time-range for all user devices when From/To set
//   - With DeviceId: time range for that specific device (defaults to last 24 h)
func (h *Handler) GetPositions(ctx context.Context, params oas.GetPositionsParams) (oas.GetPositionsRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.Error{Error: "unauthorized"}, nil
	}

	deviceID, hasDevice := params.DeviceId.Get()
	from, hasFrom := params.From.Get()
	to, hasTo := params.To.Get()

	// No deviceId, no time range: latest position per user device.
	if !hasDevice && !hasFrom && !hasTo {
		positions, err := h.cfg.Positions.GetLatestByUser(ctx, user.ID)
		if err != nil {
			slog.Error("GetLatestByUser failed", slog.Int64("userID", user.ID), slog.Any("error", err))
			return &oas.Error{Error: "failed to get positions"}, nil
		}
		result := make(oas.GetPositionsOKApplicationJSON, len(positions))
		for i, p := range positions {
			result[i] = positionToOAS(positionInKnots(p))
		}
		return &result, nil
	}

	// No deviceId, with time range: stream all user devices in window.
	if !hasDevice {
		if !hasFrom {
			from = time.Now().Add(-24 * time.Hour)
		}
		if !hasTo {
			to = time.Now()
		}
		queryCtx, cancel := context.WithTimeout(ctx, positionQueryTimeout)
		defer cancel()
		var result oas.GetPositionsOKApplicationJSON
		streamErr := h.cfg.Positions.StreamByUserAndTimeRange(queryCtx, user.ID, from, to, 0, func(p *model.Position) error {
			result = append(result, positionToOAS(positionInKnots(p)))
			return nil
		})
		if streamErr != nil {
			slog.Error("StreamByUserAndTimeRange failed", slog.Any("error", streamErr))
			return &oas.Error{Error: "failed to get positions"}, nil
		}
		if result == nil {
			result = oas.GetPositionsOKApplicationJSON{}
		}
		return &result, nil
	}

	// With deviceId: verify access, then stream time range.
	if !h.cfg.Devices.UserHasAccess(ctx, user, deviceID) {
		return &oas.Error{Error: "access denied"}, nil
	}
	if !hasFrom {
		from = time.Now().Add(-24 * time.Hour)
	}
	if !hasTo {
		to = time.Now()
	}
	queryCtx, cancel := context.WithTimeout(ctx, positionQueryTimeout)
	defer cancel()
	var result oas.GetPositionsOKApplicationJSON
	streamErr := h.cfg.Positions.StreamByDeviceAndTimeRange(queryCtx, deviceID, from, to, 0, func(p *model.Position) error {
		result = append(result, positionToOAS(positionInKnots(p)))
		return nil
	})
	if streamErr != nil {
		slog.Error("StreamByDeviceAndTimeRange failed", slog.Any("error", streamErr))
		return &oas.Error{Error: "failed to get positions"}, nil
	}
	if result == nil {
		result = oas.GetPositionsOKApplicationJSON{}
	}
	return &result, nil
}

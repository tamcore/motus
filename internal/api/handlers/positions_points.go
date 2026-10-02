package handlers

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
)

// GetPositionPoints implements oas.Handler for GET /api/positions/points.
// Range semantics match GetPositions for a single device.
func (h *Handler) GetPositionPoints(ctx context.Context, params oas.GetPositionPointsParams) (oas.GetPositionPointsRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.GetPositionPointsUnauthorized{Error: "unauthorized"}, nil
	}
	if !h.cfg.Devices.UserHasAccess(ctx, user, params.DeviceId) {
		return &oas.GetPositionPointsForbidden{Error: "access denied"}, nil
	}

	now := time.Now()
	from := params.From.Or(now.Add(-24 * time.Hour))
	to := params.To.Or(now)
	limit := positionLimit(params.Limit.Or(0))

	queryCtx, cancel := context.WithTimeout(ctx, positionQueryTimeout)
	defer cancel()
	points, err := h.cfg.Positions.PointsByDeviceAndTimeRange(queryCtx, params.DeviceId, from, to, limit)
	if err != nil {
		slog.Error("get position points failed", slog.Int64("userID", user.ID), slog.Any("error", err))
		return nil, errors.New("failed to get position points")
	}

	result := make(oas.GetPositionPointsOKApplicationJSON, len(points))
	for i, p := range points {
		result[i] = oas.PositionPoint{Lat: p.Lat, Lon: p.Lon, Speed: p.Speed * kmhToKnotsRatio, FixTime: p.FixTime}
	}
	return &result, nil
}

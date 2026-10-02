package handlers

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/model"
)

// GetPositionPoints implements oas.Handler for GET /api/positions/points.
// Range semantics match GetPositions; without deviceId all user devices are covered.
func (h *Handler) GetPositionPoints(ctx context.Context, params oas.GetPositionPointsParams) (oas.GetPositionPointsRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.GetPositionPointsUnauthorized{Error: "unauthorized"}, nil
	}
	deviceID, hasDevice := params.DeviceId.Get()
	if hasDevice && !h.cfg.Devices.UserHasAccess(ctx, user, deviceID) {
		return &oas.GetPositionPointsForbidden{Error: "access denied"}, nil
	}

	now := time.Now()
	from := params.From.Or(now.Add(-24 * time.Hour))
	to := params.To.Or(now)
	limit := positionLimit(params.Limit.Or(0))

	queryCtx, cancel := context.WithTimeout(ctx, positionQueryTimeout)
	defer cancel()
	var (
		points []model.PositionPoint
		err    error
	)
	if hasDevice {
		points, err = h.cfg.Positions.PointsByDeviceAndTimeRange(queryCtx, deviceID, from, to, limit)
	} else {
		points, err = h.cfg.Positions.PointsByUserAndTimeRange(queryCtx, user.ID, from, to, limit)
	}
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

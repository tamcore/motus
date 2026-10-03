package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/reports"
)

var errReportAccessDenied = errors.New("access denied")

// reportDevices resolves the requested devices (deduplicated), or all devices
// of the user when ids is empty. A missing or inaccessible device yields
// errReportAccessDenied, so device existence is not disclosed.
func (h *Handler) reportDevices(ctx context.Context, user *model.User, ids []int64) ([]*model.Device, error) {
	if len(ids) == 0 {
		return h.cfg.Devices.GetByUser(ctx, user.ID)
	}
	ids = slices.Clone(ids)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	devices := make([]*model.Device, 0, len(ids))
	for _, id := range ids {
		d, err := h.cfg.Devices.GetByID(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errReportAccessDenied
		}
		if err != nil {
			return nil, err
		}
		if !h.cfg.Devices.UserHasAccess(ctx, user, id) {
			return nil, errReportAccessDenied
		}
		devices = append(devices, d)
	}
	return devices, nil
}

// deviceActivity streams one device's positions in [from, to] once, unsampled
// and in timestamp order, into both the trip and the stop detector.
func (h *Handler) deviceActivity(ctx context.Context, d *model.Device, from, to time.Time) ([]oas.ReportTrip, []oas.ReportStop, error) {
	ctx, cancel := context.WithTimeout(ctx, positionQueryTimeout)
	defer cancel()
	var (
		td reports.TripDetector
		sd reports.StopDetector
	)
	err := h.cfg.Positions.StreamTrackByDeviceAndTimeRange(ctx, d.ID, from, to, func(p *model.Position) error {
		td.Add(p)
		sd.Add(p)
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("device %d: %w", d.ID, err)
	}

	var trips []oas.ReportTrip
	for _, t := range td.Trips() {
		trips = append(trips, oas.ReportTrip{
			DeviceId:   d.ID,
			DeviceName: d.Name,
			StartTime:  t.StartTime,
			EndTime:    t.EndTime,
			Duration:   t.Duration,
			Distance:   t.Distance,
			AvgSpeed:   t.AvgSpeed * kmhToKnotsRatio,
			MaxSpeed:   t.MaxSpeed * kmhToKnotsRatio,
		})
	}
	var stops []oas.ReportStop
	for _, s := range sd.Stops() {
		stops = append(stops, oas.ReportStop{
			DeviceId:      d.ID,
			DeviceName:    d.Name,
			Latitude:      s.Latitude,
			Longitude:     s.Longitude,
			Address:       s.Address,
			ArrivalTime:   s.ArrivalTime,
			DepartureTime: s.DepartureTime,
			Duration:      s.Duration,
		})
	}
	return trips, stops, nil
}

// ReportActivity implements oas.Handler for GET /api/reports/activity.
func (h *Handler) ReportActivity(ctx context.Context, params oas.ReportActivityParams) (oas.ReportActivityRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.ReportActivityUnauthorized{Error: "unauthorized"}, nil
	}
	if params.To.Before(params.From) {
		return &oas.ReportActivityBadRequest{Error: "to must not be before from"}, nil
	}
	devices, err := h.reportDevices(ctx, user, params.DeviceId)
	if errors.Is(err, errReportAccessDenied) {
		return &oas.ReportActivityForbidden{Error: err.Error()}, nil
	}
	if err != nil {
		slog.Error("resolve report devices failed", slog.Int64("userID", user.ID), slog.Any("error", err))
		return nil, errors.New("failed to build activity report")
	}

	result := &oas.ReportActivity{Trips: []oas.ReportTrip{}, Stops: []oas.ReportStop{}}
	for _, d := range devices {
		trips, stops, err := h.deviceActivity(ctx, d, params.From, params.To)
		if err != nil {
			slog.Error("activity report failed", slog.Int64("userID", user.ID), slog.Any("error", err))
			return nil, errors.New("failed to build activity report")
		}
		result.Trips = append(result.Trips, trips...)
		result.Stops = append(result.Stops, stops...)
	}
	return result, nil
}

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

// streamReport streams each device's positions in [from, to], unsampled and
// in timestamp order, into add, then calls done once the device is complete.
func (h *Handler) streamReport(ctx context.Context, devices []*model.Device, from, to time.Time,
	add func(*model.Position), done func(*model.Device),
) error {
	for _, d := range devices {
		if err := h.streamDevice(ctx, d.ID, from, to, add); err != nil {
			return fmt.Errorf("device %d: %w", d.ID, err)
		}
		done(d)
	}
	return nil
}

func (h *Handler) streamDevice(ctx context.Context, deviceID int64, from, to time.Time, add func(*model.Position)) error {
	ctx, cancel := context.WithTimeout(ctx, positionQueryTimeout)
	defer cancel()
	return h.cfg.Positions.StreamByDeviceAndTimeRange(ctx, deviceID, from, to, 0, func(p *model.Position) error {
		add(p)
		return nil
	})
}

// ReportTrips implements oas.Handler for GET /api/reports/trips.
func (h *Handler) ReportTrips(ctx context.Context, params oas.ReportTripsParams) (oas.ReportTripsRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.ReportTripsUnauthorized{Error: "unauthorized"}, nil
	}
	if params.To.Before(params.From) {
		return &oas.ReportTripsBadRequest{Error: "to must not be before from"}, nil
	}
	devices, err := h.reportDevices(ctx, user, params.DeviceId)
	if errors.Is(err, errReportAccessDenied) {
		return &oas.ReportTripsForbidden{Error: err.Error()}, nil
	}
	if err != nil {
		slog.Error("resolve report devices failed", slog.Int64("userID", user.ID), slog.Any("error", err))
		return nil, errors.New("failed to build trip report")
	}

	result := oas.ReportTripsOKApplicationJSON{}
	det := &reports.TripDetector{}
	err = h.streamReport(ctx, devices, params.From, params.To,
		func(p *model.Position) { det.Add(p) },
		func(d *model.Device) {
			for _, t := range det.Trips() {
				result = append(result, oas.ReportTrip{
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
			det = &reports.TripDetector{}
		})
	if err != nil {
		slog.Error("trip report failed", slog.Int64("userID", user.ID), slog.Any("error", err))
		return nil, errors.New("failed to build trip report")
	}
	return &result, nil
}

// ReportStops implements oas.Handler for GET /api/reports/stops.
func (h *Handler) ReportStops(ctx context.Context, params oas.ReportStopsParams) (oas.ReportStopsRes, error) {
	user := api.UserFromContext(ctx)
	if user == nil {
		return &oas.ReportStopsUnauthorized{Error: "unauthorized"}, nil
	}
	if params.To.Before(params.From) {
		return &oas.ReportStopsBadRequest{Error: "to must not be before from"}, nil
	}
	devices, err := h.reportDevices(ctx, user, params.DeviceId)
	if errors.Is(err, errReportAccessDenied) {
		return &oas.ReportStopsForbidden{Error: err.Error()}, nil
	}
	if err != nil {
		slog.Error("resolve report devices failed", slog.Int64("userID", user.ID), slog.Any("error", err))
		return nil, errors.New("failed to build stop report")
	}

	result := oas.ReportStopsOKApplicationJSON{}
	det := &reports.StopDetector{}
	err = h.streamReport(ctx, devices, params.From, params.To,
		func(p *model.Position) { det.Add(p) },
		func(d *model.Device) {
			for _, s := range det.Stops() {
				result = append(result, oas.ReportStop{
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
			det = &reports.StopDetector{}
		})
	if err != nil {
		slog.Error("stop report failed", slog.Int64("userID", user.ID), slog.Any("error", err))
		return nil, errors.New("failed to build stop report")
	}
	return &result, nil
}

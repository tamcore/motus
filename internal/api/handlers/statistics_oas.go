package handlers

import (
	"context"

	oas "github.com/tamcore/motus/internal/api/oas"
)

// AdminGetStatistics returns platform-wide statistics.
// GET /api/admin/statistics
func (h *Handler) AdminGetStatistics(ctx context.Context) (oas.AdminGetStatisticsRes, error) {
	if _, err := requireAdminCtx(ctx); err != nil {
		return &oas.AdminGetStatisticsForbidden{Error: "admin access required"}, nil
	}

	stats, err := h.cfg.Stats.GetPlatformStats(ctx)
	if err != nil {
		return &oas.AdminGetStatisticsForbidden{Error: "failed to get statistics"}, nil
	}

	result := &oas.PlatformStats{
		TotalUsers:        stats.TotalUsers,
		TotalDevices:      stats.TotalDevices,
		TotalPositions:    stats.TotalPositions,
		TotalEvents:       stats.TotalEvents,
		NotificationsSent: stats.NotificationsSent,
		PositionsToday:    stats.PositionsToday,
		ActiveUsers:       stats.ActiveUsers,
	}

	devsByStatus := make(oas.PlatformStatsDevicesByStatus, len(stats.DevicesByStatus))
	for k, v := range stats.DevicesByStatus {
		devsByStatus[k] = v
	}
	result.DevicesByStatus = devsByStatus

	return result, nil
}

// AdminGetUserStatistics returns statistics for a specific user.
// GET /api/admin/statistics/users/{id}
func (h *Handler) AdminGetUserStatistics(ctx context.Context, params oas.AdminGetUserStatisticsParams) (oas.AdminGetUserStatisticsRes, error) {
	if _, err := requireAdminCtx(ctx); err != nil {
		return &oas.AdminGetUserStatisticsForbidden{Error: "admin access required"}, nil
	}

	stats, err := h.cfg.Stats.GetUserStats(ctx, params.ID)
	if err != nil {
		return &oas.AdminGetUserStatisticsNotFound{Error: "user not found or failed to get statistics"}, nil
	}

	result := &oas.UserStats{
		UserId:          stats.UserID,
		DevicesOwned:    stats.DevicesOwned,
		TotalPositions:  stats.TotalPositions,
		EventsTriggered: stats.EventsTriggered,
		GeofencesOwned:  stats.GeofencesOwned,
		LastLogin:       ptrToOptTime(stats.LastLogin),
	}

	return result, nil
}

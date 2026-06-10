package handlers_test

// Tests for the ogen Handler statistics methods (AdminGetStatistics,
// AdminGetUserStatistics), ported from the deleted chi StatisticsHandler
// tests in statistics_test.go. The chi-specific invalid-path-param test
// (TestGetUserStats_InvalidID) is intentionally dropped: ogen owns path
// param decoding and rejects non-numeric ids before the handler runs.

import (
	"context"
	"errors"
	"testing"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// mockStatsRepo implements repository.StatisticsRepo for handler tests.
// (Moved from the deleted statistics_test.go.)
type mockStatsRepo struct {
	platformStats *repository.PlatformStats
	platformErr   error
	userStats     *repository.UserStats
	userErr       error
}

var _ repository.StatisticsRepo = (*mockStatsRepo)(nil)

func (m *mockStatsRepo) GetPlatformStats(_ context.Context) (*repository.PlatformStats, error) {
	return m.platformStats, m.platformErr
}

func (m *mockStatsRepo) GetUserStats(_ context.Context, _ int64) (*repository.UserStats, error) {
	return m.userStats, m.userErr
}

// newStatsTestHandler builds an ogen Handler around the given statistics repo.
func newStatsTestHandler(stats repository.StatisticsRepo) *handlers.Handler {
	return handlers.NewHandler(handlers.HandlerConfig{
		Stats: stats,
	})
}

// ---------------------------------------------------------------------------
// AdminGetStatistics
// ---------------------------------------------------------------------------

func TestAdminGetStatistics_RequiresAdmin(t *testing.T) {
	h := newStatsTestHandler(&mockStatsRepo{})

	// No user in context.
	res, err := h.AdminGetStatistics(context.Background())
	if err != nil {
		t.Fatalf("AdminGetStatistics returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminGetStatisticsForbidden); !ok {
		t.Errorf("expected *oas.AdminGetStatisticsForbidden, got %T", res)
	}
}

func TestAdminGetStatistics_NonAdminForbidden(t *testing.T) {
	h := newStatsTestHandler(&mockStatsRepo{})

	user := &model.User{ID: 1, Role: model.RoleUser}
	ctx := api.ContextWithUser(context.Background(), user)

	res, err := h.AdminGetStatistics(ctx)
	if err != nil {
		t.Fatalf("AdminGetStatistics returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminGetStatisticsForbidden); !ok {
		t.Errorf("expected *oas.AdminGetStatisticsForbidden for non-admin, got %T", res)
	}
}

func TestAdminGetStatistics_Success(t *testing.T) {
	stats := &repository.PlatformStats{
		TotalUsers:      10,
		TotalDevices:    5,
		TotalPositions:  1000,
		TotalEvents:     50,
		DevicesByStatus: map[string]int64{"online": 3, "offline": 2},
	}
	h := newStatsTestHandler(&mockStatsRepo{platformStats: stats})

	admin := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), admin)

	res, err := h.AdminGetStatistics(ctx)
	if err != nil {
		t.Fatalf("AdminGetStatistics returned error: %v", err)
	}
	platform, ok := res.(*oas.PlatformStats)
	if !ok {
		t.Fatalf("expected *oas.PlatformStats, got %T", res)
	}
	if platform.TotalUsers != 10 {
		t.Errorf("expected totalUsers=10, got %d", platform.TotalUsers)
	}
	if platform.TotalDevices != 5 {
		t.Errorf("expected totalDevices=5, got %d", platform.TotalDevices)
	}
	if platform.TotalPositions != 1000 {
		t.Errorf("expected totalPositions=1000, got %d", platform.TotalPositions)
	}
	if platform.DevicesByStatus["online"] != 3 || platform.DevicesByStatus["offline"] != 2 {
		t.Errorf("unexpected devicesByStatus: %v", platform.DevicesByStatus)
	}
}

func TestAdminGetStatistics_RepoError(t *testing.T) {
	h := newStatsTestHandler(&mockStatsRepo{platformErr: errors.New("db error")})

	admin := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), admin)

	res, err := h.AdminGetStatistics(ctx)
	if err != nil {
		t.Fatalf("AdminGetStatistics returned error: %v", err)
	}
	forbidden, ok := res.(*oas.AdminGetStatisticsForbidden)
	if !ok {
		t.Fatalf("expected *oas.AdminGetStatisticsForbidden on repo error, got %T", res)
	}
	if forbidden.Error != "failed to get statistics" {
		t.Errorf("unexpected error message: %q", forbidden.Error)
	}
}

// ---------------------------------------------------------------------------
// AdminGetUserStatistics
// ---------------------------------------------------------------------------

func TestAdminGetUserStatistics_RequiresAdmin(t *testing.T) {
	h := newStatsTestHandler(&mockStatsRepo{})

	res, err := h.AdminGetUserStatistics(context.Background(), oas.AdminGetUserStatisticsParams{ID: 1})
	if err != nil {
		t.Fatalf("AdminGetUserStatistics returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminGetUserStatisticsForbidden); !ok {
		t.Errorf("expected *oas.AdminGetUserStatisticsForbidden, got %T", res)
	}
}

func TestAdminGetUserStatistics_NonAdminForbidden(t *testing.T) {
	h := newStatsTestHandler(&mockStatsRepo{})

	user := &model.User{ID: 1, Role: model.RoleUser}
	ctx := api.ContextWithUser(context.Background(), user)

	res, err := h.AdminGetUserStatistics(ctx, oas.AdminGetUserStatisticsParams{ID: 1})
	if err != nil {
		t.Fatalf("AdminGetUserStatistics returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminGetUserStatisticsForbidden); !ok {
		t.Errorf("expected *oas.AdminGetUserStatisticsForbidden for non-admin, got %T", res)
	}
}

func TestAdminGetUserStatistics_Success(t *testing.T) {
	stats := &repository.UserStats{
		UserID:         42,
		DevicesOwned:   3,
		TotalPositions: 100,
	}
	h := newStatsTestHandler(&mockStatsRepo{userStats: stats})

	admin := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), admin)

	res, err := h.AdminGetUserStatistics(ctx, oas.AdminGetUserStatisticsParams{ID: 42})
	if err != nil {
		t.Fatalf("AdminGetUserStatistics returned error: %v", err)
	}
	userStats, ok := res.(*oas.UserStats)
	if !ok {
		t.Fatalf("expected *oas.UserStats, got %T", res)
	}
	if userStats.UserId != 42 {
		t.Errorf("expected userId=42, got %d", userStats.UserId)
	}
	if userStats.DevicesOwned != 3 {
		t.Errorf("expected devicesOwned=3, got %d", userStats.DevicesOwned)
	}
	if userStats.TotalPositions != 100 {
		t.Errorf("expected totalPositions=100, got %d", userStats.TotalPositions)
	}
}

func TestAdminGetUserStatistics_RepoError(t *testing.T) {
	h := newStatsTestHandler(&mockStatsRepo{userErr: errors.New("db error")})

	admin := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), admin)

	res, err := h.AdminGetUserStatistics(ctx, oas.AdminGetUserStatisticsParams{ID: 42})
	if err != nil {
		t.Fatalf("AdminGetUserStatistics returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminGetUserStatisticsNotFound); !ok {
		t.Errorf("expected *oas.AdminGetUserStatisticsNotFound on repo error, got %T", res)
	}
}

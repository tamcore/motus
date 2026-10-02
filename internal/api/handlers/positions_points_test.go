package handlers_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// pointsPositionRepo records the arguments of the points queries.
type pointsPositionRepo struct {
	repository.PositionRepo
	points   []model.PositionPoint
	deviceID int64
	limit    int
}

func (m *pointsPositionRepo) PointsByDeviceAndTimeRange(_ context.Context, deviceID int64, _, _ time.Time, limit int) ([]model.PositionPoint, error) {
	m.deviceID, m.limit = deviceID, limit
	return m.points, nil
}

func newPointsHandler(repo *pointsPositionRepo, hasAccess bool) *handlers.Handler {
	return handlers.NewHandler(handlers.HandlerConfig{
		Positions: repo,
		Devices: &mockDeviceRepo{userHasAccessFn: func(context.Context, *model.User, int64) bool {
			return hasAccess
		}},
	})
}

func pointsUserCtx() context.Context {
	return api.ContextWithUser(context.Background(), &model.User{ID: 7, Email: "points@example.com"})
}

func TestGetPositionPoints_Unauthenticated(t *testing.T) {
	h := newPointsHandler(&pointsPositionRepo{}, true)

	res, err := h.GetPositionPoints(context.Background(), oas.GetPositionPointsParams{DeviceId: 3})
	if err != nil {
		t.Fatalf("GetPositionPoints returned error: %v", err)
	}
	if _, ok := res.(*oas.GetPositionPointsUnauthorized); !ok {
		t.Fatalf("expected *oas.GetPositionPointsUnauthorized, got %T", res)
	}
}

func TestGetPositionPoints_DeviceForbidden(t *testing.T) {
	repo := &pointsPositionRepo{}
	h := newPointsHandler(repo, false)

	res, err := h.GetPositionPoints(pointsUserCtx(), oas.GetPositionPointsParams{DeviceId: 3})
	if err != nil {
		t.Fatalf("GetPositionPoints returned error: %v", err)
	}
	if _, ok := res.(*oas.GetPositionPointsForbidden); !ok {
		t.Fatalf("expected *oas.GetPositionPointsForbidden, got %T", res)
	}
	if repo.deviceID != 0 {
		t.Error("forbidden request queried positions")
	}
}

func TestGetPositionPoints_LimitClamp(t *testing.T) {
	tests := []struct {
		name  string
		limit oas.OptInt
		want  int
	}{
		{"omitted uses max", oas.OptInt{}, 10000},
		{"above max clamped", oas.NewOptInt(20000), 10000},
		{"below max kept", oas.NewOptInt(500), 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &pointsPositionRepo{}
			h := newPointsHandler(repo, true)
			if _, err := h.GetPositionPoints(pointsUserCtx(), oas.GetPositionPointsParams{DeviceId: 3, Limit: tt.limit}); err != nil {
				t.Fatalf("GetPositionPoints returned error: %v", err)
			}
			if repo.deviceID != 3 || repo.limit != tt.want {
				t.Errorf("queried device %d with limit %d, want device 3 with limit %d", repo.deviceID, repo.limit, tt.want)
			}
		})
	}
}

func TestGetPositionPoints_ConvertsToKnots(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	repo := &pointsPositionRepo{points: []model.PositionPoint{{Lat: 52, Lon: 13, Speed: 18.52, FixTime: ts}}}
	h := newPointsHandler(repo, true)

	res, err := h.GetPositionPoints(pointsUserCtx(), oas.GetPositionPointsParams{DeviceId: 3})
	if err != nil {
		t.Fatalf("GetPositionPoints returned error: %v", err)
	}
	list, ok := res.(*oas.GetPositionPointsOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.GetPositionPointsOKApplicationJSON, got %T", res)
	}
	if repo.deviceID != 3 {
		t.Errorf("queried device %d, want 3", repo.deviceID)
	}
	if len(*list) != 1 {
		t.Fatalf("got %d points, want 1", len(*list))
	}
	got := (*list)[0]
	if got.Lat != 52 || got.Lon != 13 || !got.FixTime.Equal(ts) || math.Abs(got.Speed-10) > 1e-9 {
		t.Errorf("got %+v, want lat 52, lon 13, speed 10 kn, fixTime %v", got, ts)
	}
}

func TestGetPositionPoints_EmptyIsArray(t *testing.T) {
	h := newPointsHandler(&pointsPositionRepo{points: []model.PositionPoint{}}, true)

	res, err := h.GetPositionPoints(pointsUserCtx(), oas.GetPositionPointsParams{DeviceId: 3})
	if err != nil {
		t.Fatalf("GetPositionPoints returned error: %v", err)
	}
	list, ok := res.(*oas.GetPositionPointsOKApplicationJSON)
	if !ok || *list == nil || len(*list) != 0 {
		t.Fatalf("expected empty non-nil array, got %T %#v", res, res)
	}
}

func TestGetPositionPoints_ByDevice_OAS(t *testing.T) {
	env := setupPositionsOASIntegration(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for i := range 4 {
		if err := env.posRepo.Create(ctx, &model.Position{DeviceID: env.device.ID, Latitude: 52, Longitude: 13, Timestamp: now.Add(time.Duration(-4+i) * time.Minute)}); err != nil {
			t.Fatalf("Create position %d: %v", i, err)
		}
	}

	res, err := env.handler.GetPositionPoints(env.userCtx(), oas.GetPositionPointsParams{
		DeviceId: env.device.ID,
		From:     oas.NewOptDateTime(now.Add(-time.Hour)),
		To:       oas.NewOptDateTime(now.Add(time.Minute)),
		Limit:    oas.NewOptInt(2),
	})
	if err != nil {
		t.Fatalf("GetPositionPoints returned error: %v", err)
	}
	list, ok := res.(*oas.GetPositionPointsOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.GetPositionPointsOKApplicationJSON, got %T", res)
	}
	if len(*list) != 2 {
		t.Errorf("expected 2 sampled points, got %d", len(*list))
	}

	other := api.ContextWithUser(ctx, &model.User{ID: env.user.ID + 999, Email: "other@example.com"})
	res, err = env.handler.GetPositionPoints(other, oas.GetPositionPointsParams{DeviceId: env.device.ID})
	if err != nil {
		t.Fatalf("GetPositionPoints returned error: %v", err)
	}
	if _, ok := res.(*oas.GetPositionPointsForbidden); !ok {
		t.Errorf("expected *oas.GetPositionPointsForbidden for foreign device, got %T", res)
	}
}

package repository_test

import (
	"context"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func pointLats(points []model.PositionPoint) []float64 {
	lats := make([]float64, len(points))
	for i, p := range points {
		lats[i] = math.Round((p.Lat - 52) * 100)
	}
	return lats
}

func TestPositionRepository_PointsByDeviceAndTimeRange(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	posRepo := repository.NewPositionRepository(pool)
	ctx := context.Background()

	device := testutil.CreateDevice(t, testutil.CreateUser(t, "position-points-1@example.com").ID, "position-points-1")
	now := time.Now().UTC().Truncate(time.Microsecond)
	speed, course, altitude := 36.0, 270.5, 120.0
	for i := range 10 {
		p := &model.Position{
			DeviceID:   device.ID,
			Latitude:   52.0 + float64(i)*0.01,
			Longitude:  13.0,
			Timestamp:  now.Add(time.Duration(-10+i) * time.Minute),
			Attributes: map[string]any{"big": "ignored"},
		}
		if i == 0 {
			p.Speed, p.Course, p.Altitude = &speed, &course, &altitude
		}
		if err := posRepo.Create(ctx, p); err != nil {
			t.Fatalf("Create position %d failed: %v", i, err)
		}
	}

	points := func(limit int) []model.PositionPoint {
		t.Helper()
		got, err := posRepo.PointsByDeviceAndTimeRange(ctx, device.ID, now.Add(-time.Hour), now.Add(time.Minute), limit)
		if err != nil {
			t.Fatalf("PointsByDeviceAndTimeRange(limit=%d) failed: %v", limit, err)
		}
		return got
	}

	all := points(50)
	if len(all) != 10 {
		t.Fatalf("limit above total returned %d points, want 10", len(all))
	}
	if all[0].Speed != speed || all[0].Lon != 13.0 || !all[0].FixTime.Equal(now.Add(-10*time.Minute)) {
		t.Errorf("first point = %+v, want speed %v, lon 13, fixTime %v", all[0], speed, now.Add(-10*time.Minute))
	}
	if all[0].Course == nil || *all[0].Course != course || all[0].Altitude == nil || *all[0].Altitude != altitude {
		t.Errorf("first point course/altitude = %v/%v, want %v/%v", all[0].Course, all[0].Altitude, course, altitude)
	}
	if all[1].Speed != 0 {
		t.Errorf("NULL speed returned %v, want 0", all[1].Speed)
	}
	if all[1].Course != nil || all[1].Altitude != nil {
		t.Errorf("NULL course/altitude returned %v/%v, want nil", all[1].Course, all[1].Altitude)
	}
	if got := pointLats(points(3)); !slices.Equal(got, []float64{0, 4, 8}) {
		t.Errorf("limit=3 returned %v, want every 4th", got)
	}
	if got := pointLats(points(5)); !slices.Equal(got, []float64{0, 2, 4, 6, 8}) {
		t.Errorf("limit=5 returned %v, want every 2nd", got)
	}
	if got := pointLats(points(9)); !slices.Equal(got, []float64{0, 2, 4, 6, 8}) {
		t.Errorf("limit=9 returned %v, want every 2nd", got)
	}
	if got := points(10); len(got) != 10 {
		t.Errorf("limit equal to total returned %d points, want 10", len(got))
	}

	empty, err := posRepo.PointsByDeviceAndTimeRange(ctx, device.ID, now.Add(time.Hour), now.Add(2*time.Hour), 10)
	if err != nil {
		t.Fatalf("empty range failed: %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("empty range returned %#v, want non-nil empty slice", empty)
	}
}

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
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func TestReportActivity_Unauthorized(t *testing.T) {
	h := handlers.NewHandler(handlers.HandlerConfig{})
	res, err := h.ReportActivity(context.Background(), oas.ReportActivityParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.ReportActivityUnauthorized); !ok {
		t.Fatalf("expected Unauthorized, got %T", res)
	}
}

func TestReportActivity_ToBeforeFrom(t *testing.T) {
	h := handlers.NewHandler(handlers.HandlerConfig{})
	ctx := api.ContextWithUser(context.Background(), &model.User{ID: 1})
	now := time.Now()
	res, err := h.ReportActivity(ctx, oas.ReportActivityParams{From: now, To: now.Add(-time.Hour)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.ReportActivityBadRequest); !ok {
		t.Fatalf("expected BadRequest, got %T", res)
	}
}

type reportsEnv struct {
	handler *handlers.Handler
	user    *model.User
	device  *model.Device
	other   *model.Device
	start   time.Time
}

// setupReports seeds one device with a 10-minute drive at 36 km/h followed
// by a 10-minute stop, and a second device owned by another user.
func setupReports(t *testing.T) *reportsEnv {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	userRepo := repository.NewUserRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	posRepo := repository.NewPositionRepository(pool)

	user := &model.User{Email: "reports@example.com", PasswordHash: "$2a$10$hash", Name: "Reports"}
	owner := &model.User{Email: "reports-other@example.com", PasswordHash: "$2a$10$hash", Name: "Other"}
	for _, u := range []*model.User{user, owner} {
		if err := userRepo.Create(ctx, u); err != nil {
			t.Fatalf("create user: %v", err)
		}
	}
	device := &model.Device{UniqueID: "reports-dev", Name: "Reports Device", Status: "online"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}
	other := testutil.CreateDevice(t, owner.ID, "reports-other")

	start := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	for i := range 21 {
		speed := 36.0
		lat := 52.0 + float64(i)*0.001
		if i > 10 {
			speed, lat = 0, 52.01
		}
		p := &model.Position{DeviceID: device.ID, Timestamp: start.Add(time.Duration(i) * time.Minute),
			Latitude: lat, Longitude: 13.0, Speed: &speed, Valid: true}
		if err := posRepo.Create(ctx, p); err != nil {
			t.Fatalf("create position: %v", err)
		}
	}

	h := handlers.NewHandler(handlers.HandlerConfig{Positions: posRepo, Devices: deviceRepo, Users: userRepo})
	return &reportsEnv{handler: h, user: user, device: device, other: other, start: start}
}

func (e *reportsEnv) ctx() context.Context {
	return api.ContextWithUser(context.Background(), e.user)
}

func (e *reportsEnv) activity(t *testing.T, ids []int64, from, to time.Time) oas.ReportActivityRes {
	t.Helper()
	res, err := e.handler.ReportActivity(e.ctx(), oas.ReportActivityParams{DeviceId: ids, From: from, To: to})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return res
}

func requireActivity(t *testing.T, res oas.ReportActivityRes) *oas.ReportActivity {
	t.Helper()
	a, ok := res.(*oas.ReportActivity)
	if !ok {
		t.Fatalf("expected *oas.ReportActivity, got %T", res)
	}
	return a
}

func TestReportActivity_ByDevice(t *testing.T) {
	env := setupReports(t)
	a := requireActivity(t, env.activity(t, []int64{env.device.ID}, env.start.Add(-time.Minute), env.start.Add(time.Hour)))

	if len(a.Trips) != 1 {
		t.Fatalf("expected 1 trip, got %d", len(a.Trips))
	}
	trip := a.Trips[0]
	if trip.DeviceId != env.device.ID || trip.DeviceName != "Reports Device" {
		t.Errorf("unexpected device %d %q", trip.DeviceId, trip.DeviceName)
	}
	if !trip.StartTime.Equal(env.start) {
		t.Errorf("start = %v, want %v", trip.StartTime, env.start)
	}
	// 36 km/h ≈ 19.438 knots.
	if math.Abs(trip.MaxSpeed-36/1.852) > 1e-6 || math.Abs(trip.AvgSpeed-36/1.852) > 1e-6 {
		t.Errorf("speeds must be in knots, got avg %v max %v", trip.AvgSpeed, trip.MaxSpeed)
	}

	if len(a.Stops) != 1 {
		t.Fatalf("expected 1 stop, got %d", len(a.Stops))
	}
	stop := a.Stops[0]
	if !stop.ArrivalTime.Equal(env.start.Add(11*time.Minute)) || stop.Duration != 540 {
		t.Errorf("unexpected stop %+v", stop)
	}
	if stop.DeviceName != "Reports Device" || stop.Address != "52.01000, 13.00000" {
		t.Errorf("unexpected stop %+v", stop)
	}
}

func TestReportActivity_AllUserDevices(t *testing.T) {
	env := setupReports(t)
	a := requireActivity(t, env.activity(t, nil, env.start.Add(-time.Minute), env.start.Add(time.Hour)))
	if len(a.Trips) != 1 || len(a.Stops) != 1 {
		t.Fatalf("expected 1 trip and 1 stop of the user's device, got %d/%d", len(a.Trips), len(a.Stops))
	}
}

func TestReportActivity_EmptyRangeReturnsEmptyArrays(t *testing.T) {
	env := setupReports(t)
	a := requireActivity(t, env.activity(t, []int64{env.device.ID}, env.start.Add(-48*time.Hour), env.start.Add(-47*time.Hour)))
	if a.Trips == nil || a.Stops == nil || len(a.Trips) != 0 || len(a.Stops) != 0 {
		t.Fatalf("expected empty non-nil arrays, got %#v / %#v", a.Trips, a.Stops)
	}
}

func TestReportActivity_ForeignDeviceForbidden(t *testing.T) {
	env := setupReports(t)
	for _, ids := range [][]int64{{env.other.ID}, {env.device.ID, env.other.ID}} {
		if res := env.activity(t, ids, env.start, env.start.Add(time.Hour)); res == nil {
			t.Fatal("nil response")
		} else if _, ok := res.(*oas.ReportActivityForbidden); !ok {
			t.Fatalf("ids %v: expected Forbidden, got %T", ids, res)
		}
	}
}

func TestReportActivity_DuplicateDeviceIDsDeduped(t *testing.T) {
	env := setupReports(t)
	a := requireActivity(t, env.activity(t, []int64{env.device.ID, env.device.ID}, env.start.Add(-time.Minute), env.start.Add(time.Hour)))
	if len(a.Trips) != 1 || len(a.Stops) != 1 {
		t.Fatalf("expected 1 trip and 1 stop for duplicated device id, got %d/%d", len(a.Trips), len(a.Stops))
	}
}

func TestReportActivity_NonexistentDeviceForbidden(t *testing.T) {
	env := setupReports(t)
	res := env.activity(t, []int64{env.other.ID + 1000}, env.start, env.start.Add(time.Hour))
	if _, ok := res.(*oas.ReportActivityForbidden); !ok {
		t.Fatalf("expected Forbidden, got %T", res)
	}
}

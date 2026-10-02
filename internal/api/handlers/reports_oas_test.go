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

func TestReportTrips_Unauthorized(t *testing.T) {
	h := handlers.NewHandler(handlers.HandlerConfig{})
	res, err := h.ReportTrips(context.Background(), oas.ReportTripsParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.ReportTripsUnauthorized); !ok {
		t.Fatalf("expected Unauthorized, got %T", res)
	}
}

func TestReportStops_ToBeforeFrom(t *testing.T) {
	h := handlers.NewHandler(handlers.HandlerConfig{})
	ctx := api.ContextWithUser(context.Background(), &model.User{ID: 1})
	now := time.Now()
	res, err := h.ReportStops(ctx, oas.ReportStopsParams{From: now, To: now.Add(-time.Hour)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.ReportStopsBadRequest); !ok {
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
	other := &model.Device{UniqueID: "reports-other", Name: "Other Device", Status: "online"}
	if err := deviceRepo.Create(ctx, other, owner.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

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

func TestReportTrips_ByDevice(t *testing.T) {
	env := setupReports(t)
	res, err := env.handler.ReportTrips(env.ctx(), oas.ReportTripsParams{
		DeviceId: []int64{env.device.ID}, From: env.start.Add(-time.Minute), To: env.start.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	list, ok := res.(*oas.ReportTripsOKApplicationJSON)
	if !ok {
		t.Fatalf("expected OK, got %T", res)
	}
	if len(*list) != 1 {
		t.Fatalf("expected 1 trip, got %d", len(*list))
	}
	trip := (*list)[0]
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
}

func TestReportTrips_AllUserDevices(t *testing.T) {
	env := setupReports(t)
	res, err := env.handler.ReportTrips(env.ctx(), oas.ReportTripsParams{
		From: env.start.Add(-time.Minute), To: env.start.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	list, ok := res.(*oas.ReportTripsOKApplicationJSON)
	if !ok || len(*list) != 1 {
		t.Fatalf("expected 1 trip of the user's device, got %T %v", res, list)
	}
}

func TestReportTrips_ForeignDeviceForbidden(t *testing.T) {
	env := setupReports(t)
	res, err := env.handler.ReportTrips(env.ctx(), oas.ReportTripsParams{
		DeviceId: []int64{env.other.ID}, From: env.start, To: env.start.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.ReportTripsForbidden); !ok {
		t.Fatalf("expected Forbidden, got %T", res)
	}
}

func TestReportStops_ByDevice(t *testing.T) {
	env := setupReports(t)
	res, err := env.handler.ReportStops(env.ctx(), oas.ReportStopsParams{
		DeviceId: []int64{env.device.ID}, From: env.start.Add(-time.Minute), To: env.start.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	list, ok := res.(*oas.ReportStopsOKApplicationJSON)
	if !ok {
		t.Fatalf("expected OK, got %T", res)
	}
	if len(*list) != 1 {
		t.Fatalf("expected 1 stop, got %d", len(*list))
	}
	stop := (*list)[0]
	if !stop.ArrivalTime.Equal(env.start.Add(11*time.Minute)) || stop.Duration != 540 {
		t.Errorf("unexpected stop %+v", stop)
	}
	if stop.DeviceName != "Reports Device" || stop.Address != "52.01000, 13.00000" {
		t.Errorf("unexpected stop %+v", stop)
	}
}

func TestReportStops_ForeignDeviceForbidden(t *testing.T) {
	env := setupReports(t)
	res, err := env.handler.ReportStops(env.ctx(), oas.ReportStopsParams{
		DeviceId: []int64{env.device.ID, env.other.ID}, From: env.start, To: env.start.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.ReportStopsForbidden); !ok {
		t.Fatalf("expected Forbidden, got %T", res)
	}
}

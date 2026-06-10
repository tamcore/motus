package handlers_test

// Integration tests for the ogen Handler event methods (ListEvents and the
// Traccar-compat alias ReportEvents). Ported from the deleted chi tests in
// events_test.go and reports_test.go. They require Docker (PostGIS) and are
// skipped automatically in -short mode by testutil.SetupTestDB.
//
// Dropped tests (no live equivalent):
//   - invalid deviceId query parsing: ogen owns query param decoding.
//   - repeatable deviceId[]/type[] array params: the OpenAPI spec models
//     deviceId and type as single optional parameters.
//   - reports nil-pool panic-recover hacks (TestReportEvents_Unauthenticated /
//     _Authenticated_DefaultParams): superseded by typed unauthorized
//     assertions and the integration tests below.

import (
	"context"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

// eventsOASEnv bundles the handler and repositories for event integration tests.
type eventsOASEnv struct {
	h          *handlers.Handler
	userRepo   *repository.UserRepository
	deviceRepo *repository.DeviceRepository
	eventRepo  *repository.EventRepository
	user       *model.User
	device     *model.Device
}

func setupEventsOASIntegration(t *testing.T) *eventsOASEnv {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	userRepo := repository.NewUserRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	eventRepo := repository.NewEventRepository(pool)

	user := &model.User{Email: "events-oas@example.com", PasswordHash: "$2a$10$hash", Name: "Events OAS"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	device := &model.Device{UniqueID: "events-oas-dev", Name: "Events Device", Status: "online"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	h := handlers.NewHandler(handlers.HandlerConfig{
		Users:   userRepo,
		Devices: deviceRepo,
		Events:  eventRepo,
	})
	return &eventsOASEnv{h: h, userRepo: userRepo, deviceRepo: deviceRepo, eventRepo: eventRepo, user: user, device: device}
}

func (e *eventsOASEnv) userCtx() context.Context {
	return api.ContextWithUser(context.Background(), e.user)
}

// createOtherUser inserts a second user with no access to env.device.
func (e *eventsOASEnv) createOtherUser(t *testing.T) *model.User {
	t.Helper()
	other := &model.User{Email: "events-oas-other@example.com", PasswordHash: "$2a$10$hash", Name: "Other"}
	if err := e.userRepo.Create(context.Background(), other); err != nil {
		t.Fatalf("create other user: %v", err)
	}
	return other
}

// ---------------------------------------------------------------------------
// ListEvents
// ---------------------------------------------------------------------------

func TestListEvents_AllUserEvents_Integration(t *testing.T) {
	env := setupEventsOASIntegration(t)
	ctx := context.Background()

	// Timestamps must lie in the past: the default ListEvents window is
	// [now-24h, now], so future-stamped events are excluded.
	now := time.Now().UTC()
	_ = env.eventRepo.Create(ctx, &model.Event{DeviceID: env.device.ID, Type: "geofenceEnter", Timestamp: now.Add(-2 * time.Minute)})
	_ = env.eventRepo.Create(ctx, &model.Event{DeviceID: env.device.ID, Type: "geofenceExit", Timestamp: now.Add(-time.Minute)})

	res, err := env.h.ListEvents(env.userCtx(), oas.ListEventsParams{})
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	list, ok := res.(*oas.ListEventsOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.ListEventsOKApplicationJSON, got %T", res)
	}
	if len(*list) != 2 {
		t.Errorf("expected 2 events, got %d", len(*list))
	}
}

func TestListEvents_ByDevice_Integration(t *testing.T) {
	env := setupEventsOASIntegration(t)
	ctx := context.Background()

	now := time.Now().UTC()
	_ = env.eventRepo.Create(ctx, &model.Event{DeviceID: env.device.ID, Type: "deviceOnline", Timestamp: now})

	res, err := env.h.ListEvents(env.userCtx(), oas.ListEventsParams{
		DeviceId: oas.NewOptInt64(env.device.ID),
	})
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	list, ok := res.(*oas.ListEventsOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.ListEventsOKApplicationJSON, got %T", res)
	}
	if len(*list) != 1 {
		t.Errorf("expected 1 event, got %d", len(*list))
	}
	if len(*list) == 1 && (*list)[0].DeviceId != env.device.ID {
		t.Errorf("expected event for device %d, got %d", env.device.ID, (*list)[0].DeviceId)
	}
}

// TestListEvents_DeviceForbidden_Integration verifies the IDOR protection:
// a user must not be able to query another user's device events.
func TestListEvents_DeviceForbidden_Integration(t *testing.T) {
	env := setupEventsOASIntegration(t)
	other := env.createOtherUser(t)

	res, err := env.h.ListEvents(api.ContextWithUser(context.Background(), other), oas.ListEventsParams{
		DeviceId: oas.NewOptInt64(env.device.ID),
	})
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	errRes, ok := res.(*oas.Error)
	if !ok {
		t.Fatalf("expected *oas.Error for foreign device, got %T", res)
	}
	if errRes.Error != "access denied" {
		t.Errorf("unexpected error message: %q", errRes.Error)
	}
}

func TestListEvents_Empty_Integration(t *testing.T) {
	env := setupEventsOASIntegration(t)

	res, err := env.h.ListEvents(env.userCtx(), oas.ListEventsParams{})
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	list, ok := res.(*oas.ListEventsOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.ListEventsOKApplicationJSON, got %T", res)
	}
	if len(*list) != 0 {
		t.Errorf("expected 0 events, got %d", len(*list))
	}
}

// ---------------------------------------------------------------------------
// ReportEvents (Traccar-compat alias; ported from reports_test.go)
// ---------------------------------------------------------------------------

func TestReportEvents_NoFilters_Integration(t *testing.T) {
	env := setupEventsOASIntegration(t)
	ctx := context.Background()

	now := time.Now().UTC()
	_ = env.eventRepo.Create(ctx, &model.Event{DeviceID: env.device.ID, Type: "deviceOnline", Timestamp: now})

	res, err := env.h.ReportEvents(env.userCtx(), oas.ReportEventsParams{})
	if err != nil {
		t.Fatalf("ReportEvents returned error: %v", err)
	}
	list, ok := res.(*oas.ReportEventsOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.ReportEventsOKApplicationJSON, got %T", res)
	}
	if len(*list) != 1 {
		t.Errorf("expected 1 event, got %d", len(*list))
	}
}

func TestReportEvents_DeviceIDFilter_Integration(t *testing.T) {
	env := setupEventsOASIntegration(t)
	ctx := context.Background()

	d2 := &model.Device{UniqueID: "events-oas-dev-2", Name: "D2", Status: "online"}
	if err := env.deviceRepo.Create(ctx, d2, env.user.ID); err != nil {
		t.Fatalf("create second device: %v", err)
	}

	now := time.Now().UTC()
	_ = env.eventRepo.Create(ctx, &model.Event{DeviceID: env.device.ID, Type: "deviceOnline", Timestamp: now})
	_ = env.eventRepo.Create(ctx, &model.Event{DeviceID: d2.ID, Type: "deviceOnline", Timestamp: now})

	res, err := env.h.ReportEvents(env.userCtx(), oas.ReportEventsParams{
		DeviceId: oas.NewOptInt64(env.device.ID),
	})
	if err != nil {
		t.Fatalf("ReportEvents returned error: %v", err)
	}
	list, ok := res.(*oas.ReportEventsOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.ReportEventsOKApplicationJSON, got %T", res)
	}
	if len(*list) != 1 {
		t.Errorf("expected 1 event for filtered device, got %d", len(*list))
	}
	if len(*list) == 1 && (*list)[0].DeviceId != env.device.ID {
		t.Errorf("expected event for device %d, got %d", env.device.ID, (*list)[0].DeviceId)
	}
}

func TestReportEvents_TypeFilter_Integration(t *testing.T) {
	env := setupEventsOASIntegration(t)
	ctx := context.Background()

	now := time.Now().UTC()
	_ = env.eventRepo.Create(ctx, &model.Event{DeviceID: env.device.ID, Type: "deviceOnline", Timestamp: now.Add(-2 * time.Minute)})
	_ = env.eventRepo.Create(ctx, &model.Event{DeviceID: env.device.ID, Type: "deviceOffline", Timestamp: now.Add(-time.Minute)})

	res, err := env.h.ReportEvents(env.userCtx(), oas.ReportEventsParams{
		Type: oas.NewOptString("deviceOnline"),
	})
	if err != nil {
		t.Fatalf("ReportEvents returned error: %v", err)
	}
	list, ok := res.(*oas.ReportEventsOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.ReportEventsOKApplicationJSON, got %T", res)
	}
	if len(*list) != 1 {
		t.Fatalf("expected 1 deviceOnline event, got %d", len(*list))
	}
	if (*list)[0].Type != "deviceOnline" {
		t.Errorf("expected type 'deviceOnline', got %q", (*list)[0].Type)
	}
}

func TestReportEvents_CustomTimeRange_Integration(t *testing.T) {
	env := setupEventsOASIntegration(t)
	ctx := context.Background()

	now := time.Now().UTC()
	_ = env.eventRepo.Create(ctx, &model.Event{DeviceID: env.device.ID, Type: "deviceOnline", Timestamp: now})

	res, err := env.h.ReportEvents(env.userCtx(), oas.ReportEventsParams{
		From: oas.NewOptDateTime(now.Add(-time.Hour)),
		To:   oas.NewOptDateTime(now.Add(time.Hour)),
	})
	if err != nil {
		t.Fatalf("ReportEvents returned error: %v", err)
	}
	list, ok := res.(*oas.ReportEventsOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.ReportEventsOKApplicationJSON, got %T", res)
	}
	if len(*list) != 1 {
		t.Errorf("expected 1 event in custom range, got %d", len(*list))
	}

	// A range entirely in the past must exclude the event.
	res, err = env.h.ReportEvents(env.userCtx(), oas.ReportEventsParams{
		From: oas.NewOptDateTime(now.Add(-3 * time.Hour)),
		To:   oas.NewOptDateTime(now.Add(-2 * time.Hour)),
	})
	if err != nil {
		t.Fatalf("ReportEvents returned error: %v", err)
	}
	list, ok = res.(*oas.ReportEventsOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.ReportEventsOKApplicationJSON, got %T", res)
	}
	if len(*list) != 0 {
		t.Errorf("expected 0 events outside range, got %d", len(*list))
	}
}

func TestReportEvents_AccessDenied_Integration(t *testing.T) {
	env := setupEventsOASIntegration(t)
	other := env.createOtherUser(t)

	res, err := env.h.ReportEvents(api.ContextWithUser(context.Background(), other), oas.ReportEventsParams{
		DeviceId: oas.NewOptInt64(env.device.ID),
	})
	if err != nil {
		t.Fatalf("ReportEvents returned error: %v", err)
	}
	errRes, ok := res.(*oas.Error)
	if !ok {
		t.Fatalf("expected *oas.Error for foreign device, got %T", res)
	}
	if errRes.Error != "access denied" {
		t.Errorf("unexpected error message: %q", errRes.Error)
	}
}

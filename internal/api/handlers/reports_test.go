package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func TestReportEvents_Unauthenticated(t *testing.T) {
	h := handlers.NewReportsHandler(
		repository.NewEventRepository(nil),
		repository.NewDeviceRepository(nil),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/reports/events", nil)
	rr := httptest.NewRecorder()
	h.GetEvents(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

func TestReportEvents_Authenticated_DefaultParams(t *testing.T) {
	// This test verifies that the handler accepts an authenticated user
	// and does not reject the request as unauthorized. With a nil pool
	// the DB call will panic, so we recover and just verify it got past auth.
	h := handlers.NewReportsHandler(
		repository.NewEventRepository(nil),
		repository.NewDeviceRepository(nil),
	)

	user := &model.User{ID: 1, Email: "test@example.com", Name: "Test"}
	req := httptest.NewRequest(http.MethodGet, "/api/reports/events", nil)
	ctx := api.ContextWithUser(req.Context(), user)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		h.GetEvents(rr, req)
	}()

	// Either the handler panicked (nil pool, past auth) or returned a status.
	if !panicked && rr.Code == http.StatusUnauthorized {
		t.Error("expected authenticated request not to be rejected as unauthorized")
	}
}

func setupReportsHandler(t *testing.T) (*handlers.ReportsHandler, *repository.EventRepository, *repository.DeviceRepository, *model.User) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	eventRepo := repository.NewEventRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)

	user := &model.User{Email: "reporthandler@example.com", PasswordHash: "$2a$10$hash", Name: "Report Handler"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	h := handlers.NewReportsHandler(eventRepo, deviceRepo)
	return h, eventRepo, deviceRepo, user
}

func TestReportEvents_Integration_NoFilters(t *testing.T) {
	h, eventRepo, deviceRepo, user := setupReportsHandler(t)
	ctx := context.Background()

	device := &model.Device{UniqueID: "rpt-dev-1", Name: "Report Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	now := time.Now().UTC()
	_ = eventRepo.Create(ctx, &model.Event{DeviceID: device.ID, Type: "deviceOnline", Timestamp: now})

	req := httptest.NewRequest(http.MethodGet, "/api/reports/events", nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.GetEvents(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var events []model.Event
	_ = json.NewDecoder(rr.Body).Decode(&events)
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestReportEvents_Integration_WithDeviceIDFilter(t *testing.T) {
	h, eventRepo, deviceRepo, user := setupReportsHandler(t)
	ctx := context.Background()

	d1 := &model.Device{UniqueID: "rpt-dev-2", Name: "D1", Status: "online"}
	d2 := &model.Device{UniqueID: "rpt-dev-3", Name: "D2", Status: "online"}
	_ = deviceRepo.Create(ctx, d1, user.ID)
	_ = deviceRepo.Create(ctx, d2, user.ID)

	now := time.Now().UTC()
	_ = eventRepo.Create(ctx, &model.Event{DeviceID: d1.ID, Type: "deviceOnline", Timestamp: now})
	_ = eventRepo.Create(ctx, &model.Event{DeviceID: d2.ID, Type: "deviceOnline", Timestamp: now})

	// Filter by d1 only using the array form.
	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/reports/events?deviceId[]=%d", d1.ID), nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.GetEvents(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var events []model.Event
	_ = json.NewDecoder(rr.Body).Decode(&events)
	if len(events) != 1 {
		t.Errorf("expected 1 event for d1, got %d", len(events))
	}
}

func TestReportEvents_Integration_WithTypeFilter(t *testing.T) {
	h, eventRepo, deviceRepo, user := setupReportsHandler(t)
	ctx := context.Background()

	device := &model.Device{UniqueID: "rpt-dev-4", Name: "D", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	now := time.Now().UTC()
	_ = eventRepo.Create(ctx, &model.Event{DeviceID: device.ID, Type: "deviceOnline", Timestamp: now})
	_ = eventRepo.Create(ctx, &model.Event{DeviceID: device.ID, Type: "deviceOffline", Timestamp: now.Add(time.Minute)})

	// Filter by type using the single and array form.
	req := httptest.NewRequest(http.MethodGet,
		"/api/reports/events?type[]=deviceOnline&type=deviceOffline", nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.GetEvents(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestReportEvents_Integration_WithCustomTimeRange(t *testing.T) {
	h, eventRepo, deviceRepo, user := setupReportsHandler(t)
	ctx := context.Background()

	device := &model.Device{UniqueID: "rpt-dev-5", Name: "D", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	now := time.Now().UTC()
	_ = eventRepo.Create(ctx, &model.Event{DeviceID: device.ID, Type: "deviceOnline", Timestamp: now})

	from := now.Add(-time.Hour).Format(time.RFC3339)
	to := now.Add(time.Hour).Format(time.RFC3339)
	url := fmt.Sprintf("/api/reports/events?from=%s&to=%s", from, to)

	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.GetEvents(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestReportEvents_Integration_AccessDenied(t *testing.T) {
	h, _, deviceRepo, user := setupReportsHandler(t)
	ctx := context.Background()

	// Create device owned by different user.
	pool := testutil.SetupTestDB(t)
	userRepo := repository.NewUserRepository(pool)
	otherUser := &model.User{Email: "rptother@example.com", PasswordHash: "$2a$10$hash", Name: "Other"}
	_ = userRepo.Create(ctx, otherUser)
	device := &model.Device{UniqueID: "rpt-other-dev", Name: "Other Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, otherUser.ID)

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/reports/events?deviceId=%d", device.ID), nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.GetEvents(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestGetServer_IsPublicRoute(t *testing.T) {
	h := handlers.NewServerHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/server", nil)
	rr := httptest.NewRecorder()
	h.GetServer(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var resp map[string]interface{}
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	// Verify critical fields for pytraccar.
	if _, ok := resp["id"]; !ok {
		t.Error("missing 'id' field in server response")
	}
	if _, ok := resp["version"]; !ok {
		t.Error("missing 'version' field in server response")
	}
}

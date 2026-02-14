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

func setupEventHandler(t *testing.T) (*handlers.EventHandler, *repository.EventRepository, *repository.DeviceRepository, *model.User, *model.Device) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	eventRepo := repository.NewEventRepository(pool)

	user := &model.User{Email: "evthandler@example.com", PasswordHash: "$2a$10$hash", Name: "Evt Handler"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	device := &model.Device{UniqueID: "evt-handler-dev", Name: "Evt Device", Status: "online"}
	if err := deviceRepo.Create(context.Background(), device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	h := handlers.NewEventHandler(eventRepo, deviceRepo)
	return h, eventRepo, deviceRepo, user, device
}

func TestEventHandler_List_AllUserEvents(t *testing.T) {
	h, eventRepo, _, user, device := setupEventHandler(t)
	ctx := context.Background()

	now := time.Now().UTC()
	_ = eventRepo.Create(ctx, &model.Event{DeviceID: device.ID, Type: "geofenceEnter", Timestamp: now})
	_ = eventRepo.Create(ctx, &model.Event{DeviceID: device.ID, Type: "geofenceExit", Timestamp: now.Add(time.Minute)})

	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var events []*model.Event
	_ = json.NewDecoder(rr.Body).Decode(&events)
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestEventHandler_List_ByDevice(t *testing.T) {
	h, eventRepo, _, user, device := setupEventHandler(t)
	ctx := context.Background()

	now := time.Now().UTC()
	_ = eventRepo.Create(ctx, &model.Event{DeviceID: device.ID, Type: "deviceOnline", Timestamp: now})

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/events?deviceId=%d", device.ID), nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var events []*model.Event
	_ = json.NewDecoder(rr.Body).Decode(&events)
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestEventHandler_List_InvalidDeviceID(t *testing.T) {
	h, _, _, user, _ := setupEventHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/events?deviceId=abc", nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestEventHandler_List_DeviceForbidden(t *testing.T) {
	h, _, _, user, device := setupEventHandler(t)

	otherUser := &model.User{ID: user.ID + 999, Email: "other@example.com", Name: "Other"}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/events?deviceId=%d", device.ID), nil)
	ctx := api.ContextWithUser(req.Context(), otherUser)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestEventHandler_List_Empty(t *testing.T) {
	h, _, _, user, _ := setupEventHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var events []*model.Event
	_ = json.NewDecoder(rr.Body).Decode(&events)
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

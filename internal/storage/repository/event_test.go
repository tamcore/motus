package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func TestEventRepository_Create(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	eventRepo := repository.NewEventRepository(pool)
	ctx := context.Background()

	user := testutil.CreateUser(t, "event-1@example.com")
	device := testutil.CreateDevice(t, user.ID, "event-dev-"+time.Now().Format("150405.000"))

	geoID := int64(0) // We'll use a nil geofence for this basic test.
	event := &model.Event{
		DeviceID:  device.ID,
		Type:      "deviceOnline",
		Timestamp: time.Now().UTC(),
		Attributes: map[string]any{
			"source": "test",
		},
	}

	// GeofenceID and PositionID are nullable; leave them nil.
	_ = geoID

	if err := eventRepo.Create(ctx, event); err != nil {
		t.Fatalf("Create event failed: %v", err)
	}
	if event.ID == 0 {
		t.Error("expected event ID to be set")
	}
}

func TestEventRepository_GetRecentByDeviceAndType(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	eventRepo := repository.NewEventRepository(pool)
	ctx := context.Background()

	user := testutil.CreateUser(t, "event-2@example.com")
	device := testutil.CreateDevice(t, user.ID, "evttype-"+time.Now().Format("150405.000"))

	now := time.Now().UTC()

	// Create events of different types.
	_ = eventRepo.Create(ctx, &model.Event{DeviceID: device.ID, Type: "deviceIdle", Timestamp: now.Add(-2 * time.Minute)})
	_ = eventRepo.Create(ctx, &model.Event{DeviceID: device.ID, Type: "motion", Timestamp: now.Add(-1 * time.Minute)})
	_ = eventRepo.Create(ctx, &model.Event{DeviceID: device.ID, Type: "deviceIdle", Timestamp: now})

	// Should only return deviceIdle events, limited to 1.
	events, err := eventRepo.GetRecentByDeviceAndType(ctx, device.ID, "deviceIdle", 1)
	if err != nil {
		t.Fatalf("GetRecentByDeviceAndType failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "deviceIdle" {
		t.Errorf("expected type 'deviceIdle', got %q", events[0].Type)
	}

	// All deviceIdle events.
	events, err = eventRepo.GetRecentByDeviceAndType(ctx, device.ID, "deviceIdle", 10)
	if err != nil {
		t.Fatalf("GetRecentByDeviceAndType failed: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 deviceIdle events, got %d", len(events))
	}

	// No motion events expected beyond the one we created.
	events, err = eventRepo.GetRecentByDeviceAndType(ctx, device.ID, "motion", 10)
	if err != nil {
		t.Fatalf("GetRecentByDeviceAndType failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 motion event, got %d", len(events))
	}

	// Non-existent (in this test) type returns empty.
	events, err = eventRepo.GetRecentByDeviceAndType(ctx, device.ID, "geofenceEnter", 10)
	if err != nil {
		t.Fatalf("GetRecentByDeviceAndType failed: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events for non-existent type, got %d", len(events))
	}
}

func TestEventRepository_GetByFilters(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	eventRepo := repository.NewEventRepository(pool)
	ctx := context.Background()

	user := testutil.CreateUser(t, "event-3@example.com")
	d1 := testutil.CreateDevice(t, user.ID, "filt-1-"+time.Now().Format("150405.000"))
	d2 := testutil.CreateDevice(t, user.ID, "filt-2-"+time.Now().Format("150405.000"))

	now := time.Now().UTC()
	from := now.Add(-time.Hour)
	to := now.Add(time.Hour)

	_ = eventRepo.Create(ctx, &model.Event{DeviceID: d1.ID, Type: "geofenceEnter", Timestamp: now.Add(-30 * time.Minute)})
	_ = eventRepo.Create(ctx, &model.Event{DeviceID: d1.ID, Type: "deviceIdle", Timestamp: now})
	_ = eventRepo.Create(ctx, &model.Event{DeviceID: d2.ID, Type: "geofenceEnter", Timestamp: now.Add(-10 * time.Minute)})

	// No filters — all 3 events for this user.
	all, err := eventRepo.GetByFilters(ctx, user.ID, nil, nil, from, to)
	if err != nil {
		t.Fatalf("GetByFilters (no filters) failed: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 events, got %d", len(all))
	}

	// Filter by device d1 only — 2 events.
	byDevice, err := eventRepo.GetByFilters(ctx, user.ID, []int64{d1.ID}, nil, from, to)
	if err != nil {
		t.Fatalf("GetByFilters (by device) failed: %v", err)
	}
	if len(byDevice) != 2 {
		t.Errorf("expected 2 events for d1, got %d", len(byDevice))
	}

	// Filter by event type — 2 geofenceEnter events.
	byType, err := eventRepo.GetByFilters(ctx, user.ID, nil, []string{"geofenceEnter"}, from, to)
	if err != nil {
		t.Fatalf("GetByFilters (by type) failed: %v", err)
	}
	if len(byType) != 2 {
		t.Errorf("expected 2 geofenceEnter events, got %d", len(byType))
	}

	// Combined filter: d1 + geofenceEnter → 1 event.
	combined, err := eventRepo.GetByFilters(ctx, user.ID, []int64{d1.ID}, []string{"geofenceEnter"}, from, to)
	if err != nil {
		t.Fatalf("GetByFilters (combined) failed: %v", err)
	}
	if len(combined) != 1 {
		t.Errorf("expected 1 combined event, got %d", len(combined))
	}

	// Time range excludes all events.
	empty, err := eventRepo.GetByFilters(ctx, user.ID, nil, nil, now.Add(-3*time.Hour), now.Add(-2*time.Hour))
	if err != nil {
		t.Fatalf("GetByFilters (empty range) failed: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected 0 events outside range, got %d", len(empty))
	}
}

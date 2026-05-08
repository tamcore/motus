package services

import (
	"context"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
)

// ignitionFromAttributes is tested indirectly through CheckIgnition; we also
// test the helper directly here for completeness.

func TestIgnitionFromAttributes(t *testing.T) {
	tests := []struct {
		name      string
		attrs     map[string]interface{}
		wantVal   bool
		wantFound bool
	}{
		{"nil map", nil, false, false},
		{"empty map", map[string]interface{}{}, false, false},
		{"ignition true", map[string]interface{}{"ignition": true}, true, true},
		{"ignition false", map[string]interface{}{"ignition": false}, false, true},
		{"wrong type string", map[string]interface{}{"ignition": "true"}, false, false},
		{"wrong type int", map[string]interface{}{"ignition": 1}, false, false},
		{"other keys only", map[string]interface{}{"speed": 50.0}, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := ignitionFromAttributes(tt.attrs)
			if found != tt.wantFound {
				t.Errorf("found = %v, want %v", found, tt.wantFound)
			}
			if got != tt.wantVal {
				t.Errorf("val = %v, want %v", got, tt.wantVal)
			}
		})
	}
}

// --- CheckIgnition tests using mock DeviceRepo ---

// ignitionMockDeviceRepo tracks ignition state in-memory, simulating the DB.
type ignitionMockDeviceRepo struct {
	device *model.Device
}

func (r *ignitionMockDeviceRepo) GetByID(_ context.Context, id int64) (*model.Device, error) {
	return r.device, nil
}

func (r *ignitionMockDeviceRepo) UpdateIgnitionState(_ context.Context, _ int64, on bool, ts time.Time) error {
	r.device.IgnitionOn = on
	r.device.LastIgnitionTime = &ts
	return nil
}

// Satisfy the full DeviceRepo interface with no-ops.
func (r *ignitionMockDeviceRepo) UserHasAccess(_ context.Context, _ *model.User, _ int64) bool {
	return true
}
func (r *ignitionMockDeviceRepo) GetByUniqueID(_ context.Context, _ string) (*model.Device, error) {
	return nil, nil
}
func (r *ignitionMockDeviceRepo) GetByUser(_ context.Context, _ int64) ([]*model.Device, error) {
	return nil, nil
}
func (r *ignitionMockDeviceRepo) GetAll(_ context.Context) ([]model.Device, error) { return nil, nil }
func (r *ignitionMockDeviceRepo) GetAllWithOwners(_ context.Context) ([]model.Device, error) {
	return nil, nil
}
func (r *ignitionMockDeviceRepo) GetTimedOut(_ context.Context, _ time.Time) ([]model.Device, error) {
	return nil, nil
}
func (r *ignitionMockDeviceRepo) GetUserIDs(_ context.Context, _ int64) ([]int64, error) {
	return nil, nil
}
func (r *ignitionMockDeviceRepo) Create(_ context.Context, _ *model.Device, _ int64) error {
	return nil
}
func (r *ignitionMockDeviceRepo) Update(_ context.Context, _ *model.Device) error { return nil }
func (r *ignitionMockDeviceRepo) Delete(_ context.Context, _ int64) error         { return nil }

type ignitionMockEventRepo struct {
	created []*model.Event
}

func (r *ignitionMockEventRepo) Create(_ context.Context, event *model.Event) error {
	event.ID = int64(len(r.created) + 1)
	r.created = append(r.created, event)
	return nil
}
func (r *ignitionMockEventRepo) GetByDevice(_ context.Context, _ int64, _ int) ([]*model.Event, error) {
	return nil, nil
}
func (r *ignitionMockEventRepo) GetRecentByDeviceAndType(_ context.Context, _ int64, _ string, _ int) ([]*model.Event, error) {
	return nil, nil
}
func (r *ignitionMockEventRepo) GetByUser(_ context.Context, _ int64, _ int) ([]*model.Event, error) {
	return nil, nil
}
func (r *ignitionMockEventRepo) GetByFilters(_ context.Context, _ int64, _ []int64, _ []string, _, _ time.Time) ([]*model.Event, error) {
	return nil, nil
}

func makeIgnitionPos(deviceID int64, ignition bool, ts time.Time) *model.Position {
	id := ts.Unix()
	return &model.Position{
		ID:        id,
		DeviceID:  deviceID,
		Timestamp: ts,
		Attributes: map[string]interface{}{
			"ignition": ignition,
		},
	}
}

func newIgnitionDevice(ignOn bool, lastIgnTime *time.Time) *model.Device {
	return &model.Device{
		ID:               1,
		UniqueID:         "test-ign",
		Name:             "Test",
		Status:           "online",
		IgnitionOn:       ignOn,
		LastIgnitionTime: lastIgnTime,
	}
}

func TestCheckIgnition_NoEvent_WhenNoChange(t *testing.T) {
	ts := time.Now().Add(-5 * time.Second)
	devRepo := &ignitionMockDeviceRepo{device: newIgnitionDevice(true, &ts)}
	evRepo := &ignitionMockEventRepo{}
	svc := NewIgnitionService(devRepo, evRepo, nil, nil)

	curr := makeIgnitionPos(1, true, time.Now())
	if err := svc.CheckIgnition(context.Background(), curr); err != nil {
		t.Fatal(err)
	}
	if len(evRepo.created) != 0 {
		t.Errorf("expected no events when ignition unchanged, got %d", len(evRepo.created))
	}
}

func TestCheckIgnition_EmitsIgnitionOff(t *testing.T) {
	ts := time.Now().Add(-5 * time.Second)
	devRepo := &ignitionMockDeviceRepo{device: newIgnitionDevice(true, &ts)}
	evRepo := &ignitionMockEventRepo{}
	svc := NewIgnitionService(devRepo, evRepo, nil, nil)

	curr := makeIgnitionPos(1, false, time.Now())
	if err := svc.CheckIgnition(context.Background(), curr); err != nil {
		t.Fatal(err)
	}
	if len(evRepo.created) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evRepo.created))
	}
	if evRepo.created[0].Type != "ignitionOff" {
		t.Errorf("event type = %q, want %q", evRepo.created[0].Type, "ignitionOff")
	}
}

func TestCheckIgnition_EmitsIgnitionOn(t *testing.T) {
	devRepo := &ignitionMockDeviceRepo{device: newIgnitionDevice(false, nil)}
	evRepo := &ignitionMockEventRepo{}
	svc := NewIgnitionService(devRepo, evRepo, nil, nil)

	curr := makeIgnitionPos(1, true, time.Now())
	if err := svc.CheckIgnition(context.Background(), curr); err != nil {
		t.Fatal(err)
	}
	if len(evRepo.created) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evRepo.created))
	}
	if evRepo.created[0].Type != "ignitionOn" {
		t.Errorf("event type = %q, want %q", evRepo.created[0].Type, "ignitionOn")
	}
}

func TestCheckIgnition_SkipsWhenNoIgnitionAttribute(t *testing.T) {
	devRepo := &ignitionMockDeviceRepo{device: newIgnitionDevice(true, nil)}
	evRepo := &ignitionMockEventRepo{}
	svc := NewIgnitionService(devRepo, evRepo, nil, nil)

	// Position without "ignition" attribute (e.g. WATCH protocol).
	curr := &model.Position{
		ID:         99,
		DeviceID:   1,
		Timestamp:  time.Now(),
		Attributes: map[string]interface{}{"speed": 30.0},
	}
	if err := svc.CheckIgnition(context.Background(), curr); err != nil {
		t.Fatal(err)
	}
	if len(evRepo.created) != 0 {
		t.Errorf("expected no events for non-H02 position, got %d", len(evRepo.created))
	}
}

func TestCheckIgnition_EventCarriesPositionID(t *testing.T) {
	ts := time.Now().Add(-5 * time.Second)
	devRepo := &ignitionMockDeviceRepo{device: newIgnitionDevice(true, &ts)}
	evRepo := &ignitionMockEventRepo{}
	svc := NewIgnitionService(devRepo, evRepo, nil, nil)

	curr := makeIgnitionPos(1, false, time.Now())
	curr.ID = 42
	if err := svc.CheckIgnition(context.Background(), curr); err != nil {
		t.Fatal(err)
	}
	if len(evRepo.created) != 1 {
		t.Fatalf("expected 1 event")
	}
	ev := evRepo.created[0]
	if ev.PositionID == nil || *ev.PositionID != 42 {
		t.Errorf("PositionID = %v, want 42", ev.PositionID)
	}
	if ev.DeviceID != 1 {
		t.Errorf("DeviceID = %d, want 1", ev.DeviceID)
	}
}

// TestCheckIgnition_OutOfOrderPositions_SingleEvent verifies that out-of-order
// positions arriving from the H02 tracker produce only one ignition event.
// This is the core bug fix: the old position-comparison approach created one
// event per out-of-order position.
func TestCheckIgnition_OutOfOrderPositions_SingleEvent(t *testing.T) {
	devRepo := &ignitionMockDeviceRepo{device: newIgnitionDevice(false, nil)}
	evRepo := &ignitionMockEventRepo{}
	svc := NewIgnitionService(devRepo, evRepo, nil, nil)

	ctx := context.Background()
	now := time.Now().UTC()

	// Simulate 3 positions arriving out of order (like the real Kuga data):
	// insertion order: ts=now, ts=now-20s, ts=now-4min — all ignition=true
	positions := []*model.Position{
		makeIgnitionPos(1, true, now),
		makeIgnitionPos(1, true, now.Add(-20*time.Second)),
		makeIgnitionPos(1, true, now.Add(-4*time.Minute)),
	}

	for _, pos := range positions {
		if err := svc.CheckIgnition(ctx, pos); err != nil {
			t.Fatalf("CheckIgnition failed: %v", err)
		}
	}

	if len(evRepo.created) != 1 {
		t.Errorf("expected exactly 1 ignitionOn event from out-of-order positions, got %d", len(evRepo.created))
	}
	if len(evRepo.created) > 0 && evRepo.created[0].Type != "ignitionOn" {
		t.Errorf("event type = %q, want %q", evRepo.created[0].Type, "ignitionOn")
	}
}

// TestCheckIgnition_DuplicateTimestamp_SingleEvent verifies that two positions
// with the exact same device timestamp (common with H02 sending duplicates)
// produce only one ignition event.
func TestCheckIgnition_DuplicateTimestamp_SingleEvent(t *testing.T) {
	ts := time.Now().Add(-10 * time.Second)
	devRepo := &ignitionMockDeviceRepo{device: newIgnitionDevice(true, &ts)}
	evRepo := &ignitionMockEventRepo{}
	svc := NewIgnitionService(devRepo, evRepo, nil, nil)

	ctx := context.Background()
	dupTS := time.Now().UTC()

	pos1 := makeIgnitionPos(1, false, dupTS)
	pos2 := makeIgnitionPos(1, false, dupTS)

	if err := svc.CheckIgnition(ctx, pos1); err != nil {
		t.Fatal(err)
	}
	if err := svc.CheckIgnition(ctx, pos2); err != nil {
		t.Fatal(err)
	}

	if len(evRepo.created) != 1 {
		t.Errorf("expected exactly 1 ignitionOff from duplicate timestamps, got %d", len(evRepo.created))
	}
}

// TestCheckIgnition_OutOfOrderSkipped verifies that a position with an older
// timestamp than the last state change is silently skipped.
func TestCheckIgnition_OutOfOrderSkipped(t *testing.T) {
	now := time.Now().UTC()
	lastIgn := now // device ignition changed at "now"
	devRepo := &ignitionMockDeviceRepo{device: newIgnitionDevice(true, &lastIgn)}
	evRepo := &ignitionMockEventRepo{}
	svc := NewIgnitionService(devRepo, evRepo, nil, nil)

	// Position with ignition=false but older timestamp → should be skipped
	oldPos := makeIgnitionPos(1, false, now.Add(-30*time.Second))
	if err := svc.CheckIgnition(context.Background(), oldPos); err != nil {
		t.Fatal(err)
	}

	if len(evRepo.created) != 0 {
		t.Errorf("expected no events for out-of-order position, got %d", len(evRepo.created))
	}
	// Device state should remain unchanged
	if !devRepo.device.IgnitionOn {
		t.Error("device ignition_on should still be true")
	}
}

// TestCheckIgnition_NormalOnOffCycle verifies a complete on→off→on cycle.
func TestCheckIgnition_NormalOnOffCycle(t *testing.T) {
	devRepo := &ignitionMockDeviceRepo{device: newIgnitionDevice(false, nil)}
	evRepo := &ignitionMockEventRepo{}
	svc := NewIgnitionService(devRepo, evRepo, nil, nil)

	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Ignition on
	pos1 := makeIgnitionPos(1, true, now)
	if err := svc.CheckIgnition(ctx, pos1); err != nil {
		t.Fatal(err)
	}
	if len(evRepo.created) != 1 || evRepo.created[0].Type != "ignitionOn" {
		t.Fatalf("expected 1 ignitionOn event, got %d events", len(evRepo.created))
	}

	// 2. Still on — no event
	pos2 := makeIgnitionPos(1, true, now.Add(10*time.Second))
	if err := svc.CheckIgnition(ctx, pos2); err != nil {
		t.Fatal(err)
	}
	if len(evRepo.created) != 1 {
		t.Fatalf("expected still 1 event after same-state position, got %d", len(evRepo.created))
	}

	// 3. Ignition off
	pos3 := makeIgnitionPos(1, false, now.Add(5*time.Minute))
	if err := svc.CheckIgnition(ctx, pos3); err != nil {
		t.Fatal(err)
	}
	if len(evRepo.created) != 2 || evRepo.created[1].Type != "ignitionOff" {
		t.Fatalf("expected 2 events (on, off), got %d", len(evRepo.created))
	}

	// 4. Ignition on again
	pos4 := makeIgnitionPos(1, true, now.Add(10*time.Minute))
	if err := svc.CheckIgnition(ctx, pos4); err != nil {
		t.Fatal(err)
	}
	if len(evRepo.created) != 3 || evRepo.created[2].Type != "ignitionOn" {
		t.Fatalf("expected 3 events (on, off, on), got %d", len(evRepo.created))
	}
}

// TestCheckIgnition_FirstPositionWithIgnitionOn fires an event when the
// device has never had ignition state set (LastIgnitionTime is nil).
func TestCheckIgnition_FirstPositionWithIgnitionOn(t *testing.T) {
	devRepo := &ignitionMockDeviceRepo{device: newIgnitionDevice(false, nil)}
	evRepo := &ignitionMockEventRepo{}
	svc := NewIgnitionService(devRepo, evRepo, nil, nil)

	curr := makeIgnitionPos(1, true, time.Now())
	if err := svc.CheckIgnition(context.Background(), curr); err != nil {
		t.Fatal(err)
	}
	if len(evRepo.created) != 1 {
		t.Fatalf("expected 1 event for first ignition, got %d", len(evRepo.created))
	}
	if evRepo.created[0].Type != "ignitionOn" {
		t.Errorf("event type = %q, want %q", evRepo.created[0].Type, "ignitionOn")
	}
}

// TestCheckIgnition_FirstPositionIgnitionOff_NoEvent verifies that a device
// with no prior ignition state receiving ignition=false does not fire an event.
func TestCheckIgnition_FirstPositionIgnitionOff_NoEvent(t *testing.T) {
	devRepo := &ignitionMockDeviceRepo{device: newIgnitionDevice(false, nil)}
	evRepo := &ignitionMockEventRepo{}
	svc := NewIgnitionService(devRepo, evRepo, nil, nil)

	curr := makeIgnitionPos(1, false, time.Now())
	if err := svc.CheckIgnition(context.Background(), curr); err != nil {
		t.Fatal(err)
	}
	if len(evRepo.created) != 0 {
		t.Errorf("expected no events for first position with ignition off, got %d", len(evRepo.created))
	}
}

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

// --- CheckIgnition integration tests using stub repos ---

type ignitionMockPositionRepo struct {
	prev *model.Position
}

func (r *ignitionMockPositionRepo) GetPreviousByDevice(_ context.Context, _ int64, _ time.Time) (*model.Position, error) {
	return r.prev, nil
}

// Satisfy the full PositionRepo interface with no-ops.
func (r *ignitionMockPositionRepo) Create(_ context.Context, _ *model.Position) error { return nil }
func (r *ignitionMockPositionRepo) GetByID(_ context.Context, _ int64) (*model.Position, error) {
	return nil, nil
}
func (r *ignitionMockPositionRepo) GetByIDs(_ context.Context, _ []int64) ([]*model.Position, error) {
	return nil, nil
}
func (r *ignitionMockPositionRepo) GetLatestByDevice(_ context.Context, _ int64) (*model.Position, error) {
	return nil, nil
}
func (r *ignitionMockPositionRepo) GetLatestByUser(_ context.Context, _ int64) ([]*model.Position, error) {
	return nil, nil
}
func (r *ignitionMockPositionRepo) UpdateGeofenceIDs(_ context.Context, _ int64, _ []int64) error {
	return nil
}
func (r *ignitionMockPositionRepo) GetByDeviceAndTimeRange(_ context.Context, _ int64, _, _ time.Time, _ int) ([]*model.Position, error) {
	return nil, nil
}
func (r *ignitionMockPositionRepo) UpdateAddress(_ context.Context, _ int64, _ string) error {
	return nil
}
func (r *ignitionMockPositionRepo) GetLatestAll(_ context.Context) ([]*model.Position, error) {
	return nil, nil
}
func (r *ignitionMockPositionRepo) GetLastMovingPosition(_ context.Context, _ int64, _ float64) (*model.Position, error) {
	return nil, nil
}

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

func TestCheckIgnition_NoEvent_WhenNoChange(t *testing.T) {
	posRepo := &ignitionMockPositionRepo{
		prev: makeIgnitionPos(1, true, time.Now().Add(-5*time.Second)),
	}
	evRepo := &ignitionMockEventRepo{}
	svc := NewIgnitionService(posRepo, evRepo, nil, nil)

	curr := makeIgnitionPos(1, true, time.Now())
	if err := svc.CheckIgnition(context.Background(), curr); err != nil {
		t.Fatal(err)
	}
	if len(evRepo.created) != 0 {
		t.Errorf("expected no events when ignition unchanged, got %d", len(evRepo.created))
	}
}

func TestCheckIgnition_EmitsIgnitionOff(t *testing.T) {
	posRepo := &ignitionMockPositionRepo{
		prev: makeIgnitionPos(1, true, time.Now().Add(-5*time.Second)),
	}
	evRepo := &ignitionMockEventRepo{}
	svc := NewIgnitionService(posRepo, evRepo, nil, nil)

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
	posRepo := &ignitionMockPositionRepo{
		prev: makeIgnitionPos(1, false, time.Now().Add(-5*time.Second)),
	}
	evRepo := &ignitionMockEventRepo{}
	svc := NewIgnitionService(posRepo, evRepo, nil, nil)

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
	posRepo := &ignitionMockPositionRepo{
		prev: makeIgnitionPos(1, true, time.Now().Add(-5*time.Second)),
	}
	evRepo := &ignitionMockEventRepo{}
	svc := NewIgnitionService(posRepo, evRepo, nil, nil)

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

func TestCheckIgnition_SkipsWhenNoPreviousPosition(t *testing.T) {
	posRepo := &ignitionMockPositionRepo{prev: nil}
	evRepo := &ignitionMockEventRepo{}
	svc := NewIgnitionService(posRepo, evRepo, nil, nil)

	curr := makeIgnitionPos(1, false, time.Now())
	if err := svc.CheckIgnition(context.Background(), curr); err != nil {
		t.Fatal(err)
	}
	if len(evRepo.created) != 0 {
		t.Errorf("expected no events for first position, got %d", len(evRepo.created))
	}
}

func TestCheckIgnition_SkipsWhenPreviousHasNoIgnitionAttribute(t *testing.T) {
	prev := &model.Position{
		ID:         1,
		DeviceID:   1,
		Timestamp:  time.Now().Add(-5 * time.Second),
		Attributes: map[string]interface{}{"speed": 10.0}, // no ignition key
	}
	posRepo := &ignitionMockPositionRepo{prev: prev}
	evRepo := &ignitionMockEventRepo{}
	svc := NewIgnitionService(posRepo, evRepo, nil, nil)

	curr := makeIgnitionPos(1, false, time.Now())
	if err := svc.CheckIgnition(context.Background(), curr); err != nil {
		t.Fatal(err)
	}
	if len(evRepo.created) != 0 {
		t.Errorf("expected no events when previous has no ignition attribute, got %d", len(evRepo.created))
	}
}

func TestCheckIgnition_EventCarriesPositionID(t *testing.T) {
	posRepo := &ignitionMockPositionRepo{
		prev: makeIgnitionPos(1, true, time.Now().Add(-5*time.Second)),
	}
	evRepo := &ignitionMockEventRepo{}
	svc := NewIgnitionService(posRepo, evRepo, nil, nil)

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

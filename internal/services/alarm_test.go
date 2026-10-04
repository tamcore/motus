package services

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
	"github.com/tamcore/motus/internal/websocket"
)

func TestAlarmFromAttributes(t *testing.T) {
	tests := []struct {
		name      string
		attrs     map[string]any
		wantVal   string
		wantFound bool
	}{
		{"nil map", nil, "", false},
		{"empty map", map[string]any{}, "", false},
		{"sos alarm", map[string]any{"alarm": "sos"}, "sos", true},
		{"powerCut alarm", map[string]any{"alarm": "powerCut"}, "powerCut", true},
		{"vibration alarm", map[string]any{"alarm": "vibration"}, "vibration", true},
		{"empty string", map[string]any{"alarm": ""}, "", false},
		{"wrong type int", map[string]any{"alarm": 1}, "", false},
		{"other keys only", map[string]any{"ignition": true}, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := alarmFromAttributes(tt.attrs)
			if found != tt.wantFound {
				t.Errorf("found = %v, want %v", found, tt.wantFound)
			}
			if got != tt.wantVal {
				t.Errorf("val = %q, want %q", got, tt.wantVal)
			}
		})
	}
}

// --- CheckAlarm integration tests using stub repos ---

type recordingEventRepo struct {
	created []*model.Event
}

func (r *recordingEventRepo) Create(_ context.Context, e *model.Event) error {
	e.ID = int64(len(r.created) + 1)
	r.created = append(r.created, e)
	return nil
}

func (r *recordingEventRepo) GetRecentByDeviceAndType(_ context.Context, _ int64, _ string, _ int) ([]*model.Event, error) {
	return nil, nil
}
func TestCheckAlarm_NoAlarmAttribute(t *testing.T) {
	repo := &recordingEventRepo{}
	svc := NewAlarmService(repo, nil, nil, nil)

	pos := &model.Position{
		ID:         1,
		DeviceID:   42,
		Timestamp:  time.Now(),
		Attributes: map[string]any{"ignition": true},
	}

	if err := svc.CheckAlarm(context.Background(), pos); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.created) != 0 {
		t.Errorf("expected no events, got %d", len(repo.created))
	}
}

func TestCheckAlarm_NilAttributes(t *testing.T) {
	repo := &recordingEventRepo{}
	svc := NewAlarmService(repo, nil, nil, nil)

	pos := &model.Position{ID: 1, DeviceID: 42, Timestamp: time.Now()}

	if err := svc.CheckAlarm(context.Background(), pos); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.created) != 0 {
		t.Errorf("expected no events, got %d", len(repo.created))
	}
}

func TestCheckAlarm_SOSAlarm(t *testing.T) {
	repo := &recordingEventRepo{}
	svc := NewAlarmService(repo, nil, nil, nil)

	pos := &model.Position{
		ID:        1,
		DeviceID:  42,
		Timestamp: time.Now(),
		Attributes: map[string]any{
			"alarm": "sos",
		},
	}

	if err := svc.CheckAlarm(context.Background(), pos); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("expected 1 event, got %d", len(repo.created))
	}

	ev := repo.created[0]
	if ev.Type != "alarm" {
		t.Errorf("event type = %q, want alarm", ev.Type)
	}
	if ev.DeviceID != 42 {
		t.Errorf("deviceID = %d, want 42", ev.DeviceID)
	}
	if ev.PositionID == nil || *ev.PositionID != 1 {
		t.Errorf("positionID = %v, want 1", ev.PositionID)
	}
	if ev.Attributes["alarm"] != "sos" {
		t.Errorf("alarm attribute = %v, want sos", ev.Attributes["alarm"])
	}
}

func TestCheckAlarm_PowerCutAlarm(t *testing.T) {
	repo := &recordingEventRepo{}
	svc := NewAlarmService(repo, nil, nil, nil)

	pos := &model.Position{
		ID:         1,
		DeviceID:   7,
		Timestamp:  time.Now(),
		Attributes: map[string]any{"alarm": "powerCut"},
	}

	if err := svc.CheckAlarm(context.Background(), pos); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("expected 1 event, got %d", len(repo.created))
	}
	if repo.created[0].Attributes["alarm"] != "powerCut" {
		t.Errorf("alarm attribute = %v, want powerCut", repo.created[0].Attributes["alarm"])
	}
}

func TestCheckAlarm_EachAlarmFiresImmediately(t *testing.T) {
	// Two consecutive positions both with alarm — both should emit events
	// (no deduplication, matching Traccar's behaviour).
	repo := &recordingEventRepo{}
	svc := NewAlarmService(repo, nil, nil, nil)

	ts := time.Now()
	for i := int64(1); i <= 2; i++ {
		pos := &model.Position{
			ID:         i,
			DeviceID:   5,
			Timestamp:  ts.Add(time.Duration(i) * time.Second),
			Attributes: map[string]any{"alarm": "vibration"},
		}
		if err := svc.CheckAlarm(context.Background(), pos); err != nil {
			t.Fatalf("pos %d: unexpected error: %v", i, err)
		}
	}

	if len(repo.created) != 2 {
		t.Errorf("expected 2 events (no dedup), got %d", len(repo.created))
	}
}

// TestCheckAlarm_WithHub verifies the hub.BroadcastEvent branch is exercised
// when a non-nil hub is configured.
func TestCheckAlarm_WithHub(t *testing.T) {
	repo := &recordingEventRepo{}
	hub := websocket.NewHub(nil, nil, func(_ *http.Request) int64 { return 0 })
	svc := NewAlarmService(repo, hub, nil, nil)

	pos := &model.Position{
		ID:        10,
		DeviceID:  42,
		Timestamp: time.Now(),
		Attributes: map[string]any{
			"alarm": "sos",
		},
	}

	if err := svc.CheckAlarm(context.Background(), pos); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.created) != 1 {
		t.Errorf("expected 1 event, got %d", len(repo.created))
	}
}

// TestCheckAlarm_WithNotificationService verifies the notificationService
// branch is executed when a non-nil service is configured.
func TestCheckAlarm_WithNotificationService(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	notifRepo := repository.NewNotificationRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	geoRepo := repository.NewGeofenceRepository(pool)
	posRepo := repository.NewPositionRepository(pool)
	ctx := context.Background()

	notifSvc := NewNotificationService(notifRepo, deviceRepo, geoRepo, posRepo, nil, nil)

	// Use the mock event repo so that event creation does not hit the DB
	// (the "alarm" type is not in the DB constraint, but we only need to
	// exercise the notificationService branch, not the DB write).
	mockRepo := &recordingEventRepo{}
	svc := NewAlarmService(mockRepo, nil, notifSvc, nil)

	// Seed a user + device so ProcessEvent can look up the device owner.
	user := testutil.CreateUser(t, "alarm-notif@example.com")
	device := testutil.CreateDevice(t, user.ID, "alarm-notif-dev")

	checkPos := &model.Position{
		ID:        1,
		DeviceID:  device.ID,
		Timestamp: time.Now().UTC(),
		Attributes: map[string]any{
			"alarm": "vibration",
		},
	}

	// Should not error — no notification rules are configured so ProcessEvent
	// returns nil after finding no matching rules.
	if err := svc.CheckAlarm(ctx, checkPos); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mockRepo.created) != 1 {
		t.Errorf("expected 1 event, got %d", len(mockRepo.created))
	}
}

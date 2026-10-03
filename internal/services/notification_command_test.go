package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// Unit tests (no database) for rule matching and the command channel of
// NotificationService. Unimplemented interface methods panic via the
// embedded nil interfaces, which flags unexpected repository access.

type fakeNotifRepo struct {
	repository.NotificationRepo
	rules []*model.NotificationRule
	logs  chan *model.NotificationLog
}

func (f *fakeNotifRepo) GetByEventType(_ context.Context, userID int64, eventType string) ([]*model.NotificationRule, error) {
	var out []*model.NotificationRule
	for _, r := range f.rules {
		for _, et := range r.EventTypes {
			if r.UserID == userID && r.Enabled && et == eventType {
				out = append(out, r)
			}
		}
	}
	return out, nil
}

func (f *fakeNotifRepo) LogDelivery(_ context.Context, entry *model.NotificationLog) error {
	f.logs <- entry
	return nil
}

type fakeNotifDeviceRepo struct {
	repository.DeviceRepo
	device  *model.Device
	userIDs []int64
}

func (f *fakeNotifDeviceRepo) GetByID(_ context.Context, _ int64) (*model.Device, error) {
	return f.device, nil
}

func (f *fakeNotifDeviceRepo) GetUserIDs(_ context.Context, _ int64) ([]int64, error) {
	return f.userIDs, nil
}

type fakeNotifGeofenceRepo struct{ repository.GeofenceRepo }

func (fakeNotifGeofenceRepo) GetByID(_ context.Context, id int64) (*model.Geofence, error) {
	return &model.Geofence{ID: id, Name: "Home"}, nil
}

type fakeCommandSubmitter struct {
	mu        sync.Mutex
	submitted []*model.Command
	devices   []*model.Device
	status    string
	err       error
}

func (f *fakeCommandSubmitter) Submit(_ context.Context, device *model.Device, cmdType string, attrs map[string]any) (*model.Command, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	cmd := &model.Command{ID: int64(len(f.submitted) + 1), DeviceID: device.ID, Type: cmdType, Attributes: attrs, Status: f.status}
	f.submitted = append(f.submitted, cmd)
	f.devices = append(f.devices, device)
	return cmd, nil
}

func petIntervalRules() []*model.NotificationRule {
	return []*model.NotificationRule{
		{
			ID: 1, UserID: 10, Name: "Left home", Enabled: true,
			EventTypes:  []string{model.EventTypeGeofenceExit},
			GeofenceIDs: []int64{100},
			Channel:     model.NotificationChannelCommand,
			Config: map[string]any{
				"commandType": model.CommandPositionPeriodic,
				"attributes":  map[string]any{"frequency": float64(20)},
			},
		},
		{
			ID: 2, UserID: 10, Name: "Back home", Enabled: true,
			EventTypes:  []string{model.EventTypeGeofenceEnter},
			GeofenceIDs: []int64{100},
			Channel:     model.NotificationChannelCommand,
			Config: map[string]any{
				"commandType": model.CommandPositionPeriodic,
				"attributes":  map[string]any{"frequency": float64(300)},
			},
		},
	}
}

func newCommandTestService(rules []*model.NotificationRule, submitter CommandSubmitter) (*NotificationService, *fakeNotifRepo) {
	notif := &fakeNotifRepo{rules: rules, logs: make(chan *model.NotificationLog, 10)}
	devices := &fakeNotifDeviceRepo{
		device:  &model.Device{ID: 7, UniqueID: "9000000000001", Name: "Rex", Protocol: "h02"},
		userIDs: []int64{10},
	}
	svc := NewNotificationService(notif, devices, fakeNotifGeofenceRepo{}, nil, submitter, nil)
	return svc, notif
}

// orderedSubmitter records the frequency of each submitted command in
// submission order. Submissions of the frequency in block wait until release
// is closed, simulating a slow submit (DB latency, device write).
type orderedSubmitter struct {
	mu      sync.Mutex
	started []any
	done    []any
	block   any
	release chan struct{}
}

func (o *orderedSubmitter) Submit(_ context.Context, device *model.Device, cmdType string, attrs map[string]any) (*model.Command, error) {
	freq := attrs["frequency"]
	o.mu.Lock()
	o.started = append(o.started, freq)
	o.mu.Unlock()
	if freq == o.block {
		<-o.release
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.done = append(o.done, freq)
	return &model.Command{ID: int64(len(o.done)), DeviceID: device.ID, Type: cmdType, Attributes: attrs, Status: model.CommandStatusSent}, nil
}

func (o *orderedSubmitter) snapshot() (started, done []any) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]any(nil), o.started...), append([]any(nil), o.done...)
}

// TestNotificationService_CommandRules_PreserveEventOrderPerDevice: an exit
// event followed by an enter event must submit the exit command (20 s) before
// the enter command (300 s), even when the first submission is slow.
// Otherwise a device that came home would end up on the away interval.
func TestNotificationService_CommandRules_PreserveEventOrderPerDevice(t *testing.T) {
	sub := &orderedSubmitter{block: float64(20), release: make(chan struct{})}
	svc, notif := newCommandTestService(petIntervalRules(), sub)

	if err := svc.ProcessEvent(context.Background(), geofenceEvent(model.EventTypeGeofenceExit, 100)); err != nil {
		t.Fatalf("ProcessEvent exit: %v", err)
	}
	if err := svc.ProcessEvent(context.Background(), geofenceEvent(model.EventTypeGeofenceEnter, 100)); err != nil {
		t.Fatalf("ProcessEvent enter: %v", err)
	}

	// While the exit command is still being submitted, the enter command
	// must not start.
	time.Sleep(100 * time.Millisecond)
	if started, _ := sub.snapshot(); len(started) != 1 || started[0] != float64(20) {
		t.Fatalf("started = %v, want only the exit command [20] while it is in flight", started)
	}

	close(sub.release)
	waitLog(t, notif.logs)
	waitLog(t, notif.logs)

	_, done := sub.snapshot()
	if len(done) != 2 || done[0] != float64(20) || done[1] != float64(300) {
		t.Fatalf("submission order = %v, want [20 300]", done)
	}
}

// TestNotificationService_CommandRules_OtherDevicesNotBlocked: a slow command
// for one device must not delay commands for another device.
func TestNotificationService_CommandRules_OtherDevicesNotBlocked(t *testing.T) {
	sub := &orderedSubmitter{block: float64(20), release: make(chan struct{})}
	defer close(sub.release)
	notif := &fakeNotifRepo{rules: petIntervalRules(), logs: make(chan *model.NotificationLog, 10)}
	devices := &multiDeviceRepo{userIDs: []int64{10}}
	svc := NewNotificationService(notif, devices, fakeNotifGeofenceRepo{}, nil, sub, nil)

	exit := geofenceEvent(model.EventTypeGeofenceExit, 100)
	exit.DeviceID = 1
	enter := geofenceEvent(model.EventTypeGeofenceEnter, 100)
	enter.DeviceID = 2
	_ = svc.ProcessEvent(context.Background(), exit)
	_ = svc.ProcessEvent(context.Background(), enter)

	entry := waitLog(t, notif.logs)
	if entry.RuleID != 2 {
		t.Fatalf("expected device 2's enter command to complete while device 1 is blocked, got %+v", entry)
	}
}

// multiDeviceRepo returns a distinct device per ID.
type multiDeviceRepo struct {
	repository.DeviceRepo
	userIDs []int64
}

func (m *multiDeviceRepo) GetByID(_ context.Context, id int64) (*model.Device, error) {
	return &model.Device{ID: id, UniqueID: "dev", Name: "Dev", Protocol: "h02"}, nil
}

func (m *multiDeviceRepo) GetUserIDs(_ context.Context, _ int64) ([]int64, error) {
	return m.userIDs, nil
}

func waitLog(t *testing.T, logs chan *model.NotificationLog) *model.NotificationLog {
	t.Helper()
	select {
	case l := <-logs:
		return l
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for notification log entry")
		return nil
	}
}

func expectNoLog(t *testing.T, logs chan *model.NotificationLog) {
	t.Helper()
	select {
	case l := <-logs:
		t.Fatalf("unexpected notification log entry: %+v", l)
	case <-time.After(100 * time.Millisecond):
	}
}

func geofenceEvent(eventType string, geofenceID int64) *model.Event {
	return &model.Event{ID: 55, DeviceID: 7, Type: eventType, GeofenceID: &geofenceID, Timestamp: time.Now().UTC()}
}

func TestNotificationService_CommandRule_ExitHomeSetsFastInterval(t *testing.T) {
	sub := &fakeCommandSubmitter{status: model.CommandStatusSent}
	svc, notif := newCommandTestService(petIntervalRules(), sub)

	if err := svc.ProcessEvent(context.Background(), geofenceEvent(model.EventTypeGeofenceExit, 100)); err != nil {
		t.Fatalf("ProcessEvent: %v", err)
	}
	entry := waitLog(t, notif.logs)
	if entry.RuleID != 1 || entry.Status != "sent" || entry.Error != "" {
		t.Errorf("unexpected log entry: %+v", entry)
	}
	if entry.EventID == nil || *entry.EventID != 55 {
		t.Errorf("log entry must reference the event, got %v", entry.EventID)
	}

	sub.mu.Lock()
	defer sub.mu.Unlock()
	if len(sub.submitted) != 1 {
		t.Fatalf("expected exactly one command, got %d", len(sub.submitted))
	}
	cmd := sub.submitted[0]
	if cmd.DeviceID != 7 || cmd.Type != model.CommandPositionPeriodic || cmd.Attributes["frequency"] != float64(20) {
		t.Errorf("unexpected command: %+v", cmd)
	}
	if sub.devices[0].UniqueID != "9000000000001" {
		t.Errorf("command must target the triggering device, got %+v", sub.devices[0])
	}
}

func TestNotificationService_CommandRule_EnterHomeSetsSlowInterval(t *testing.T) {
	sub := &fakeCommandSubmitter{status: model.CommandStatusPending}
	svc, notif := newCommandTestService(petIntervalRules(), sub)

	_ = svc.ProcessEvent(context.Background(), geofenceEvent(model.EventTypeGeofenceEnter, 100))
	entry := waitLog(t, notif.logs)
	if entry.RuleID != 2 || entry.Status != "queued" {
		t.Errorf("offline device should log a queued command, got %+v", entry)
	}
	sub.mu.Lock()
	defer sub.mu.Unlock()
	if len(sub.submitted) != 1 || sub.submitted[0].Attributes["frequency"] != float64(300) {
		t.Errorf("unexpected commands: %+v", sub.submitted)
	}
}

func TestNotificationService_GeofenceFilter_SkipsOtherGeofences(t *testing.T) {
	sub := &fakeCommandSubmitter{status: model.CommandStatusSent}
	svc, notif := newCommandTestService(petIntervalRules(), sub)

	_ = svc.ProcessEvent(context.Background(), geofenceEvent(model.EventTypeGeofenceExit, 999))
	expectNoLog(t, notif.logs)

	sub.mu.Lock()
	defer sub.mu.Unlock()
	if len(sub.submitted) != 0 {
		t.Errorf("rules filtered to another geofence must not fire, got %+v", sub.submitted)
	}
}

func TestNotificationService_CommandRule_SubmitErrorLogsFailure(t *testing.T) {
	sub := &fakeCommandSubmitter{err: errors.New("command type positionPeriodic is not supported by device protocol osmand")}
	svc, notif := newCommandTestService(petIntervalRules(), sub)

	_ = svc.ProcessEvent(context.Background(), geofenceEvent(model.EventTypeGeofenceExit, 100))
	entry := waitLog(t, notif.logs)
	if entry.Status != "failed" || entry.Error == "" {
		t.Errorf("expected failed log entry with error, got %+v", entry)
	}
}

func TestNotificationService_CommandRule_NoSubmitterLogsFailure(t *testing.T) {
	svc, notif := newCommandTestService(petIntervalRules(), nil)

	_ = svc.ProcessEvent(context.Background(), geofenceEvent(model.EventTypeGeofenceExit, 100))
	entry := waitLog(t, notif.logs)
	if entry.Status != "failed" {
		t.Errorf("expected failed log entry without a command submitter, got %+v", entry)
	}
}

func TestNotificationService_CommandRule_InvalidConfigLogsFailure(t *testing.T) {
	rules := petIntervalRules()
	rules[0].Config = map[string]any{"commandType": model.CommandFactoryReset}
	sub := &fakeCommandSubmitter{status: model.CommandStatusSent}
	svc, notif := newCommandTestService(rules, sub)

	_ = svc.ProcessEvent(context.Background(), geofenceEvent(model.EventTypeGeofenceExit, 100))
	entry := waitLog(t, notif.logs)
	if entry.Status != "failed" {
		t.Errorf("expected failed log entry, got %+v", entry)
	}
	sub.mu.Lock()
	defer sub.mu.Unlock()
	if len(sub.submitted) != 0 {
		t.Error("factoryReset must never be submitted by a notification rule")
	}
}

func TestNotificationService_SendTestNotification_CommandRuleRejected(t *testing.T) {
	sub := &fakeCommandSubmitter{status: model.CommandStatusSent}
	svc, _ := newCommandTestService(nil, sub)

	_, err := svc.SendTestNotification(context.Background(), petIntervalRules()[0])
	if !errors.Is(err, ErrTestNotSupported) {
		t.Fatalf("err = %v, want ErrTestNotSupported", err)
	}
	if len(sub.submitted) != 0 {
		t.Error("test must not send a real command")
	}
}

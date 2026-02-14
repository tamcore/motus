package protocol_test

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/protocol"
)

// --- minimal mock implementations ---

type mockCommandRepo struct {
	mu       sync.Mutex
	pending  map[int64][]*model.Command
	statuses map[int64]string
}

func newMockCommandRepo() *mockCommandRepo {
	return &mockCommandRepo{
		pending:  make(map[int64][]*model.Command),
		statuses: make(map[int64]string),
	}
}

func (m *mockCommandRepo) Create(_ context.Context, _ *model.Command) error { return nil }

func (m *mockCommandRepo) GetPendingByDevice(_ context.Context, deviceID int64) ([]*model.Command, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pending[deviceID], nil
}

func (m *mockCommandRepo) UpdateStatus(_ context.Context, id int64, status string) error {
	m.mu.Lock()
	m.statuses[id] = status
	m.mu.Unlock()
	return nil
}

func (m *mockCommandRepo) ListByDevice(_ context.Context, _ int64, _ int) ([]*model.Command, error) {
	return nil, nil
}

func (m *mockCommandRepo) AppendResult(_ context.Context, _ int64, _ string) error { return nil }

func (m *mockCommandRepo) GetLatestSentByDevice(_ context.Context, _ int64) (*model.Command, error) {
	return nil, nil
}

type mockDeviceRepo struct {
	devices map[string]*model.Device
}

func (m *mockDeviceRepo) GetByUniqueID(_ context.Context, uniqueID string) (*model.Device, error) {
	dev, ok := m.devices[uniqueID]
	if !ok {
		return nil, nil
	}
	return dev, nil
}

// Unused interface methods — satisfy repository.DeviceRepo.
func (m *mockDeviceRepo) UserHasAccess(_ context.Context, _ *model.User, _ int64) bool { return false }
func (m *mockDeviceRepo) GetByID(_ context.Context, _ int64) (*model.Device, error)    { return nil, nil }
func (m *mockDeviceRepo) GetByUser(_ context.Context, _ int64) ([]*model.Device, error) {
	return nil, nil
}
func (m *mockDeviceRepo) GetAll(_ context.Context) ([]model.Device, error) { return nil, nil }
func (m *mockDeviceRepo) GetAllWithOwners(_ context.Context) ([]model.Device, error) {
	return nil, nil
}
func (m *mockDeviceRepo) GetTimedOut(_ context.Context, _ time.Time) ([]model.Device, error) {
	return nil, nil
}
func (m *mockDeviceRepo) GetUserIDs(_ context.Context, _ int64) ([]int64, error) {
	return nil, nil
}
func (m *mockDeviceRepo) Create(_ context.Context, _ *model.Device, _ int64) error { return nil }
func (m *mockDeviceRepo) Update(_ context.Context, _ *model.Device) error          { return nil }
func (m *mockDeviceRepo) Delete(_ context.Context, _ int64) error                  { return nil }

// --- tests ---

func TestCommandDispatcher_DispatchesPendingCommand(t *testing.T) {
	registry := protocol.NewDeviceRegistry()
	outCh := make(chan []byte, 4)
	registry.Register("IMEI001", outCh)

	cmdRepo := newMockCommandRepo()
	cmd := &model.Command{
		ID:       1,
		DeviceID: 42,
		Type:     model.CommandRebootDevice,
		Status:   model.CommandStatusPending,
	}
	cmdRepo.pending[42] = []*model.Command{cmd}

	devRepo := &mockDeviceRepo{
		devices: map[string]*model.Device{
			"IMEI001": {ID: 42, UniqueID: "IMEI001", Protocol: "h02"},
		},
	}

	d := protocol.NewCommandDispatcher(registry, cmdRepo, devRepo, protocol.NewEncoderRegistry())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go d.Start(ctx)

	// Wait until the command channel receives data or timeout.
	select {
	case payload := <-outCh:
		if string(payload) != "*HQ,IMEI001,reset#" {
			t.Errorf("expected '*HQ,IMEI001,reset#', got %q", string(payload))
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for command dispatch")
	}

	// Wait a moment for the status update goroutine to run.
	time.Sleep(50 * time.Millisecond)
	cmdRepo.mu.Lock()
	got := cmdRepo.statuses[1]
	cmdRepo.mu.Unlock()
	if got != model.CommandStatusSent {
		t.Errorf("expected status %q, got %q", model.CommandStatusSent, got)
	}
}

func TestCommandDispatcher_SkipsOfflineDevice(t *testing.T) {
	registry := protocol.NewDeviceRegistry()
	// IMEI002 is NOT registered — device offline on this pod.

	cmdRepo := newMockCommandRepo()
	cmd := &model.Command{ID: 2, DeviceID: 99, Type: model.CommandRebootDevice, Status: model.CommandStatusPending}
	cmdRepo.pending[99] = []*model.Command{cmd}

	devRepo := &mockDeviceRepo{
		devices: map[string]*model.Device{
			"IMEI002": {ID: 99, UniqueID: "IMEI002", Protocol: "h02"},
		},
	}

	d := protocol.NewCommandDispatcher(registry, cmdRepo, devRepo, protocol.NewEncoderRegistry())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	d.Start(ctx)

	// No status update should have been made.
	cmdRepo.mu.Lock()
	_, updated := cmdRepo.statuses[2]
	cmdRepo.mu.Unlock()
	if updated {
		t.Error("command status should not be updated for offline device")
	}
}

func TestCommandDispatcher_CustomCommand(t *testing.T) {
	registry := protocol.NewDeviceRegistry()
	outCh := make(chan []byte, 4)
	registry.Register("IMEI003", outCh)

	cmdRepo := newMockCommandRepo()
	cmd := &model.Command{
		ID:         3,
		DeviceID:   7,
		Type:       model.CommandCustom,
		Attributes: map[string]interface{}{"text": "rconf"},
		Status:     model.CommandStatusPending,
	}
	cmdRepo.pending[7] = []*model.Command{cmd}

	devRepo := &mockDeviceRepo{
		devices: map[string]*model.Device{
			"IMEI003": {ID: 7, UniqueID: "IMEI003", Protocol: "h02"},
		},
	}

	d := protocol.NewCommandDispatcher(registry, cmdRepo, devRepo, protocol.NewEncoderRegistry())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go d.Start(ctx)

	select {
	case payload := <-outCh:
		if string(payload) != "rconf" {
			t.Errorf("expected 'rconf', got %q", string(payload))
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for custom command dispatch")
	}
}

func TestCommandDispatcher_NoPendingCommands(t *testing.T) {
	registry := protocol.NewDeviceRegistry()
	outCh := make(chan []byte, 4)
	registry.Register("IMEI004", outCh)

	cmdRepo := newMockCommandRepo() // no pending commands for any device

	devRepo := &mockDeviceRepo{
		devices: map[string]*model.Device{
			"IMEI004": {ID: 10, UniqueID: "IMEI004", Protocol: "h02"},
		},
	}

	d := protocol.NewCommandDispatcher(registry, cmdRepo, devRepo, protocol.NewEncoderRegistry())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	d.Start(ctx)

	if len(outCh) != 0 {
		t.Errorf("expected no bytes on channel, got %d messages", len(outCh))
	}
}

func TestCommandDispatcher_SetLogger(t *testing.T) {
	d := protocol.NewCommandDispatcher(nil, nil, nil, nil)
	d.SetLogger(slog.Default())
}

// TestCommandDispatcher_SendFails_RevertsStatus verifies that when the registry
// Send returns false (channel full / device disconnected), the command status is
// reverted to "pending" so the next tick can retry.
func TestCommandDispatcher_SendFails_RevertsStatus(t *testing.T) {
	registry := protocol.NewDeviceRegistry()
	// Register with a zero-capacity (unbuffered) channel. Because no one reads
	// from it the non-blocking select in registry.Send will immediately fall to
	// the default case and return false.
	outCh := make(chan []byte) // unbuffered
	registry.Register("IMEI005", outCh)

	cmdRepo := newMockCommandRepo()
	cmd := &model.Command{
		ID:       5,
		DeviceID: 50,
		Type:     model.CommandRebootDevice,
		Status:   model.CommandStatusPending,
	}
	cmdRepo.pending[50] = []*model.Command{cmd}

	devRepo := &mockDeviceRepo{
		devices: map[string]*model.Device{
			"IMEI005": {ID: 50, UniqueID: "IMEI005", Protocol: "h02"},
		},
	}

	d := protocol.NewCommandDispatcher(registry, cmdRepo, devRepo, protocol.NewEncoderRegistry())

	// Run one dispatch tick synchronously.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go d.Start(ctx)

	// Give the dispatcher time to attempt one send.
	time.Sleep(1500 * time.Millisecond)
	cancel()

	cmdRepo.mu.Lock()
	finalStatus := cmdRepo.statuses[5]
	cmdRepo.mu.Unlock()

	// The command should have been reverted to "pending" after Send failed.
	if finalStatus != model.CommandStatusPending {
		t.Errorf("expected status reverted to %q, got %q", model.CommandStatusPending, finalStatus)
	}
}

// TestCommandDispatcher_DispatchForDevice_DeviceNotFound verifies that
// dispatchForDevice exits gracefully when the device is not in the repo.
func TestCommandDispatcher_DispatchForDevice_DeviceNotFound(t *testing.T) {
	registry := protocol.NewDeviceRegistry()
	outCh := make(chan []byte, 4)
	registry.Register("IMEI006", outCh)

	cmdRepo := newMockCommandRepo()
	// devRepo has no entry for IMEI006.
	devRepo := &mockDeviceRepo{devices: map[string]*model.Device{}}

	d := protocol.NewCommandDispatcher(registry, cmdRepo, devRepo, protocol.NewEncoderRegistry())

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	d.Start(ctx)

	if len(outCh) != 0 {
		t.Errorf("expected no dispatch for unknown device, got %d messages", len(outCh))
	}
}

// failingCommandRepo returns an error from GetPendingByDevice.
type failingCommandRepo struct{ mockCommandRepo }

func (r *failingCommandRepo) GetPendingByDevice(_ context.Context, _ int64) ([]*model.Command, error) {
	return nil, fmt.Errorf("db error")
}

// TestCommandDispatcher_GetPendingError covers the GetPendingByDevice error
// branch in dispatchForDevice. Uses a 1.5s timeout to exceed the 1s dispatch
// interval and ensure at least one tick fires.
func TestCommandDispatcher_GetPendingError(t *testing.T) {
	registry := protocol.NewDeviceRegistry()
	outCh := make(chan []byte, 4)
	registry.Register("IMEI007", outCh)

	cmdRepo := &failingCommandRepo{}
	devRepo := &mockDeviceRepo{
		devices: map[string]*model.Device{
			"IMEI007": {ID: 20, UniqueID: "IMEI007", Protocol: "h02"},
		},
	}

	d := protocol.NewCommandDispatcher(registry, cmdRepo, devRepo, protocol.NewEncoderRegistry())

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	d.Start(ctx)

	if len(outCh) != 0 {
		t.Errorf("expected no dispatch on GetPendingByDevice error, got %d messages", len(outCh))
	}
}

// TestCommandDispatcher_EncodeError covers the encodePayload error path in
// sendCommand: an unsupported command type with a known protocol encoder.
func TestCommandDispatcher_EncodeError(t *testing.T) {
	registry := protocol.NewDeviceRegistry()
	outCh := make(chan []byte, 4)
	registry.Register("IMEI008", outCh)

	cmdRepo := newMockCommandRepo()
	// Use an unsupported command type — H02 encoder returns an error for it.
	cmd := &model.Command{
		ID:       8,
		DeviceID: 30,
		Type:     "unsupportedCmdType",
		Status:   model.CommandStatusPending,
	}
	cmdRepo.pending[30] = []*model.Command{cmd}

	devRepo := &mockDeviceRepo{
		devices: map[string]*model.Device{
			"IMEI008": {ID: 30, UniqueID: "IMEI008", Protocol: "h02"},
		},
	}

	d := protocol.NewCommandDispatcher(registry, cmdRepo, devRepo, protocol.NewEncoderRegistry())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	d.Start(ctx)

	// encodePayload returned an error — sendCommand should have returned early.
	if len(outCh) != 0 {
		t.Errorf("expected no dispatch on encode error, got %d messages", len(outCh))
	}
	cmdRepo.mu.Lock()
	_, statusUpdated := cmdRepo.statuses[8]
	cmdRepo.mu.Unlock()
	if statusUpdated {
		t.Error("status should not be updated when encode fails")
	}
}

package handlers_test

// Tests for the ogen Handler command methods (CreateCommand, SendCommand,
// ListCommands, GetCommandTypes). Ported from the deleted chi CommandHandler
// tests in commands_handler_test.go and commands_extra_test.go.
//
// Dropped tests (no live equivalent):
//   - invalid JSON / non-numeric deviceId query parsing: ogen owns request
//     decoding.
//   - SetAuditLogger setter test: the ogen Handler takes the audit logger
//     via HandlerConfig; the nil-pool logger below exercises the audit path.
//
// Access-denied mapping note: the live CreateCommand/SendCommand methods
// respond with the BadRequest envelope carrying "access denied" (the OpenAPI
// schema has no dedicated 403 for these operations); ListCommands uses the
// generic *oas.Error envelope. The IDOR assertions reflect that.

import (
	"context"
	"errors"
	"testing"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/protocol"
	"github.com/tamcore/motus/internal/storage/repository"
)

// mockCommandRepo is a mock implementation of repository.CommandRepo.
type mockCommandRepo struct {
	createFn         func(ctx context.Context, cmd *model.Command) error
	updateStatusFn   func(ctx context.Context, id int64, status string) error
	listByDeviceFn   func(ctx context.Context, deviceID int64, limit int) ([]*model.Command, error)
	getPendingFn     func(ctx context.Context, deviceID int64) ([]*model.Command, error)
	appendResultFn   func(ctx context.Context, id int64, chunk string) error
	getLatestSentFn  func(ctx context.Context, deviceID int64) (*model.Command, error)
	statusTransition []string // records UpdateStatus calls
}

var _ repository.CommandRepo = (*mockCommandRepo)(nil)

func (m *mockCommandRepo) Create(ctx context.Context, cmd *model.Command) error {
	if m.createFn != nil {
		return m.createFn(ctx, cmd)
	}
	cmd.ID = 1
	return nil
}

func (m *mockCommandRepo) UpdateStatus(ctx context.Context, id int64, status string) error {
	m.statusTransition = append(m.statusTransition, status)
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, id, status)
	}
	return nil
}

func (m *mockCommandRepo) ListByDevice(ctx context.Context, deviceID int64, limit int) ([]*model.Command, error) {
	if m.listByDeviceFn != nil {
		return m.listByDeviceFn(ctx, deviceID, limit)
	}
	return nil, nil
}

func (m *mockCommandRepo) GetPendingByDevice(ctx context.Context, deviceID int64) ([]*model.Command, error) {
	if m.getPendingFn != nil {
		return m.getPendingFn(ctx, deviceID)
	}
	return nil, nil
}

func (m *mockCommandRepo) AppendResult(ctx context.Context, id int64, chunk string) error {
	if m.appendResultFn != nil {
		return m.appendResultFn(ctx, id, chunk)
	}
	return nil
}

func (m *mockCommandRepo) GetLatestSentByDevice(ctx context.Context, deviceID int64) (*model.Command, error) {
	if m.getLatestSentFn != nil {
		return m.getLatestSentFn(ctx, deviceID)
	}
	return nil, errors.New("not found")
}

// accessGrantingDeviceRepo returns a device repo mock that grants access and
// resolves devices to the given uniqueID/protocol.
func accessGrantingDeviceRepo(uniqueID, proto string) *mockDeviceRepo {
	return &mockDeviceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return true },
		getByIDFn: func(_ context.Context, id int64) (*model.Device, error) {
			return &model.Device{ID: id, UniqueID: uniqueID, Name: "Cmd Device", Status: "online", Protocol: proto}, nil
		},
	}
}

// newCommandTestHandler builds an ogen Handler for command tests. registry
// and encoders may be nil (device treated as offline / custom-only payloads).
func newCommandTestHandler(commands repository.CommandRepo, devices repository.DeviceRepo,
	registry *protocol.DeviceRegistry, encoders *protocol.EncoderRegistry) *handlers.Handler {
	return handlers.NewHandler(handlers.HandlerConfig{
		Commands:        commands,
		Devices:         devices,
		DeviceRegistry:  registry,
		EncoderRegistry: encoders,
		AuditLogger:     audit.NewLogger(nil),
	})
}

func commandTestUserCtx(id int64) context.Context {
	return api.ContextWithUser(context.Background(), &model.User{ID: id, Email: "cmd@example.com", Role: model.RoleUser})
}

// customTextAttrs builds typed attributes for a custom command.
func customTextAttrs(text string) oas.OptCommandAttributes {
	return oas.NewOptCommandAttributes(oas.NewCommandAttrCustomCommandAttributes(oas.CommandAttrCustom{
		Type: oas.CommandAttrCustomTypeCustom,
		Text: text,
	}))
}

// ---------------------------------------------------------------------------
// CreateCommand
// ---------------------------------------------------------------------------

func TestCreateCommand_Success(t *testing.T) {
	cmdRepo := &mockCommandRepo{
		createFn: func(_ context.Context, cmd *model.Command) error {
			cmd.ID = 21
			return nil
		},
	}
	h := newCommandTestHandler(cmdRepo, accessGrantingDeviceRepo("cmd-dev", "h02"), nil, nil)

	res, err := h.CreateCommand(commandTestUserCtx(1), &oas.CommandInput{
		DeviceId: 5,
		Type:     "rebootDevice",
	})
	if err != nil {
		t.Fatalf("CreateCommand returned error: %v", err)
	}
	cmd, ok := res.(*oas.Command)
	if !ok {
		t.Fatalf("expected *oas.Command, got %T", res)
	}
	if cmd.Type != "rebootDevice" {
		t.Errorf("expected type 'rebootDevice', got %q", cmd.Type)
	}
	if cmd.Status != model.CommandStatusPending {
		t.Errorf("expected status 'pending', got %q", cmd.Status)
	}
}

func TestCreateCommand_MissingFields(t *testing.T) {
	h := newCommandTestHandler(&mockCommandRepo{}, accessGrantingDeviceRepo("cmd-dev", "h02"), nil, nil)

	tests := []struct {
		name     string
		deviceID int64
		cmdType  string
	}{
		{"missing deviceId", 0, "rebootDevice"},
		{"missing type", 5, ""},
		{"empty", 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := h.CreateCommand(commandTestUserCtx(1), &oas.CommandInput{
				DeviceId: tt.deviceID,
				Type:     tt.cmdType,
			})
			if err != nil {
				t.Fatalf("CreateCommand returned error: %v", err)
			}
			badReq, ok := res.(*oas.CreateCommandBadRequest)
			if !ok {
				t.Fatalf("expected *oas.CreateCommandBadRequest, got %T", res)
			}
			if badReq.Error != "deviceId and type are required" {
				t.Errorf("unexpected error message: %q", badReq.Error)
			}
		})
	}
}

func TestCreateCommand_InvalidCommandType(t *testing.T) {
	h := newCommandTestHandler(&mockCommandRepo{}, accessGrantingDeviceRepo("cmd-dev", "h02"), nil, nil)

	res, err := h.CreateCommand(commandTestUserCtx(1), &oas.CommandInput{
		DeviceId: 5,
		Type:     "deleteAllData",
	})
	if err != nil {
		t.Fatalf("CreateCommand returned error: %v", err)
	}
	badReq, ok := res.(*oas.CreateCommandBadRequest)
	if !ok {
		t.Fatalf("expected *oas.CreateCommandBadRequest, got %T", res)
	}
	if badReq.Error != "invalid command type" {
		t.Errorf("unexpected error message: %q", badReq.Error)
	}
}

// TestCreateCommand_Forbidden verifies the IDOR protection: a user without
// access to the device must not be able to queue commands for it.
func TestCreateCommand_Forbidden(t *testing.T) {
	createCalled := false
	cmdRepo := &mockCommandRepo{
		createFn: func(_ context.Context, _ *model.Command) error {
			createCalled = true
			return nil
		},
	}
	devices := &mockDeviceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return false },
	}
	h := newCommandTestHandler(cmdRepo, devices, nil, nil)

	res, err := h.CreateCommand(commandTestUserCtx(1), &oas.CommandInput{
		DeviceId: 5,
		Type:     "rebootDevice",
	})
	if err != nil {
		t.Fatalf("CreateCommand returned error: %v", err)
	}
	badReq, ok := res.(*oas.CreateCommandBadRequest)
	if !ok {
		t.Fatalf("expected *oas.CreateCommandBadRequest, got %T", res)
	}
	if badReq.Error != "access denied" {
		t.Errorf("unexpected error message: %q", badReq.Error)
	}
	if createCalled {
		t.Error("Create must not be called for a foreign device")
	}
}

// ---------------------------------------------------------------------------
// SendCommand
// ---------------------------------------------------------------------------

// TestSendCommand_OfflineDeviceQueues verifies that with no device registry
// the command is persisted as pending instead of being delivered.
func TestSendCommand_OfflineDeviceQueues(t *testing.T) {
	cmdRepo := &mockCommandRepo{}
	h := newCommandTestHandler(cmdRepo, accessGrantingDeviceRepo("offline-dev", "h02"), nil, nil)

	res, err := h.SendCommand(commandTestUserCtx(1), &oas.SendCommandRequest{
		DeviceId: 5,
		Type:     "positionSingle",
	})
	if err != nil {
		t.Fatalf("SendCommand returned error: %v", err)
	}
	cmd, ok := res.(*oas.Command)
	if !ok {
		t.Fatalf("expected *oas.Command, got %T", res)
	}
	if cmd.Status != model.CommandStatusPending {
		t.Errorf("expected status 'pending' for offline device, got %q", cmd.Status)
	}
	if len(cmdRepo.statusTransition) != 0 {
		t.Errorf("expected no status transitions for offline device, got %v", cmdRepo.statusTransition)
	}
}

// TestSendCommand_OnlineDeviceSends registers a live outbound channel in the
// device registry so delivery succeeds and the command is marked sent.
func TestSendCommand_OnlineDeviceSends(t *testing.T) {
	cmdRepo := &mockCommandRepo{}

	reg := protocol.NewDeviceRegistry()
	outCh := make(chan []byte, 4)
	reg.Register("online-dev", outCh)

	h := newCommandTestHandler(cmdRepo, accessGrantingDeviceRepo("online-dev", "h02"),
		reg, protocol.NewEncoderRegistry())

	res, err := h.SendCommand(commandTestUserCtx(1), &oas.SendCommandRequest{
		DeviceId: 5,
		Type:     "rebootDevice",
	})
	if err != nil {
		t.Fatalf("SendCommand returned error: %v", err)
	}
	cmd, ok := res.(*oas.Command)
	if !ok {
		t.Fatalf("expected *oas.Command, got %T", res)
	}
	if cmd.Status != model.CommandStatusSent {
		t.Errorf("expected status 'sent', got %q", cmd.Status)
	}
	// Command bytes should have been delivered to the outbound channel.
	if len(outCh) == 0 {
		t.Error("expected command bytes to be written to outbound channel")
	}
	// Status must have been flipped to sent before dispatch.
	if len(cmdRepo.statusTransition) == 0 || cmdRepo.statusTransition[0] != model.CommandStatusSent {
		t.Errorf("expected UpdateStatus(sent) before dispatch, got %v", cmdRepo.statusTransition)
	}
}

func TestSendCommand_CustomCommand(t *testing.T) {
	h := newCommandTestHandler(&mockCommandRepo{}, accessGrantingDeviceRepo("offline-dev", "h02"), nil, nil)

	res, err := h.SendCommand(commandTestUserCtx(1), &oas.SendCommandRequest{
		DeviceId:   5,
		Type:       "custom",
		Attributes: customTextAttrs("rconf"),
	})
	if err != nil {
		t.Fatalf("SendCommand returned error: %v", err)
	}
	cmd, ok := res.(*oas.Command)
	if !ok {
		t.Fatalf("expected *oas.Command, got %T", res)
	}
	// Device is offline, so the custom command queues as pending.
	if cmd.Status != model.CommandStatusPending {
		t.Errorf("expected status 'pending', got %q", cmd.Status)
	}
}

func TestSendCommand_CustomCommand_MissingText(t *testing.T) {
	h := newCommandTestHandler(&mockCommandRepo{}, accessGrantingDeviceRepo("offline-dev", "h02"), nil, nil)

	res, err := h.SendCommand(commandTestUserCtx(1), &oas.SendCommandRequest{
		DeviceId: 5,
		Type:     "custom",
	})
	if err != nil {
		t.Fatalf("SendCommand returned error: %v", err)
	}
	badReq, ok := res.(*oas.SendCommandBadRequest)
	if !ok {
		t.Fatalf("expected *oas.SendCommandBadRequest, got %T", res)
	}
	if badReq.Error != "custom commands require a non-empty 'text' attribute" {
		t.Errorf("unexpected error message: %q", badReq.Error)
	}
}

func TestSendCommand_Forbidden(t *testing.T) {
	createCalled := false
	cmdRepo := &mockCommandRepo{
		createFn: func(_ context.Context, _ *model.Command) error {
			createCalled = true
			return nil
		},
	}
	devices := &mockDeviceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return false },
	}
	h := newCommandTestHandler(cmdRepo, devices, nil, nil)

	res, err := h.SendCommand(commandTestUserCtx(1), &oas.SendCommandRequest{
		DeviceId: 5,
		Type:     "rebootDevice",
	})
	if err != nil {
		t.Fatalf("SendCommand returned error: %v", err)
	}
	badReq, ok := res.(*oas.SendCommandBadRequest)
	if !ok {
		t.Fatalf("expected *oas.SendCommandBadRequest, got %T", res)
	}
	if badReq.Error != "access denied" {
		t.Errorf("unexpected error message: %q", badReq.Error)
	}
	if createCalled {
		t.Error("Create must not be called for a foreign device")
	}
}

// ---------------------------------------------------------------------------
// ListCommands
// ---------------------------------------------------------------------------

func TestListCommands_Forbidden(t *testing.T) {
	devices := &mockDeviceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return false },
	}
	h := newCommandTestHandler(&mockCommandRepo{}, devices, nil, nil)

	res, err := h.ListCommands(commandTestUserCtx(1), oas.ListCommandsParams{
		DeviceId: oas.NewOptInt64(5),
	})
	if err != nil {
		t.Fatalf("ListCommands returned error: %v", err)
	}
	errRes, ok := res.(*oas.Error)
	if !ok {
		t.Fatalf("expected *oas.Error, got %T", res)
	}
	if errRes.Error != "access denied" {
		t.Errorf("unexpected error message: %q", errRes.Error)
	}
}

func TestListCommands_Success(t *testing.T) {
	cmdRepo := &mockCommandRepo{
		listByDeviceFn: func(_ context.Context, deviceID int64, _ int) ([]*model.Command, error) {
			return []*model.Command{
				{ID: 1, DeviceID: deviceID, Type: "rebootDevice", Status: model.CommandStatusPending},
				{ID: 2, DeviceID: deviceID, Type: "rebootDevice", Status: model.CommandStatusPending},
				{ID: 3, DeviceID: deviceID, Type: "rebootDevice", Status: model.CommandStatusSent},
			}, nil
		},
	}
	h := newCommandTestHandler(cmdRepo, accessGrantingDeviceRepo("cmd-dev", "h02"), nil, nil)

	res, err := h.ListCommands(commandTestUserCtx(1), oas.ListCommandsParams{
		DeviceId: oas.NewOptInt64(5),
	})
	if err != nil {
		t.Fatalf("ListCommands returned error: %v", err)
	}
	list, ok := res.(*oas.ListCommandsOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.ListCommandsOKApplicationJSON, got %T", res)
	}
	if len(*list) != 3 {
		t.Errorf("expected 3 commands, got %d", len(*list))
	}
}

func TestListCommands_Empty(t *testing.T) {
	h := newCommandTestHandler(&mockCommandRepo{}, accessGrantingDeviceRepo("cmd-dev", "h02"), nil, nil)

	res, err := h.ListCommands(commandTestUserCtx(1), oas.ListCommandsParams{
		DeviceId: oas.NewOptInt64(5),
	})
	if err != nil {
		t.Fatalf("ListCommands returned error: %v", err)
	}
	list, ok := res.(*oas.ListCommandsOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.ListCommandsOKApplicationJSON, got %T", res)
	}
	if len(*list) != 0 {
		t.Errorf("expected 0 commands for new device, got %d", len(*list))
	}
}

// ---------------------------------------------------------------------------
// GetCommandTypes
// ---------------------------------------------------------------------------

func TestGetCommandTypes(t *testing.T) {
	h := newCommandTestHandler(&mockCommandRepo{}, &mockDeviceRepo{}, nil, nil)

	res, err := h.GetCommandTypes(context.Background())
	if err != nil {
		t.Fatalf("GetCommandTypes returned error: %v", err)
	}
	list, ok := res.(*oas.GetCommandTypesOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.GetCommandTypesOKApplicationJSON, got %T", res)
	}

	expected := map[string]bool{
		"rebootDevice":     true,
		"positionPeriodic": true,
		"positionSingle":   true,
		"sosNumber":        true,
		"custom":           true,
		"setSpeedAlarm":    true,
		"factoryReset":     true,
	}
	if len(*list) != len(expected) {
		t.Errorf("expected %d command types, got %d", len(expected), len(*list))
	}
	for _, ct := range *list {
		if !expected[ct.Type] {
			t.Errorf("unexpected command type: %q", ct.Type)
		}
	}
}

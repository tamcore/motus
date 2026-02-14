package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/protocol"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func setupCommandHandler(t *testing.T) (*handlers.CommandHandler, *repository.DeviceRepository, *model.User, *model.Device) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	cmdRepo := repository.NewCommandRepository(pool)

	user := &model.User{Email: "cmdhandler@example.com", PasswordHash: "$2a$10$hash", Name: "Cmd Handler"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	device := &model.Device{UniqueID: "cmd-handler-dev", Name: "Cmd Device", Status: "online"}
	if err := deviceRepo.Create(context.Background(), device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	// Pass nil registry and encoders — device will be treated as offline.
	h := handlers.NewCommandHandler(cmdRepo, deviceRepo, nil, nil)
	return h, deviceRepo, user, device
}

func TestCommandHandler_Create_Success(t *testing.T) {
	h, _, user, device := setupCommandHandler(t)

	body, _ := json.Marshal(map[string]interface{}{
		"deviceId": device.ID,
		"type":     "rebootDevice",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/commands", bytes.NewReader(body))
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var cmd model.Command
	_ = json.NewDecoder(rr.Body).Decode(&cmd)
	if cmd.Type != "rebootDevice" {
		t.Errorf("expected type 'rebootDevice', got %q", cmd.Type)
	}
	if cmd.Status != model.CommandStatusPending {
		t.Errorf("expected status 'pending', got %q", cmd.Status)
	}
}

func TestCommandHandler_Create_MissingFields(t *testing.T) {
	h, _, user, _ := setupCommandHandler(t)

	tests := []struct {
		name string
		body string
	}{
		{"missing deviceId", `{"type":"rebootDevice"}`},
		{"missing type", `{"deviceId":1}`},
		{"empty", `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/commands", bytes.NewReader([]byte(tt.body)))
			req = withUser(req, user)
			rr := httptest.NewRecorder()

			h.Create(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", rr.Code)
			}
		})
	}
}

func TestCommandHandler_Create_InvalidJSON(t *testing.T) {
	h, _, user, _ := setupCommandHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/commands", bytes.NewReader([]byte("not json")))
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestCommandHandler_Create_InvalidCommandType(t *testing.T) {
	h, _, user, device := setupCommandHandler(t)

	body, _ := json.Marshal(map[string]interface{}{
		"deviceId": device.ID,
		"type":     "deleteAllData",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/commands", bytes.NewReader(body))
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid command type, got %d", rr.Code)
	}
}

func TestCommandHandler_Create_Forbidden(t *testing.T) {
	h, _, user, device := setupCommandHandler(t)

	otherUser := &model.User{ID: user.ID + 999, Email: "other@example.com", Name: "Other"}

	body, _ := json.Marshal(map[string]interface{}{
		"deviceId": device.ID,
		"type":     "rebootDevice",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/commands", bytes.NewReader(body))
	ctx := api.ContextWithUser(req.Context(), otherUser)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestCommandHandler_Send_OfflineDevice(t *testing.T) {
	// With no registry configured the device is offline → pending → 202.
	h, _, user, device := setupCommandHandler(t)

	body, _ := json.Marshal(map[string]interface{}{
		"deviceId": device.ID,
		"type":     "positionSingle",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/commands/send", bytes.NewReader(body))
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.Send(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var cmd model.Command
	_ = json.NewDecoder(rr.Body).Decode(&cmd)
	if cmd.Status != model.CommandStatusPending {
		t.Errorf("expected status 'pending', got %q", cmd.Status)
	}
}

func TestCommandHandler_Send_OnlineDevice(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	cmdRepo := repository.NewCommandRepository(pool)

	user := &model.User{Email: "sendonline@example.com", PasswordHash: "$2a$10$hash", Name: "Send Online"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	device := &model.Device{UniqueID: "online-dev", Name: "Online Device", Status: "online", Protocol: "h02"}
	if err := deviceRepo.Create(context.Background(), device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	// Register the device in the registry so it appears online.
	reg := protocol.NewDeviceRegistry()
	outCh := make(chan []byte, 4)
	reg.Register(device.UniqueID, outCh)

	h := handlers.NewCommandHandler(cmdRepo, deviceRepo, reg, protocol.NewEncoderRegistry())

	body, _ := json.Marshal(map[string]interface{}{
		"deviceId": device.ID,
		"type":     "rebootDevice",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/commands/send", bytes.NewReader(body))
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.Send(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var cmd model.Command
	_ = json.NewDecoder(rr.Body).Decode(&cmd)
	if cmd.Status != "sent" {
		t.Errorf("expected status 'sent', got %q", cmd.Status)
	}
	// Command bytes should have been delivered.
	if len(outCh) == 0 {
		t.Error("expected command bytes to be written to outbound channel")
	}
}

func TestCommandHandler_Send_CustomCommand(t *testing.T) {
	h, _, user, device := setupCommandHandler(t)

	body, _ := json.Marshal(map[string]interface{}{
		"deviceId":   device.ID,
		"type":       "custom",
		"attributes": map[string]interface{}{"text": "rconf"},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/commands/send", bytes.NewReader(body))
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.Send(rr, req)

	// Device is offline → 202.
	if rr.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestCommandHandler_Send_CustomCommand_MissingText(t *testing.T) {
	h, _, user, device := setupCommandHandler(t)

	body, _ := json.Marshal(map[string]interface{}{
		"deviceId": device.ID,
		"type":     "custom",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/commands/send", bytes.NewReader(body))
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.Send(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestCommandHandler_GetTypes(t *testing.T) {
	h, _, _, _ := setupCommandHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/commands/types", nil)
	rr := httptest.NewRecorder()

	h.GetTypes(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var types []map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&types)
	expected := map[string]bool{
		"rebootDevice":     true,
		"positionPeriodic": true,
		"positionSingle":   true,
		"sosNumber":        true,
		"custom":           true,
		"setSpeedAlarm":    true,
		"factoryReset":     true,
	}
	if len(types) != len(expected) {
		t.Errorf("expected %d command types, got %d", len(expected), len(types))
	}
	for _, ct := range types {
		if !expected[ct["type"]] {
			t.Errorf("unexpected command type: %q", ct["type"])
		}
	}
}

func TestCommandHandler_SetAuditLogger(t *testing.T) {
	h, _, _, _ := setupCommandHandler(t)
	// SetAuditLogger should not panic and should set the logger field.
	h.SetAuditLogger(nil) // nil is fine
}

func TestCommandHandler_List_MissingDeviceID(t *testing.T) {
	h, _, user, _ := setupCommandHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/commands", nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestCommandHandler_List_InvalidDeviceID(t *testing.T) {
	h, _, user, _ := setupCommandHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/commands?deviceId=notanumber", nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestCommandHandler_List_Forbidden(t *testing.T) {
	h, _, _, device := setupCommandHandler(t)

	otherUser := &model.User{ID: 9999, Email: "other@example.com", Name: "Other"}

	req := httptest.NewRequest(http.MethodGet, "/api/commands?deviceId="+strconv.FormatInt(device.ID, 10), nil)
	req = req.WithContext(api.ContextWithUser(req.Context(), otherUser))
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestCommandHandler_List_Success(t *testing.T) {
	h, _, user, device := setupCommandHandler(t)

	// Pre-create some commands.
	for i := 0; i < 3; i++ {
		body, _ := json.Marshal(map[string]interface{}{
			"deviceId": device.ID,
			"type":     "rebootDevice",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/commands", bytes.NewReader(body))
		req = withUser(req, user)
		rr := httptest.NewRecorder()
		h.Create(rr, req)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/commands?deviceId="+strconv.FormatInt(device.ID, 10), nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var cmds []model.Command
	_ = json.NewDecoder(rr.Body).Decode(&cmds)
	if len(cmds) != 3 {
		t.Errorf("expected 3 commands, got %d", len(cmds))
	}
}

func TestCommandHandler_List_Empty(t *testing.T) {
	h, _, user, device := setupCommandHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/commands?deviceId="+strconv.FormatInt(device.ID, 10), nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var cmds []model.Command
	_ = json.NewDecoder(rr.Body).Decode(&cmds)
	if len(cmds) != 0 {
		t.Errorf("expected 0 commands for new device, got %d", len(cmds))
	}
}

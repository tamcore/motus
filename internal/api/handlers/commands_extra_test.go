package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/model"
)

func TestCommandHandler_Send_MissingFields(t *testing.T) {
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
			req := httptest.NewRequest(http.MethodPost, "/api/commands/send", bytes.NewReader([]byte(tt.body)))
			req = withUser(req, user)
			rr := httptest.NewRecorder()

			h.Send(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", rr.Code)
			}
		})
	}
}

func TestCommandHandler_Send_InvalidJSON(t *testing.T) {
	h, _, user, _ := setupCommandHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/commands/send", bytes.NewReader([]byte("not json")))
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.Send(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestCommandHandler_Send_InvalidCommandType(t *testing.T) {
	h, _, user, device := setupCommandHandler(t)

	body, _ := json.Marshal(map[string]interface{}{
		"deviceId": device.ID,
		"type":     "maliciousCommand",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/commands/send", bytes.NewReader(body))
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.Send(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid command type, got %d", rr.Code)
	}
}

func TestCommandHandler_Send_Forbidden(t *testing.T) {
	h, _, user, device := setupCommandHandler(t)

	otherUser := &model.User{ID: user.ID + 999, Email: "sendforbid@example.com", Name: "Other"}

	body, _ := json.Marshal(map[string]interface{}{
		"deviceId": device.ID,
		"type":     "rebootDevice",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/commands/send", bytes.NewReader(body))
	ctx := api.ContextWithUser(req.Context(), otherUser)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	h.Send(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

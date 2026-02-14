package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/tamcore/motus/internal/model"
)

func TestRespondJSON(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		data       interface{}
		wantStatus int
		wantBody   bool
	}{
		{"200 with data", 200, map[string]string{"key": "value"}, 200, true},
		{"201 with data", 201, map[string]string{"created": "true"}, 201, true},
		{"204 with nil data", 204, nil, 204, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			RespondJSON(rr, tt.status, tt.data)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}

			if got := rr.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("expected Content-Type 'application/json', got %q", got)
			}

			if tt.wantBody {
				var body map[string]string
				if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
					t.Errorf("failed to decode body: %v", err)
				}
			}
		})
	}
}

func TestRespondError(t *testing.T) {
	rr := httptest.NewRecorder()
	RespondError(rr, 400, "bad request")

	if rr.Code != 400 {
		t.Errorf("expected status 400, got %d", rr.Code)
	}

	var body map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["error"] != "bad request" {
		t.Errorf("expected error 'bad request', got %q", body["error"])
	}
}

func TestRespondError_StatusCodes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		message string
	}{
		{"unauthorized", 401, "authentication required"},
		{"forbidden", 403, "access denied"},
		{"not found", 404, "not found"},
		{"internal error", 500, "internal server error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			RespondError(rr, tt.status, tt.message)

			if rr.Code != tt.status {
				t.Errorf("expected status %d, got %d", tt.status, rr.Code)
			}

			var body map[string]string
			_ = json.NewDecoder(rr.Body).Decode(&body)
			if body["error"] != tt.message {
				t.Errorf("expected error %q, got %q", tt.message, body["error"])
			}
		})
	}
}

func TestContextWithUser_AndUserFromContext(t *testing.T) {
	user := &model.User{ID: 42, Email: "test@example.com", Name: "Test"}

	// Without user.
	if got := UserFromContext(context.Background()); got != nil {
		t.Error("expected nil user from empty context")
	}

	// With user.
	ctx := ContextWithUser(context.Background(), user)
	got := UserFromContext(ctx)
	if got == nil {
		t.Fatal("expected non-nil user")
	}
	if got.ID != 42 {
		t.Errorf("expected ID 42, got %d", got.ID)
	}
	if got.Email != "test@example.com" {
		t.Errorf("expected email 'test@example.com', got %q", got.Email)
	}
}

func TestUserFromContext_WrongType(t *testing.T) {
	// Put something other than *model.User in the context.
	ctx := context.WithValue(context.Background(), userContextKey, "not a user")
	got := UserFromContext(ctx)
	if got != nil {
		t.Error("expected nil for wrong type in context")
	}
}

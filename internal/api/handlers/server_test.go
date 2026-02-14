package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tamcore/motus/internal/api/handlers"
)

func TestGetServer_ReturnsTraccarCompatibleInfo(t *testing.T) {
	h := handlers.NewServerHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/server", nil)
	rr := httptest.NewRecorder()
	h.GetServer(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify required fields for Traccar compatibility.
	requiredFields := []string{
		"id", "registration", "readonly", "deviceReadonly",
		"map", "latitude", "longitude", "zoom", "version",
		"coordinateFormat", "attributes",
	}
	for _, field := range requiredFields {
		if _, ok := resp[field]; !ok {
			t.Errorf("missing required field %q in server response", field)
		}
	}

	if version, ok := resp["version"].(string); !ok || version != "3.0.0" {
		t.Errorf("expected version '3.0.0', got %v", resp["version"])
	}
}

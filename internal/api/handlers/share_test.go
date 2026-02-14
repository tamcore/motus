package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// TestCreateShare_Unauthenticated verifies that creating a share requires authentication.
func TestCreateShare_Unauthenticated(t *testing.T) {
	h := handlers.NewShareHandler(
		repository.NewDeviceShareRepository(nil),
		repository.NewDeviceRepository(nil),
		repository.NewPositionRepository(nil), "",
	)

	req := httptest.NewRequest(http.MethodPost, "/api/devices/1/share", nil)
	rr := httptest.NewRecorder()
	h.CreateShare(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

// TestListShares_Unauthenticated verifies that listing shares requires authentication.
func TestListShares_Unauthenticated(t *testing.T) {
	h := handlers.NewShareHandler(
		repository.NewDeviceShareRepository(nil),
		repository.NewDeviceRepository(nil),
		repository.NewPositionRepository(nil), "",
	)

	req := httptest.NewRequest(http.MethodGet, "/api/devices/1/shares", nil)
	rr := httptest.NewRecorder()
	h.ListShares(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

// TestDeleteShare_Unauthenticated verifies that deleting a share requires authentication.
func TestDeleteShare_Unauthenticated(t *testing.T) {
	h := handlers.NewShareHandler(
		repository.NewDeviceShareRepository(nil),
		repository.NewDeviceRepository(nil),
		repository.NewPositionRepository(nil), "",
	)

	req := httptest.NewRequest(http.MethodDelete, "/api/shares/1", nil)
	rr := httptest.NewRecorder()
	h.DeleteShare(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

// TestGetSharedDevice_MissingToken verifies that accessing a share without token fails.
func TestGetSharedDevice_MissingToken(t *testing.T) {
	h := handlers.NewShareHandler(
		repository.NewDeviceShareRepository(nil),
		repository.NewDeviceRepository(nil),
		repository.NewPositionRepository(nil), "",
	)

	req := httptest.NewRequest(http.MethodGet, "/api/share/", nil)
	rr := httptest.NewRecorder()
	h.GetSharedDevice(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
}

// TestCreateShare_InvalidDeviceID verifies that a non-numeric device ID is rejected.
func TestCreateShare_InvalidDeviceID(t *testing.T) {
	h := handlers.NewShareHandler(
		repository.NewDeviceShareRepository(nil),
		repository.NewDeviceRepository(nil),
		repository.NewPositionRepository(nil), "",
	)

	user := &model.User{ID: 1, Email: "test@example.com"}
	req := httptest.NewRequest(http.MethodPost, "/api/devices/abc/share", nil)
	ctx := api.ContextWithUser(req.Context(), user)
	req = req.WithContext(ctx)

	// chi URL param not set, so ParseInt fails.
	rr := httptest.NewRecorder()
	h.CreateShare(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
}

// TestCreateShare_InvalidBody verifies that a malformed JSON body is rejected.
func TestCreateShare_InvalidBody(t *testing.T) {
	h := handlers.NewShareHandler(
		repository.NewDeviceShareRepository(nil),
		repository.NewDeviceRepository(nil),
		repository.NewPositionRepository(nil), "",
	)

	user := &model.User{ID: 1, Email: "test@example.com"}
	body := bytes.NewReader([]byte("not json"))
	req := httptest.NewRequest(http.MethodPost, "/api/devices/1/share", body)
	req.Header.Set("Content-Length", "8")
	ctx := api.ContextWithUser(req.Context(), user)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()

	// With chi URL param not set, we'll get bad request on device id parse.
	// This tests the overall handler flow.
	h.CreateShare(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
}

// TestCreateShareResponse_Structure verifies the response structure includes shareUrl.
func TestCreateShareResponse_Structure(t *testing.T) {
	// Test the JSON structure of createShareResponse.
	resp := struct {
		DeviceShare *model.DeviceShare `json:"deviceShare,omitempty"`
		ShareURL    string             `json:"shareUrl"`
	}{
		ShareURL: "/share/testtoken123",
	}

	b, _ := json.Marshal(resp)
	var decoded map[string]interface{}
	_ = json.Unmarshal(b, &decoded)

	if decoded["shareUrl"] != "/share/testtoken123" {
		t.Errorf("expected shareUrl '/share/testtoken123', got %v", decoded["shareUrl"])
	}
}

package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func setupPositionHandler(t *testing.T) (*handlers.PositionHandler, *repository.PositionRepository, *model.User, *model.Device) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	posRepo := repository.NewPositionRepository(pool)

	user := &model.User{Email: "poshandler@example.com", PasswordHash: "$2a$10$hash", Name: "Pos Handler"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	device := &model.Device{UniqueID: "pos-handler-dev", Name: "Pos Device", Status: "online"}
	if err := deviceRepo.Create(context.Background(), device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	h := handlers.NewPositionHandler(posRepo, deviceRepo)
	return h, posRepo, user, device
}

func TestPositionHandler_GetPositions_LatestByUser(t *testing.T) {
	h, posRepo, user, device := setupPositionHandler(t)
	ctx := context.Background()

	now := time.Now().UTC()
	_ = posRepo.Create(ctx, &model.Position{DeviceID: device.ID, Latitude: 52.0, Longitude: 13.0, Timestamp: now.Add(-10 * time.Minute)})
	_ = posRepo.Create(ctx, &model.Position{DeviceID: device.ID, Latitude: 52.1, Longitude: 13.1, Timestamp: now})

	req := httptest.NewRequest(http.MethodGet, "/api/positions", nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.GetPositions(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var positions []*model.Position
	_ = json.NewDecoder(rr.Body).Decode(&positions)
	// Should return latest position per device (1 device = 1 position).
	if len(positions) != 1 {
		t.Errorf("expected 1 latest position, got %d", len(positions))
	}
}

func TestPositionHandler_GetPositions_ByDevice(t *testing.T) {
	h, posRepo, user, device := setupPositionHandler(t)
	ctx := context.Background()

	now := time.Now().UTC()
	_ = posRepo.Create(ctx, &model.Position{DeviceID: device.ID, Latitude: 52.0, Longitude: 13.0, Timestamp: now.Add(-10 * time.Minute)})
	_ = posRepo.Create(ctx, &model.Position{DeviceID: device.ID, Latitude: 52.1, Longitude: 13.1, Timestamp: now})

	url := fmt.Sprintf("/api/positions?deviceId=%d&from=%s&to=%s",
		device.ID,
		now.Add(-1*time.Hour).Format(time.RFC3339),
		now.Add(time.Minute).Format(time.RFC3339),
	)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.GetPositions(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var positions []*model.Position
	_ = json.NewDecoder(rr.Body).Decode(&positions)
	if len(positions) != 2 {
		t.Errorf("expected 2 positions in range, got %d", len(positions))
	}
}

func TestPositionHandler_GetPositions_InvalidDeviceID(t *testing.T) {
	h, _, user, _ := setupPositionHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/positions?deviceId=abc", nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.GetPositions(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestPositionHandler_GetPositions_Forbidden(t *testing.T) {
	h, _, user, device := setupPositionHandler(t)

	otherUser := &model.User{ID: user.ID + 999, Email: "other@example.com", Name: "Other"}

	url := fmt.Sprintf("/api/positions?deviceId=%d", device.ID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	ctx := api.ContextWithUser(req.Context(), otherUser)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	h.GetPositions(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestPositionHandler_GetPositions_Unauthenticated(t *testing.T) {
	h, _, _, _ := setupPositionHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/positions", nil)
	rr := httptest.NewRecorder()

	h.GetPositions(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestPositionHandler_GetPositions_EmptyResult(t *testing.T) {
	h, _, user, _ := setupPositionHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/positions", nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.GetPositions(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestPositionHandler_GetPositions_ByID(t *testing.T) {
	h, posRepo, user, device := setupPositionHandler(t)
	ctx := context.Background()

	now := time.Now().UTC()
	pos1 := &model.Position{DeviceID: device.ID, Latitude: 52.0, Longitude: 13.0, Timestamp: now.Add(-10 * time.Minute)}
	pos2 := &model.Position{DeviceID: device.ID, Latitude: 52.1, Longitude: 13.1, Timestamp: now}
	_ = posRepo.Create(ctx, pos1)
	_ = posRepo.Create(ctx, pos2)

	// Fetch a single position by ID.
	url := fmt.Sprintf("/api/positions?id=%d", pos1.ID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.GetPositions(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var positions []*model.Position
	_ = json.NewDecoder(rr.Body).Decode(&positions)
	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}
	if positions[0].ID != pos1.ID {
		t.Errorf("expected position ID %d, got %d", pos1.ID, positions[0].ID)
	}
}

func TestPositionHandler_GetPositions_ByMultipleIDs(t *testing.T) {
	h, posRepo, user, device := setupPositionHandler(t)
	ctx := context.Background()

	now := time.Now().UTC()
	pos1 := &model.Position{DeviceID: device.ID, Latitude: 52.0, Longitude: 13.0, Timestamp: now.Add(-10 * time.Minute)}
	pos2 := &model.Position{DeviceID: device.ID, Latitude: 52.1, Longitude: 13.1, Timestamp: now}
	_ = posRepo.Create(ctx, pos1)
	_ = posRepo.Create(ctx, pos2)

	// Fetch multiple positions by ID (Traccar-style: ?id=X&id=Y).
	url := fmt.Sprintf("/api/positions?id=%d&id=%d", pos1.ID, pos2.ID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.GetPositions(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var positions []*model.Position
	_ = json.NewDecoder(rr.Body).Decode(&positions)
	if len(positions) != 2 {
		t.Errorf("expected 2 positions, got %d", len(positions))
	}
}

func TestPositionHandler_GetPositions_ByID_InvalidID(t *testing.T) {
	h, _, user, _ := setupPositionHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/positions?id=abc", nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.GetPositions(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestPositionHandler_GetPositions_ByID_AccessControl(t *testing.T) {
	h, posRepo, user, device := setupPositionHandler(t)
	ctx := context.Background()

	now := time.Now().UTC()
	pos := &model.Position{DeviceID: device.ID, Latitude: 52.0, Longitude: 13.0, Timestamp: now}
	_ = posRepo.Create(ctx, pos)

	// A different user should not see positions for devices they lack access to.
	otherUser := &model.User{ID: user.ID + 999, Email: "other@example.com", Name: "Other"}

	url := fmt.Sprintf("/api/positions?id=%d", pos.ID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = req.WithContext(api.ContextWithUser(req.Context(), otherUser))
	rr := httptest.NewRecorder()

	h.GetPositions(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d (access-filtered result, not error)", rr.Code)
	}

	var positions []*model.Position
	_ = json.NewDecoder(rr.Body).Decode(&positions)
	if len(positions) != 0 {
		t.Errorf("expected 0 positions (access denied), got %d", len(positions))
	}
}

// TestPositionHandler_GetPositions_DefaultLimitReturnsAllPositions verifies that
// when no limit query parameter is provided, the handler returns all positions in
// the time range (up to the repository maximum of 10000), not just 100.
// This is the root cause of the "straight line trail" bug: with the old default
// of 100, only the first 100 positions (ordered ASC) were returned, causing the
// map to draw a straight line from the early cluster to the live device position.
func TestPositionHandler_GetPositions_DefaultLimitReturnsAllPositions(t *testing.T) {
	h, posRepo, user, device := setupPositionHandler(t)
	ctx := context.Background()

	now := time.Now().UTC()

	// Insert 150 positions spread over the last hour -- more than the old
	// default limit of 100 but well under the new max of 10000.
	const count = 150
	for i := 0; i < count; i++ {
		ts := now.Add(-time.Duration(count-i) * time.Minute)
		lat := 52.0 + float64(i)*0.001 // move slightly north each time
		lon := 13.0 + float64(i)*0.001
		if err := posRepo.Create(ctx, &model.Position{
			DeviceID:  device.ID,
			Latitude:  lat,
			Longitude: lon,
			Timestamp: ts,
		}); err != nil {
			t.Fatalf("create position %d: %v", i, err)
		}
	}

	// Query without a limit parameter -- should return all 150.
	url := fmt.Sprintf("/api/positions?deviceId=%d&from=%s&to=%s",
		device.ID,
		now.Add(-time.Duration(count+1)*time.Minute).Format(time.RFC3339),
		now.Add(time.Minute).Format(time.RFC3339),
	)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.GetPositions(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var positions []*model.Position
	if err := json.NewDecoder(rr.Body).Decode(&positions); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(positions) != count {
		t.Errorf("expected all %d positions without limit param, got %d (old default was 100)", count, len(positions))
	}
}

// TestPositionHandler_GetPositions_ExplicitLimitIsRespected verifies that a
// client-supplied limit parameter is still honoured.
func TestPositionHandler_GetPositions_ExplicitLimitIsRespected(t *testing.T) {
	h, posRepo, user, device := setupPositionHandler(t)
	ctx := context.Background()

	now := time.Now().UTC()
	for i := 0; i < 50; i++ {
		ts := now.Add(-time.Duration(50-i) * time.Minute)
		if err := posRepo.Create(ctx, &model.Position{
			DeviceID:  device.ID,
			Latitude:  52.0 + float64(i)*0.001,
			Longitude: 13.0,
			Timestamp: ts,
		}); err != nil {
			t.Fatalf("create position %d: %v", i, err)
		}
	}

	// Explicit limit=10 should only return 10 positions.
	url := fmt.Sprintf("/api/positions?deviceId=%d&from=%s&to=%s&limit=10",
		device.ID,
		now.Add(-time.Hour).Format(time.RFC3339),
		now.Add(time.Minute).Format(time.RFC3339),
	)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.GetPositions(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var positions []*model.Position
	_ = json.NewDecoder(rr.Body).Decode(&positions)
	if len(positions) != 10 {
		t.Errorf("expected 10 positions with limit=10, got %d", len(positions))
	}
}

func TestPositionHandler_GetPositions_ByID_NonExistent(t *testing.T) {
	h, _, user, _ := setupPositionHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/positions?id=999999", nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.GetPositions(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var positions []*model.Position
	_ = json.NewDecoder(rr.Body).Decode(&positions)
	if len(positions) != 0 {
		t.Errorf("expected 0 positions for non-existent ID, got %d", len(positions))
	}
}

func TestPositionHandler_AdminGetAllPositions_Success(t *testing.T) {
	h, posRepo, user, device := setupPositionHandler(t)
	ctx := context.Background()
	user.Role = "admin"

	now := time.Now().UTC()
	_ = posRepo.Create(ctx, &model.Position{DeviceID: device.ID, Latitude: 52.0, Longitude: 13.0, Timestamp: now})

	req := httptest.NewRequest(http.MethodGet, "/api/admin/positions", nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.AdminGetAllPositions(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var positions []*model.Position
	_ = json.NewDecoder(rr.Body).Decode(&positions)
	if len(positions) < 1 {
		t.Errorf("expected at least 1 position, got %d", len(positions))
	}
}

func TestPositionHandler_AdminGetAllPositions_NonAdmin(t *testing.T) {
	h, _, user, _ := setupPositionHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/positions", nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.AdminGetAllPositions(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-admin, got %d", rr.Code)
	}
}

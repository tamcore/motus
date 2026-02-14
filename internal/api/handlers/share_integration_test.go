package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func setupShareHandler(t *testing.T) (*handlers.ShareHandler, *repository.DeviceShareRepository, *repository.DeviceRepository, *repository.UserRepository, *repository.PositionRepository) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	shareRepo := repository.NewDeviceShareRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	positionRepo := repository.NewPositionRepository(pool)

	h := handlers.NewShareHandler(shareRepo, deviceRepo, positionRepo, "")
	return h, shareRepo, deviceRepo, userRepo, positionRepo
}

// createTestUserAndDevice creates a user and device for testing.
func createTestUserAndDevice(t *testing.T, userRepo *repository.UserRepository, deviceRepo *repository.DeviceRepository) (*model.User, *model.Device) {
	t.Helper()
	ctx := context.Background()

	user := &model.User{Email: "share@example.com", PasswordHash: "hash", Name: "Share User"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	device := &model.Device{UniqueID: "share-test-001", Name: "Test Device", Status: "online"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	return user, device
}

// requestWithChi sets up a chi context with URL params.
func requestWithChi(req *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestCreateShare_Success(t *testing.T) {
	h, _, deviceRepo, userRepo, _ := setupShareHandler(t)
	user, device := createTestUserAndDevice(t, userRepo, deviceRepo)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/devices/%d/share", device.ID), nil)
	ctx := api.ContextWithUser(req.Context(), user)
	req = req.WithContext(ctx)
	req = requestWithChi(req, map[string]string{"id": fmt.Sprintf("%d", device.ID)})

	rr := httptest.NewRecorder()
	h.CreateShare(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	if resp["token"] == nil || resp["token"] == "" {
		t.Error("expected non-empty token in response")
	}
	if resp["shareUrl"] == nil || resp["shareUrl"] == "" {
		t.Error("expected non-empty shareUrl in response")
	}
}

func TestCreateShare_WithExpiry(t *testing.T) {
	h, _, deviceRepo, userRepo, _ := setupShareHandler(t)
	user, device := createTestUserAndDevice(t, userRepo, deviceRepo)

	body := []byte(`{"expiresInHours": 24}`)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/devices/%d/share", device.ID),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := api.ContextWithUser(req.Context(), user)
	req = req.WithContext(ctx)
	req = requestWithChi(req, map[string]string{"id": fmt.Sprintf("%d", device.ID)})

	rr := httptest.NewRecorder()
	h.CreateShare(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	if resp["expiresAt"] == nil {
		t.Error("expected expiresAt to be set when expiresInHours is provided")
	}
}

func TestCreateShare_NoAccess(t *testing.T) {
	h, _, deviceRepo, userRepo, _ := setupShareHandler(t)
	_, device := createTestUserAndDevice(t, userRepo, deviceRepo)

	// Create a different user who does NOT have access.
	otherUser := &model.User{Email: "other@example.com", PasswordHash: "hash", Name: "Other User"}
	if err := userRepo.Create(context.Background(), otherUser); err != nil {
		t.Fatalf("create other user: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/devices/%d/share", device.ID), nil)
	ctx := api.ContextWithUser(req.Context(), otherUser)
	req = req.WithContext(ctx)
	req = requestWithChi(req, map[string]string{"id": fmt.Sprintf("%d", device.ID)})

	rr := httptest.NewRecorder()
	h.CreateShare(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rr.Code)
	}
}

func TestListShares_Success(t *testing.T) {
	h, shareRepo, deviceRepo, userRepo, _ := setupShareHandler(t)
	user, device := createTestUserAndDevice(t, userRepo, deviceRepo)

	// Create two shares.
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		share := &model.DeviceShare{DeviceID: device.ID, CreatedBy: user.ID}
		if err := shareRepo.Create(ctx, share); err != nil {
			t.Fatalf("create share: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/devices/%d/shares", device.ID), nil)
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	req = requestWithChi(req, map[string]string{"id": fmt.Sprintf("%d", device.ID)})

	rr := httptest.NewRecorder()
	h.ListShares(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var shares []model.DeviceShare
	_ = json.NewDecoder(rr.Body).Decode(&shares)
	if len(shares) != 2 {
		t.Errorf("expected 2 shares, got %d", len(shares))
	}
}

func TestGetSharedDevice_Success(t *testing.T) {
	h, shareRepo, deviceRepo, userRepo, positionRepo := setupShareHandler(t)
	user, device := createTestUserAndDevice(t, userRepo, deviceRepo)

	// Create a share.
	ctx := context.Background()
	share := &model.DeviceShare{DeviceID: device.ID, CreatedBy: user.ID}
	if err := shareRepo.Create(ctx, share); err != nil {
		t.Fatalf("create share: %v", err)
	}

	// Create a position for the device.
	pos := &model.Position{
		DeviceID:  device.ID,
		Latitude:  52.52,
		Longitude: 13.405,
		Timestamp: time.Now(),
	}
	if err := positionRepo.Create(ctx, pos); err != nil {
		t.Fatalf("create position: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/share/"+share.Token, nil)
	req = requestWithChi(req, map[string]string{"token": share.Token})

	rr := httptest.NewRecorder()
	h.GetSharedDevice(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp["device"] == nil {
		t.Error("expected device in response")
	}
	if resp["positions"] == nil {
		t.Error("expected positions in response")
	}
}

func TestGetSharedDevice_ExpiredToken(t *testing.T) {
	h, shareRepo, deviceRepo, userRepo, _ := setupShareHandler(t)
	user, device := createTestUserAndDevice(t, userRepo, deviceRepo)

	// Create a share with an expiry in the past.
	ctx := context.Background()
	expired := time.Now().Add(-1 * time.Hour)
	share := &model.DeviceShare{DeviceID: device.ID, CreatedBy: user.ID, ExpiresAt: &expired}
	if err := shareRepo.Create(ctx, share); err != nil {
		t.Fatalf("create share: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/share/"+share.Token, nil)
	req = requestWithChi(req, map[string]string{"token": share.Token})

	rr := httptest.NewRecorder()
	h.GetSharedDevice(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for expired share, got %d", rr.Code)
	}
}

func TestDeleteShare_Unauthorized(t *testing.T) {
	h, shareRepo, deviceRepo, userRepo, _ := setupShareHandler(t)
	user, device := createTestUserAndDevice(t, userRepo, deviceRepo)

	// Create a share belonging to user's device.
	ctx := context.Background()
	share := &model.DeviceShare{DeviceID: device.ID, CreatedBy: user.ID}
	if err := shareRepo.Create(ctx, share); err != nil {
		t.Fatalf("create share: %v", err)
	}

	// Create a different user who does NOT have access to the device.
	otherUser := &model.User{Email: "other-del@example.com", PasswordHash: "hash", Name: "Other Del User"}
	if err := userRepo.Create(ctx, otherUser); err != nil {
		t.Fatalf("create other user: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/shares/%d", share.ID), nil)
	req = req.WithContext(api.ContextWithUser(req.Context(), otherUser))
	req = requestWithChi(req, map[string]string{"id": fmt.Sprintf("%d", share.ID)})

	rr := httptest.NewRecorder()
	h.DeleteShare(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Verify share was NOT deleted.
	found, err := shareRepo.GetByToken(ctx, share.Token)
	if err != nil || found == nil {
		t.Error("expected share to still exist after unauthorized delete attempt")
	}
}

func TestDeleteShare_NotFound(t *testing.T) {
	h, _, _, userRepo, _ := setupShareHandler(t)
	ctx := context.Background()

	user := &model.User{Email: "del-notfound@example.com", PasswordHash: "hash", Name: "Del NotFound"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/shares/99999", nil)
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	req = requestWithChi(req, map[string]string{"id": "99999"})

	rr := httptest.NewRecorder()
	h.DeleteShare(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteShare_Success(t *testing.T) {
	h, shareRepo, deviceRepo, userRepo, _ := setupShareHandler(t)
	user, device := createTestUserAndDevice(t, userRepo, deviceRepo)

	ctx := context.Background()
	share := &model.DeviceShare{DeviceID: device.ID, CreatedBy: user.ID}
	if err := shareRepo.Create(ctx, share); err != nil {
		t.Fatalf("create share: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/shares/%d", share.ID), nil)
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	req = requestWithChi(req, map[string]string{"id": fmt.Sprintf("%d", share.ID)})

	rr := httptest.NewRecorder()
	h.DeleteShare(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", rr.Code)
	}

	// Verify share is deleted.
	_, err := shareRepo.GetByToken(ctx, share.Token)
	if err == nil {
		t.Error("expected share to be deleted")
	}
}

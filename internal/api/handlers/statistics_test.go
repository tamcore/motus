package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// mockStatsRepo implements repository.StatisticsRepo for handler tests.
type mockStatsRepo struct {
	platformStats *repository.PlatformStats
	platformErr   error
	userStats     *repository.UserStats
	userErr       error
}

func (m *mockStatsRepo) GetPlatformStats(_ context.Context) (*repository.PlatformStats, error) {
	return m.platformStats, m.platformErr
}

func (m *mockStatsRepo) GetUserStats(_ context.Context, _ int64) (*repository.UserStats, error) {
	return m.userStats, m.userErr
}

func TestGetPlatformStats_RequiresAdmin(t *testing.T) {
	h := NewStatisticsHandler(nil)

	// No user in context.
	req := httptest.NewRequest(http.MethodGet, "/api/admin/statistics", nil)
	rec := httptest.NewRecorder()

	h.GetPlatformStats(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestGetPlatformStats_NonAdminForbidden(t *testing.T) {
	h := NewStatisticsHandler(nil)

	user := &model.User{ID: 1, Role: model.RoleUser}
	ctx := api.ContextWithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/statistics", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.GetPlatformStats(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestGetUserStats_RequiresAdmin(t *testing.T) {
	h := NewStatisticsHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/statistics/users/1", nil)
	rec := httptest.NewRecorder()

	h.GetUserStats(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestGetUserStats_InvalidID(t *testing.T) {
	h := NewStatisticsHandler(nil)

	admin := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), admin)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "abc")
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/statistics/users/abc", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.GetUserStats(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}

	var body map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] != "invalid user id" {
		t.Errorf("unexpected error: %s", body["error"])
	}
}

func TestGetPlatformStats_Success(t *testing.T) {
	stats := &repository.PlatformStats{
		TotalUsers:      10,
		TotalDevices:    5,
		TotalPositions:  1000,
		TotalEvents:     50,
		DevicesByStatus: map[string]int64{"online": 3, "offline": 2},
	}
	h := NewStatisticsHandler(&mockStatsRepo{platformStats: stats})

	admin := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), admin)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/statistics", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.GetPlatformStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["totalUsers"].(float64) != 10 {
		t.Errorf("expected totalUsers=10, got %v", body["totalUsers"])
	}
	if body["totalDevices"].(float64) != 5 {
		t.Errorf("expected totalDevices=5, got %v", body["totalDevices"])
	}
}

func TestGetPlatformStats_Error(t *testing.T) {
	h := NewStatisticsHandler(&mockStatsRepo{platformErr: errors.New("db error")})

	admin := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), admin)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/statistics", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.GetPlatformStats(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestGetUserStats_Success(t *testing.T) {
	stats := &repository.UserStats{
		UserID:         42,
		DevicesOwned:   3,
		TotalPositions: 100,
	}
	h := NewStatisticsHandler(&mockStatsRepo{userStats: stats})

	admin := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), admin)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "42")
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/statistics/users/42", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.GetUserStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["userId"].(float64) != 42 {
		t.Errorf("expected userId=42, got %v", body["userId"])
	}
	if body["devicesOwned"].(float64) != 3 {
		t.Errorf("expected devicesOwned=3, got %v", body["devicesOwned"])
	}
}

func TestGetUserStats_Error(t *testing.T) {
	h := NewStatisticsHandler(&mockStatsRepo{userErr: errors.New("db error")})

	admin := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), admin)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "42")
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/statistics/users/42", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.GetUserStats(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
)

// mockAuditQuerier implements AuditQuerier for handler tests.
type mockAuditQuerier struct {
	queryFn func(ctx context.Context, params audit.QueryParams) ([]audit.Entry, int64, error)
}

func (m *mockAuditQuerier) Query(ctx context.Context, params audit.QueryParams) ([]audit.Entry, int64, error) {
	if m.queryFn != nil {
		return m.queryFn(ctx, params)
	}
	return nil, 0, nil
}

func TestGetAuditLog_RequiresAdmin(t *testing.T) {
	// Use a nil-pool logger (won't be called since we fail auth first).
	logger := audit.NewLogger(nil)
	h := NewAuditHandler(logger)

	// No user in context.
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit", nil)
	rec := httptest.NewRecorder()

	h.GetAuditLog(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestGetAuditLog_NonAdminForbidden(t *testing.T) {
	logger := audit.NewLogger(nil)
	h := NewAuditHandler(logger)

	user := &model.User{ID: 1, Role: model.RoleUser}
	ctx := api.ContextWithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.GetAuditLog(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestGetAuditLog_Success(t *testing.T) {
	now := time.Now()
	entries := []audit.Entry{
		{ID: 1, Timestamp: now, Action: "session.login"},
		{ID: 2, Timestamp: now.Add(-time.Minute), Action: "device.create"},
		{ID: 3, Timestamp: now.Add(-2 * time.Minute), Action: "user.update"},
	}
	q := &mockAuditQuerier{
		queryFn: func(_ context.Context, _ audit.QueryParams) ([]audit.Entry, int64, error) {
			return entries, 3, nil
		},
	}
	h := NewAuditHandler(q)

	admin := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), admin)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.GetAuditLog(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["total"].(float64) != 3 {
		t.Errorf("expected total=3, got %v", body["total"])
	}
	entriesArr, ok := body["entries"].([]interface{})
	if !ok || len(entriesArr) != 3 {
		t.Errorf("expected 3 entries, got %v", body["entries"])
	}
}

func TestGetAuditLog_EmptyEntries(t *testing.T) {
	q := &mockAuditQuerier{
		queryFn: func(_ context.Context, _ audit.QueryParams) ([]audit.Entry, int64, error) {
			return nil, 0, nil
		},
	}
	h := NewAuditHandler(q)

	admin := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), admin)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.GetAuditLog(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	entriesArr, ok := body["entries"].([]interface{})
	if !ok {
		t.Fatalf("expected entries array, got %T: %v", body["entries"], body["entries"])
	}
	if len(entriesArr) != 0 {
		t.Errorf("expected empty entries array, got %d entries", len(entriesArr))
	}
	if body["total"].(float64) != 0 {
		t.Errorf("expected total=0, got %v", body["total"])
	}
}

func TestGetAuditLog_QueryError(t *testing.T) {
	q := &mockAuditQuerier{
		queryFn: func(_ context.Context, _ audit.QueryParams) ([]audit.Entry, int64, error) {
			return nil, 0, errors.New("db error")
		},
	}
	h := NewAuditHandler(q)

	admin := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), admin)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.GetAuditLog(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestGetAuditLog_LimitOffsetParams(t *testing.T) {
	var capturedParams audit.QueryParams
	q := &mockAuditQuerier{
		queryFn: func(_ context.Context, params audit.QueryParams) ([]audit.Entry, int64, error) {
			capturedParams = params
			return []audit.Entry{}, 0, nil
		},
	}
	h := NewAuditHandler(q)

	admin := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), admin)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit?limit=5&offset=10", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.GetAuditLog(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if capturedParams.Limit != 5 {
		t.Errorf("expected limit=5, got %d", capturedParams.Limit)
	}
	if capturedParams.Offset != 10 {
		t.Errorf("expected offset=10, got %d", capturedParams.Offset)
	}
}

func TestGetAuditLog_UserIDFilter(t *testing.T) {
	var capturedParams audit.QueryParams
	q := &mockAuditQuerier{
		queryFn: func(_ context.Context, params audit.QueryParams) ([]audit.Entry, int64, error) {
			capturedParams = params
			return []audit.Entry{}, 0, nil
		},
	}
	h := NewAuditHandler(q)

	admin := &model.User{ID: 1, Role: model.RoleAdmin}
	ctx := api.ContextWithUser(context.Background(), admin)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit?userId=42", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.GetAuditLog(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if capturedParams.UserID == nil || *capturedParams.UserID != 42 {
		t.Errorf("expected UserID=42, got %v", capturedParams.UserID)
	}
}

package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// mockApiKeyRepo is a mock implementation of repository.ApiKeyRepo for
// unit testing handlers without a database.
type mockApiKeyRepo struct {
	createFn         func(ctx context.Context, key *model.ApiKey) error
	getByTokenFn     func(ctx context.Context, token string) (*model.ApiKey, error)
	getByIDFn        func(ctx context.Context, id int64) (*model.ApiKey, error)
	listByUserFn     func(ctx context.Context, userID int64) ([]*model.ApiKey, error)
	deleteFn         func(ctx context.Context, id int64) error
	updateLastUsedFn func(ctx context.Context, id int64) error
}

// Compile-time assertion that mockApiKeyRepo satisfies repository.ApiKeyRepo.
var _ repository.ApiKeyRepo = (*mockApiKeyRepo)(nil)

func (m *mockApiKeyRepo) Create(ctx context.Context, key *model.ApiKey) error {
	if m.createFn != nil {
		return m.createFn(ctx, key)
	}
	key.ID = 1
	key.Token = "generated-test-token-0123456789abcdef0123456789abcdef"
	return nil
}

func (m *mockApiKeyRepo) GetByToken(ctx context.Context, token string) (*model.ApiKey, error) {
	if m.getByTokenFn != nil {
		return m.getByTokenFn(ctx, token)
	}
	return nil, errors.New("not found")
}

func (m *mockApiKeyRepo) GetByID(ctx context.Context, id int64) (*model.ApiKey, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, errors.New("not found")
}

func (m *mockApiKeyRepo) ListByUser(ctx context.Context, userID int64) ([]*model.ApiKey, error) {
	if m.listByUserFn != nil {
		return m.listByUserFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockApiKeyRepo) Delete(ctx context.Context, id int64) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockApiKeyRepo) UpdateLastUsed(ctx context.Context, id int64) error {
	if m.updateLastUsedFn != nil {
		return m.updateLastUsedFn(ctx, id)
	}
	return nil
}

// --- Create tests ---

func TestApiKeyHandler_Mock_Create_Success(t *testing.T) {
	mock := &mockApiKeyRepo{
		createFn: func(_ context.Context, key *model.ApiKey) error {
			key.ID = 42
			key.Token = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
			return nil
		},
	}
	h := handlers.NewApiKeyHandler(mock)
	user := &model.User{ID: 1, Email: "test@example.com"}

	body := `{"name":"My API Key","permissions":"full"}`
	req := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var key model.ApiKey
	if err := json.NewDecoder(rr.Body).Decode(&key); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if key.ID != 42 {
		t.Errorf("expected ID 42, got %d", key.ID)
	}
	if key.Token == "" {
		t.Error("expected non-empty token in creation response")
	}
}

func TestApiKeyHandler_Mock_Create_ReadonlyPermission(t *testing.T) {
	mock := &mockApiKeyRepo{
		createFn: func(_ context.Context, key *model.ApiKey) error {
			if key.Permissions != model.PermissionReadonly {
				t.Errorf("expected permissions %q, got %q", model.PermissionReadonly, key.Permissions)
			}
			key.ID = 1
			key.Token = "test-token"
			return nil
		},
	}
	h := handlers.NewApiKeyHandler(mock)
	user := &model.User{ID: 1, Email: "test@example.com"}

	body := `{"name":"Read Only Key","permissions":"readonly"}`
	req := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestApiKeyHandler_Mock_Create_DefaultPermission(t *testing.T) {
	mock := &mockApiKeyRepo{
		createFn: func(_ context.Context, key *model.ApiKey) error {
			if key.Permissions != model.PermissionFull {
				t.Errorf("expected default permissions %q, got %q", model.PermissionFull, key.Permissions)
			}
			key.ID = 1
			key.Token = "test-token"
			return nil
		},
	}
	h := handlers.NewApiKeyHandler(mock)
	user := &model.User{ID: 1, Email: "test@example.com"}

	body := `{"name":"Default Key"}`
	req := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestApiKeyHandler_Mock_Create_ValidationErrors(t *testing.T) {
	h := handlers.NewApiKeyHandler(&mockApiKeyRepo{})
	user := &model.User{ID: 1, Email: "test@example.com"}

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"invalid JSON", "not json", http.StatusBadRequest},
		{"empty name", `{"name":"","permissions":"full"}`, http.StatusBadRequest},
		{"missing name", `{"permissions":"full"}`, http.StatusBadRequest},
		{"invalid permissions", `{"name":"Test","permissions":"admin"}`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader([]byte(tt.body)))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(api.ContextWithUser(req.Context(), user))
			rr := httptest.NewRecorder()

			h.Create(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected %d, got %d; body: %s", tt.wantStatus, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestApiKeyHandler_Mock_Create_DBError(t *testing.T) {
	mock := &mockApiKeyRepo{
		createFn: func(_ context.Context, _ *model.ApiKey) error {
			return errors.New("database error")
		},
	}
	h := handlers.NewApiKeyHandler(mock)
	user := &model.User{ID: 1, Email: "test@example.com"}

	body := `{"name":"Test Key","permissions":"full"}`
	req := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestApiKeyHandler_Mock_Create_NoAuth(t *testing.T) {
	h := handlers.NewApiKeyHandler(&mockApiKeyRepo{})

	body := `{"name":"Test Key"}`
	req := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// --- Create with expiration tests ---

func TestApiKeyHandler_Mock_Create_WithExpiresInHours(t *testing.T) {
	var capturedExpiresAt *time.Time
	mock := &mockApiKeyRepo{
		createFn: func(_ context.Context, key *model.ApiKey) error {
			capturedExpiresAt = key.ExpiresAt
			key.ID = 1
			key.Token = "test-token"
			return nil
		},
	}
	h := handlers.NewApiKeyHandler(mock)
	user := &model.User{ID: 1, Email: "test@example.com"}

	body := `{"name":"Expiring Key","permissions":"full","expiresInHours":168}`
	req := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if capturedExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set")
	}
	// Should be approximately 168 hours from now (within 5 seconds tolerance).
	expected := time.Now().Add(168 * time.Hour)
	diff := capturedExpiresAt.Sub(expected)
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("ExpiresAt %v not within 5s of expected %v", capturedExpiresAt, expected)
	}
}

func TestApiKeyHandler_Mock_Create_WithExpiresAt(t *testing.T) {
	var capturedExpiresAt *time.Time
	mock := &mockApiKeyRepo{
		createFn: func(_ context.Context, key *model.ApiKey) error {
			capturedExpiresAt = key.ExpiresAt
			key.ID = 1
			key.Token = "test-token"
			return nil
		},
	}
	h := handlers.NewApiKeyHandler(mock)
	user := &model.User{ID: 1, Email: "test@example.com"}

	futureTime := time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339)
	body := `{"name":"Custom Expiry Key","permissions":"full","expiresAt":"` + futureTime + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if capturedExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set")
	}
}

func TestApiKeyHandler_Mock_Create_NeverExpires(t *testing.T) {
	var capturedExpiresAt *time.Time
	mock := &mockApiKeyRepo{
		createFn: func(_ context.Context, key *model.ApiKey) error {
			capturedExpiresAt = key.ExpiresAt
			key.ID = 1
			key.Token = "test-token"
			return nil
		},
	}
	h := handlers.NewApiKeyHandler(mock)
	user := &model.User{ID: 1, Email: "test@example.com"}

	body := `{"name":"No Expiry Key","permissions":"full"}`
	req := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if capturedExpiresAt != nil {
		t.Error("expected ExpiresAt to be nil for never-expiring key")
	}
}

func TestApiKeyHandler_Mock_Create_ExpirationValidationErrors(t *testing.T) {
	h := handlers.NewApiKeyHandler(&mockApiKeyRepo{})
	user := &model.User{ID: 1, Email: "test@example.com"}

	pastTime := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantError  string
	}{
		{
			"both expiresInHours and expiresAt",
			`{"name":"Test","permissions":"full","expiresInHours":24,"expiresAt":"2099-01-01T00:00:00Z"}`,
			http.StatusBadRequest,
			"specify either expiresInHours or expiresAt, not both",
		},
		{
			"negative expiresInHours",
			`{"name":"Test","permissions":"full","expiresInHours":-1}`,
			http.StatusBadRequest,
			"expiresInHours must be positive",
		},
		{
			"zero expiresInHours",
			`{"name":"Test","permissions":"full","expiresInHours":0}`,
			http.StatusBadRequest,
			"expiresInHours must be positive",
		},
		{
			"invalid expiresAt format",
			`{"name":"Test","permissions":"full","expiresAt":"not-a-date"}`,
			http.StatusBadRequest,
			"expiresAt must be a valid RFC 3339 timestamp",
		},
		{
			"expiresAt in the past",
			`{"name":"Test","permissions":"full","expiresAt":"` + pastTime + `"}`,
			http.StatusBadRequest,
			"expiresAt must be in the future",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader([]byte(tt.body)))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(api.ContextWithUser(req.Context(), user))
			rr := httptest.NewRecorder()

			h.Create(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected %d, got %d; body: %s", tt.wantStatus, rr.Code, rr.Body.String())
			}

			var body map[string]string
			if err := json.NewDecoder(rr.Body).Decode(&body); err == nil {
				if body["error"] != tt.wantError {
					t.Errorf("expected error %q, got %q", tt.wantError, body["error"])
				}
			}
		})
	}
}

func TestApiKeyHandler_Mock_Create_ExpiresAtInResponse(t *testing.T) {
	mock := &mockApiKeyRepo{
		createFn: func(_ context.Context, key *model.ApiKey) error {
			key.ID = 1
			key.Token = "test-token-abcdef01234567890abcdef01234567890abcdef01234567"
			return nil
		},
	}
	h := handlers.NewApiKeyHandler(mock)
	user := &model.User{ID: 1, Email: "test@example.com"}

	body := `{"name":"Expiring Key","permissions":"full","expiresInHours":24}`
	req := httptest.NewRequest(http.MethodPost, "/api/keys", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Verify the response includes expiresAt.
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["expiresAt"]; !ok {
		t.Error("expected expiresAt in response for expiring key")
	}
}

// --- List tests ---

func TestApiKeyHandler_Mock_List_ReturnsKeys(t *testing.T) {
	expiresAt := time.Now().Add(24 * time.Hour)
	mock := &mockApiKeyRepo{
		listByUserFn: func(_ context.Context, userID int64) ([]*model.ApiKey, error) {
			return []*model.ApiKey{
				{ID: 1, UserID: userID, Token: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", Name: "Key A", Permissions: "full"},
				{ID: 2, UserID: userID, Token: "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef", Name: "Key B", Permissions: "readonly", ExpiresAt: &expiresAt},
			}, nil
		},
	}
	h := handlers.NewApiKeyHandler(mock)
	user := &model.User{ID: 1, Email: "test@example.com"}

	req := httptest.NewRequest(http.MethodGet, "/api/keys", nil)
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var keys []map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&keys); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}

	// Verify tokens are redacted.
	for _, k := range keys {
		token, _ := k["token"].(string)
		if len(token) > 11 {
			t.Errorf("expected redacted token, got %q (len=%d)", token, len(token))
		}
	}

	// Verify expiresAt is present for the second key.
	if _, ok := keys[1]["expiresAt"]; !ok {
		t.Error("expected expiresAt in response for key with expiration")
	}
}

func TestApiKeyHandler_Mock_List_DBError(t *testing.T) {
	mock := &mockApiKeyRepo{
		listByUserFn: func(_ context.Context, _ int64) ([]*model.ApiKey, error) {
			return nil, errors.New("database error")
		},
	}
	h := handlers.NewApiKeyHandler(mock)
	user := &model.User{ID: 1, Email: "test@example.com"}

	req := httptest.NewRequest(http.MethodGet, "/api/keys", nil)
	req = req.WithContext(api.ContextWithUser(req.Context(), user))
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// --- Delete tests ---

func TestApiKeyHandler_Mock_Delete_OwnKey(t *testing.T) {
	var deletedID int64
	mock := &mockApiKeyRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.ApiKey, error) {
			return &model.ApiKey{ID: id, UserID: 1, Name: "My Key"}, nil
		},
		deleteFn: func(_ context.Context, id int64) error {
			deletedID = id
			return nil
		},
	}
	h := handlers.NewApiKeyHandler(mock)
	user := &model.User{ID: 1, Email: "test@example.com", Role: model.RoleUser}

	req := httptest.NewRequest(http.MethodDelete, "/api/keys/10", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "10")
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if deletedID != 10 {
		t.Errorf("expected delete called with ID=10, got %d", deletedID)
	}
}

func TestApiKeyHandler_Mock_Delete_OtherUsersKey_AsAdmin(t *testing.T) {
	var deletedID int64
	mock := &mockApiKeyRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.ApiKey, error) {
			return &model.ApiKey{ID: id, UserID: 99, Name: "Other User Key"}, nil
		},
		deleteFn: func(_ context.Context, id int64) error {
			deletedID = id
			return nil
		},
	}
	h := handlers.NewApiKeyHandler(mock)
	admin := &model.User{ID: 1, Email: "admin@example.com", Role: model.RoleAdmin}

	req := httptest.NewRequest(http.MethodDelete, "/api/keys/10", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "10")
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), admin), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if deletedID != 10 {
		t.Errorf("expected delete called with ID=10, got %d", deletedID)
	}
}

func TestApiKeyHandler_Mock_Delete_OtherUsersKey_Forbidden(t *testing.T) {
	mock := &mockApiKeyRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.ApiKey, error) {
			return &model.ApiKey{ID: id, UserID: 99, Name: "Other User Key"}, nil
		},
	}
	h := handlers.NewApiKeyHandler(mock)
	user := &model.User{ID: 1, Email: "user@example.com", Role: model.RoleUser}

	req := httptest.NewRequest(http.MethodDelete, "/api/keys/10", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "10")
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.Delete(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestApiKeyHandler_Mock_Delete_NotFound(t *testing.T) {
	mock := &mockApiKeyRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.ApiKey, error) {
			return nil, errors.New("not found")
		},
	}
	h := handlers.NewApiKeyHandler(mock)
	user := &model.User{ID: 1, Email: "test@example.com"}

	req := httptest.NewRequest(http.MethodDelete, "/api/keys/999", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "999")
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.Delete(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestApiKeyHandler_Mock_Delete_InvalidID(t *testing.T) {
	h := handlers.NewApiKeyHandler(&mockApiKeyRepo{})
	user := &model.User{ID: 1, Email: "test@example.com"}

	req := httptest.NewRequest(http.MethodDelete, "/api/keys/abc", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "abc")
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.Delete(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// --- AdminListUserKeys tests ---

func TestApiKeyHandler_Mock_AdminListUserKeys(t *testing.T) {
	mock := &mockApiKeyRepo{
		listByUserFn: func(_ context.Context, userID int64) ([]*model.ApiKey, error) {
			if userID != 42 {
				t.Errorf("expected userID=42, got %d", userID)
			}
			return []*model.ApiKey{
				{ID: 1, UserID: 42, Token: "abcdef0123456789", Name: "Key A", Permissions: "full"},
			}, nil
		},
	}
	h := handlers.NewApiKeyHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/users/42/keys", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "42")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.AdminListUserKeys(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var keys []*model.ApiKey
	if err := json.NewDecoder(rr.Body).Decode(&keys); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
}

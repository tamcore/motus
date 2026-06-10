package handlers_test

// Tests for the ogen Handler API key methods (ListApiKeys, CreateApiKey,
// DeleteApiKey, AdminListUserKeys). Ported from the deleted chi
// ApiKeyHandler tests in apikey_mock_test.go.
//
// Dropped tests (no live equivalent):
//   - expiresInHours variants: the OpenAPI spec's ApiKeyInput only supports
//     expiresAt; expiresInHours does not exist in the live API.
//   - invalid JSON / missing name / invalid expiresAt format / invalid path
//     ID: request decoding is owned by ogen, not the handler.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// newApiKeyTestHandler builds an ogen Handler from mock repositories with a
// nil-pool audit logger (Log is a documented no-op without a pool).
func newApiKeyTestHandler(apiKeys repository.ApiKeyRepo, users repository.UserRepo) *handlers.Handler {
	return handlers.NewHandler(handlers.HandlerConfig{
		Users:       users,
		ApiKeys:     apiKeys,
		AuditLogger: audit.NewLogger(nil),
	})
}

// ---------------------------------------------------------------------------
// CreateApiKey
// ---------------------------------------------------------------------------

func TestCreateApiKey_Success(t *testing.T) {
	mock := &mockApiKeyRepo{
		createFn: func(_ context.Context, key *model.ApiKey) error {
			key.ID = 42
			key.Token = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
			return nil
		},
	}
	h := newApiKeyTestHandler(mock, &mockUserRepo{})
	user := &model.User{ID: 1, Email: "test@example.com"}
	ctx := api.ContextWithUser(context.Background(), user)

	res, err := h.CreateApiKey(ctx, &oas.ApiKeyInput{
		Name:        "My API Key",
		Permissions: oas.NewOptApiKeyInputPermissions(oas.ApiKeyInputPermissionsFull),
	})
	if err != nil {
		t.Fatalf("CreateApiKey returned error: %v", err)
	}
	key, ok := res.(*oas.ApiKey)
	if !ok {
		t.Fatalf("expected *oas.ApiKey, got %T", res)
	}
	if key.ID != 42 {
		t.Errorf("expected ID 42, got %d", key.ID)
	}
	// The raw token must be exposed exactly once: on creation.
	if !key.Token.Set || key.Token.Value == "" {
		t.Error("expected non-empty token in creation response")
	}
}

func TestCreateApiKey_ReadonlyPermission(t *testing.T) {
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
	h := newApiKeyTestHandler(mock, &mockUserRepo{})
	ctx := api.ContextWithUser(context.Background(), &model.User{ID: 1, Email: "test@example.com"})

	res, err := h.CreateApiKey(ctx, &oas.ApiKeyInput{
		Name:        "Read Only Key",
		Permissions: oas.NewOptApiKeyInputPermissions(oas.ApiKeyInputPermissionsReadonly),
	})
	if err != nil {
		t.Fatalf("CreateApiKey returned error: %v", err)
	}
	if _, ok := res.(*oas.ApiKey); !ok {
		t.Fatalf("expected *oas.ApiKey, got %T", res)
	}
}

func TestCreateApiKey_DefaultPermission(t *testing.T) {
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
	h := newApiKeyTestHandler(mock, &mockUserRepo{})
	ctx := api.ContextWithUser(context.Background(), &model.User{ID: 1, Email: "test@example.com"})

	res, err := h.CreateApiKey(ctx, &oas.ApiKeyInput{Name: "Default Key"})
	if err != nil {
		t.Fatalf("CreateApiKey returned error: %v", err)
	}
	if _, ok := res.(*oas.ApiKey); !ok {
		t.Fatalf("expected *oas.ApiKey, got %T", res)
	}
}

func TestCreateApiKey_EmptyName(t *testing.T) {
	h := newApiKeyTestHandler(&mockApiKeyRepo{}, &mockUserRepo{})
	ctx := api.ContextWithUser(context.Background(), &model.User{ID: 1, Email: "test@example.com"})

	res, err := h.CreateApiKey(ctx, &oas.ApiKeyInput{Name: ""})
	if err != nil {
		t.Fatalf("CreateApiKey returned error: %v", err)
	}
	badReq, ok := res.(*oas.CreateApiKeyBadRequest)
	if !ok {
		t.Fatalf("expected *oas.CreateApiKeyBadRequest, got %T", res)
	}
	if badReq.Error != "name is required" {
		t.Errorf("unexpected error message: %q", badReq.Error)
	}
}

func TestCreateApiKey_InvalidPermissions(t *testing.T) {
	h := newApiKeyTestHandler(&mockApiKeyRepo{}, &mockUserRepo{})
	ctx := api.ContextWithUser(context.Background(), &model.User{ID: 1, Email: "test@example.com"})

	res, err := h.CreateApiKey(ctx, &oas.ApiKeyInput{
		Name:        "Test",
		Permissions: oas.NewOptApiKeyInputPermissions("admin"),
	})
	if err != nil {
		t.Fatalf("CreateApiKey returned error: %v", err)
	}
	badReq, ok := res.(*oas.CreateApiKeyBadRequest)
	if !ok {
		t.Fatalf("expected *oas.CreateApiKeyBadRequest, got %T", res)
	}
	if badReq.Error != "permissions must be 'full' or 'readonly'" {
		t.Errorf("unexpected error message: %q", badReq.Error)
	}
}

func TestCreateApiKey_ExpiresAtInPast(t *testing.T) {
	h := newApiKeyTestHandler(&mockApiKeyRepo{}, &mockUserRepo{})
	ctx := api.ContextWithUser(context.Background(), &model.User{ID: 1, Email: "test@example.com"})

	res, err := h.CreateApiKey(ctx, &oas.ApiKeyInput{
		Name:      "Test",
		ExpiresAt: oas.NewOptDateTime(time.Now().Add(-1 * time.Hour)),
	})
	if err != nil {
		t.Fatalf("CreateApiKey returned error: %v", err)
	}
	badReq, ok := res.(*oas.CreateApiKeyBadRequest)
	if !ok {
		t.Fatalf("expected *oas.CreateApiKeyBadRequest, got %T", res)
	}
	if badReq.Error != "expiresAt must be in the future" {
		t.Errorf("unexpected error message: %q", badReq.Error)
	}
}

func TestCreateApiKey_WithExpiresAt(t *testing.T) {
	var capturedExpiresAt *time.Time
	mock := &mockApiKeyRepo{
		createFn: func(_ context.Context, key *model.ApiKey) error {
			capturedExpiresAt = key.ExpiresAt
			key.ID = 1
			key.Token = "test-token"
			return nil
		},
	}
	h := newApiKeyTestHandler(mock, &mockUserRepo{})
	ctx := api.ContextWithUser(context.Background(), &model.User{ID: 1, Email: "test@example.com"})

	future := time.Now().Add(30 * 24 * time.Hour)
	res, err := h.CreateApiKey(ctx, &oas.ApiKeyInput{
		Name:      "Custom Expiry Key",
		ExpiresAt: oas.NewOptDateTime(future),
	})
	if err != nil {
		t.Fatalf("CreateApiKey returned error: %v", err)
	}
	key, ok := res.(*oas.ApiKey)
	if !ok {
		t.Fatalf("expected *oas.ApiKey, got %T", res)
	}
	if capturedExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be passed to the repository")
	}
	if !capturedExpiresAt.Equal(future) {
		t.Errorf("expected ExpiresAt %v, got %v", future, capturedExpiresAt)
	}
	// The creation response must echo the expiry back.
	if !key.ExpiresAt.Set {
		t.Error("expected expiresAt in creation response for expiring key")
	}
}

func TestCreateApiKey_NeverExpires(t *testing.T) {
	var capturedExpiresAt *time.Time
	mock := &mockApiKeyRepo{
		createFn: func(_ context.Context, key *model.ApiKey) error {
			capturedExpiresAt = key.ExpiresAt
			key.ID = 1
			key.Token = "test-token"
			return nil
		},
	}
	h := newApiKeyTestHandler(mock, &mockUserRepo{})
	ctx := api.ContextWithUser(context.Background(), &model.User{ID: 1, Email: "test@example.com"})

	res, err := h.CreateApiKey(ctx, &oas.ApiKeyInput{
		Name:        "No Expiry Key",
		Permissions: oas.NewOptApiKeyInputPermissions(oas.ApiKeyInputPermissionsFull),
	})
	if err != nil {
		t.Fatalf("CreateApiKey returned error: %v", err)
	}
	if _, ok := res.(*oas.ApiKey); !ok {
		t.Fatalf("expected *oas.ApiKey, got %T", res)
	}
	if capturedExpiresAt != nil {
		t.Error("expected ExpiresAt to be nil for never-expiring key")
	}
}

func TestCreateApiKey_DBError(t *testing.T) {
	mock := &mockApiKeyRepo{
		createFn: func(_ context.Context, _ *model.ApiKey) error {
			return errors.New("database error")
		},
	}
	h := newApiKeyTestHandler(mock, &mockUserRepo{})
	ctx := api.ContextWithUser(context.Background(), &model.User{ID: 1, Email: "test@example.com"})

	res, err := h.CreateApiKey(ctx, &oas.ApiKeyInput{
		Name:        "Test Key",
		Permissions: oas.NewOptApiKeyInputPermissions(oas.ApiKeyInputPermissionsFull),
	})
	if err != nil {
		t.Fatalf("CreateApiKey returned error: %v", err)
	}
	// The live handler maps repository failures to a typed bad-request.
	badReq, ok := res.(*oas.CreateApiKeyBadRequest)
	if !ok {
		t.Fatalf("expected *oas.CreateApiKeyBadRequest on repository failure, got %T", res)
	}
	if badReq.Error != "failed to create API key" {
		t.Errorf("unexpected error message: %q", badReq.Error)
	}
}

func TestCreateApiKey_Unauthenticated(t *testing.T) {
	h := newApiKeyTestHandler(&mockApiKeyRepo{}, &mockUserRepo{})

	res, err := h.CreateApiKey(context.Background(), &oas.ApiKeyInput{Name: "Test Key"})
	if err != nil {
		t.Fatalf("CreateApiKey returned error: %v", err)
	}
	if _, ok := res.(*oas.CreateApiKeyUnauthorized); !ok {
		t.Errorf("expected *oas.CreateApiKeyUnauthorized, got %T", res)
	}
}

// ---------------------------------------------------------------------------
// ListApiKeys
// ---------------------------------------------------------------------------

// TestListApiKeys_RedactsTokens verifies that list responses never expose the
// raw token (apiKeyToOAS with includeToken=false leaves Token unset).
func TestListApiKeys_RedactsTokens(t *testing.T) {
	expiresAt := time.Now().Add(24 * time.Hour)
	mock := &mockApiKeyRepo{
		listByUserFn: func(_ context.Context, userID int64) ([]*model.ApiKey, error) {
			return []*model.ApiKey{
				{ID: 1, UserID: userID, Token: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", Name: "Key A", Permissions: "full"},
				{ID: 2, UserID: userID, Token: "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef", Name: "Key B", Permissions: "readonly", ExpiresAt: &expiresAt},
			}, nil
		},
	}
	h := newApiKeyTestHandler(mock, &mockUserRepo{})
	ctx := api.ContextWithUser(context.Background(), &model.User{ID: 1, Email: "test@example.com"})

	res, err := h.ListApiKeys(ctx)
	if err != nil {
		t.Fatalf("ListApiKeys returned error: %v", err)
	}
	list, ok := res.(*oas.ListApiKeysOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.ListApiKeysOKApplicationJSON, got %T", res)
	}
	if len(*list) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(*list))
	}

	for _, k := range *list {
		if k.Token.Set || k.Token.Value != "" {
			t.Errorf("expected redacted (unset) token for key %d, got %q", k.ID, k.Token.Value)
		}
	}

	// expiresAt must be present for the second key.
	if !(*list)[1].ExpiresAt.Set {
		t.Error("expected expiresAt in response for key with expiration")
	}
}

func TestListApiKeys_DBError(t *testing.T) {
	mock := &mockApiKeyRepo{
		listByUserFn: func(_ context.Context, _ int64) ([]*model.ApiKey, error) {
			return nil, errors.New("database error")
		},
	}
	h := newApiKeyTestHandler(mock, &mockUserRepo{})
	ctx := api.ContextWithUser(context.Background(), &model.User{ID: 1, Email: "test@example.com"})

	res, err := h.ListApiKeys(ctx)
	if err != nil {
		t.Fatalf("ListApiKeys returned error: %v", err)
	}
	if _, ok := res.(*oas.Error); !ok {
		t.Errorf("expected *oas.Error on repository failure, got %T", res)
	}
}

func TestListApiKeys_Unauthenticated(t *testing.T) {
	h := newApiKeyTestHandler(&mockApiKeyRepo{}, &mockUserRepo{})

	res, err := h.ListApiKeys(context.Background())
	if err != nil {
		t.Fatalf("ListApiKeys returned error: %v", err)
	}
	if _, ok := res.(*oas.Error); !ok {
		t.Errorf("expected *oas.Error for unauthenticated request, got %T", res)
	}
}

// ---------------------------------------------------------------------------
// DeleteApiKey
// ---------------------------------------------------------------------------

func TestDeleteApiKey_OwnKey(t *testing.T) {
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
	h := newApiKeyTestHandler(mock, &mockUserRepo{})
	ctx := api.ContextWithUser(context.Background(), &model.User{ID: 1, Email: "test@example.com", Role: model.RoleUser})

	res, err := h.DeleteApiKey(ctx, oas.DeleteApiKeyParams{ID: 10})
	if err != nil {
		t.Fatalf("DeleteApiKey returned error: %v", err)
	}
	if _, ok := res.(*oas.DeleteApiKeyNoContent); !ok {
		t.Fatalf("expected *oas.DeleteApiKeyNoContent, got %T", res)
	}
	if deletedID != 10 {
		t.Errorf("expected delete called with ID=10, got %d", deletedID)
	}
}

func TestDeleteApiKey_OtherUsersKey_AsAdmin(t *testing.T) {
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
	h := newApiKeyTestHandler(mock, &mockUserRepo{})
	ctx := api.ContextWithUser(context.Background(), &model.User{ID: 1, Email: "admin@example.com", Role: model.RoleAdmin})

	res, err := h.DeleteApiKey(ctx, oas.DeleteApiKeyParams{ID: 10})
	if err != nil {
		t.Fatalf("DeleteApiKey returned error: %v", err)
	}
	if _, ok := res.(*oas.DeleteApiKeyNoContent); !ok {
		t.Fatalf("expected *oas.DeleteApiKeyNoContent for admin, got %T", res)
	}
	if deletedID != 10 {
		t.Errorf("expected delete called with ID=10, got %d", deletedID)
	}
}

// TestDeleteApiKey_OtherUsersKey_Forbidden verifies the IDOR protection: a
// non-admin must not be able to delete another user's API key.
func TestDeleteApiKey_OtherUsersKey_Forbidden(t *testing.T) {
	deleteCalled := false
	mock := &mockApiKeyRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.ApiKey, error) {
			return &model.ApiKey{ID: id, UserID: 99, Name: "Other User Key"}, nil
		},
		deleteFn: func(_ context.Context, _ int64) error {
			deleteCalled = true
			return nil
		},
	}
	h := newApiKeyTestHandler(mock, &mockUserRepo{})
	ctx := api.ContextWithUser(context.Background(), &model.User{ID: 1, Email: "user@example.com", Role: model.RoleUser})

	res, err := h.DeleteApiKey(ctx, oas.DeleteApiKeyParams{ID: 10})
	if err != nil {
		t.Fatalf("DeleteApiKey returned error: %v", err)
	}
	if _, ok := res.(*oas.DeleteApiKeyForbidden); !ok {
		t.Fatalf("expected *oas.DeleteApiKeyForbidden, got %T", res)
	}
	if deleteCalled {
		t.Error("Delete must not be called for another user's key")
	}
}

func TestDeleteApiKey_NotFound(t *testing.T) {
	mock := &mockApiKeyRepo{
		getByIDFn: func(_ context.Context, _ int64) (*model.ApiKey, error) {
			return nil, errors.New("not found")
		},
	}
	h := newApiKeyTestHandler(mock, &mockUserRepo{})
	ctx := api.ContextWithUser(context.Background(), &model.User{ID: 1, Email: "test@example.com"})

	res, err := h.DeleteApiKey(ctx, oas.DeleteApiKeyParams{ID: 999})
	if err != nil {
		t.Fatalf("DeleteApiKey returned error: %v", err)
	}
	if _, ok := res.(*oas.DeleteApiKeyNotFound); !ok {
		t.Errorf("expected *oas.DeleteApiKeyNotFound, got %T", res)
	}
}

func TestDeleteApiKey_Unauthenticated(t *testing.T) {
	h := newApiKeyTestHandler(&mockApiKeyRepo{}, &mockUserRepo{})

	res, err := h.DeleteApiKey(context.Background(), oas.DeleteApiKeyParams{ID: 10})
	if err != nil {
		t.Fatalf("DeleteApiKey returned error: %v", err)
	}
	if _, ok := res.(*oas.DeleteApiKeyUnauthorized); !ok {
		t.Errorf("expected *oas.DeleteApiKeyUnauthorized, got %T", res)
	}
}

// ---------------------------------------------------------------------------
// AdminListUserKeys
// ---------------------------------------------------------------------------

func TestAdminListUserKeys_AdminOnly(t *testing.T) {
	h := newApiKeyTestHandler(&mockApiKeyRepo{}, &mockUserRepo{})

	// Unauthenticated.
	res, err := h.AdminListUserKeys(context.Background(), oas.AdminListUserKeysParams{ID: 42})
	if err != nil {
		t.Fatalf("AdminListUserKeys returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminListUserKeysForbidden); !ok {
		t.Errorf("expected *oas.AdminListUserKeysForbidden for unauthenticated request, got %T", res)
	}

	// Non-admin.
	ctx := api.ContextWithUser(context.Background(), &model.User{ID: 1, Role: model.RoleUser})
	res, err = h.AdminListUserKeys(ctx, oas.AdminListUserKeysParams{ID: 42})
	if err != nil {
		t.Fatalf("AdminListUserKeys returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminListUserKeysForbidden); !ok {
		t.Errorf("expected *oas.AdminListUserKeysForbidden for non-admin, got %T", res)
	}
}

func TestAdminListUserKeys_Success(t *testing.T) {
	users := &mockUserRepo{
		getByIDFn: func(_ context.Context, id int64) (*model.User, error) {
			if id == 42 {
				return &model.User{ID: 42, Email: "target@example.com"}, nil
			}
			return nil, errors.New("not found")
		},
	}
	apiKeys := &mockApiKeyRepo{
		listByUserFn: func(_ context.Context, userID int64) ([]*model.ApiKey, error) {
			if userID != 42 {
				t.Errorf("expected userID=42, got %d", userID)
			}
			return []*model.ApiKey{
				{ID: 1, UserID: 42, Token: "abcdef0123456789", Name: "Key A", Permissions: "full"},
			}, nil
		},
	}
	h := newApiKeyTestHandler(apiKeys, users)

	ctx := api.ContextWithUser(context.Background(), &model.User{ID: 1, Email: "admin@example.com", Role: model.RoleAdmin})
	res, err := h.AdminListUserKeys(ctx, oas.AdminListUserKeysParams{ID: 42})
	if err != nil {
		t.Fatalf("AdminListUserKeys returned error: %v", err)
	}
	list, ok := res.(*oas.AdminListUserKeysOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.AdminListUserKeysOKApplicationJSON, got %T", res)
	}
	if len(*list) != 1 {
		t.Fatalf("expected 1 key, got %d", len(*list))
	}
	// Admin list responses are redacted as well.
	if (*list)[0].Token.Set {
		t.Errorf("expected redacted (unset) token, got %q", (*list)[0].Token.Value)
	}
}

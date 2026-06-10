package handlers_test

// Integration tests for the ogen Handler device-share methods (CreateShare,
// ListShares, DeleteShare, GetSharedDevice). Ported from the deleted chi
// ShareHandler test files share_integration_test.go and share_test.go.
// They require Docker (testcontainers) and are skipped automatically in
// -short mode by testutil.SetupTestDB.
//
// Dropped tests (no live equivalent):
//   - unauthenticated/invalid-ID/invalid-JSON transport tests
//     (share_test.go): ogen owns request decoding and path-param parsing;
//     the unauthenticated branch requires a missing user context which the
//     SecurityHandler prevents in production.
//   - shareUrl response-structure test: the live API returns the bare
//     DeviceShare object; clients build the /share/<token> URL themselves.
//   - "positions included in shared device response": the live
//     GetSharedDevice returns only the device; positions are delivered via
//     the WebSocket share-token channel.
//
// Behavioural delta vs the chi handler, asserted as the live contract:
//   - expiry is supplied as an absolute expiresAt timestamp instead of
//     expiresInHours.

import (
	"context"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

type shareOASIntegrationEnv struct {
	handler    *handlers.Handler
	shareRepo  *repository.DeviceShareRepository
	deviceRepo *repository.DeviceRepository
	userRepo   *repository.UserRepository
}

func setupShareOASIntegration(t *testing.T) *shareOASIntegrationEnv {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	shareRepo := repository.NewDeviceShareRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)

	h := handlers.NewHandler(handlers.HandlerConfig{
		Shares:      shareRepo,
		Devices:     deviceRepo,
		Users:       userRepo,
		AuditLogger: audit.NewLogger(nil),
	})
	return &shareOASIntegrationEnv{
		handler:    h,
		shareRepo:  shareRepo,
		deviceRepo: deviceRepo,
		userRepo:   userRepo,
	}
}

// createShareUserAndDevice creates a user and a device owned by that user.
func (e *shareOASIntegrationEnv) createShareUserAndDevice(t *testing.T) (*model.User, *model.Device) {
	t.Helper()
	ctx := context.Background()

	user := &model.User{Email: "share@example.com", PasswordHash: "hash", Name: "Share User"}
	if err := e.userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	device := &model.Device{UniqueID: "share-test-001", Name: "Test Device", Status: "online"}
	if err := e.deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}
	return user, device
}

func (e *shareOASIntegrationEnv) createOtherUser(t *testing.T, email string) *model.User {
	t.Helper()
	other := &model.User{Email: email, PasswordHash: "hash", Name: "Other User"}
	if err := e.userRepo.Create(context.Background(), other); err != nil {
		t.Fatalf("create other user: %v", err)
	}
	return other
}

func shareUserCtx(user *model.User) context.Context {
	return api.ContextWithUser(context.Background(), user)
}

// ---------------------------------------------------------------------------
// CreateShare
// ---------------------------------------------------------------------------

func TestCreateShare_Success_OAS(t *testing.T) {
	env := setupShareOASIntegration(t)
	user, device := env.createShareUserAndDevice(t)

	res, err := env.handler.CreateShare(shareUserCtx(user),
		oas.OptCreateShareRequest{}, oas.CreateShareParams{ID: device.ID})
	if err != nil {
		t.Fatalf("CreateShare returned error: %v", err)
	}
	share, ok := res.(*oas.DeviceShare)
	if !ok {
		t.Fatalf("expected *oas.DeviceShare, got %T", res)
	}
	if share.Token == "" {
		t.Error("expected non-empty token in response")
	}
	if share.DeviceId != device.ID {
		t.Errorf("expected deviceId %d, got %d", device.ID, share.DeviceId)
	}
	if share.CreatedBy != user.ID {
		t.Errorf("expected createdBy %d, got %d", user.ID, share.CreatedBy)
	}
}

func TestCreateShare_WithExpiry_OAS(t *testing.T) {
	env := setupShareOASIntegration(t)
	user, device := env.createShareUserAndDevice(t)

	expiresAt := time.Now().Add(24 * time.Hour).UTC()
	req := oas.NewOptCreateShareRequest(oas.CreateShareRequest{
		ExpiresAt: oas.NewOptDateTime(expiresAt),
	})

	res, err := env.handler.CreateShare(shareUserCtx(user), req, oas.CreateShareParams{ID: device.ID})
	if err != nil {
		t.Fatalf("CreateShare returned error: %v", err)
	}
	share, ok := res.(*oas.DeviceShare)
	if !ok {
		t.Fatalf("expected *oas.DeviceShare, got %T", res)
	}
	got, hasExpiry := share.ExpiresAt.Get()
	if !hasExpiry {
		t.Fatal("expected expiresAt to be set when supplied in the request")
	}
	if diff := got.Sub(expiresAt); diff > time.Second || diff < -time.Second {
		t.Errorf("expected expiresAt ~%v, got %v", expiresAt, got)
	}
}

func TestCreateShare_NoAccess_OAS(t *testing.T) {
	env := setupShareOASIntegration(t)
	_, device := env.createShareUserAndDevice(t)
	other := env.createOtherUser(t, "other@example.com")

	// IDOR: a user without access to the device must not be able to share it.
	res, err := env.handler.CreateShare(shareUserCtx(other),
		oas.OptCreateShareRequest{}, oas.CreateShareParams{ID: device.ID})
	if err != nil {
		t.Fatalf("CreateShare returned error: %v", err)
	}
	if _, ok := res.(*oas.CreateShareForbidden); !ok {
		t.Errorf("expected *oas.CreateShareForbidden, got %T", res)
	}
}

// ---------------------------------------------------------------------------
// ListShares
// ---------------------------------------------------------------------------

func TestListShares_Success_OAS(t *testing.T) {
	env := setupShareOASIntegration(t)
	user, device := env.createShareUserAndDevice(t)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		share := &model.DeviceShare{DeviceID: device.ID, CreatedBy: user.ID}
		if err := env.shareRepo.Create(ctx, share); err != nil {
			t.Fatalf("create share: %v", err)
		}
	}

	res, err := env.handler.ListShares(shareUserCtx(user), oas.ListSharesParams{ID: device.ID})
	if err != nil {
		t.Fatalf("ListShares returned error: %v", err)
	}
	list, ok := res.(*oas.ListSharesOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.ListSharesOKApplicationJSON, got %T", res)
	}
	if len(*list) != 2 {
		t.Errorf("expected 2 shares, got %d", len(*list))
	}
}

// ---------------------------------------------------------------------------
// GetSharedDevice (public, token-based)
// ---------------------------------------------------------------------------

func TestGetSharedDevice_Success_OAS(t *testing.T) {
	env := setupShareOASIntegration(t)
	user, device := env.createShareUserAndDevice(t)
	ctx := context.Background()

	share := &model.DeviceShare{DeviceID: device.ID, CreatedBy: user.ID}
	if err := env.shareRepo.Create(ctx, share); err != nil {
		t.Fatalf("create share: %v", err)
	}

	// No authenticated user in context: the share token alone grants access.
	res, err := env.handler.GetSharedDevice(context.Background(),
		oas.GetSharedDeviceParams{Token: share.Token})
	if err != nil {
		t.Fatalf("GetSharedDevice returned error: %v", err)
	}
	dev, ok := res.(*oas.Device)
	if !ok {
		t.Fatalf("expected *oas.Device, got %T", res)
	}
	if dev.ID != device.ID {
		t.Errorf("expected device ID %d, got %d", device.ID, dev.ID)
	}
	if dev.Name != "Test Device" {
		t.Errorf("expected device name 'Test Device', got %q", dev.Name)
	}
}

func TestGetSharedDevice_ExpiredToken_OAS(t *testing.T) {
	env := setupShareOASIntegration(t)
	user, device := env.createShareUserAndDevice(t)
	ctx := context.Background()

	expired := time.Now().Add(-1 * time.Hour)
	share := &model.DeviceShare{DeviceID: device.ID, CreatedBy: user.ID, ExpiresAt: &expired}
	if err := env.shareRepo.Create(ctx, share); err != nil {
		t.Fatalf("create share: %v", err)
	}

	res, err := env.handler.GetSharedDevice(context.Background(),
		oas.GetSharedDeviceParams{Token: share.Token})
	if err != nil {
		t.Fatalf("GetSharedDevice returned error: %v", err)
	}
	if _, ok := res.(*oas.Error); !ok {
		t.Errorf("expected *oas.Error for expired share token, got %T", res)
	}
}

// ---------------------------------------------------------------------------
// DeleteShare
// ---------------------------------------------------------------------------

func TestDeleteShare_Unauthorized_OAS(t *testing.T) {
	env := setupShareOASIntegration(t)
	user, device := env.createShareUserAndDevice(t)
	ctx := context.Background()

	share := &model.DeviceShare{DeviceID: device.ID, CreatedBy: user.ID}
	if err := env.shareRepo.Create(ctx, share); err != nil {
		t.Fatalf("create share: %v", err)
	}
	other := env.createOtherUser(t, "other-del@example.com")

	// IDOR: a user without access to the underlying device must not be able
	// to delete the share.
	res, err := env.handler.DeleteShare(shareUserCtx(other), oas.DeleteShareParams{ID: share.ID})
	if err != nil {
		t.Fatalf("DeleteShare returned error: %v", err)
	}
	if _, ok := res.(*oas.DeleteShareForbidden); !ok {
		t.Errorf("expected *oas.DeleteShareForbidden, got %T", res)
	}

	// The share must NOT have been deleted.
	if found, err := env.shareRepo.GetByToken(ctx, share.Token); err != nil || found == nil {
		t.Error("expected share to still exist after unauthorized delete attempt")
	}
}

func TestDeleteShare_NotFound_OAS(t *testing.T) {
	env := setupShareOASIntegration(t)
	user := env.createOtherUser(t, "del-notfound@example.com")

	res, err := env.handler.DeleteShare(shareUserCtx(user), oas.DeleteShareParams{ID: 99999})
	if err != nil {
		t.Fatalf("DeleteShare returned error: %v", err)
	}
	if _, ok := res.(*oas.DeleteShareNotFound); !ok {
		t.Errorf("expected *oas.DeleteShareNotFound, got %T", res)
	}
}

func TestDeleteShare_Success_OAS(t *testing.T) {
	env := setupShareOASIntegration(t)
	user, device := env.createShareUserAndDevice(t)
	ctx := context.Background()

	share := &model.DeviceShare{DeviceID: device.ID, CreatedBy: user.ID}
	if err := env.shareRepo.Create(ctx, share); err != nil {
		t.Fatalf("create share: %v", err)
	}

	res, err := env.handler.DeleteShare(shareUserCtx(user), oas.DeleteShareParams{ID: share.ID})
	if err != nil {
		t.Fatalf("DeleteShare returned error: %v", err)
	}
	if _, ok := res.(*oas.DeleteShareNoContent); !ok {
		t.Fatalf("expected *oas.DeleteShareNoContent, got %T", res)
	}

	if _, err := env.shareRepo.GetByToken(ctx, share.Token); err == nil {
		t.Error("expected share to be deleted")
	}
}

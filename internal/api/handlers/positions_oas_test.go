package handlers_test

// Tests for the ogen Handler position methods (GetPositions in positions.go,
// AdminListPositions in server_oas.go). Ported from the deleted chi
// PositionHandler tests in positions_test.go. The integration tests require
// Docker (testcontainers) and are skipped automatically in -short mode by
// testutil.SetupTestDB.
//
// Dropped tests (no live equivalent):
//   - ?id=X&id=Y position-by-ID lookups (ByID, ByMultipleIDs, ByID_InvalidID,
//     ByID_AccessControl, ByID_NonExistent): the live GET /api/positions has
//     no id parameter.
//   - invalid deviceId transport test: ogen owns query-param parsing.
//   - explicit limit tests: the live GET /api/positions has no limit
//     parameter; the repository maximum applies.
//   - AdminGetAllPositions time-range mode: the live AdminListPositions
//     returns the latest position per device only.

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

type positionsOASIntegrationEnv struct {
	handler *handlers.Handler
	posRepo *repository.PositionRepository
	user    *model.User
	device  *model.Device
}

func setupPositionsOASIntegration(t *testing.T) *positionsOASIntegrationEnv {
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

	h := handlers.NewHandler(handlers.HandlerConfig{
		Positions:   posRepo,
		Devices:     deviceRepo,
		Users:       userRepo,
		AuditLogger: audit.NewLogger(nil),
	})
	return &positionsOASIntegrationEnv{handler: h, posRepo: posRepo, user: user, device: device}
}

func (e *positionsOASIntegrationEnv) userCtx() context.Context {
	return api.ContextWithUser(context.Background(), e.user)
}

// decodePositionsRes unwraps the bare-array success response.
func decodePositionsRes(t *testing.T, res oas.GetPositionsRes) oas.GetPositionsOKApplicationJSON {
	t.Helper()
	list, ok := res.(*oas.GetPositionsOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.GetPositionsOKApplicationJSON, got %T", res)
	}
	return *list
}

func TestGetPositions_LatestByUser_OAS(t *testing.T) {
	env := setupPositionsOASIntegration(t)
	ctx := context.Background()

	now := time.Now().UTC()
	_ = env.posRepo.Create(ctx, &model.Position{DeviceID: env.device.ID, Latitude: 52.0, Longitude: 13.0, Timestamp: now.Add(-10 * time.Minute)})
	_ = env.posRepo.Create(ctx, &model.Position{DeviceID: env.device.ID, Latitude: 52.1, Longitude: 13.1, Timestamp: now})

	res, err := env.handler.GetPositions(env.userCtx(), oas.GetPositionsParams{})
	if err != nil {
		t.Fatalf("GetPositions returned error: %v", err)
	}
	positions := decodePositionsRes(t, res)
	// Latest position per device: 1 device = 1 position.
	if len(positions) != 1 {
		t.Errorf("expected 1 latest position, got %d", len(positions))
	}
}

func TestGetPositions_ByDevice_OAS(t *testing.T) {
	env := setupPositionsOASIntegration(t)
	ctx := context.Background()

	now := time.Now().UTC()
	_ = env.posRepo.Create(ctx, &model.Position{DeviceID: env.device.ID, Latitude: 52.0, Longitude: 13.0, Timestamp: now.Add(-10 * time.Minute)})
	_ = env.posRepo.Create(ctx, &model.Position{DeviceID: env.device.ID, Latitude: 52.1, Longitude: 13.1, Timestamp: now})

	res, err := env.handler.GetPositions(env.userCtx(), oas.GetPositionsParams{
		DeviceId: oas.NewOptInt64(env.device.ID),
		From:     oas.NewOptDateTime(now.Add(-1 * time.Hour)),
		To:       oas.NewOptDateTime(now.Add(time.Minute)),
	})
	if err != nil {
		t.Fatalf("GetPositions returned error: %v", err)
	}
	positions := decodePositionsRes(t, res)
	if len(positions) != 2 {
		t.Errorf("expected 2 positions in range, got %d", len(positions))
	}
}

func TestGetPositions_DeviceForbidden_OAS(t *testing.T) {
	env := setupPositionsOASIntegration(t)

	// IDOR: a different user must not read another user's device trail.
	otherUser := &model.User{ID: env.user.ID + 999, Email: "other@example.com", Name: "Other"}
	otherCtx := api.ContextWithUser(context.Background(), otherUser)

	res, err := env.handler.GetPositions(otherCtx, oas.GetPositionsParams{
		DeviceId: oas.NewOptInt64(env.device.ID),
	})
	if err != nil {
		t.Fatalf("GetPositions returned error: %v", err)
	}
	errRes, ok := res.(*oas.Error)
	if !ok {
		t.Fatalf("expected *oas.Error for foreign device, got %T", res)
	}
	if errRes.Error != "access denied" {
		t.Errorf("expected 'access denied', got %q", errRes.Error)
	}
}

func TestGetPositions_Unauthenticated_OAS(t *testing.T) {
	env := setupPositionsOASIntegration(t)

	res, err := env.handler.GetPositions(context.Background(), oas.GetPositionsParams{})
	if err != nil {
		t.Fatalf("GetPositions returned error: %v", err)
	}
	errRes, ok := res.(*oas.Error)
	if !ok {
		t.Fatalf("expected *oas.Error without user context, got %T", res)
	}
	if errRes.Error != "unauthorized" {
		t.Errorf("expected 'unauthorized', got %q", errRes.Error)
	}
}

func TestGetPositions_EmptyResult_OAS(t *testing.T) {
	env := setupPositionsOASIntegration(t)

	res, err := env.handler.GetPositions(env.userCtx(), oas.GetPositionsParams{})
	if err != nil {
		t.Fatalf("GetPositions returned error: %v", err)
	}
	positions := decodePositionsRes(t, res)
	if len(positions) != 0 {
		t.Errorf("expected 0 positions, got %d", len(positions))
	}
}

func TestGetPositions_TimeRange_StreamsAll_OAS(t *testing.T) {
	env := setupPositionsOASIntegration(t)
	ctx := context.Background()

	now := time.Now().UTC()
	const total = 5
	for i := range total {
		p := &model.Position{
			DeviceID:  env.device.ID,
			Latitude:  52.0 + float64(i)*0.01,
			Longitude: 13.0,
			Timestamp: now.Add(time.Duration(-total+i) * time.Minute),
		}
		if err := env.posRepo.Create(ctx, p); err != nil {
			t.Fatalf("Create position %d: %v", i, err)
		}
	}

	res, err := env.handler.GetPositions(env.userCtx(), oas.GetPositionsParams{
		DeviceId: oas.NewOptInt64(env.device.ID),
		From:     oas.NewOptDateTime(now.Add(-time.Hour)),
		To:       oas.NewOptDateTime(now.Add(time.Minute)),
	})
	if err != nil {
		t.Fatalf("GetPositions returned error: %v", err)
	}
	positions := decodePositionsRes(t, res)
	if len(positions) != total {
		t.Errorf("expected all %d positions streamed, got %d", total, len(positions))
	}
}

// TestGetPositions_DefaultLimitReturnsAll_OAS verifies that the live handler
// returns every position in the time range (up to the repository maximum of
// 10000). This guards against the "straight line trail" bug where a default
// limit of 100 truncated the trail.
func TestGetPositions_DefaultLimitReturnsAll_OAS(t *testing.T) {
	env := setupPositionsOASIntegration(t)
	ctx := context.Background()

	now := time.Now().UTC()
	// More than the old default limit of 100, well under the max of 10000.
	const count = 150
	for i := 0; i < count; i++ {
		ts := now.Add(-time.Duration(count-i) * time.Minute)
		if err := env.posRepo.Create(ctx, &model.Position{
			DeviceID:  env.device.ID,
			Latitude:  52.0 + float64(i)*0.001,
			Longitude: 13.0 + float64(i)*0.001,
			Timestamp: ts,
		}); err != nil {
			t.Fatalf("create position %d: %v", i, err)
		}
	}

	res, err := env.handler.GetPositions(env.userCtx(), oas.GetPositionsParams{
		DeviceId: oas.NewOptInt64(env.device.ID),
		From:     oas.NewOptDateTime(now.Add(-time.Duration(count+1) * time.Minute)),
		To:       oas.NewOptDateTime(now.Add(time.Minute)),
	})
	if err != nil {
		t.Fatalf("GetPositions returned error: %v", err)
	}
	positions := decodePositionsRes(t, res)
	if len(positions) != count {
		t.Errorf("expected all %d positions without a limit, got %d (old default was 100)", count, len(positions))
	}
}

// ---------------------------------------------------------------------------
// AdminListPositions (live equivalent of the chi AdminGetAllPositions)
// ---------------------------------------------------------------------------

func TestAdminListPositions_Success_OAS(t *testing.T) {
	env := setupPositionsOASIntegration(t)
	ctx := context.Background()

	now := time.Now().UTC()
	if err := env.posRepo.Create(ctx, &model.Position{DeviceID: env.device.ID, Latitude: 52.0, Longitude: 13.0, Timestamp: now}); err != nil {
		t.Fatalf("create position: %v", err)
	}

	admin := &model.User{ID: env.user.ID, Email: env.user.Email, Role: model.RoleAdmin}
	adminCtx := api.ContextWithUser(context.Background(), admin)

	res, err := env.handler.AdminListPositions(adminCtx)
	if err != nil {
		t.Fatalf("AdminListPositions returned error: %v", err)
	}
	list, ok := res.(*oas.AdminListPositionsOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.AdminListPositionsOKApplicationJSON, got %T", res)
	}
	if len(*list) < 1 {
		t.Errorf("expected at least 1 position, got %d", len(*list))
	}
}

func TestAdminListPositions_NonAdmin_OAS(t *testing.T) {
	// No repository access happens on the forbidden path, so no DB is needed.
	h := handlers.NewHandler(handlers.HandlerConfig{})
	regular := &model.User{ID: 1, Email: "user@example.com", Role: model.RoleUser}
	ctx := api.ContextWithUser(context.Background(), regular)

	res, err := h.AdminListPositions(ctx)
	if err != nil {
		t.Fatalf("AdminListPositions returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminListPositionsForbidden); !ok {
		t.Errorf("expected *oas.AdminListPositionsForbidden for non-admin, got %T", res)
	}
}

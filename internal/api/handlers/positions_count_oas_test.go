package handlers_test

import (
	"context"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func TestCountPositions_OAS(t *testing.T) {
	env := setupPositionsOASIntegration(t)
	ctx := context.Background()
	now := time.Now().UTC()

	for _, ts := range []time.Time{now.Add(-2 * time.Hour), now.Add(-time.Minute), now.Add(-30 * time.Second)} {
		if err := env.posRepo.Create(ctx, &model.Position{DeviceID: env.device.ID, Latitude: 52, Longitude: 13, Timestamp: ts}); err != nil {
			t.Fatalf("create position: %v", err)
		}
	}
	pool := testutil.SetupTestDB(t)
	other := &model.User{Email: "count-other@example.com", PasswordHash: "$2a$10$hash", Name: "Other"}
	if err := repository.NewUserRepository(pool).Create(ctx, other); err != nil {
		t.Fatalf("create user: %v", err)
	}
	otherDevice := &model.Device{UniqueID: "count-other-dev", Name: "Other", Status: "online"}
	if err := repository.NewDeviceRepository(pool).Create(ctx, otherDevice, other.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}
	if err := env.posRepo.Create(ctx, &model.Position{DeviceID: otherDevice.ID, Latitude: 52, Longitude: 13, Timestamp: now.Add(-time.Minute)}); err != nil {
		t.Fatalf("create position: %v", err)
	}

	lastHour := oas.CountPositionsParams{From: now.Add(-time.Hour), To: now}
	count := func(ctx context.Context, p oas.CountPositionsParams) oas.CountPositionsRes {
		t.Helper()
		res, err := env.handler.CountPositions(ctx, p)
		if err != nil {
			t.Fatalf("CountPositions: %v", err)
		}
		return res
	}

	if res, ok := count(env.userCtx(), lastHour).(*oas.CountPositionsOK); !ok || res.Count != 2 {
		t.Errorf("own devices, last hour = %#v, want count 2", res)
	}

	all := lastHour
	all.All = oas.NewOptBool(true)
	if _, ok := count(env.userCtx(), all).(*oas.CountPositionsForbidden); !ok {
		t.Error("all=true as non-admin: want Forbidden")
	}

	admin := &model.User{ID: env.user.ID, Email: env.user.Email, Role: model.RoleAdmin}
	if res, ok := count(api.ContextWithUser(ctx, admin), all).(*oas.CountPositionsOK); !ok || res.Count != 3 {
		t.Errorf("admin all=true, last hour = %#v, want count 3", res)
	}

	if _, ok := count(context.Background(), lastHour).(*oas.CountPositionsUnauthorized); !ok {
		t.Error("no user: want Unauthorized")
	}
}

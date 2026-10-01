package services

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testutil.Cleanup()
	os.Exit(code)
}

func deviceEvents(t *testing.T, deviceID int64) ([]*model.Event, error) {
	t.Helper()
	ctx := context.Background()
	pool := testutil.SetupTestDB(t)
	owners, err := repository.NewDeviceRepository(pool).GetUserIDs(ctx, deviceID)
	if err != nil || len(owners) == 0 {
		return nil, err
	}
	return repository.NewEventRepository(pool).GetByFilters(ctx, owners[0], []int64{deviceID}, nil,
		time.Time{}, time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC))
}

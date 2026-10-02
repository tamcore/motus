package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

type trackRow struct {
	ts                time.Time
	lat, lon          float64
	speed, address    any
	id, deviceID      int64
	hasExtraFieldsSet bool
}

func TestPositionRepository_StreamTrackByDeviceAndTimeRange(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	posRepo := repository.NewPositionRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	_, device := createTestDevice(t, pool, deviceRepo, userRepo)
	other := &model.Device{UniqueID: "track-other", Name: "Other", Status: "online"}
	if err := deviceRepo.Create(ctx, other, createTestUser(t, userRepo).ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(10 * time.Minute)
	speed, addr := 42.5, "Main St 1"
	create := func(deviceID int64, ts time.Time, lat float64, s *float64, a *string) {
		t.Helper()
		p := &model.Position{DeviceID: deviceID, Timestamp: ts, Latitude: lat, Longitude: 13, Speed: s, Address: a,
			Attributes: map[string]any{"k": "v"}}
		if err := posRepo.Create(ctx, p); err != nil {
			t.Fatalf("create position: %v", err)
		}
	}
	// Inserted out of order; bounds are inclusive.
	create(device.ID, to, 52.3, nil, nil)
	create(device.ID, from, 52.0, &speed, &addr)
	create(device.ID, from.Add(5*time.Minute), 52.1, nil, nil)
	create(device.ID, from.Add(-time.Second), 51.0, nil, nil)
	create(device.ID, to.Add(time.Second), 53.0, nil, nil)
	create(other.ID, from.Add(time.Minute), 60.0, nil, nil)

	// The callback must not retain p or its pointers, so copy values here.
	var got []trackRow
	err := posRepo.StreamTrackByDeviceAndTimeRange(ctx, device.ID, from, to, func(p *model.Position) error {
		r := trackRow{ts: p.Timestamp, lat: p.Latitude, lon: p.Longitude, id: p.ID, deviceID: p.DeviceID,
			hasExtraFieldsSet: p.Attributes != nil || p.Network != nil || p.GeofenceIDs != nil}
		if p.Speed != nil {
			r.speed = *p.Speed
		}
		if p.Address != nil {
			r.address = *p.Address
		}
		got = append(got, r)
		return nil
	})
	if err != nil {
		t.Fatalf("stream failed: %v", err)
	}

	want := []trackRow{
		{ts: from, lat: 52.0, lon: 13, speed: speed, address: addr},
		{ts: from.Add(5 * time.Minute), lat: 52.1, lon: 13},
		{ts: to, lat: 52.3, lon: 13},
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d positions, got %d: %+v", len(want), len(got), got)
	}
	for i := range want {
		g, w := got[i], want[i]
		if !g.ts.Equal(w.ts) || g.lat != w.lat || g.lon != w.lon || g.speed != w.speed || g.address != w.address {
			t.Errorf("position %d = %+v, want %+v", i, g, w)
		}
		if g.id != 0 || g.deviceID != 0 || g.hasExtraFieldsSet {
			t.Errorf("position %d: only track columns may be populated, got %+v", i, g)
		}
	}
}

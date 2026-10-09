package repository_test

import (
	"context"
	"slices"
	"testing"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func TestDeviceRepository_SetGeofences(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	geoRepo := repository.NewGeofenceRepository(pool)
	ctx := context.Background()

	user := testutil.CreateUser(t, "devgeo-set@example.com")
	d1 := testutil.CreateDevice(t, user.ID, "devgeo-set-1")
	d2 := testutil.CreateDevice(t, user.ID, "devgeo-set-2")
	g1 := &model.Geofence{Name: "A", Geometry: berlinPolygonGeoJSON}
	g2 := &model.Geofence{Name: "B", Geometry: berlinEastPolygonGeoJSON}
	for _, g := range []*model.Geofence{g1, g2} {
		if err := geoRepo.Create(ctx, g); err != nil {
			t.Fatalf("create geofence: %v", err)
		}
	}

	// Many-to-many: d1 gets both fences (duplicates ignored), d2 shares g2.
	if err := deviceRepo.SetGeofences(ctx, d1.ID, []int64{g2.ID, g1.ID, g1.ID}); err != nil {
		t.Fatalf("SetGeofences d1: %v", err)
	}
	if err := deviceRepo.SetGeofences(ctx, d2.ID, []int64{g2.ID}); err != nil {
		t.Fatalf("SetGeofences d2: %v", err)
	}

	got, err := deviceRepo.GetGeofenceIDs(ctx, []int64{d1.ID, d2.ID})
	if err != nil {
		t.Fatalf("GetGeofenceIDs: %v", err)
	}
	want1 := []int64{g1.ID, g2.ID}
	slices.Sort(want1)
	if !slices.Equal(got[d1.ID], want1) || !slices.Equal(got[d2.ID], []int64{g2.ID}) {
		t.Fatalf("GetGeofenceIDs = %v, want d1=%v d2=[%d]", got, want1, g2.ID)
	}

	// Replacing the set removes the previous attachments; [] clears.
	if err := deviceRepo.SetGeofences(ctx, d1.ID, []int64{g2.ID}); err != nil {
		t.Fatalf("SetGeofences replace: %v", err)
	}
	if err := deviceRepo.SetGeofences(ctx, d2.ID, []int64{}); err != nil {
		t.Fatalf("SetGeofences clear: %v", err)
	}
	got, _ = deviceRepo.GetGeofenceIDs(ctx, []int64{d1.ID, d2.ID})
	if !slices.Equal(got[d1.ID], []int64{g2.ID}) || len(got[d2.ID]) != 0 {
		t.Fatalf("after replace/clear = %v", got)
	}

	// Deleting the geofence cascades.
	if err := geoRepo.Delete(ctx, g2.ID); err != nil {
		t.Fatalf("delete geofence: %v", err)
	}
	got, _ = deviceRepo.GetGeofenceIDs(ctx, []int64{d1.ID})
	if len(got[d1.ID]) != 0 {
		t.Fatalf("expected cascade on geofence delete, got %v", got)
	}

	// Unknown geofence IDs are rejected by the foreign key, keeping the old set.
	if err := deviceRepo.SetGeofences(ctx, d1.ID, []int64{g1.ID}); err != nil {
		t.Fatalf("SetGeofences: %v", err)
	}
	if err := deviceRepo.SetGeofences(ctx, d1.ID, []int64{999999}); err == nil {
		t.Error("expected error for unknown geofence id")
	}
	got, _ = deviceRepo.GetGeofenceIDs(ctx, []int64{d1.ID})
	if !slices.Equal(got[d1.ID], []int64{g1.ID}) {
		t.Errorf("failed SetGeofences must roll back, got %v", got)
	}
}

// TestGeofenceRepository_CheckContainmentForDevice_AttachedOptIn verifies the
// opt-in filter: without attachments all user geofences are evaluated; with
// attachments only the attached ones.
func TestGeofenceRepository_CheckContainmentForDevice_AttachedOptIn(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	geoRepo := repository.NewGeofenceRepository(pool)
	ctx := context.Background()

	user := testutil.CreateUser(t, "devgeo-optin@example.com")
	device := testutil.CreateDevice(t, user.ID, "devgeo-optin")
	inner := &model.Geofence{Name: "Inner", Geometry: berlinPolygonGeoJSON}
	outer := &model.Geofence{Name: "Outer", Geometry: berlinPolygonGeoJSON}
	for _, g := range []*model.Geofence{inner, outer} {
		if err := geoRepo.Create(ctx, g); err != nil {
			t.Fatalf("create geofence: %v", err)
		}
		if err := geoRepo.AssociateUser(ctx, user.ID, g.ID); err != nil {
			t.Fatalf("associate: %v", err)
		}
	}

	contained := func() []int64 {
		t.Helper()
		ids, err := geoRepo.CheckContainmentForDevice(ctx, device.ID, 52.52, 13.37)
		if err != nil {
			t.Fatalf("CheckContainmentForDevice: %v", err)
		}
		slices.Sort(ids)
		return ids
	}

	all := []int64{inner.ID, outer.ID}
	slices.Sort(all)
	if got := contained(); !slices.Equal(got, all) {
		t.Fatalf("no attachments: got %v, want %v", got, all)
	}

	if err := deviceRepo.SetGeofences(ctx, device.ID, []int64{inner.ID}); err != nil {
		t.Fatalf("SetGeofences: %v", err)
	}
	if got := contained(); !slices.Equal(got, []int64{inner.ID}) {
		t.Fatalf("attached [inner]: got %v, want [%d]", got, inner.ID)
	}
	// One query also filters the previous membership to the evaluated set,
	// so the ingest path needs no second device_geofences lookup.
	current, prev, err := geoRepo.EvaluateGeofences(ctx, device.ID, 52.52, 13.37, []int64{outer.ID, inner.ID})
	if err != nil {
		t.Fatalf("EvaluateGeofences: %v", err)
	}
	if !slices.Equal(current, []int64{inner.ID}) || !slices.Equal(prev, []int64{inner.ID}) {
		t.Fatalf("EvaluateGeofences = %v / %v, want [%d] / [%d]", current, prev, inner.ID, inner.ID)
	}

	if err := deviceRepo.SetGeofences(ctx, device.ID, nil); err != nil {
		t.Fatalf("SetGeofences clear: %v", err)
	}
	if got := contained(); !slices.Equal(got, all) {
		t.Fatalf("cleared: got %v, want %v", got, all)
	}
}

// TestGeofenceRepository_EvaluateGeofences_NoAttachments keeps the previous
// membership unfiltered while the device has no attachments.
func TestGeofenceRepository_EvaluateGeofences_NoAttachments(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	geoRepo := repository.NewGeofenceRepository(pool)
	ctx := context.Background()

	user := testutil.CreateUser(t, "devgeo-eval@example.com")
	device := testutil.CreateDevice(t, user.ID, "devgeo-eval")
	g := &model.Geofence{Name: "Home", Geometry: berlinPolygonGeoJSON}
	if err := geoRepo.Create(ctx, g); err != nil {
		t.Fatalf("create geofence: %v", err)
	}
	if err := geoRepo.AssociateUser(ctx, user.ID, g.ID); err != nil {
		t.Fatalf("associate: %v", err)
	}

	current, prev, err := geoRepo.EvaluateGeofences(ctx, device.ID, 52.52, 13.37, []int64{g.ID, 777})
	if err != nil {
		t.Fatalf("EvaluateGeofences: %v", err)
	}
	if !slices.Equal(current, []int64{g.ID}) || !slices.Equal(prev, []int64{g.ID, 777}) {
		t.Fatalf("EvaluateGeofences = %v / %v, want [%d] / [%d 777]", current, prev, g.ID, g.ID)
	}
	if _, prev, _ := geoRepo.EvaluateGeofences(ctx, device.ID, 52.52, 13.37, nil); len(prev) != 0 {
		t.Fatalf("nil previous membership = %v, want empty", prev)
	}
}

// TestDeviceRepository_CreateWithGeofences attaches geofences in the same
// transaction as the device; an invalid geofence leaves no device behind.
func TestDeviceRepository_CreateWithGeofences(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	geoRepo := repository.NewGeofenceRepository(pool)
	ctx := context.Background()

	user := testutil.CreateUser(t, "devgeo-create@example.com")
	g := &model.Geofence{Name: "Home", Geometry: berlinPolygonGeoJSON}
	if err := geoRepo.Create(ctx, g); err != nil {
		t.Fatalf("create geofence: %v", err)
	}

	d := &model.Device{UniqueID: "devgeo-create-ok", Name: "Ok", Status: "unknown", GeofenceIDs: []int64{g.ID}}
	if err := deviceRepo.Create(ctx, d, user.ID); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got, _ := deviceRepo.GetGeofenceIDs(ctx, []int64{d.ID}); !slices.Equal(got[d.ID], []int64{g.ID}) {
		t.Fatalf("attached = %v, want [%d]", got[d.ID], g.ID)
	}

	bad := &model.Device{UniqueID: "devgeo-create-bad", Name: "Bad", Status: "unknown", GeofenceIDs: []int64{999999}}
	if err := deviceRepo.Create(ctx, bad, user.ID); err == nil {
		t.Fatal("expected FK error for unknown geofence")
	}
	if _, err := deviceRepo.GetByUniqueID(ctx, "devgeo-create-bad"); err == nil {
		t.Fatal("device must not exist after a failed create")
	}
}

package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

// GeoJSON polygon around central Berlin (roughly Tiergarten area).
const berlinPolygonGeoJSON = `{"type":"Polygon","coordinates":[[[13.35,52.51],[13.35,52.53],[13.40,52.53],[13.40,52.51],[13.35,52.51]]]}`

func TestGeofenceRepository_Create(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	geoRepo := repository.NewGeofenceRepository(pool)
	ctx := context.Background()

	g := &model.Geofence{
		Name:        "Test Geofence",
		Description: "A test area",
		Geometry:    berlinPolygonGeoJSON,
		Attributes: map[string]interface{}{
			"color": "red",
		},
	}

	if err := geoRepo.Create(ctx, g); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if g.ID == 0 {
		t.Error("expected geofence ID to be set")
	}
	if g.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestGeofenceRepository_GetByID(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	geoRepo := repository.NewGeofenceRepository(pool)
	ctx := context.Background()

	g := &model.Geofence{
		Name:     "GetByID Fence",
		Geometry: berlinPolygonGeoJSON,
	}
	if err := geoRepo.Create(ctx, g); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	found, err := geoRepo.GetByID(ctx, g.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found.Name != "GetByID Fence" {
		t.Errorf("expected name 'GetByID Fence', got %q", found.Name)
	}
	// Geometry should come back as GeoJSON (ST_AsGeoJSON).
	if found.Geometry == "" {
		t.Error("expected non-empty geometry")
	}
}

func TestGeofenceRepository_GetByID_NotFound(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	geoRepo := repository.NewGeofenceRepository(pool)
	ctx := context.Background()

	_, err := geoRepo.GetByID(ctx, 99999)
	if err == nil {
		t.Error("expected error for nonexistent geofence")
	}
}

func TestGeofenceRepository_GetByUser(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	geoRepo := repository.NewGeofenceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "geo-user@example.com", PasswordHash: "hash", Name: "Geo User"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("Create user failed: %v", err)
	}

	g1 := &model.Geofence{Name: "Alpha Fence", Geometry: berlinPolygonGeoJSON}
	g2 := &model.Geofence{Name: "Beta Fence", Geometry: berlinPolygonGeoJSON}
	if err := geoRepo.Create(ctx, g1); err != nil {
		t.Fatalf("Create g1 failed: %v", err)
	}
	if err := geoRepo.Create(ctx, g2); err != nil {
		t.Fatalf("Create g2 failed: %v", err)
	}

	if err := geoRepo.AssociateUser(ctx, user.ID, g1.ID); err != nil {
		t.Fatalf("AssociateUser g1 failed: %v", err)
	}
	if err := geoRepo.AssociateUser(ctx, user.ID, g2.ID); err != nil {
		t.Fatalf("AssociateUser g2 failed: %v", err)
	}

	geofences, err := geoRepo.GetByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByUser failed: %v", err)
	}
	if len(geofences) != 2 {
		t.Fatalf("expected 2 geofences, got %d", len(geofences))
	}
	// Ordered by name.
	if geofences[0].Name != "Alpha Fence" {
		t.Errorf("expected first 'Alpha Fence', got %q", geofences[0].Name)
	}
}

func TestGeofenceRepository_GetAll(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	geoRepo := repository.NewGeofenceRepository(pool)
	ctx := context.Background()

	g1 := &model.Geofence{Name: "All Fence 1", Geometry: berlinPolygonGeoJSON}
	g2 := &model.Geofence{Name: "All Fence 2", Geometry: berlinPolygonGeoJSON}
	_ = geoRepo.Create(ctx, g1)
	_ = geoRepo.Create(ctx, g2)

	all, err := geoRepo.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 geofences, got %d", len(all))
	}
}

func TestGeofenceRepository_Update(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	geoRepo := repository.NewGeofenceRepository(pool)
	ctx := context.Background()

	g := &model.Geofence{Name: "Before Update", Geometry: berlinPolygonGeoJSON}
	if err := geoRepo.Create(ctx, g); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	g.Name = "After Update"
	g.Description = "Updated description"
	if err := geoRepo.Update(ctx, g); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	found, err := geoRepo.GetByID(ctx, g.ID)
	if err != nil {
		t.Fatalf("GetByID after update failed: %v", err)
	}
	if found.Name != "After Update" {
		t.Errorf("expected name 'After Update', got %q", found.Name)
	}
	if found.Description != "Updated description" {
		t.Errorf("expected description 'Updated description', got %q", found.Description)
	}
}

func TestGeofenceRepository_Delete(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	geoRepo := repository.NewGeofenceRepository(pool)
	ctx := context.Background()

	g := &model.Geofence{Name: "Delete Me", Geometry: berlinPolygonGeoJSON}
	if err := geoRepo.Create(ctx, g); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := geoRepo.Delete(ctx, g.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := geoRepo.GetByID(ctx, g.ID)
	if err == nil {
		t.Error("expected error after deletion")
	}
}

func TestGeofenceRepository_AssociateUser_Idempotent(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	geoRepo := repository.NewGeofenceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "assoc@example.com", PasswordHash: "hash", Name: "Assoc User"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("Create user failed: %v", err)
	}

	g := &model.Geofence{Name: "Assoc Fence", Geometry: berlinPolygonGeoJSON}
	if err := geoRepo.Create(ctx, g); err != nil {
		t.Fatalf("Create geofence failed: %v", err)
	}

	// Associate twice -- should not error (ON CONFLICT DO NOTHING).
	if err := geoRepo.AssociateUser(ctx, user.ID, g.ID); err != nil {
		t.Fatalf("First AssociateUser failed: %v", err)
	}
	if err := geoRepo.AssociateUser(ctx, user.ID, g.ID); err != nil {
		t.Fatalf("Second AssociateUser failed: %v", err)
	}
}

func TestGeofenceRepository_UserHasAccess(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	geoRepo := repository.NewGeofenceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "access-geo@example.com", PasswordHash: "hash", Name: "Access Geo"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("Create user failed: %v", err)
	}

	g := &model.Geofence{Name: "Access Fence", Geometry: berlinPolygonGeoJSON}
	if err := geoRepo.Create(ctx, g); err != nil {
		t.Fatalf("Create geofence failed: %v", err)
	}

	// Before association.
	if geoRepo.UserHasAccess(ctx, &model.User{ID: user.ID}, g.ID) {
		t.Error("expected no access before association")
	}

	if err := geoRepo.AssociateUser(ctx, user.ID, g.ID); err != nil {
		t.Fatalf("AssociateUser failed: %v", err)
	}

	// After association.
	if !geoRepo.UserHasAccess(ctx, &model.User{ID: user.ID}, g.ID) {
		t.Error("expected access after association")
	}
}

func TestGeofenceRepository_CheckContainment(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	geoRepo := repository.NewGeofenceRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{
		Email:        "contain-" + time.Now().Format("150405") + "@example.com",
		PasswordHash: "hash",
		Name:         "Containment User",
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("Create user failed: %v", err)
	}

	// Polygon around central Berlin: lat 52.51-52.53, lon 13.35-13.40
	g := &model.Geofence{Name: "Berlin Center", Geometry: berlinPolygonGeoJSON}
	if err := geoRepo.Create(ctx, g); err != nil {
		t.Fatalf("Create geofence failed: %v", err)
	}
	if err := geoRepo.AssociateUser(ctx, user.ID, g.ID); err != nil {
		t.Fatalf("AssociateUser failed: %v", err)
	}

	tests := []struct {
		name     string
		lat, lon float64
		inside   bool
	}{
		{"point inside polygon", 52.52, 13.37, true},
		{"point outside polygon (east)", 52.52, 13.50, false},
		{"point outside polygon (north)", 52.55, 13.37, false},
		{"point on edge", 52.51, 13.35, false}, // PostGIS: boundary points may not be contained
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids, err := geoRepo.CheckContainment(ctx, user.ID, tt.lat, tt.lon)
			if err != nil {
				t.Fatalf("CheckContainment failed: %v", err)
			}
			found := len(ids) > 0 && ids[0] == g.ID
			if found != tt.inside {
				t.Errorf("containment for (%f, %f): got %v, want %v", tt.lat, tt.lon, found, tt.inside)
			}
		})
	}
}

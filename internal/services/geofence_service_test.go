package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func setupGeofenceServiceCRUD(t *testing.T) (*GeofenceService, *repository.GeofenceRepository, *repository.CalendarRepository) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	geoRepo := repository.NewGeofenceRepository(pool)
	calRepo := repository.NewCalendarRepository(pool)
	svc := NewGeofenceService(geoRepo, nil)
	return svc, geoRepo, calRepo
}

const testGeoJSONCircle = `{"type":"Polygon","coordinates":[[[13.40,52.51],[13.40,52.53],[13.42,52.53],[13.42,52.51],[13.40,52.51]]]}`

func createTestUserAndGeofence(t *testing.T, ctx context.Context, svc *GeofenceService, email string) (*model.User, *model.Geofence) {
	t.Helper()
	user := testutil.CreateUser(t, email)
	g, err := svc.CreateForUser(ctx, user, CreateGeofenceInput{Name: "Original", Geometry: testGeoJSONCircle})
	if err != nil {
		t.Fatalf("create geofence: %v", err)
	}
	return user, g
}

func TestGeofenceService_InvalidGeometryIsClientError(t *testing.T) {
	svc, _, _ := setupGeofenceServiceCRUD(t)
	ctx := context.Background()
	user, g := createTestUserAndGeofence(t, ctx, svc, "geo-badgeom@example.com")

	for name, run := range map[string]func() error{
		"create wkt": func() error {
			_, err := svc.CreateForUser(ctx, user, CreateGeofenceInput{Name: "Bad", Area: "POLYGON((1 2, 3"})
			return err
		},
		"create geojson": func() error {
			_, err := svc.CreateForUser(ctx, user, CreateGeofenceInput{Name: "Bad", Geometry: `{"type":"Nope"}`})
			return err
		},
		"update wkt": func() error {
			bad := "POLYGON((1 2, 3"
			_, err := svc.UpdateForUser(ctx, user, g.ID, UpdateGeofenceInput{Area: &bad})
			return err
		},
	} {
		err := run()
		if !errors.Is(err, ErrInvalid) || !strings.HasPrefix(err.Error(), "invalid geometry") {
			t.Errorf("%s: got %v, want client-safe invalid geometry error", name, err)
		}
	}
}

func TestGeofenceService_UpdateForUser_RenameName(t *testing.T) {
	svc, _, _ := setupGeofenceServiceCRUD(t)
	ctx := context.Background()
	user, g := createTestUserAndGeofence(t, ctx, svc, "geo-update@example.com")

	newName := "Renamed"
	updated, err := svc.UpdateForUser(ctx, user, g.ID, UpdateGeofenceInput{Name: &newName})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Name != newName {
		t.Errorf("expected name %q, got %q", newName, updated.Name)
	}
}

func TestGeofenceService_UpdateForUser_AttachCalendar(t *testing.T) {
	svc, _, calRepo := setupGeofenceServiceCRUD(t)
	ctx := context.Background()
	user, g := createTestUserAndGeofence(t, ctx, svc, "geo-attach@example.com")

	cal := &model.Calendar{UserID: user.ID, Name: "Cal", Data: testIcal}
	if err := calRepo.Create(ctx, cal); err != nil {
		t.Fatalf("create calendar: %v", err)
	}

	updated, err := svc.UpdateForUser(ctx, user, g.ID, UpdateGeofenceInput{CalendarID: &cal.ID, CalendarIDSet: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.CalendarID == nil || *updated.CalendarID != cal.ID {
		t.Errorf("expected calendarId %d, got %v", cal.ID, updated.CalendarID)
	}
}

func TestGeofenceService_UpdateForUser_DetachCalendar(t *testing.T) {
	svc, _, calRepo := setupGeofenceServiceCRUD(t)
	ctx := context.Background()
	user, g := createTestUserAndGeofence(t, ctx, svc, "geo-detach@example.com")

	cal := &model.Calendar{UserID: user.ID, Name: "Cal2", Data: testIcal}
	_ = calRepo.Create(ctx, cal)
	_, _ = svc.UpdateForUser(ctx, user, g.ID, UpdateGeofenceInput{CalendarID: &cal.ID, CalendarIDSet: true})

	updated, err := svc.UpdateForUser(ctx, user, g.ID, UpdateGeofenceInput{CalendarID: nil, CalendarIDSet: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.CalendarID != nil {
		t.Errorf("expected nil calendarId after detach, got %v", *updated.CalendarID)
	}
}

func TestGeofenceService_UpdateForUser_AccessDenied(t *testing.T) {
	svc, _, _ := setupGeofenceServiceCRUD(t)
	ctx := context.Background()
	_, g := createTestUserAndGeofence(t, ctx, svc, "geo-owner@example.com")

	other := testutil.CreateUser(t, "geo-other@example.com")

	newName := "Hack"
	_, err := svc.UpdateForUser(ctx, other, g.ID, UpdateGeofenceInput{Name: &newName})
	if err == nil {
		t.Fatal("expected access denied error")
	}
}

// eastPolygonGeoJSON is disjoint from testGeoJSONCircle — used to verify geometry replacement.
const eastPolygonGeoJSON = `{"type":"Polygon","coordinates":[[[13.60,52.55],[13.60,52.57],[13.65,52.57],[13.65,52.55],[13.60,52.55]]]}`

func TestGeofenceService_UpdateForUser_ShapeChange(t *testing.T) {
	svc, geoRepo, _ := setupGeofenceServiceCRUD(t)
	ctx := context.Background()
	user, g := createTestUserAndGeofence(t, ctx, svc, "geo-shape@example.com")
	device := testutil.CreateDevice(t, user.ID, "geo-svc-"+user.Email)

	// Point inside original polygon.
	insideOrig, _ := geoRepo.CheckContainmentForDevice(ctx, device.ID, 52.52, 13.41)
	if len(insideOrig) == 0 {
		t.Fatal("expected point inside original polygon before update")
	}

	// Update geometry to a completely different polygon.
	newGeom := eastPolygonGeoJSON
	updated, err := svc.UpdateForUser(ctx, user, g.ID, UpdateGeofenceInput{Geometry: &newGeom})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Geometry == "" {
		t.Error("expected non-empty Geometry on returned geofence")
	}

	// Old point must now be outside the updated shape.
	nowOutside, _ := geoRepo.CheckContainmentForDevice(ctx, device.ID, 52.52, 13.41)
	if len(nowOutside) > 0 {
		t.Error("expected (52.52, 13.41) to be OUTSIDE updated polygon")
	}

	// New point must be inside.
	nowInside, _ := geoRepo.CheckContainmentForDevice(ctx, device.ID, 52.56, 13.62)
	if len(nowInside) == 0 || nowInside[0] != g.ID {
		t.Error("expected (52.56, 13.62) to be INSIDE updated polygon")
	}
}

func TestGeofenceService_UpdateForUser_AreaWKT(t *testing.T) {
	svc, geoRepo, _ := setupGeofenceServiceCRUD(t)
	ctx := context.Background()
	user, g := createTestUserAndGeofence(t, ctx, svc, "geo-area@example.com")
	device := testutil.CreateDevice(t, user.ID, "geo-svc-"+user.Email)

	// Update via WKT area string (east polygon).
	newArea := "POLYGON((13.60 52.55, 13.60 52.57, 13.65 52.57, 13.65 52.55, 13.60 52.55))"
	updated, err := svc.UpdateForUser(ctx, user, g.ID, UpdateGeofenceInput{Area: &newArea})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Area == "" && updated.Geometry == "" {
		t.Error("expected non-empty Area or Geometry on returned geofence")
	}

	// New point inside the WKT polygon must now be contained.
	nowInside, _ := geoRepo.CheckContainmentForDevice(ctx, device.ID, 52.56, 13.62)
	if len(nowInside) == 0 || nowInside[0] != g.ID {
		t.Error("expected (52.56, 13.62) to be INSIDE updated WKT polygon")
	}
}

func TestGeofenceService_DeleteForUser_HappyPath(t *testing.T) {
	svc, geoRepo, _ := setupGeofenceServiceCRUD(t)
	ctx := context.Background()
	user, g := createTestUserAndGeofence(t, ctx, svc, "geo-del@example.com")

	if err := svc.DeleteForUser(ctx, user, g.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	all, _ := geoRepo.GetByUser(ctx, user.ID)
	if len(all) != 0 {
		t.Errorf("expected 0 geofences after delete, got %d", len(all))
	}
}

func TestGeofenceService_DeleteForUser_AccessDenied(t *testing.T) {
	svc, _, _ := setupGeofenceServiceCRUD(t)
	ctx := context.Background()
	_, g := createTestUserAndGeofence(t, ctx, svc, "geo-delacc@example.com")

	other := testutil.CreateUser(t, "geo-del-other@example.com")

	if err := svc.DeleteForUser(ctx, other, g.ID); err == nil {
		t.Fatal("expected access denied error")
	}
}

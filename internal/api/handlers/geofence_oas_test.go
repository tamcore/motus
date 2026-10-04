package handlers_test

// Tests for the ogen Handler geofence methods (CreateGeofence, GetGeofence,
// UpdateGeofence, DeleteGeofence). Ported from the deleted chi
// GeofenceHandler tests in geofence_test.go.
//
// Validation now lives in services.GeofenceService, which the live handler
// delegates to; the handler is constructed with a GeofenceService backed by
// the same repository.
//
// Dropped tests (no live equivalent):
//   - invalid path-param ID parsing: ogen owns path param decoding.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/services"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

const testPolygonGeoJSON = `{"type":"Polygon","coordinates":[[[13.35,52.51],[13.35,52.53],[13.40,52.53],[13.40,52.51],[13.35,52.51]]]}`

// testPolygonEastGeoJSON is disjoint from testPolygonGeoJSON.
const testPolygonEastGeoJSON = `{"type":"Polygon","coordinates":[[[13.60,52.55],[13.60,52.57],[13.65,52.57],[13.65,52.55],[13.60,52.55]]]}`

// newGeofenceTestHandler builds an ogen Handler from a mock geofence repo.
// The live create/update paths delegate to GeofenceService, so the service
// is wired over the same repository. The nil-pool audit logger exercises
// the audit code paths as documented no-ops.
func newGeofenceTestHandler(geofences repository.GeofenceRepo) *handlers.Handler {
	auditLogger := audit.NewLogger(nil)
	return handlers.NewHandler(handlers.HandlerConfig{
		Geofences:       geofences,
		GeofenceService: services.NewGeofenceService(geofences, auditLogger),
		AuditLogger:     auditLogger,
	})
}

// ---------------------------------------------------------------------------
// CreateGeofence
// ---------------------------------------------------------------------------

func TestCreateGeofence_Success(t *testing.T) {
	var associatedUserID int64
	mock := &auditMockGeofenceRepo{
		createFn: func(_ context.Context, g *model.Geofence) error {
			g.ID = 11
			return nil
		},
		associateUserFn: func(_ context.Context, userID, _ int64) error {
			associatedUserID = userID
			return nil
		},
	}
	h := newGeofenceTestHandler(mock)

	res, err := h.CreateGeofence(ctxAs(1, model.RoleUser), &oas.GeofenceInput{
		Name:        "Test Fence",
		Description: oas.NewOptString("A test"),
		Geometry:    oas.NewOptString(testPolygonGeoJSON),
	})
	if err != nil {
		t.Fatalf("CreateGeofence returned error: %v", err)
	}
	g, ok := res.(*oas.Geofence)
	if !ok {
		t.Fatalf("expected *oas.Geofence, got %T", res)
	}
	if g.Name != "Test Fence" {
		t.Errorf("expected name 'Test Fence', got %q", g.Name)
	}
	if g.ID != 11 {
		t.Errorf("expected geofence ID 11, got %d", g.ID)
	}
	if associatedUserID != 1 {
		t.Errorf("expected geofence associated with user 1, got %d", associatedUserID)
	}
}

func TestGeofence_StorageErrorIsGeneric(t *testing.T) {
	secret := errors.New("pq: connection to secret-db-host refused")
	h := newGeofenceTestHandler(&auditMockGeofenceRepo{
		createFn:        func(context.Context, *model.Geofence) error { return secret },
		updateFn:        func(context.Context, *model.Geofence) error { return secret },
		userHasAccessFn: func(context.Context, *model.User, int64) bool { return true },
		getByIDFn: func(_ context.Context, id int64) (*model.Geofence, error) {
			return &model.Geofence{ID: id, Name: "Geo"}, nil
		},
	})

	res, _ := h.CreateGeofence(ctxAs(1, model.RoleUser), &oas.GeofenceInput{
		Name:     "Geo",
		Geometry: oas.NewOptString(testPolygonGeoJSON),
	})
	if bad, ok := res.(*oas.CreateGeofenceBadRequest); !ok || bad.Error != "failed to create geofence" {
		t.Errorf("create: got %#v, want generic failure", res)
	}

	upd, _ := h.UpdateGeofence(ctxAs(1, model.RoleUser), &oas.GeofenceUpdateInput{Name: oas.NewOptString("New")},
		oas.UpdateGeofenceParams{ID: 3})
	if bad, ok := upd.(*oas.UpdateGeofenceBadRequest); !ok || bad.Error != "failed to update geofence" {
		t.Errorf("update: got %#v, want generic failure", upd)
	}
}

func TestDeleteGeofence_Errors(t *testing.T) {
	hasAccess := true
	h := newGeofenceTestHandler(&auditMockGeofenceRepo{
		userHasAccessFn: func(context.Context, *model.User, int64) bool { return hasAccess },
		deleteFn: func(context.Context, int64) error {
			return errors.New("pq: connection to secret-db-host refused")
		},
	})

	res, _ := h.DeleteGeofence(ctxAs(1, model.RoleUser), oas.DeleteGeofenceParams{ID: 3})
	if f, ok := res.(*oas.DeleteGeofenceForbidden); !ok || f.Error != "failed to delete geofence" {
		t.Errorf("storage error: got %#v", res)
	}
	hasAccess = false
	res, _ = h.DeleteGeofence(ctxAs(1, model.RoleUser), oas.DeleteGeofenceParams{ID: 3})
	if f, ok := res.(*oas.DeleteGeofenceForbidden); !ok || f.Error != "access denied" {
		t.Errorf("no access: got %#v", res)
	}
}

func TestCreateGeofence_MissingName(t *testing.T) {
	h := newGeofenceTestHandler(&auditMockGeofenceRepo{})

	res, err := h.CreateGeofence(ctxAs(1, model.RoleUser), &oas.GeofenceInput{
		Geometry: oas.NewOptString(testPolygonGeoJSON),
	})
	if err != nil {
		t.Fatalf("CreateGeofence returned error: %v", err)
	}
	badReq, ok := res.(*oas.CreateGeofenceBadRequest)
	if !ok {
		t.Fatalf("expected *oas.CreateGeofenceBadRequest, got %T", res)
	}
	if badReq.Error != "name is required" {
		t.Errorf("unexpected error message: %q", badReq.Error)
	}
}

func TestCreateGeofence_MissingGeometry(t *testing.T) {
	h := newGeofenceTestHandler(&auditMockGeofenceRepo{})

	res, err := h.CreateGeofence(ctxAs(1, model.RoleUser), &oas.GeofenceInput{Name: "No Geometry"})
	if err != nil {
		t.Fatalf("CreateGeofence returned error: %v", err)
	}
	badReq, ok := res.(*oas.CreateGeofenceBadRequest)
	if !ok {
		t.Fatalf("expected *oas.CreateGeofenceBadRequest, got %T", res)
	}
	if badReq.Error != "geometry or area is required" {
		t.Errorf("unexpected error message: %q", badReq.Error)
	}
}

func TestCreateGeofence_InvalidNameOrDescription(t *testing.T) {
	h := newGeofenceTestHandler(&auditMockGeofenceRepo{})

	tests := []struct {
		name        string
		fenceName   string
		description string
	}{
		{"script in name", `<script>alert(1)</script>`, ""},
		{"script in description", "ok", `<img src=x onerror=alert(1)>`},
		{"name too long", strings.Repeat("a", 201), ""},
		{"description too long", "ok", strings.Repeat("b", 2001)},
		{"NUL in name", "foo\x00bar", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := h.CreateGeofence(ctxAs(1, model.RoleUser), &oas.GeofenceInput{
				Name:        tt.fenceName,
				Description: oas.NewOptString(tt.description),
				Geometry:    oas.NewOptString(testPolygonGeoJSON),
			})
			if err != nil {
				t.Fatalf("CreateGeofence returned error: %v", err)
			}
			if _, ok := res.(*oas.CreateGeofenceBadRequest); !ok {
				t.Errorf("expected *oas.CreateGeofenceBadRequest, got %T", res)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetGeofence
// ---------------------------------------------------------------------------

// TestGetGeofence_Forbidden verifies the IDOR protection: a user without an
// association to the geofence must not be able to read it.
func TestGetGeofence_Forbidden(t *testing.T) {
	mock := &auditMockGeofenceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return false },
	}
	h := newGeofenceTestHandler(mock)

	res, err := h.GetGeofence(ctxAs(1, model.RoleUser), oas.GetGeofenceParams{ID: 9})
	if err != nil {
		t.Fatalf("GetGeofence returned error: %v", err)
	}
	forbidden, ok := res.(*oas.GetGeofenceForbidden)
	if !ok {
		t.Fatalf("expected *oas.GetGeofenceForbidden, got %T", res)
	}
	if forbidden.Error != "access denied" {
		t.Errorf("unexpected error message: %q", forbidden.Error)
	}
}

// ---------------------------------------------------------------------------
// UpdateGeofence
// ---------------------------------------------------------------------------

func TestUpdateGeofence_Success(t *testing.T) {
	var updated *model.Geofence
	mock := &auditMockGeofenceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return true },
		getByIDFn: func(_ context.Context, id int64) (*model.Geofence, error) {
			return &model.Geofence{ID: id, Name: "Before", Geometry: testPolygonGeoJSON}, nil
		},
		updateFn: func(_ context.Context, g *model.Geofence) error {
			updated = g
			return nil
		},
	}
	h := newGeofenceTestHandler(mock)

	res, err := h.UpdateGeofence(ctxAs(1, model.RoleUser), &oas.GeofenceUpdateInput{
		Name: oas.NewOptString("After"),
	}, oas.UpdateGeofenceParams{ID: 4})
	if err != nil {
		t.Fatalf("UpdateGeofence returned error: %v", err)
	}
	g, ok := res.(*oas.Geofence)
	if !ok {
		t.Fatalf("expected *oas.Geofence, got %T", res)
	}
	if g.Name != "After" {
		t.Errorf("expected updated name 'After', got %q", g.Name)
	}
	if updated == nil || updated.Geometry != testPolygonGeoJSON {
		t.Error("expected existing geometry preserved on name-only update")
	}
}

// TestUpdateGeofence_GeometryClearsArea verifies the shape-edit semantics:
// supplying a new GeoJSON geometry replaces the shape and clears any stored
// WKT area so the two representations cannot diverge.
func TestUpdateGeofence_GeometryClearsArea(t *testing.T) {
	var updated *model.Geofence
	mock := &auditMockGeofenceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return true },
		getByIDFn: func(_ context.Context, id int64) (*model.Geofence, error) {
			return &model.Geofence{ID: id, Name: "Shape Test", Area: "POLYGON((1 1, 1 2, 2 2, 2 1, 1 1))"}, nil
		},
		updateFn: func(_ context.Context, g *model.Geofence) error {
			updated = g
			return nil
		},
	}
	h := newGeofenceTestHandler(mock)

	res, err := h.UpdateGeofence(ctxAs(1, model.RoleUser), &oas.GeofenceUpdateInput{
		Geometry: oas.NewOptString(testPolygonEastGeoJSON),
	}, oas.UpdateGeofenceParams{ID: 4})
	if err != nil {
		t.Fatalf("UpdateGeofence returned error: %v", err)
	}
	if _, ok := res.(*oas.Geofence); !ok {
		t.Fatalf("expected *oas.Geofence, got %T", res)
	}
	if updated == nil {
		t.Fatal("expected repository Update to be called")
	}
	if updated.Geometry != testPolygonEastGeoJSON {
		t.Errorf("expected geometry replaced, got %q", updated.Geometry)
	}
	if updated.Area != "" {
		t.Errorf("expected WKT area cleared when geometry is set, got %q", updated.Area)
	}
}

// TestUpdateGeofence_AreaClearsGeometry is the WKT counterpart: supplying a
// new area replaces the shape and clears the stored GeoJSON geometry.
func TestUpdateGeofence_AreaClearsGeometry(t *testing.T) {
	const newArea = "POLYGON((13.60 52.55, 13.60 52.57, 13.65 52.57, 13.65 52.55, 13.60 52.55))"
	var updated *model.Geofence
	mock := &auditMockGeofenceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return true },
		getByIDFn: func(_ context.Context, id int64) (*model.Geofence, error) {
			return &model.Geofence{ID: id, Name: "Shape Test", Geometry: testPolygonGeoJSON}, nil
		},
		updateFn: func(_ context.Context, g *model.Geofence) error {
			updated = g
			return nil
		},
	}
	h := newGeofenceTestHandler(mock)

	res, err := h.UpdateGeofence(ctxAs(1, model.RoleUser), &oas.GeofenceUpdateInput{
		Area: oas.NewOptString(newArea),
	}, oas.UpdateGeofenceParams{ID: 4})
	if err != nil {
		t.Fatalf("UpdateGeofence returned error: %v", err)
	}
	if _, ok := res.(*oas.Geofence); !ok {
		t.Fatalf("expected *oas.Geofence, got %T", res)
	}
	if updated == nil {
		t.Fatal("expected repository Update to be called")
	}
	if updated.Area != newArea {
		t.Errorf("expected area replaced, got %q", updated.Area)
	}
	if updated.Geometry != "" {
		t.Errorf("expected GeoJSON geometry cleared when area is set, got %q", updated.Geometry)
	}
}

// TestUpdateGeofence_GeometryAndArea_Integration ports the PostGIS
// containment regression test: after replacing the polygon via the live
// UpdateGeofence method, points inside the old polygon must no longer match
// and points inside the new polygon must match.
func TestUpdateGeofence_GeometryAndArea_Integration(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	geoRepo := repository.NewGeofenceRepository(pool)

	user := testutil.CreateUser(t, "geo-oas@example.com")
	device := testutil.CreateDevice(t, user.ID, "geo-oas-dev")

	g := &model.Geofence{Name: "Shape Test", Geometry: testPolygonGeoJSON}
	if err := geoRepo.Create(ctx, g); err != nil {
		t.Fatalf("create geofence: %v", err)
	}
	if err := geoRepo.AssociateUser(ctx, user.ID, g.ID); err != nil {
		t.Fatalf("associate user: %v", err)
	}

	// Verify original point is contained.
	insideOrig, _ := geoRepo.CheckContainmentForDevice(ctx, device.ID, 52.52, 13.37)
	if len(insideOrig) == 0 {
		t.Fatal("expected (52.52, 13.37) inside original polygon")
	}

	h := handlers.NewHandler(handlers.HandlerConfig{
		Geofences:       geoRepo,
		GeofenceService: services.NewGeofenceService(geoRepo, nil),
	})

	res, err := h.UpdateGeofence(api.ContextWithUser(ctx, user), &oas.GeofenceUpdateInput{
		Geometry: oas.NewOptString(testPolygonEastGeoJSON),
	}, oas.UpdateGeofenceParams{ID: g.ID})
	if err != nil {
		t.Fatalf("UpdateGeofence returned error: %v", err)
	}
	if _, ok := res.(*oas.Geofence); !ok {
		t.Fatalf("expected *oas.Geofence, got %T", res)
	}

	// Old point must now be outside.
	nowOutside, _ := geoRepo.CheckContainmentForDevice(ctx, device.ID, 52.52, 13.37)
	if len(nowOutside) > 0 {
		t.Error("expected (52.52, 13.37) outside after geometry update")
	}

	// New point (inside east polygon) must now be contained.
	nowInside, _ := geoRepo.CheckContainmentForDevice(ctx, device.ID, 52.56, 13.62)
	if len(nowInside) == 0 || nowInside[0] != g.ID {
		t.Error("expected (52.56, 13.62) inside updated polygon")
	}
}

func TestUpdateGeofence_InvalidName(t *testing.T) {
	mock := &auditMockGeofenceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return true },
		getByIDFn: func(_ context.Context, id int64) (*model.Geofence, error) {
			return &model.Geofence{ID: id, Name: "Valid Fence", Geometry: testPolygonGeoJSON}, nil
		},
	}
	h := newGeofenceTestHandler(mock)

	tests := []struct {
		name        string
		fenceName   oas.OptString
		description oas.OptString
	}{
		{"script in name", oas.NewOptString("<script>x</script>"), oas.OptString{}},
		{"script in description", oas.NewOptString("ok"), oas.NewOptString("<img src=x>")},
		{"name too long", oas.NewOptString(strings.Repeat("x", 201)), oas.OptString{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := h.UpdateGeofence(ctxAs(1, model.RoleUser), &oas.GeofenceUpdateInput{
				Name:        tt.fenceName,
				Description: tt.description,
			}, oas.UpdateGeofenceParams{ID: 4})
			if err != nil {
				t.Fatalf("UpdateGeofence returned error: %v", err)
			}
			if _, ok := res.(*oas.UpdateGeofenceBadRequest); !ok {
				t.Errorf("expected *oas.UpdateGeofenceBadRequest, got %T", res)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// DeleteGeofence
// ---------------------------------------------------------------------------

func TestDeleteGeofence_Success(t *testing.T) {
	var deletedID int64
	mock := &auditMockGeofenceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return true },
		deleteFn: func(_ context.Context, id int64) error {
			deletedID = id
			return nil
		},
	}
	h := newGeofenceTestHandler(mock)

	res, err := h.DeleteGeofence(ctxAs(1, model.RoleUser), oas.DeleteGeofenceParams{ID: 6})
	if err != nil {
		t.Fatalf("DeleteGeofence returned error: %v", err)
	}
	if _, ok := res.(*oas.DeleteGeofenceNoContent); !ok {
		t.Fatalf("expected *oas.DeleteGeofenceNoContent, got %T", res)
	}
	if deletedID != 6 {
		t.Errorf("expected delete called with ID=6, got %d", deletedID)
	}
}

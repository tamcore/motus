package handlers_test

// Regression tests for POST /api/geofences decoded through the generated ogen
// server, using the exact JSON body the geofences page sends. The web UI sends
// only a GeoJSON "geometry" (no WKT "area"); the spec previously marked "area"
// as required, so ogen rejected every UI-drawn geofence with 400 before the
// handler ran.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tamcore/motus/internal/api"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/model"
)

// userSecurity accepts any credential and injects a fixed user into the
// context, mirroring what the real SecurityHandler does after validation.
type userSecurity struct{ user *model.User }

func (s userSecurity) HandleBearerAuth(ctx context.Context, _ oas.OperationName, _ oas.BearerAuth) (context.Context, error) {
	return api.ContextWithUser(ctx, s.user), nil
}

func (s userSecurity) HandleCookieAuth(ctx context.Context, _ oas.OperationName, _ oas.CookieAuth) (context.Context, error) {
	return api.ContextWithUser(ctx, s.user), nil
}

func (s userSecurity) HandleXAuthToken(ctx context.Context, _ oas.OperationName, _ oas.XAuthToken) (context.Context, error) {
	return api.ContextWithUser(ctx, s.user), nil
}

func postGeofence(t *testing.T, mock *auditMockGeofenceRepo, body string) *httptest.ResponseRecorder {
	t.Helper()
	srv, err := oas.NewServer(newGeofenceTestHandler(mock), userSecurity{
		user: &model.User{ID: 1, Email: "geo@example.com", Role: model.RoleUser},
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/geofences", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer x")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func TestCreateGeofenceHTTP_GeometryOnlyPayloadFromUI(t *testing.T) {
	var stored *model.Geofence
	mock := &auditMockGeofenceRepo{
		createFn: func(_ context.Context, g *model.Geofence) error {
			g.ID = 42
			stored = g
			return nil
		},
		associateUserFn: func(_ context.Context, _, _ int64) error { return nil },
	}

	// Exact shape built by saveGeofence() in web/src/routes/geofences/+page.svelte.
	body := `{"name":"Drawn Fence","description":"","geometry":` +
		`"{\"type\":\"Polygon\",\"coordinates\":[[[13.35,52.51],[13.35,52.53],[13.4,52.53],[13.4,52.51],[13.35,52.51]]]}",` +
		`"calendarId":null}`

	rec := postGeofence(t, mock, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if stored == nil {
		t.Fatal("repository Create was not called")
	}
	if !strings.Contains(stored.Geometry, `"Polygon"`) {
		t.Errorf("expected GeoJSON geometry to reach the repository, got %q", stored.Geometry)
	}
	if stored.Area != "" {
		t.Errorf("expected empty area, got %q", stored.Area)
	}
}

func TestCreateGeofenceHTTP_AreaOnlyPayloadStillAccepted(t *testing.T) {
	var stored *model.Geofence
	mock := &auditMockGeofenceRepo{
		createFn: func(_ context.Context, g *model.Geofence) error {
			stored = g
			return nil
		},
		associateUserFn: func(_ context.Context, _, _ int64) error { return nil },
	}

	rec := postGeofence(t, mock, `{"name":"Traccar Fence","area":"POLYGON((11.57 48.12,11.6 48.12,11.6 48.15,11.57 48.15,11.57 48.12))"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if stored == nil || !strings.HasPrefix(stored.Area, "POLYGON") {
		t.Fatalf("expected WKT area to reach the repository, got %+v", stored)
	}
}

func TestCreateGeofenceHTTP_NeitherGeometryNorArea(t *testing.T) {
	rec := postGeofence(t, &auditMockGeofenceRepo{}, `{"name":"Empty Fence"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "geometry or area is required") {
		t.Errorf("expected service validation message, got %s", rec.Body.String())
	}
}

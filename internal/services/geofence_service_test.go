package services

import (
	"context"
	"testing"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func setupGeofenceServiceCRUD(t *testing.T) (*GeofenceService, *repository.GeofenceRepository, *repository.CalendarRepository, *repository.UserRepository) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	geoRepo := repository.NewGeofenceRepository(pool)
	calRepo := repository.NewCalendarRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	svc := NewGeofenceService(geoRepo, nil)
	return svc, geoRepo, calRepo, userRepo
}

const testGeoJSONCircle = `{"type":"Polygon","coordinates":[[[13.40,52.51],[13.40,52.53],[13.42,52.53],[13.42,52.51],[13.40,52.51]]]}`

func createTestUserAndGeofence(t *testing.T, ctx context.Context, svc *GeofenceService, userRepo *repository.UserRepository, email string) (*model.User, *model.Geofence) {
	t.Helper()
	user := &model.User{Email: email, PasswordHash: "hash", Name: "Test"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	g, err := svc.CreateForUser(ctx, user, CreateGeofenceInput{Name: "Original", Geometry: testGeoJSONCircle})
	if err != nil {
		t.Fatalf("create geofence: %v", err)
	}
	return user, g
}

func TestGeofenceService_UpdateForUser_RenameName(t *testing.T) {
	svc, _, _, userRepo := setupGeofenceServiceCRUD(t)
	ctx := context.Background()
	user, g := createTestUserAndGeofence(t, ctx, svc, userRepo, "geo-update@example.com")

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
	svc, _, calRepo, userRepo := setupGeofenceServiceCRUD(t)
	ctx := context.Background()
	user, g := createTestUserAndGeofence(t, ctx, svc, userRepo, "geo-attach@example.com")

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
	svc, _, calRepo, userRepo := setupGeofenceServiceCRUD(t)
	ctx := context.Background()
	user, g := createTestUserAndGeofence(t, ctx, svc, userRepo, "geo-detach@example.com")

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
	svc, _, _, userRepo := setupGeofenceServiceCRUD(t)
	ctx := context.Background()
	_, g := createTestUserAndGeofence(t, ctx, svc, userRepo, "geo-owner@example.com")

	other := &model.User{Email: "geo-other@example.com", PasswordHash: "hash", Name: "Other"}
	_ = userRepo.Create(ctx, other)

	newName := "Hack"
	_, err := svc.UpdateForUser(ctx, other, g.ID, UpdateGeofenceInput{Name: &newName})
	if err == nil {
		t.Fatal("expected access denied error")
	}
}

func TestGeofenceService_DeleteForUser_HappyPath(t *testing.T) {
	svc, geoRepo, _, userRepo := setupGeofenceServiceCRUD(t)
	ctx := context.Background()
	user, g := createTestUserAndGeofence(t, ctx, svc, userRepo, "geo-del@example.com")

	if err := svc.DeleteForUser(ctx, user, g.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	all, _ := geoRepo.GetByUser(ctx, user.ID)
	if len(all) != 0 {
		t.Errorf("expected 0 geofences after delete, got %d", len(all))
	}
}

func TestGeofenceService_DeleteForUser_AccessDenied(t *testing.T) {
	svc, _, _, userRepo := setupGeofenceServiceCRUD(t)
	ctx := context.Background()
	_, g := createTestUserAndGeofence(t, ctx, svc, userRepo, "geo-delacc@example.com")

	other := &model.User{Email: "geo-del-other@example.com", PasswordHash: "hash", Name: "Other2"}
	_ = userRepo.Create(ctx, other)

	if err := svc.DeleteForUser(ctx, other, g.ID); err == nil {
		t.Fatal("expected access denied error")
	}
}

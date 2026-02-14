package repository_test

import (
	"context"
	"testing"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

const sampleICalData = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Motus//Test//EN
BEGIN:VEVENT
DTSTART:20260115T090000Z
DTEND:20260115T170000Z
RRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR
SUMMARY:Business Hours
END:VEVENT
END:VCALENDAR`

func TestCalendarRepository_Create(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	calRepo := repository.NewCalendarRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "cal-create@example.com", PasswordHash: "hash", Name: "Cal Create"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	cal := &model.Calendar{
		UserID: user.ID,
		Name:   "Business Hours",
		Data:   sampleICalData,
	}

	if err := calRepo.Create(ctx, cal); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if cal.ID == 0 {
		t.Error("expected calendar ID to be set")
	}
	if cal.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if cal.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}

	// Verify user_calendars association was created.
	if !calRepo.UserHasAccess(ctx, &model.User{ID: user.ID}, cal.ID) {
		t.Error("expected user to have access to created calendar")
	}
}

func TestCalendarRepository_GetByID(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	calRepo := repository.NewCalendarRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "cal-getid@example.com", PasswordHash: "hash", Name: "Cal GetID"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	cal := &model.Calendar{UserID: user.ID, Name: "Test Cal", Data: sampleICalData}
	if err := calRepo.Create(ctx, cal); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	found, err := calRepo.GetByID(ctx, cal.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found.Name != "Test Cal" {
		t.Errorf("expected name 'Test Cal', got %q", found.Name)
	}
	if found.Data != sampleICalData {
		t.Error("expected iCal data to match")
	}
	if found.UserID != user.ID {
		t.Errorf("expected userID %d, got %d", user.ID, found.UserID)
	}
}

func TestCalendarRepository_GetByID_NotFound(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	calRepo := repository.NewCalendarRepository(pool)
	ctx := context.Background()

	_, err := calRepo.GetByID(ctx, 99999)
	if err == nil {
		t.Error("expected error for nonexistent calendar")
	}
}

func TestCalendarRepository_GetByUser(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	calRepo := repository.NewCalendarRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "cal-user@example.com", PasswordHash: "hash", Name: "Cal User"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	cal1 := &model.Calendar{UserID: user.ID, Name: "Alpha Calendar", Data: sampleICalData}
	cal2 := &model.Calendar{UserID: user.ID, Name: "Beta Calendar", Data: sampleICalData}
	if err := calRepo.Create(ctx, cal1); err != nil {
		t.Fatalf("Create cal1 failed: %v", err)
	}
	if err := calRepo.Create(ctx, cal2); err != nil {
		t.Fatalf("Create cal2 failed: %v", err)
	}

	calendars, err := calRepo.GetByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByUser failed: %v", err)
	}
	if len(calendars) != 2 {
		t.Fatalf("expected 2 calendars, got %d", len(calendars))
	}
	// Ordered by name.
	if calendars[0].Name != "Alpha Calendar" {
		t.Errorf("expected first 'Alpha Calendar', got %q", calendars[0].Name)
	}
	if calendars[1].Name != "Beta Calendar" {
		t.Errorf("expected second 'Beta Calendar', got %q", calendars[1].Name)
	}
}

func TestCalendarRepository_GetByUser_Empty(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	calRepo := repository.NewCalendarRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "cal-empty@example.com", PasswordHash: "hash", Name: "Cal Empty"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	calendars, err := calRepo.GetByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByUser failed: %v", err)
	}
	if len(calendars) != 0 {
		t.Errorf("expected 0 calendars, got %d", len(calendars))
	}
}

func TestCalendarRepository_Update(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	calRepo := repository.NewCalendarRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "cal-update@example.com", PasswordHash: "hash", Name: "Cal Update"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	cal := &model.Calendar{UserID: user.ID, Name: "Before Update", Data: sampleICalData}
	if err := calRepo.Create(ctx, cal); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	cal.Name = "After Update"
	cal.Data = "BEGIN:VCALENDAR\nVERSION:2.0\nBEGIN:VEVENT\nDTSTART:20260201T000000Z\nDTEND:20260201T235959Z\nSUMMARY:Updated\nEND:VEVENT\nEND:VCALENDAR"
	if err := calRepo.Update(ctx, cal); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	found, err := calRepo.GetByID(ctx, cal.ID)
	if err != nil {
		t.Fatalf("GetByID after update failed: %v", err)
	}
	if found.Name != "After Update" {
		t.Errorf("expected name 'After Update', got %q", found.Name)
	}
}

func TestCalendarRepository_Delete(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	calRepo := repository.NewCalendarRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "cal-delete@example.com", PasswordHash: "hash", Name: "Cal Delete"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	cal := &model.Calendar{UserID: user.ID, Name: "Delete Me", Data: sampleICalData}
	if err := calRepo.Create(ctx, cal); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := calRepo.Delete(ctx, cal.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := calRepo.GetByID(ctx, cal.ID)
	if err == nil {
		t.Error("expected error after deletion")
	}
}

func TestCalendarRepository_UserHasAccess(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	calRepo := repository.NewCalendarRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user1 := &model.User{Email: "cal-access1@example.com", PasswordHash: "hash", Name: "Access 1"}
	user2 := &model.User{Email: "cal-access2@example.com", PasswordHash: "hash", Name: "Access 2"}
	if err := userRepo.Create(ctx, user1); err != nil {
		t.Fatalf("create user1: %v", err)
	}
	if err := userRepo.Create(ctx, user2); err != nil {
		t.Fatalf("create user2: %v", err)
	}

	cal := &model.Calendar{UserID: user1.ID, Name: "Access Cal", Data: sampleICalData}
	if err := calRepo.Create(ctx, cal); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// user1 should have access (created it).
	if !calRepo.UserHasAccess(ctx, &model.User{ID: user1.ID}, cal.ID) {
		t.Error("expected user1 to have access")
	}

	// user2 should NOT have access.
	if calRepo.UserHasAccess(ctx, &model.User{ID: user2.ID}, cal.ID) {
		t.Error("expected user2 to NOT have access")
	}

	// Associate user2 and verify.
	if err := calRepo.AssociateUser(ctx, user2.ID, cal.ID); err != nil {
		t.Fatalf("AssociateUser failed: %v", err)
	}
	if !calRepo.UserHasAccess(ctx, &model.User{ID: user2.ID}, cal.ID) {
		t.Error("expected user2 to have access after association")
	}
}

func TestCalendarRepository_AssociateUser_Idempotent(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	calRepo := repository.NewCalendarRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "cal-idempotent@example.com", PasswordHash: "hash", Name: "Idempotent"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	cal := &model.Calendar{UserID: user.ID, Name: "Idempotent Cal", Data: sampleICalData}
	if err := calRepo.Create(ctx, cal); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Associate twice -- should not error (ON CONFLICT DO NOTHING).
	if err := calRepo.AssociateUser(ctx, user.ID, cal.ID); err != nil {
		t.Fatalf("First AssociateUser failed: %v", err)
	}
	if err := calRepo.AssociateUser(ctx, user.ID, cal.ID); err != nil {
		t.Fatalf("Second AssociateUser failed: %v", err)
	}
}

func TestCalendarRepository_Delete_NullsGeofenceCalendarID(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	calRepo := repository.NewCalendarRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	geoRepo := repository.NewGeofenceRepository(pool)
	ctx := context.Background()

	user := &model.User{Email: "cal-geo-null@example.com", PasswordHash: "hash", Name: "Cal GeoNull"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	cal := &model.Calendar{UserID: user.ID, Name: "Linked Cal", Data: sampleICalData}
	if err := calRepo.Create(ctx, cal); err != nil {
		t.Fatalf("Create calendar failed: %v", err)
	}

	// Create a geofence and link it to the calendar via raw SQL.
	g := &model.Geofence{Name: "Cal Fence", Geometry: berlinPolygonGeoJSON}
	if err := geoRepo.Create(ctx, g); err != nil {
		t.Fatalf("Create geofence failed: %v", err)
	}
	_, err := pool.Exec(ctx, `UPDATE geofences SET calendar_id = $1 WHERE id = $2`, cal.ID, g.ID)
	if err != nil {
		t.Fatalf("Link geofence to calendar failed: %v", err)
	}

	// Delete the calendar.
	if err := calRepo.Delete(ctx, cal.ID); err != nil {
		t.Fatalf("Delete calendar failed: %v", err)
	}

	// Verify geofence still exists but calendar_id is NULL.
	var calendarID *int64
	err = pool.QueryRow(ctx, `SELECT calendar_id FROM geofences WHERE id = $1`, g.ID).Scan(&calendarID)
	if err != nil {
		t.Fatalf("Query geofence failed: %v", err)
	}
	if calendarID != nil {
		t.Errorf("expected calendar_id to be NULL after calendar deletion, got %d", *calendarID)
	}
}

package services

import (
	"context"
	"testing"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func setupCalendarService(t *testing.T) (*CalendarService, *repository.CalendarRepository, *repository.UserRepository) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	calRepo := repository.NewCalendarRepository(pool)
	userRepo := repository.NewUserRepository(pool)
	svc := NewCalendarService(calRepo, nil)
	return svc, calRepo, userRepo
}

const testIcal = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//test//EN\r\nBEGIN:VEVENT\r\nUID:test@test\r\nSUMMARY:Test\r\nDTSTART:20260606T180000Z\r\nDTEND:20260606T200000Z\r\nEND:VEVENT\r\nEND:VCALENDAR"

func TestCalendarService_CreateForUser_HappyPath(t *testing.T) {
	svc, calRepo, userRepo := setupCalendarService(t)
	ctx := context.Background()

	user := &model.User{Email: "calsvc@example.com", PasswordHash: "hash", Name: "Cal Svc"}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	cal, err := svc.CreateForUser(ctx, user, CreateCalendarInput{Name: "Test Cal", Data: testIcal})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cal.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if cal.Name != "Test Cal" {
		t.Errorf("name mismatch: %q", cal.Name)
	}

	cals, err := calRepo.GetByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByUser: %v", err)
	}
	if len(cals) != 1 {
		t.Fatalf("expected 1 calendar, got %d", len(cals))
	}
}

func TestCalendarService_CreateForUser_EmptyName(t *testing.T) {
	svc, _, userRepo := setupCalendarService(t)
	ctx := context.Background()

	user := &model.User{Email: "calname@example.com", PasswordHash: "hash", Name: "Cal Name"}
	_ = userRepo.Create(ctx, user)

	_, err := svc.CreateForUser(ctx, user, CreateCalendarInput{Name: "", Data: testIcal})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestCalendarService_CreateForUser_InvalidIcal(t *testing.T) {
	svc, _, userRepo := setupCalendarService(t)
	ctx := context.Background()

	user := &model.User{Email: "calical@example.com", PasswordHash: "hash", Name: "Cal Ical"}
	_ = userRepo.Create(ctx, user)

	_, err := svc.CreateForUser(ctx, user, CreateCalendarInput{Name: "Bad Cal", Data: "not-ical"})
	if err == nil {
		t.Fatal("expected error for invalid iCal data")
	}
}

func TestCalendarService_CreateForUser_NameTooLong(t *testing.T) {
	svc, _, userRepo := setupCalendarService(t)
	ctx := context.Background()

	user := &model.User{Email: "callong@example.com", PasswordHash: "hash", Name: "Cal Long"}
	_ = userRepo.Create(ctx, user)

	longName := string(make([]byte, 256))
	_, err := svc.CreateForUser(ctx, user, CreateCalendarInput{Name: longName, Data: testIcal})
	if err == nil {
		t.Fatal("expected error for name exceeding max length")
	}
}

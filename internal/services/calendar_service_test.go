package services

import (
	"context"
	"testing"

	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

func setupCalendarService(t *testing.T) (*CalendarService, *repository.CalendarRepository) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	calRepo := repository.NewCalendarRepository(pool)
	svc := NewCalendarService(calRepo, nil)
	return svc, calRepo
}

const testIcal = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//test//EN\r\nBEGIN:VEVENT\r\nUID:test@test\r\nSUMMARY:Test\r\nDTSTART:20260606T180000Z\r\nDTEND:20260606T200000Z\r\nEND:VEVENT\r\nEND:VCALENDAR"

func TestCalendarService_CreateForUser_HappyPath(t *testing.T) {
	svc, calRepo := setupCalendarService(t)
	ctx := context.Background()

	user := testutil.CreateUser(t, "calsvc@example.com")

	cal, err := svc.CreateForUser(ctx, user, CalendarInput{Name: "Test Cal", Data: testIcal})
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
	svc, _ := setupCalendarService(t)
	ctx := context.Background()

	user := testutil.CreateUser(t, "calname@example.com")

	_, err := svc.CreateForUser(ctx, user, CalendarInput{Name: "", Data: testIcal})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestCalendarService_CreateForUser_InvalidIcal(t *testing.T) {
	svc, _ := setupCalendarService(t)
	ctx := context.Background()

	user := testutil.CreateUser(t, "calical@example.com")

	_, err := svc.CreateForUser(ctx, user, CalendarInput{Name: "Bad Cal", Data: "not-ical"})
	if err == nil {
		t.Fatal("expected error for invalid iCal data")
	}
}

func TestCalendarService_CreateForUser_NameTooLong(t *testing.T) {
	svc, _ := setupCalendarService(t)
	ctx := context.Background()

	user := testutil.CreateUser(t, "callong@example.com")

	longName := string(make([]byte, 256))
	_, err := svc.CreateForUser(ctx, user, CalendarInput{Name: longName, Data: testIcal})
	if err == nil {
		t.Fatal("expected error for name exceeding max length")
	}
}

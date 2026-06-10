package handlers_test

// Tests for the ogen Handler calendar methods (CreateCalendar,
// UpdateCalendar, DeleteCalendar, CheckCalendar, AdminListCalendars).
// Ported from the deleted chi CalendarHandler tests in calendar_test.go.
//
// Dropped tests (no live equivalent):
//   - invalid path-param ID parsing: ogen owns path param decoding.
//
// Access-denied mapping note: the live handler responds with NotFound (not
// Forbidden) when the user has no access to the calendar, so the calendar's
// existence is not leaked. The IDOR assertions below reflect that.

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
	"github.com/tamcore/motus/internal/storage/repository"
)

// testICalData is a minimal valid iCalendar (RFC 5545) document.
const testICalData = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Motus//Test//EN
BEGIN:VEVENT
DTSTART:20260115T090000Z
DTEND:20260115T170000Z
RRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR
SUMMARY:Business Hours
END:VEVENT
END:VCALENDAR`

// newCalendarTestHandler builds an ogen Handler from a mock calendar repo
// with a nil-pool audit logger (Log is a documented no-op without a pool),
// so the audit code paths in create/update/delete are exercised.
func newCalendarTestHandler(calendars repository.CalendarRepo) *handlers.Handler {
	return handlers.NewHandler(handlers.HandlerConfig{
		Calendars:   calendars,
		AuditLogger: audit.NewLogger(nil),
	})
}

func calendarTestUserCtx(id int64) context.Context {
	return api.ContextWithUser(context.Background(), &model.User{ID: id, Email: "cal@example.com", Role: model.RoleUser})
}

// ---------------------------------------------------------------------------
// CreateCalendar
// ---------------------------------------------------------------------------

func TestCreateCalendar_Success(t *testing.T) {
	var created *model.Calendar
	mock := &auditMockCalendarRepo{
		createFn: func(_ context.Context, c *model.Calendar) error {
			c.ID = 7
			created = c
			return nil
		},
	}
	h := newCalendarTestHandler(mock)

	res, err := h.CreateCalendar(calendarTestUserCtx(1), &oas.CalendarInput{
		Name: "Test Calendar",
		Data: testICalData,
	})
	if err != nil {
		t.Fatalf("CreateCalendar returned error: %v", err)
	}
	cal, ok := res.(*oas.Calendar)
	if !ok {
		t.Fatalf("expected *oas.Calendar, got %T", res)
	}
	if cal.Name != "Test Calendar" {
		t.Errorf("expected name 'Test Calendar', got %q", cal.Name)
	}
	if cal.ID != 7 {
		t.Errorf("expected calendar ID 7, got %d", cal.ID)
	}
	if created == nil || created.UserID != 1 {
		t.Error("expected calendar created with UserID 1")
	}
}

func TestCreateCalendar_MissingName(t *testing.T) {
	h := newCalendarTestHandler(&auditMockCalendarRepo{})

	res, err := h.CreateCalendar(calendarTestUserCtx(1), &oas.CalendarInput{Data: testICalData})
	if err != nil {
		t.Fatalf("CreateCalendar returned error: %v", err)
	}
	badReq, ok := res.(*oas.CreateCalendarBadRequest)
	if !ok {
		t.Fatalf("expected *oas.CreateCalendarBadRequest, got %T", res)
	}
	if badReq.Error != "name is required" {
		t.Errorf("unexpected error message: %q", badReq.Error)
	}
}

func TestCreateCalendar_MissingData(t *testing.T) {
	h := newCalendarTestHandler(&auditMockCalendarRepo{})

	res, err := h.CreateCalendar(calendarTestUserCtx(1), &oas.CalendarInput{Name: "No Data"})
	if err != nil {
		t.Fatalf("CreateCalendar returned error: %v", err)
	}
	badReq, ok := res.(*oas.CreateCalendarBadRequest)
	if !ok {
		t.Fatalf("expected *oas.CreateCalendarBadRequest, got %T", res)
	}
	if badReq.Error != "data is required" {
		t.Errorf("unexpected error message: %q", badReq.Error)
	}
}

func TestCreateCalendar_InvalidICalData(t *testing.T) {
	h := newCalendarTestHandler(&auditMockCalendarRepo{})

	res, err := h.CreateCalendar(calendarTestUserCtx(1), &oas.CalendarInput{
		Name: "Bad Cal",
		Data: "this is not valid ical",
	})
	if err != nil {
		t.Fatalf("CreateCalendar returned error: %v", err)
	}
	badReq, ok := res.(*oas.CreateCalendarBadRequest)
	if !ok {
		t.Fatalf("expected *oas.CreateCalendarBadRequest, got %T", res)
	}
	if !strings.HasPrefix(badReq.Error, "invalid iCalendar data:") {
		t.Errorf("unexpected error message: %q", badReq.Error)
	}
}

func TestCreateCalendar_InvalidName(t *testing.T) {
	h := newCalendarTestHandler(&auditMockCalendarRepo{})

	tests := []struct {
		name    string
		calName string
	}{
		{"script in name", "<script>alert(1)</script>"},
		{"angle bracket in name", "a > b"},
		{"name too long", strings.Repeat("x", 201)},
		{"NUL in name", "foo\x00bar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := h.CreateCalendar(calendarTestUserCtx(1), &oas.CalendarInput{
				Name: tt.calName,
				Data: testICalData,
			})
			if err != nil {
				t.Fatalf("CreateCalendar returned error: %v", err)
			}
			if _, ok := res.(*oas.CreateCalendarBadRequest); !ok {
				t.Errorf("expected *oas.CreateCalendarBadRequest, got %T", res)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// UpdateCalendar
// ---------------------------------------------------------------------------

func TestUpdateCalendar_Success(t *testing.T) {
	var updated *model.Calendar
	mock := &auditMockCalendarRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return true },
		getByIDFn: func(_ context.Context, id int64) (*model.Calendar, error) {
			return &model.Calendar{ID: id, UserID: 1, Name: "Before", Data: testICalData}, nil
		},
		updateFn: func(_ context.Context, c *model.Calendar) error {
			updated = c
			return nil
		},
	}
	h := newCalendarTestHandler(mock)

	res, err := h.UpdateCalendar(calendarTestUserCtx(1),
		&oas.CalendarInput{Name: "After"}, oas.UpdateCalendarParams{ID: 3})
	if err != nil {
		t.Fatalf("UpdateCalendar returned error: %v", err)
	}
	cal, ok := res.(*oas.Calendar)
	if !ok {
		t.Fatalf("expected *oas.Calendar, got %T", res)
	}
	if cal.Name != "After" {
		t.Errorf("expected updated name 'After', got %q", cal.Name)
	}
	if updated == nil || updated.Name != "After" {
		t.Fatal("expected repository Update called with new name")
	}
	// Data was omitted in the request and must be preserved.
	if updated.Data != testICalData {
		t.Error("expected existing iCal data to be preserved on name-only update")
	}
}

// TestUpdateCalendar_Forbidden verifies the IDOR protection: a user without
// access must not be able to modify another user's calendar.
func TestUpdateCalendar_Forbidden(t *testing.T) {
	updateCalled := false
	mock := &auditMockCalendarRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return false },
		getByIDFn: func(_ context.Context, id int64) (*model.Calendar, error) {
			return &model.Calendar{ID: id, UserID: 99, Name: "Other Cal", Data: testICalData}, nil
		},
		updateFn: func(_ context.Context, _ *model.Calendar) error {
			updateCalled = true
			return nil
		},
	}
	h := newCalendarTestHandler(mock)

	res, err := h.UpdateCalendar(calendarTestUserCtx(1),
		&oas.CalendarInput{Name: "Hacked"}, oas.UpdateCalendarParams{ID: 3})
	if err != nil {
		t.Fatalf("UpdateCalendar returned error: %v", err)
	}
	if _, ok := res.(*oas.UpdateCalendarNotFound); !ok {
		t.Fatalf("expected *oas.UpdateCalendarNotFound for no-access user, got %T", res)
	}
	if updateCalled {
		t.Error("Update must not be called for another user's calendar")
	}
}

func TestUpdateCalendar_InvalidName(t *testing.T) {
	mock := &auditMockCalendarRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return true },
		getByIDFn: func(_ context.Context, id int64) (*model.Calendar, error) {
			return &model.Calendar{ID: id, UserID: 1, Name: "Valid Cal", Data: testICalData}, nil
		},
	}
	h := newCalendarTestHandler(mock)

	tests := []struct {
		name    string
		calName string
	}{
		{"script in name", "<script>x</script>"},
		{"name too long", strings.Repeat("x", 201)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := h.UpdateCalendar(calendarTestUserCtx(1),
				&oas.CalendarInput{Name: tt.calName}, oas.UpdateCalendarParams{ID: 3})
			if err != nil {
				t.Fatalf("UpdateCalendar returned error: %v", err)
			}
			if _, ok := res.(*oas.UpdateCalendarBadRequest); !ok {
				t.Errorf("expected *oas.UpdateCalendarBadRequest, got %T", res)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// DeleteCalendar
// ---------------------------------------------------------------------------

func TestDeleteCalendar_Success(t *testing.T) {
	var deletedID int64
	mock := &auditMockCalendarRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return true },
		deleteFn: func(_ context.Context, id int64) error {
			deletedID = id
			return nil
		},
	}
	h := newCalendarTestHandler(mock)

	res, err := h.DeleteCalendar(calendarTestUserCtx(1), oas.DeleteCalendarParams{ID: 5})
	if err != nil {
		t.Fatalf("DeleteCalendar returned error: %v", err)
	}
	if _, ok := res.(*oas.DeleteCalendarNoContent); !ok {
		t.Fatalf("expected *oas.DeleteCalendarNoContent, got %T", res)
	}
	if deletedID != 5 {
		t.Errorf("expected delete called with ID=5, got %d", deletedID)
	}
}

func TestDeleteCalendar_Forbidden(t *testing.T) {
	deleteCalled := false
	mock := &auditMockCalendarRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return false },
		deleteFn: func(_ context.Context, _ int64) error {
			deleteCalled = true
			return nil
		},
	}
	h := newCalendarTestHandler(mock)

	res, err := h.DeleteCalendar(calendarTestUserCtx(1), oas.DeleteCalendarParams{ID: 5})
	if err != nil {
		t.Fatalf("DeleteCalendar returned error: %v", err)
	}
	if _, ok := res.(*oas.DeleteCalendarNotFound); !ok {
		t.Fatalf("expected *oas.DeleteCalendarNotFound for no-access user, got %T", res)
	}
	if deleteCalled {
		t.Error("Delete must not be called for another user's calendar")
	}
}

// ---------------------------------------------------------------------------
// CheckCalendar
// ---------------------------------------------------------------------------

func TestCheckCalendar_Success(t *testing.T) {
	mock := &auditMockCalendarRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return true },
		getByIDFn: func(_ context.Context, id int64) (*model.Calendar, error) {
			return &model.Calendar{ID: id, UserID: 1, Name: "Check Cal", Data: testICalData}, nil
		},
	}
	h := newCalendarTestHandler(mock)

	res, err := h.CheckCalendar(calendarTestUserCtx(1), oas.CheckCalendarParams{ID: 5})
	if err != nil {
		t.Fatalf("CheckCalendar returned error: %v", err)
	}
	// The typed result carries the 'active' flag; its value depends on the
	// current time vs. the test schedule, so only the shape is asserted.
	if _, ok := res.(*oas.CalendarCheckResult); !ok {
		t.Fatalf("expected *oas.CalendarCheckResult, got %T", res)
	}
}

func TestCheckCalendar_Forbidden(t *testing.T) {
	mock := &auditMockCalendarRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return false },
		getByIDFn: func(_ context.Context, _ int64) (*model.Calendar, error) {
			return nil, errors.New("must not be reached")
		},
	}
	h := newCalendarTestHandler(mock)

	res, err := h.CheckCalendar(calendarTestUserCtx(1), oas.CheckCalendarParams{ID: 5})
	if err != nil {
		t.Fatalf("CheckCalendar returned error: %v", err)
	}
	if _, ok := res.(*oas.CheckCalendarNotFound); !ok {
		t.Fatalf("expected *oas.CheckCalendarNotFound for no-access user, got %T", res)
	}
}

// ---------------------------------------------------------------------------
// AdminListCalendars
// ---------------------------------------------------------------------------

func TestAdminListCalendars_NonAdminForbidden(t *testing.T) {
	h := newCalendarTestHandler(&auditMockCalendarRepo{})

	// Unauthenticated.
	res, err := h.AdminListCalendars(context.Background())
	if err != nil {
		t.Fatalf("AdminListCalendars returned error: %v", err)
	}
	if _, ok := res.(*oas.AdminListCalendarsForbidden); !ok {
		t.Errorf("expected *oas.AdminListCalendarsForbidden for unauthenticated request, got %T", res)
	}

	// Non-admin.
	res, err = h.AdminListCalendars(calendarTestUserCtx(1))
	if err != nil {
		t.Fatalf("AdminListCalendars returned error: %v", err)
	}
	forbidden, ok := res.(*oas.AdminListCalendarsForbidden)
	if !ok {
		t.Fatalf("expected *oas.AdminListCalendarsForbidden for non-admin, got %T", res)
	}
	if forbidden.Error == "" {
		t.Error("expected non-empty error message")
	}
}

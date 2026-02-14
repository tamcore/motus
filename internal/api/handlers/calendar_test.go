package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

const integrationICalData = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Motus//Test//EN
BEGIN:VEVENT
DTSTART:20260115T090000Z
DTEND:20260115T170000Z
RRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR
SUMMARY:Business Hours
END:VEVENT
END:VCALENDAR`

func setupCalendarHandler(t *testing.T) (*handlers.CalendarHandler, *repository.CalendarRepository, *model.User) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	calRepo := repository.NewCalendarRepository(pool)

	user := &model.User{Email: "calhandler@example.com", PasswordHash: "$2a$10$hash", Name: "Cal Handler"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	h := handlers.NewCalendarHandler(calRepo)
	return h, calRepo, user
}

func TestCalendarHandler_List_Empty(t *testing.T) {
	h, _, user := setupCalendarHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/calendars", nil)
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var cals []model.Calendar
	_ = json.NewDecoder(rr.Body).Decode(&cals)
	if len(cals) != 0 {
		t.Errorf("expected empty list, got %d", len(cals))
	}
}

func TestCalendarHandler_Create_Success(t *testing.T) {
	h, _, user := setupCalendarHandler(t)

	body := fmt.Sprintf(`{"name":"Test Calendar","data":%q}`, integrationICalData)
	req := httptest.NewRequest(http.MethodPost, "/api/calendars", bytes.NewReader([]byte(body)))
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var cal model.Calendar
	_ = json.NewDecoder(rr.Body).Decode(&cal)
	if cal.Name != "Test Calendar" {
		t.Errorf("expected name 'Test Calendar', got %q", cal.Name)
	}
	if cal.ID == 0 {
		t.Error("expected calendar ID to be set")
	}
}

func TestCalendarHandler_Create_MissingName(t *testing.T) {
	h, _, user := setupCalendarHandler(t)

	body := fmt.Sprintf(`{"data":%q}`, integrationICalData)
	req := httptest.NewRequest(http.MethodPost, "/api/calendars", bytes.NewReader([]byte(body)))
	req = withUser(req, user)
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestCalendarHandler_Update_Success(t *testing.T) {
	h, calRepo, user := setupCalendarHandler(t)
	ctx := context.Background()

	cal := &model.Calendar{UserID: user.ID, Name: "Before", Data: integrationICalData}
	_ = calRepo.Create(ctx, cal)

	body := `{"name":"After"}`
	req := httptest.NewRequest(http.MethodPut, "/api/calendars/1", bytes.NewReader([]byte(body)))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", cal.ID))
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestCalendarHandler_Update_Forbidden(t *testing.T) {
	h, calRepo, user := setupCalendarHandler(t)
	ctx := context.Background()

	// Create calendar as another user (simulate no access).
	pool := testutil.SetupTestDB(t)
	userRepo := repository.NewUserRepository(pool)
	otherUser := &model.User{Email: "calother@example.com", PasswordHash: "$2a$10$hash", Name: "Other"}
	if err := userRepo.Create(ctx, otherUser); err != nil {
		t.Fatalf("create other user: %v", err)
	}

	cal := &model.Calendar{UserID: otherUser.ID, Name: "Other Cal", Data: integrationICalData}
	_ = calRepo.Create(ctx, cal)

	body := `{"name":"Hacked"}`
	req := httptest.NewRequest(http.MethodPut, "/api/calendars/1", bytes.NewReader([]byte(body)))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", cal.ID))
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestCalendarHandler_Delete_Success(t *testing.T) {
	h, calRepo, user := setupCalendarHandler(t)
	ctx := context.Background()

	cal := &model.Calendar{UserID: user.ID, Name: "Delete Me", Data: integrationICalData}
	_ = calRepo.Create(ctx, cal)

	req := httptest.NewRequest(http.MethodDelete, "/api/calendars/1", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", cal.ID))
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

func TestCalendarHandler_Check_Success(t *testing.T) {
	h, calRepo, user := setupCalendarHandler(t)
	ctx := context.Background()

	cal := &model.Calendar{UserID: user.ID, Name: "Check Cal", Data: integrationICalData}
	_ = calRepo.Create(ctx, cal)

	req := httptest.NewRequest(http.MethodGet, "/api/calendars/1/check", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", cal.ID))
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.Check(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var result map[string]interface{}
	_ = json.NewDecoder(rr.Body).Decode(&result)
	if _, ok := result["active"]; !ok {
		t.Error("expected 'active' field in response")
	}
	if _, ok := result["checkedAt"]; !ok {
		t.Error("expected 'checkedAt' field in response")
	}
}

func TestCalendarHandler_Create_MissingData(t *testing.T) {
	h, _, user := setupCalendarHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/calendars", bytes.NewReader([]byte(`{"name":"No Data"}`)))
	req = withUser(req, user)
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing data, got %d", rr.Code)
	}
}

func TestCalendarHandler_Create_InvalidICalData(t *testing.T) {
	h, _, user := setupCalendarHandler(t)

	body := `{"name":"Bad Cal","data":"this is not valid ical"}`
	req := httptest.NewRequest(http.MethodPost, "/api/calendars", bytes.NewReader([]byte(body)))
	req = withUser(req, user)
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid iCal, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestCalendarHandler_Delete_InvalidID(t *testing.T) {
	h, _, user := setupCalendarHandler(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/calendars/bad", nil)
	req = withUser(req, user)
	req = withChiParam(req, "id", "bad")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestCalendarHandler_Delete_Forbidden(t *testing.T) {
	h, calRepo, user := setupCalendarHandler(t)
	ctx := context.Background()

	pool := testutil.SetupTestDB(t)
	userRepo := repository.NewUserRepository(pool)
	otherUser := &model.User{Email: "caldelete@example.com", PasswordHash: "$2a$10$hash", Name: "Other"}
	_ = userRepo.Create(ctx, otherUser)

	cal := &model.Calendar{UserID: otherUser.ID, Name: "Other Cal", Data: integrationICalData}
	_ = calRepo.Create(ctx, cal)

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/calendars/%d", cal.ID), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", cal.ID))
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestCalendarHandler_Update_InvalidID(t *testing.T) {
	h, _, user := setupCalendarHandler(t)

	req := httptest.NewRequest(http.MethodPut, "/api/calendars/notanumber", bytes.NewReader([]byte(`{"name":"X"}`)))
	req = withUser(req, user)
	req = withChiParam(req, "id", "notanumber")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestCalendarHandler_Check_InvalidID(t *testing.T) {
	h, _, user := setupCalendarHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/calendars/bad/check", nil)
	req = withUser(req, user)
	req = withChiParam(req, "id", "bad")
	rr := httptest.NewRecorder()
	h.Check(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestCalendarHandler_Check_Forbidden(t *testing.T) {
	h, calRepo, user := setupCalendarHandler(t)
	ctx := context.Background()

	pool := testutil.SetupTestDB(t)
	userRepo := repository.NewUserRepository(pool)
	otherUser := &model.User{Email: "calcheckother@example.com", PasswordHash: "$2a$10$hash", Name: "Other"}
	_ = userRepo.Create(ctx, otherUser)

	cal := &model.Calendar{UserID: otherUser.ID, Name: "Other Cal", Data: integrationICalData}
	_ = calRepo.Create(ctx, cal)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/calendars/%d/check", cal.ID), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fmt.Sprintf("%d", cal.ID))
	req = req.WithContext(context.WithValue(api.ContextWithUser(req.Context(), user), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.Check(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

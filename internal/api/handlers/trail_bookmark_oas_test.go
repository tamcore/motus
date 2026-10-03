package handlers_test

// Unit tests for the trail bookmark handlers using mock repositories (no
// database). Access-denied cases respond with NotFound so the existence of
// other users' bookmarks and devices is not leaked. Storage failures are
// returned as handler errors, which ogen turns into HTTP 500.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
)

// mockTrailBookmarkRepo is a test double for repository.TrailBookmarkRepo.
type mockTrailBookmarkRepo struct {
	bookmarks map[int64]*model.TrailBookmark
	nextID    int64

	listFn func(ctx context.Context, user *model.User, deviceID *int64) ([]*model.TrailBookmark, error)

	// Simulated storage failures.
	getErr, createErr, updateErr, deleteErr error

	created *model.TrailBookmark
	updated *model.TrailBookmark
	deleted []int64
}

var _ repository.TrailBookmarkRepo = (*mockTrailBookmarkRepo)(nil)

// errDB simulates an unexpected database failure (connection lost, ...).
var errDB = errors.New("connection refused: secret-db-host:5432")

func newMockTrailBookmarkRepo(bookmarks ...*model.TrailBookmark) *mockTrailBookmarkRepo {
	m := &mockTrailBookmarkRepo{bookmarks: map[int64]*model.TrailBookmark{}, nextID: 100}
	for _, b := range bookmarks {
		m.bookmarks[b.ID] = b
	}
	return m
}

func (m *mockTrailBookmarkRepo) Create(_ context.Context, b *model.TrailBookmark) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.nextID++
	b.ID = m.nextID
	b.CreatedAt = time.Now()
	b.UpdatedAt = b.CreatedAt
	cp := *b
	m.bookmarks[b.ID] = &cp
	m.created = &cp
	return nil
}

func (m *mockTrailBookmarkRepo) GetByID(_ context.Context, id int64) (*model.TrailBookmark, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	b, ok := m.bookmarks[id]
	if !ok {
		return nil, fmt.Errorf("get trail bookmark by id: %w", repository.ErrTrailBookmarkNotFound)
	}
	cp := *b
	return &cp, nil
}

func (m *mockTrailBookmarkRepo) ListForUser(ctx context.Context, user *model.User, deviceID *int64) ([]*model.TrailBookmark, error) {
	if m.listFn != nil {
		return m.listFn(ctx, user, deviceID)
	}
	var out []*model.TrailBookmark
	for _, b := range m.bookmarks {
		if b.UserID == user.ID && (deviceID == nil || b.DeviceID == *deviceID) {
			out = append(out, b)
		}
	}
	return out, nil
}

func (m *mockTrailBookmarkRepo) Update(_ context.Context, b *model.TrailBookmark) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	cp := *b
	m.bookmarks[b.ID] = &cp
	m.updated = &cp
	return nil
}

func (m *mockTrailBookmarkRepo) Delete(_ context.Context, id int64) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.bookmarks, id)
	m.deleted = append(m.deleted, id)
	return nil
}

// --- helpers ---

var (
	bmFrom = time.Date(2026, 6, 6, 8, 0, 0, 0, time.UTC)
	bmTo   = time.Date(2026, 6, 6, 16, 30, 0, 0, time.UTC)
)

// bookmarkDevices returns a device repo granting access to the given device IDs.
func bookmarkDevices(accessible ...int64) *mockDeviceRepo {
	set := map[int64]bool{}
	for _, id := range accessible {
		set[id] = true
	}
	return &mockDeviceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, deviceID int64) bool { return set[deviceID] },
		getByIDFn: func(_ context.Context, id int64) (*model.Device, error) {
			return &model.Device{ID: id, Name: "Backpack"}, nil
		},
	}
}

func newBookmarkTestHandler(repo repository.TrailBookmarkRepo, devices repository.DeviceRepo) *handlers.Handler {
	return handlers.NewHandler(handlers.HandlerConfig{
		TrailBookmarks: repo,
		Devices:        devices,
		AuditLogger:    audit.NewLogger(nil),
	})
}

func bookmarkUserCtx(id int64) context.Context {
	return api.ContextWithUser(context.Background(), &model.User{ID: id, Email: "hiker@example.com", Role: model.RoleUser})
}

func bookmarkAdminCtx() context.Context {
	return api.ContextWithUser(context.Background(), &model.User{ID: 1, Email: "admin@example.com", Role: model.RoleAdmin})
}

func validBookmarkInput() *oas.TrailBookmarkInput {
	return &oas.TrailBookmarkInput{
		DeviceId:    5,
		Name:        "Zugspitze",
		Description: oas.NewOptString("Via Höllental"),
		From:        bmFrom,
		To:          bmTo,
	}
}

func existingBookmark(id, userID, deviceID int64) *model.TrailBookmark {
	return &model.TrailBookmark{
		ID: id, UserID: userID, DeviceID: deviceID, DeviceName: "Backpack",
		Name: "Old", Description: "old desc", From: bmFrom, To: bmTo,
	}
}

// assertInternalError checks that a storage failure surfaces as a handler
// error (HTTP 500) with a safe message instead of a client-error response.
func assertInternalError(t *testing.T, res any, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected a handler error (500), got response %T", res)
	}
	if res != nil {
		t.Errorf("expected nil response alongside the error, got %T", res)
	}
	if strings.Contains(err.Error(), "secret-db-host") {
		t.Errorf("error leaks storage details: %q", err.Error())
	}
}

// --- List ---

func TestListTrailBookmarks_Unauthorized(t *testing.T) {
	h := newBookmarkTestHandler(newMockTrailBookmarkRepo(), bookmarkDevices())
	res, err := h.ListTrailBookmarks(context.Background(), oas.ListTrailBookmarksParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.Error); !ok {
		t.Fatalf("expected *oas.Error, got %T", res)
	}
}

func TestListTrailBookmarks_ReturnsOwnAndPassesDeviceFilter(t *testing.T) {
	repo := newMockTrailBookmarkRepo()
	var gotUser *model.User
	var gotDevice *int64
	repo.listFn = func(_ context.Context, user *model.User, deviceID *int64) ([]*model.TrailBookmark, error) {
		gotUser, gotDevice = user, deviceID
		return []*model.TrailBookmark{existingBookmark(1, user.ID, 5)}, nil
	}
	h := newBookmarkTestHandler(repo, bookmarkDevices(5))

	res, err := h.ListTrailBookmarks(bookmarkUserCtx(7), oas.ListTrailBookmarksParams{DeviceId: oas.NewOptInt64(5)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	list, ok := res.(*oas.ListTrailBookmarksOKApplicationJSON)
	if !ok {
		t.Fatalf("expected list, got %T", res)
	}
	if len(*list) != 1 || (*list)[0].ID != 1 || (*list)[0].DeviceName.Value != "Backpack" {
		t.Errorf("unexpected list: %+v", *list)
	}
	if !(*list)[0].From.Equal(bmFrom) || !(*list)[0].To.Equal(bmTo) {
		t.Errorf("unexpected range in response")
	}
	if gotUser == nil || gotUser.ID != 7 {
		t.Errorf("expected list for user 7, got %+v", gotUser)
	}
	if gotDevice == nil || *gotDevice != 5 {
		t.Errorf("expected device filter 5, got %v", gotDevice)
	}
}

func TestListTrailBookmarks_EmptyIsArray(t *testing.T) {
	h := newBookmarkTestHandler(newMockTrailBookmarkRepo(), bookmarkDevices())
	res, err := h.ListTrailBookmarks(bookmarkUserCtx(7), oas.ListTrailBookmarksParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	list, ok := res.(*oas.ListTrailBookmarksOKApplicationJSON)
	if !ok || list == nil || *list == nil {
		t.Fatalf("expected non-nil empty list, got %T %v", res, list)
	}
}

func TestListTrailBookmarks_StorageFailureIs500(t *testing.T) {
	repo := newMockTrailBookmarkRepo()
	repo.listFn = func(context.Context, *model.User, *int64) ([]*model.TrailBookmark, error) {
		return nil, errDB
	}
	h := newBookmarkTestHandler(repo, bookmarkDevices())
	res, err := h.ListTrailBookmarks(bookmarkUserCtx(7), oas.ListTrailBookmarksParams{})
	// An *oas.Error response would be encoded as 401 and look like an
	// expired session to the frontend.
	assertInternalError(t, res, err)
}

// --- Create ---

func TestCreateTrailBookmark_Success(t *testing.T) {
	repo := newMockTrailBookmarkRepo()
	h := newBookmarkTestHandler(repo, bookmarkDevices(5))

	in := validBookmarkInput()
	in.Name = "  Zugspitze  "
	res, err := h.CreateTrailBookmark(bookmarkUserCtx(7), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, ok := res.(*oas.TrailBookmark)
	if !ok {
		t.Fatalf("expected *oas.TrailBookmark, got %T", res)
	}
	if b.ID == 0 || b.Name != "Zugspitze" || b.Description != "Via Höllental" {
		t.Errorf("unexpected response: %+v", b)
	}
	if b.DeviceName.Value != "Backpack" {
		t.Errorf("expected device name in response, got %q", b.DeviceName.Value)
	}
	if repo.created == nil || repo.created.UserID != 7 || repo.created.DeviceID != 5 {
		t.Fatalf("expected bookmark owned by user 7 on device 5, got %+v", repo.created)
	}
	if !repo.created.From.Equal(bmFrom) || !repo.created.To.Equal(bmTo) {
		t.Errorf("unexpected stored range")
	}
}

func TestCreateTrailBookmark_Unauthorized(t *testing.T) {
	h := newBookmarkTestHandler(newMockTrailBookmarkRepo(), bookmarkDevices(5))
	res, err := h.CreateTrailBookmark(context.Background(), validBookmarkInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.CreateTrailBookmarkUnauthorized); !ok {
		t.Fatalf("expected Unauthorized, got %T", res)
	}
}

func TestCreateTrailBookmark_Validation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(in *oas.TrailBookmarkInput)
		want   string
	}{
		{"missing name", func(in *oas.TrailBookmarkInput) { in.Name = "" }, "name is required"},
		{"blank name", func(in *oas.TrailBookmarkInput) { in.Name = "   " }, "name is required"},
		{"name too long", func(in *oas.TrailBookmarkInput) { in.Name = strings.Repeat("x", 201) }, "name"},
		{"name too long (multibyte)", func(in *oas.TrailBookmarkInput) { in.Name = strings.Repeat("ö", 201) }, "name"},
		{"html in name", func(in *oas.TrailBookmarkInput) { in.Name = "<script>" }, "name"},
		{"html in description", func(in *oas.TrailBookmarkInput) { in.Description = oas.NewOptString("<b>") }, "description"},
		{"description too long", func(in *oas.TrailBookmarkInput) {
			in.Description = oas.NewOptString(strings.Repeat("x", 2001))
		}, "description"},
		{"missing from", func(in *oas.TrailBookmarkInput) { in.From = time.Time{} }, "from and to are required"},
		{"missing to", func(in *oas.TrailBookmarkInput) { in.To = time.Time{} }, "from and to are required"},
		{"to before from", func(in *oas.TrailBookmarkInput) { in.From, in.To = bmTo, bmFrom }, "to must be after from"},
		{"empty range", func(in *oas.TrailBookmarkInput) { in.To = in.From }, "to must be after from"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockTrailBookmarkRepo()
			h := newBookmarkTestHandler(repo, bookmarkDevices(5))
			in := validBookmarkInput()
			tc.mutate(in)
			res, err := h.CreateTrailBookmark(bookmarkUserCtx(7), in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			bad, ok := res.(*oas.CreateTrailBookmarkBadRequest)
			if !ok {
				t.Fatalf("expected BadRequest, got %T", res)
			}
			if !strings.Contains(bad.Error, tc.want) {
				t.Errorf("error %q does not contain %q", bad.Error, tc.want)
			}
			if repo.created != nil {
				t.Error("bookmark must not be created on validation failure")
			}
		})
	}
}

// Limits count characters (Unicode code points), the same unit the web UI
// uses, so multibyte names are not cut short by a byte limit.
func TestCreateTrailBookmark_LimitsCountCharacters(t *testing.T) {
	repo := newMockTrailBookmarkRepo()
	h := newBookmarkTestHandler(repo, bookmarkDevices(5))
	in := validBookmarkInput()
	in.Name = strings.Repeat("ö", 200)                           // 400 bytes
	in.Description = oas.NewOptString(strings.Repeat("🥾", 2000)) // 8000 bytes
	res, err := h.CreateTrailBookmark(bookmarkUserCtx(7), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.TrailBookmark); !ok {
		t.Fatalf("expected success at the character limit, got %T %+v", res, res)
	}
}

func TestCreateTrailBookmark_DeviceWithoutAccess(t *testing.T) {
	repo := newMockTrailBookmarkRepo()
	h := newBookmarkTestHandler(repo, bookmarkDevices( /* none */ ))
	res, err := h.CreateTrailBookmark(bookmarkUserCtx(7), validBookmarkInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.CreateTrailBookmarkNotFound); !ok {
		t.Fatalf("expected NotFound, got %T", res)
	}
	if repo.created != nil {
		t.Error("bookmark must not be created for an inaccessible device")
	}
}

func TestCreateTrailBookmark_StorageFailureIs500(t *testing.T) {
	repo := newMockTrailBookmarkRepo()
	repo.createErr = errDB
	h := newBookmarkTestHandler(repo, bookmarkDevices(5))
	res, err := h.CreateTrailBookmark(bookmarkUserCtx(7), validBookmarkInput())
	assertInternalError(t, res, err)
}

// --- Update ---

func TestUpdateTrailBookmark_Success(t *testing.T) {
	repo := newMockTrailBookmarkRepo(existingBookmark(1, 7, 5))
	h := newBookmarkTestHandler(repo, bookmarkDevices(5, 6))

	in := validBookmarkInput()
	in.DeviceId = 6
	in.Name = "New name"
	in.Description = oas.OptString{}
	res, err := h.UpdateTrailBookmark(bookmarkUserCtx(7), in, oas.UpdateTrailBookmarkParams{ID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, ok := res.(*oas.TrailBookmark)
	if !ok {
		t.Fatalf("expected *oas.TrailBookmark, got %T", res)
	}
	if b.Name != "New name" || b.Description != "" || b.DeviceId != 6 {
		t.Errorf("unexpected response: %+v", b)
	}
	if repo.updated == nil || repo.updated.UserID != 7 {
		t.Fatalf("expected owner to be preserved, got %+v", repo.updated)
	}
}

func TestUpdateTrailBookmark_PreservesExactTimestamps(t *testing.T) {
	repo := newMockTrailBookmarkRepo(existingBookmark(1, 7, 5))
	h := newBookmarkTestHandler(repo, bookmarkDevices(5))
	in := validBookmarkInput()
	in.From = time.Date(2026, 6, 6, 8, 0, 12, 345_000_000, time.UTC)
	in.To = time.Date(2026, 6, 6, 16, 30, 59, 999_000_000, time.UTC)
	if _, err := h.UpdateTrailBookmark(bookmarkUserCtx(7), in, oas.UpdateTrailBookmarkParams{ID: 1}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repo.updated.From.Equal(in.From) || !repo.updated.To.Equal(in.To) {
		t.Errorf("stored range %v – %v, want %v – %v", repo.updated.From, repo.updated.To, in.From, in.To)
	}
}

func TestUpdateTrailBookmark_NotFound(t *testing.T) {
	repo := newMockTrailBookmarkRepo()
	h := newBookmarkTestHandler(repo, bookmarkDevices(5))
	res, err := h.UpdateTrailBookmark(bookmarkUserCtx(7), validBookmarkInput(), oas.UpdateTrailBookmarkParams{ID: 42})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.UpdateTrailBookmarkNotFound); !ok {
		t.Fatalf("expected NotFound, got %T", res)
	}
}

func TestUpdateTrailBookmark_OtherUser(t *testing.T) {
	repo := newMockTrailBookmarkRepo(existingBookmark(1, 7, 5))
	h := newBookmarkTestHandler(repo, bookmarkDevices(5))
	res, err := h.UpdateTrailBookmark(bookmarkUserCtx(8), validBookmarkInput(), oas.UpdateTrailBookmarkParams{ID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.UpdateTrailBookmarkNotFound); !ok {
		t.Fatalf("expected NotFound, got %T", res)
	}
	if repo.updated != nil {
		t.Error("bookmark must not be updated by another user")
	}
}

func TestUpdateTrailBookmark_DeviceAccessRevoked(t *testing.T) {
	repo := newMockTrailBookmarkRepo(existingBookmark(1, 7, 5))
	h := newBookmarkTestHandler(repo, bookmarkDevices())
	res, err := h.UpdateTrailBookmark(bookmarkUserCtx(7), validBookmarkInput(), oas.UpdateTrailBookmarkParams{ID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.UpdateTrailBookmarkNotFound); !ok {
		t.Fatalf("expected NotFound, got %T", res)
	}
	if repo.updated != nil {
		t.Error("bookmark of an inaccessible device must not be updated")
	}
}

func TestUpdateTrailBookmark_MoveToInaccessibleDevice(t *testing.T) {
	repo := newMockTrailBookmarkRepo(existingBookmark(1, 7, 5))
	h := newBookmarkTestHandler(repo, bookmarkDevices(5))
	in := validBookmarkInput()
	in.DeviceId = 99
	res, err := h.UpdateTrailBookmark(bookmarkUserCtx(7), in, oas.UpdateTrailBookmarkParams{ID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.UpdateTrailBookmarkNotFound); !ok {
		t.Fatalf("expected NotFound, got %T", res)
	}
	if repo.updated != nil {
		t.Error("bookmark must not be moved to an inaccessible device")
	}
}

func TestUpdateTrailBookmark_Validation(t *testing.T) {
	repo := newMockTrailBookmarkRepo(existingBookmark(1, 7, 5))
	h := newBookmarkTestHandler(repo, bookmarkDevices(5))
	in := validBookmarkInput()
	in.From, in.To = bmTo, bmFrom
	res, err := h.UpdateTrailBookmark(bookmarkUserCtx(7), in, oas.UpdateTrailBookmarkParams{ID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.UpdateTrailBookmarkBadRequest); !ok {
		t.Fatalf("expected BadRequest, got %T", res)
	}
}

func TestUpdateTrailBookmark_AdminCanEditOthers(t *testing.T) {
	repo := newMockTrailBookmarkRepo(existingBookmark(1, 7, 5))
	h := newBookmarkTestHandler(repo, bookmarkDevices(5))
	res, err := h.UpdateTrailBookmark(bookmarkAdminCtx(), validBookmarkInput(), oas.UpdateTrailBookmarkParams{ID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.TrailBookmark); !ok {
		t.Fatalf("expected success, got %T", res)
	}
	if repo.updated.UserID != 7 {
		t.Errorf("admin edit must keep the original owner, got %d", repo.updated.UserID)
	}
}

func TestUpdateTrailBookmark_LoadFailureIs500(t *testing.T) {
	repo := newMockTrailBookmarkRepo(existingBookmark(1, 7, 5))
	repo.getErr = errDB
	h := newBookmarkTestHandler(repo, bookmarkDevices(5))
	res, err := h.UpdateTrailBookmark(bookmarkUserCtx(7), validBookmarkInput(), oas.UpdateTrailBookmarkParams{ID: 1})
	assertInternalError(t, res, err)
}

func TestUpdateTrailBookmark_StorageFailureIs500(t *testing.T) {
	repo := newMockTrailBookmarkRepo(existingBookmark(1, 7, 5))
	repo.updateErr = errDB
	h := newBookmarkTestHandler(repo, bookmarkDevices(5))
	res, err := h.UpdateTrailBookmark(bookmarkUserCtx(7), validBookmarkInput(), oas.UpdateTrailBookmarkParams{ID: 1})
	assertInternalError(t, res, err)
}

// --- Delete ---

func TestDeleteTrailBookmark_Owner(t *testing.T) {
	repo := newMockTrailBookmarkRepo(existingBookmark(1, 7, 5))
	// Deleting is allowed even after device access was revoked (cleanup).
	h := newBookmarkTestHandler(repo, bookmarkDevices())
	res, err := h.DeleteTrailBookmark(bookmarkUserCtx(7), oas.DeleteTrailBookmarkParams{ID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.DeleteTrailBookmarkNoContent); !ok {
		t.Fatalf("expected NoContent, got %T", res)
	}
	if len(repo.deleted) != 1 || repo.deleted[0] != 1 {
		t.Errorf("expected bookmark 1 deleted, got %v", repo.deleted)
	}
}

func TestDeleteTrailBookmark_AdminCanDeleteOthers(t *testing.T) {
	repo := newMockTrailBookmarkRepo(existingBookmark(1, 7, 5))
	h := newBookmarkTestHandler(repo, bookmarkDevices(5))
	res, err := h.DeleteTrailBookmark(bookmarkAdminCtx(), oas.DeleteTrailBookmarkParams{ID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.DeleteTrailBookmarkNoContent); !ok {
		t.Fatalf("expected NoContent, got %T", res)
	}
}

func TestDeleteTrailBookmark_OtherUser(t *testing.T) {
	repo := newMockTrailBookmarkRepo(existingBookmark(1, 7, 5))
	h := newBookmarkTestHandler(repo, bookmarkDevices(5))
	res, err := h.DeleteTrailBookmark(bookmarkUserCtx(8), oas.DeleteTrailBookmarkParams{ID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.DeleteTrailBookmarkNotFound); !ok {
		t.Fatalf("expected NotFound, got %T", res)
	}
	if len(repo.deleted) != 0 {
		t.Error("bookmark must not be deleted by another user")
	}
}

func TestDeleteTrailBookmark_NotFound(t *testing.T) {
	h := newBookmarkTestHandler(newMockTrailBookmarkRepo(), bookmarkDevices(5))
	res, err := h.DeleteTrailBookmark(bookmarkUserCtx(7), oas.DeleteTrailBookmarkParams{ID: 42})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.DeleteTrailBookmarkNotFound); !ok {
		t.Fatalf("expected NotFound, got %T", res)
	}
}

func TestDeleteTrailBookmark_Unauthorized(t *testing.T) {
	h := newBookmarkTestHandler(newMockTrailBookmarkRepo(existingBookmark(1, 7, 5)), bookmarkDevices(5))
	res, err := h.DeleteTrailBookmark(context.Background(), oas.DeleteTrailBookmarkParams{ID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := res.(*oas.DeleteTrailBookmarkUnauthorized); !ok {
		t.Fatalf("expected Unauthorized, got %T", res)
	}
}

func TestDeleteTrailBookmark_LoadFailureIs500(t *testing.T) {
	repo := newMockTrailBookmarkRepo(existingBookmark(1, 7, 5))
	repo.getErr = errDB
	h := newBookmarkTestHandler(repo, bookmarkDevices(5))
	res, err := h.DeleteTrailBookmark(bookmarkUserCtx(7), oas.DeleteTrailBookmarkParams{ID: 1})
	assertInternalError(t, res, err)
}

func TestDeleteTrailBookmark_StorageFailureIs500(t *testing.T) {
	repo := newMockTrailBookmarkRepo(existingBookmark(1, 7, 5))
	repo.deleteErr = errDB
	h := newBookmarkTestHandler(repo, bookmarkDevices(5))
	res, err := h.DeleteTrailBookmark(bookmarkUserCtx(7), oas.DeleteTrailBookmarkParams{ID: 1})
	assertInternalError(t, res, err)
	if len(repo.deleted) != 0 {
		t.Error("nothing should be recorded as deleted")
	}
}

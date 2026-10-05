package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/model"
)

type stubTrailBookmarkRepo struct {
	gotUser   *model.User
	gotDevice *int64
	result    []*model.TrailBookmark
}

func (s *stubTrailBookmarkRepo) Create(context.Context, *model.TrailBookmark) error { return nil }
func (s *stubTrailBookmarkRepo) GetByID(context.Context, int64) (*model.TrailBookmark, error) {
	return nil, nil
}
func (s *stubTrailBookmarkRepo) ListForUser(_ context.Context, user *model.User, deviceID *int64) ([]*model.TrailBookmark, error) {
	s.gotUser, s.gotDevice = user, deviceID
	return s.result, nil
}
func (s *stubTrailBookmarkRepo) Update(context.Context, *model.TrailBookmark) error { return nil }
func (s *stubTrailBookmarkRepo) Delete(context.Context, int64) error                { return nil }

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if res == nil || len(res.Content) == 0 {
		t.Fatal("empty tool result")
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", res.Content[0])
	}
	return tc.Text
}

func TestListTrailBookmarks_RequiresUser(t *testing.T) {
	res, err := handleListTrailBookmarks(context.Background(), toolRequest(nil), Deps{TrailBookmarks: &stubTrailBookmarkRepo{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected tool error without user")
	}
}

func TestListTrailBookmarks_ReturnsUserBookmarks(t *testing.T) {
	from := time.Date(2026, 6, 6, 8, 0, 0, 0, time.UTC)
	repo := &stubTrailBookmarkRepo{result: []*model.TrailBookmark{{
		ID: 3, DeviceID: 5, DeviceName: "Backpack", Name: "Zugspitze", Description: "Höllental",
		From: from, To: from.Add(8 * time.Hour),
	}}}
	ctx := api.ContextWithUser(context.Background(), &model.User{ID: 7, Role: model.RoleUser})

	res, err := handleListTrailBookmarks(ctx, toolRequest(map[string]any{"device_id": "5"}), Deps{TrailBookmarks: repo})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", resultText(t, res))
	}
	if repo.gotUser == nil || repo.gotUser.ID != 7 {
		t.Errorf("expected lookup for user 7")
	}
	if repo.gotDevice == nil || *repo.gotDevice != 5 {
		t.Errorf("expected device filter 5, got %v", repo.gotDevice)
	}

	var out []map[string]any
	if err := json.Unmarshal([]byte(resultText(t, res)), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 bookmark, got %d", len(out))
	}
	b := out[0]
	if b["name"] != "Zugspitze" || b["deviceName"] != "Backpack" || b["description"] != "Höllental" {
		t.Errorf("unexpected entry: %v", b)
	}
	if b["from"] != "2026-06-06T08:00:00Z" || b["to"] != "2026-06-06T16:00:00Z" {
		t.Errorf("unexpected range: %v – %v", b["from"], b["to"])
	}
}

// Fractional seconds must survive so the range can be passed on to the
// distance/event tools without dropping the end of the interval.
func TestListTrailBookmarks_KeepsFractionalSeconds(t *testing.T) {
	from := time.Date(2026, 6, 6, 8, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 6, 16, 30, 59, 999_000_000, time.UTC)
	repo := &stubTrailBookmarkRepo{result: []*model.TrailBookmark{{ID: 3, DeviceID: 5, Name: "Hike", From: from, To: to}}}
	ctx := api.ContextWithUser(context.Background(), &model.User{ID: 7, Role: model.RoleUser})

	res, err := handleListTrailBookmarks(ctx, toolRequest(nil), Deps{TrailBookmarks: repo})
	if err != nil || res.IsError {
		t.Fatalf("unexpected error: %v %v", err, res)
	}
	var out []map[string]any
	if err := json.Unmarshal([]byte(resultText(t, res)), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(out) != 1 || out[0]["to"] != "2026-06-06T16:30:59.999Z" {
		t.Errorf("expected to=2026-06-06T16:30:59.999Z, got %v", out)
	}
}

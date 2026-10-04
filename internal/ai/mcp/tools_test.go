package mcp

import (
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/tamcore/motus/internal/model"
)

func toolRequest(args map[string]any) mcp.CallToolRequest {
	var req mcp.CallToolRequest
	req.Params.Arguments = args
	return req
}

func TestRequireID(t *testing.T) {
	for _, tc := range []struct {
		args    map[string]any
		want    int64
		wantErr string
	}{
		{map[string]any{"id": "42"}, 42, ""},
		{map[string]any{"id": float64(7)}, 7, ""},
		{map[string]any{"id": "abc"}, 0, "invalid id"},
		{map[string]any{}, 0, "id is required"},
		{map[string]any{"id": nil}, 0, "id is required"},
	} {
		got, err := requireID(toolRequest(tc.args), "id")
		if errString(err) != tc.wantErr || got != tc.want {
			t.Errorf("requireID(%v) = %d, %v; want %d, %q", tc.args, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestResolveCoords(t *testing.T) {
	for _, tc := range []struct {
		args     map[string]any
		lat, lon float64
		wantErr  bool
	}{
		{map[string]any{"latitude": 52.5, "longitude": 13.4}, 52.5, 13.4, false},
		{map[string]any{"latitude": "52.5", "longitude": "13.4"}, 52.5, 13.4, false},
		{map[string]any{"latitude": 52.5}, 0, 0, true},
		{map[string]any{"latitude": "north", "longitude": 13.4}, 0, 0, true},
	} {
		lat, lon, err := resolveCoords(t.Context(), toolRequest(tc.args), Deps{})
		if (err != nil) != tc.wantErr || lat != tc.lat || lon != tc.lon {
			t.Errorf("resolveCoords(%v) = %v, %v, %v", tc.args, lat, lon, err)
		}
	}
}

func TestParseRange(t *testing.T) {
	from, to, err := parseRange(toolRequest(map[string]any{"from": "2026-01-01T00:00:00Z", "to": "2026-02-01T00:00:00Z"}))
	if err != nil {
		t.Fatalf("parseRange: %v", err)
	}
	if !from.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) || !to.Equal(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("parseRange = %v, %v", from, to)
	}
	for _, tc := range []struct {
		args    map[string]any
		wantErr string
	}{
		{map[string]any{"to": "2026-02-01T00:00:00Z"}, "from is required"},
		{map[string]any{"from": "2026-01-01T00:00:00Z"}, "to is required"},
		{map[string]any{"from": "bad", "to": "2026-02-01T00:00:00Z"}, "invalid from"},
		{map[string]any{"from": "2026-01-01T00:00:00Z", "to": "bad"}, "invalid to"},
	} {
		if _, _, err := parseRange(toolRequest(tc.args)); !strings.HasPrefix(errString(err), tc.wantErr) {
			t.Errorf("parseRange(%v) error = %v, want prefix %q", tc.args, err, tc.wantErr)
		}
	}
}

func TestResolveDeviceIDNullFallsThrough(t *testing.T) {
	id, err := resolveDeviceID(t.Context(), toolRequest(map[string]any{"device_id": nil}), &model.User{}, Deps{})
	if id != 0 || err != nil {
		t.Errorf("resolveDeviceID(null) = %d, %v; want 0, nil", id, err)
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

package mcp

import (
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
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
		wantErr bool
	}{
		{map[string]any{"id": "42"}, 42, false},
		{map[string]any{"id": float64(7)}, 7, false},
		{map[string]any{"id": "abc"}, 0, true},
		{map[string]any{}, 0, true},
	} {
		got, err := requireID(toolRequest(tc.args), "id")
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("requireID(%v) = %d, %v; want %d, err=%v", tc.args, got, err, tc.want, tc.wantErr)
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
	for _, args := range []map[string]any{
		{"to": "2026-02-01T00:00:00Z"},
		{"from": "2026-01-01T00:00:00Z"},
		{"from": "bad", "to": "2026-02-01T00:00:00Z"},
		{"from": "2026-01-01T00:00:00Z", "to": "bad"},
	} {
		if _, _, err := parseRange(toolRequest(args)); err == nil {
			t.Errorf("parseRange(%v) succeeded, want error", args)
		}
	}
}

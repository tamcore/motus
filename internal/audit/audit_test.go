package audit

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"testing"
)

func TestLogWithNilLogger(t *testing.T) {
	// A nil logger should not panic.
	var l *Logger
	l.Log(context.Background(), nil, ActionSessionLogin, ResourceSession, nil, nil, "", "")
}

func TestLogWithNilPool(t *testing.T) {
	// A logger with nil pool should not panic.
	l := NewLogger(nil)
	l.Log(context.Background(), nil, ActionSessionLogin, ResourceSession, nil, nil, "127.0.0.1", "test-agent")
}

func TestLogFromRequestWithNilPool(t *testing.T) {
	l := NewLogger(nil)
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	l.LogFromRequest(req, nil, ActionSessionLogin, ResourceSession, nil, nil)
}

func TestExtractIP(t *testing.T) {
	tests := []struct {
		remoteAddr string
		want       string
	}{
		{"192.168.1.1:1234", "192.168.1.1"},
		{"10.0.0.1:80", "10.0.0.1"},
		{"invalid", "invalid"}, // no port -> return as-is
	}

	for _, tt := range tests {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = tt.remoteAddr
		got := extractIP(req)
		if got != tt.want {
			t.Errorf("extractIP(RemoteAddr=%q) = %q, want %q", tt.remoteAddr, got, tt.want)
		}
	}
}

func TestQueryParams_DefaultLimit(t *testing.T) {
	p := QueryParams{}
	if p.Limit != 0 {
		t.Errorf("expected default limit 0 (will be set to 50 in Query), got %d", p.Limit)
	}
}

func TestConstants(t *testing.T) {
	// Verify action constants are defined.
	actions := []string{
		ActionSessionLogin, ActionSessionLogout, ActionUserCreate, ActionUserUpdate,
		ActionUserDelete, ActionDeviceOnline, ActionDeviceOffline,
		ActionNotifSent, ActionSessionSudo, ActionSessionSudoEnd,
	}
	for _, a := range actions {
		if a == "" {
			t.Error("found empty action constant")
		}
	}

	// Verify resource type constants.
	resources := []string{ResourceUser, ResourceDevice, ResourceNotification, ResourceSession}
	for _, r := range resources {
		if r == "" {
			t.Error("found empty resource type constant")
		}
	}
}

func TestLogger_SetLogger(t *testing.T) {
	l := NewLogger(nil)
	initial := l.logger
	l.SetLogger(nil) // nil should not change the logger
	if l.logger != initial {
		t.Error("SetLogger(nil) should not change the logger")
	}
	custom := slog.Default()
	l.SetLogger(custom)
	if l.logger != custom {
		t.Error("SetLogger(custom) should replace the logger")
	}
}

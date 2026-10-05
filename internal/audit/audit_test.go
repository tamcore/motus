package audit

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestLogWithNilLogger(t *testing.T) {
	// A nil logger should not panic.
	var l *Logger
	l.Log(context.Background(), nil, ActionSessionLogin, ResourceSession, nil, nil)
}

func TestLogWithNilPool(t *testing.T) {
	// A logger with nil pool should not panic.
	l := NewLogger(nil)
	l.Log(context.Background(), nil, ActionSessionLogin, ResourceSession, nil, nil)
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
		got := ExtractIP(req)
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
		ActionUserDelete, ActionSessionSudo, ActionSessionSudoEnd,
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

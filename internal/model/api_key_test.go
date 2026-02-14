package model

import (
	"testing"
	"time"
)

func TestIsValidPermission(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"full", true},
		{"readonly", true},
		{"", false},
		{"admin", false},
		{"write", false},
		{"FULL", false},
		{"READONLY", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsValidPermission(tt.input)
			if got != tt.want {
				t.Errorf("IsValidPermission(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidPermissions(t *testing.T) {
	perms := ValidPermissions()
	if len(perms) != 2 {
		t.Fatalf("expected 2 valid permissions, got %d", len(perms))
	}
	if perms[0] != PermissionFull {
		t.Errorf("expected first permission %q, got %q", PermissionFull, perms[0])
	}
	if perms[1] != PermissionReadonly {
		t.Errorf("expected second permission %q, got %q", PermissionReadonly, perms[1])
	}
}

func TestApiKey_IsReadonly(t *testing.T) {
	tests := []struct {
		permissions string
		want        bool
	}{
		{PermissionReadonly, true},
		{PermissionFull, false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.permissions, func(t *testing.T) {
			key := &ApiKey{Permissions: tt.permissions}
			got := key.IsReadonly()
			if got != tt.want {
				t.Errorf("ApiKey{Permissions: %q}.IsReadonly() = %v, want %v", tt.permissions, got, tt.want)
			}
		})
	}
}

func TestApiKey_IsExpired(t *testing.T) {
	pastTime := time.Now().Add(-1 * time.Hour)
	futureTime := time.Now().Add(1 * time.Hour)

	tests := []struct {
		name      string
		expiresAt *time.Time
		want      bool
	}{
		{"nil expiration (never expires)", nil, false},
		{"expired (past time)", &pastTime, true},
		{"not expired (future time)", &futureTime, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := &ApiKey{ExpiresAt: tt.expiresAt}
			got := key.IsExpired()
			if got != tt.want {
				t.Errorf("ApiKey.IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

package model

import "testing"

func TestIsValidRole(t *testing.T) {
	tests := []struct {
		role string
		want bool
	}{
		{RoleAdmin, true},
		{RoleUser, true},
		{RoleReadonly, true},
		{"superadmin", false},
		{"", false},
		{"Admin", false}, // case-sensitive
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			if got := IsValidRole(tt.role); got != tt.want {
				t.Errorf("IsValidRole(%q) = %v, want %v", tt.role, got, tt.want)
			}
		})
	}
}

func TestValidRoles(t *testing.T) {
	roles := ValidRoles()
	if len(roles) != 3 {
		t.Fatalf("expected 3 roles, got %d", len(roles))
	}

	expected := map[string]bool{
		RoleAdmin:    true,
		RoleUser:     true,
		RoleReadonly: true,
	}
	for _, r := range roles {
		if !expected[r] {
			t.Errorf("unexpected role: %q", r)
		}
	}
}

func TestUser_IsAdmin(t *testing.T) {
	tests := []struct {
		name string
		role string
		want bool
	}{
		{"admin user", RoleAdmin, true},
		{"regular user", RoleUser, false},
		{"readonly user", RoleReadonly, false},
		{"empty role", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &User{Role: tt.role}
			if got := u.IsAdmin(); got != tt.want {
				t.Errorf("User{Role: %q}.IsAdmin() = %v, want %v", tt.role, got, tt.want)
			}
		})
	}
}

func TestRoleConstants(t *testing.T) {
	if RoleAdmin != "admin" {
		t.Errorf("expected 'admin', got %q", RoleAdmin)
	}
	if RoleUser != "user" {
		t.Errorf("expected 'user', got %q", RoleUser)
	}
	if RoleReadonly != "readonly" {
		t.Errorf("expected 'readonly', got %q", RoleReadonly)
	}
}

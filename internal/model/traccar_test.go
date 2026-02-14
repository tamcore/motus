package model

import "testing"

func TestUser_PopulateTraccarFields(t *testing.T) {
	tests := []struct {
		name         string
		role         string
		wantAdmin    bool
		wantReadonly bool
		wantDisabled bool
	}{
		{"admin", RoleAdmin, true, false, false},
		{"user", RoleUser, false, false, false},
		{"readonly", RoleReadonly, false, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &User{Role: tt.role}
			u.PopulateTraccarFields()

			if u.Administrator != tt.wantAdmin {
				t.Errorf("Administrator = %v, want %v", u.Administrator, tt.wantAdmin)
			}
			if u.Readonly != tt.wantReadonly {
				t.Errorf("Readonly = %v, want %v", u.Readonly, tt.wantReadonly)
			}
			if u.Disabled != tt.wantDisabled {
				t.Errorf("Disabled = %v, want %v", u.Disabled, tt.wantDisabled)
			}
		})
	}
}

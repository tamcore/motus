package validation

import (
	"testing"
)

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		wantErr bool
	}{
		{"valid simple", "user@example.com", false},
		{"valid subdomain", "user@sub.example.com", false},
		{"valid plus tag", "user+tag@example.com", false},
		{"valid dots", "first.last@example.com", false},
		{"empty", "", true},
		{"no at sign", "userexample.com", true},
		{"no domain", "user@", true},
		{"no local part", "@example.com", true},
		{"double at", "user@@example.com", true},
		{"spaces", "user @example.com", true},
		{"no tld", "user@example", true},
		{"too long", string(make([]byte, 255)) + "@example.com", true},
		{"special chars in local", "user<>@example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmail(tt.email)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEmail(%q) error = %v, wantErr %v", tt.email, err, tt.wantErr)
			}
		})
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "John Doe", false},
		{"valid with numbers", "Device 1", false},
		{"valid unicode", "Hans Mueller", false},
		{"valid hyphen", "Mary-Jane", false},
		{"valid underscore", "my_device", false},
		{"valid dot", "Dr. Smith", false},
		{"empty", "", true},
		{"too long", string(make([]byte, 256)), true},
		{"only spaces", "   ", true},
		{"contains angle brackets", "name<script>", true},
		{"contains backtick", "name`test", true},
		{"contains null byte", "name\x00test", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateDeviceUniqueID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid IMEI", "123456789012345", false},
		{"valid alphanumeric", "DEVICE000001", false},
		{"valid with hyphen", "device-001", false},
		{"valid with underscore", "device_001", false},
		{"empty", "", true},
		{"too long", string(make([]byte, 129)), true},
		{"spaces", "device 001", true},
		{"special chars", "device@001", true},
		{"angle brackets", "device<001>", true},
		{"semicolon", "device;001", true},
		{"only whitespace", "  ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDeviceUniqueID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDeviceUniqueID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name    string
		pw      string
		wantErr bool
	}{
		{"valid 8 chars", "password", false},
		{"valid long", "a-very-long-and-complex-password-123!", false},
		{"empty", "", true},
		{"too short", "1234567", true},
		{"exactly 8", "12345678", false},
		{"too long", string(make([]byte, 129)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.pw)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePassword(%q) error = %v, wantErr %v", tt.pw, err, tt.wantErr)
			}
		})
	}
}

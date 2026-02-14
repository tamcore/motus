package config

import (
	"strings"
	"testing"
)

func TestValidate_OIDCConfig(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr string
	}{
		{
			name:    "oidc disabled - no validation required",
			modify:  func(c *Config) { c.OIDC.Enabled = false },
			wantErr: "",
		},
		{
			name: "oidc enabled - all required fields set",
			modify: func(c *Config) {
				c.OIDC = OIDCConfig{
					Enabled:      true,
					Issuer:       "https://accounts.example.com",
					ClientID:     "my-client",
					ClientSecret: "my-secret",
					RedirectURL:  "https://app.example.com/api/auth/oidc/callback",
				}
			},
			wantErr: "",
		},
		{
			name: "oidc enabled - missing issuer",
			modify: func(c *Config) {
				c.OIDC = OIDCConfig{
					Enabled:      true,
					ClientID:     "id",
					ClientSecret: "secret",
					RedirectURL:  "https://app.example.com/api/auth/oidc/callback",
				}
			},
			wantErr: "MOTUS_OIDC_ISSUER must be set",
		},
		{
			name: "oidc enabled - missing client id",
			modify: func(c *Config) {
				c.OIDC = OIDCConfig{
					Enabled:      true,
					Issuer:       "https://accounts.example.com",
					ClientSecret: "secret",
					RedirectURL:  "https://app.example.com/api/auth/oidc/callback",
				}
			},
			wantErr: "MOTUS_OIDC_CLIENT_ID must be set",
		},
		{
			name: "oidc enabled - missing client secret",
			modify: func(c *Config) {
				c.OIDC = OIDCConfig{
					Enabled:     true,
					Issuer:      "https://accounts.example.com",
					ClientID:    "id",
					RedirectURL: "https://app.example.com/api/auth/oidc/callback",
				}
			},
			wantErr: "MOTUS_OIDC_CLIENT_SECRET must be set",
		},
		{
			name: "oidc enabled - missing redirect url",
			modify: func(c *Config) {
				c.OIDC = OIDCConfig{
					Enabled:      true,
					Issuer:       "https://accounts.example.com",
					ClientID:     "id",
					ClientSecret: "secret",
				}
			},
			wantErr: "MOTUS_OIDC_REDIRECT_URL must be set",
		},
		{
			name: "oidc enabled - invalid admin email regex",
			modify: func(c *Config) {
				c.OIDC = OIDCConfig{
					Enabled:         true,
					Issuer:          "https://accounts.example.com",
					ClientID:        "id",
					ClientSecret:    "secret",
					RedirectURL:     "https://app.example.com/api/auth/oidc/callback",
					AdminEmailRegex: "[invalid(regex",
				}
			},
			wantErr: "MOTUS_OIDC_ADMIN_EMAIL_REGEX: invalid regular expression",
		},
		{
			name: "oidc enabled - valid admin email regex",
			modify: func(c *Config) {
				c.OIDC = OIDCConfig{
					Enabled:         true,
					Issuer:          "https://accounts.example.com",
					ClientID:        "id",
					ClientSecret:    "secret",
					RedirectURL:     "https://app.example.com/api/auth/oidc/callback",
					AdminEmailRegex: `@example\.com$`,
				}
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.modify(cfg)
			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

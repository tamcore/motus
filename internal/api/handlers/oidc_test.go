package handlers

import (
	"regexp"
	"testing"

	"github.com/tamcore/motus/internal/config"
)

// TestOIDCHandler_isAdminByFilter checks every combination of email regex and
// claim-based admin filter.
func TestOIDCHandler_isAdminByFilter(t *testing.T) {
	compileRe := func(s string) *regexp.Regexp {
		if s == "" {
			return nil
		}
		return regexp.MustCompile(s)
	}

	tests := []struct {
		name      string
		cfg       config.OIDCConfig
		adminRe   string
		email     string
		allClaims map[string]interface{}
		wantAdmin bool
	}{
		{
			name:      "no filters configured",
			email:     "user@example.com",
			allClaims: map[string]interface{}{},
			wantAdmin: false,
		},
		{
			name:      "email regex matches",
			adminRe:   `@example\.com$`,
			email:     "admin@example.com",
			allClaims: map[string]interface{}{},
			wantAdmin: true,
		},
		{
			name:      "email regex does not match",
			adminRe:   `@example\.com$`,
			email:     "admin@other.com",
			allClaims: map[string]interface{}{},
			wantAdmin: false,
		},
		{
			name:  "claim string value matches",
			email: "user@other.com",
			cfg: config.OIDCConfig{
				AdminClaim:      "role",
				AdminClaimValue: "admin",
			},
			allClaims: map[string]interface{}{"role": "admin"},
			wantAdmin: true,
		},
		{
			name:  "claim string value does not match",
			email: "user@other.com",
			cfg: config.OIDCConfig{
				AdminClaim:      "role",
				AdminClaimValue: "admin",
			},
			allClaims: map[string]interface{}{"role": "viewer"},
			wantAdmin: false,
		},
		{
			name:  "claim array contains value",
			email: "user@other.com",
			cfg: config.OIDCConfig{
				AdminClaim:      "groups",
				AdminClaimValue: "motus-admin",
			},
			allClaims: map[string]interface{}{
				"groups": []interface{}{"viewers", "motus-admin", "editors"},
			},
			wantAdmin: true,
		},
		{
			name:  "claim array does not contain value",
			email: "user@other.com",
			cfg: config.OIDCConfig{
				AdminClaim:      "groups",
				AdminClaimValue: "motus-admin",
			},
			allClaims: map[string]interface{}{
				"groups": []interface{}{"viewers", "editors"},
			},
			wantAdmin: false,
		},
		{
			name:  "claim missing from token",
			email: "user@other.com",
			cfg: config.OIDCConfig{
				AdminClaim:      "groups",
				AdminClaimValue: "motus-admin",
			},
			allClaims: map[string]interface{}{},
			wantAdmin: false,
		},
		{
			name:    "email regex matches - claim also set but does not match",
			adminRe: `@example\.com$`,
			email:   "admin@example.com",
			cfg: config.OIDCConfig{
				AdminClaim:      "groups",
				AdminClaimValue: "motus-admin",
			},
			allClaims: map[string]interface{}{"groups": []interface{}{"viewers"}},
			wantAdmin: true, // email regex is sufficient
		},
		{
			name:    "neither filter matches",
			adminRe: `@example\.com$`,
			email:   "admin@other.com",
			cfg: config.OIDCConfig{
				AdminClaim:      "groups",
				AdminClaimValue: "motus-admin",
			},
			allClaims: map[string]interface{}{"groups": []interface{}{"viewers"}},
			wantAdmin: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &OIDCHandler{
				cfg:             tt.cfg,
				adminEmailRegex: compileRe(tt.adminRe),
			}
			got := h.isAdminByFilter(tt.email, tt.allClaims)
			if got != tt.wantAdmin {
				t.Errorf("isAdminByFilter(%q, %v) = %v, want %v", tt.email, tt.allClaims, got, tt.wantAdmin)
			}
		})
	}
}

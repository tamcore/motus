package serve

import (
	"encoding/hex"
	"testing"

	"github.com/tamcore/motus/internal/config"
)

func TestLoadCSRFSecret_ValidHex(t *testing.T) {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i)
	}
	hexStr := hex.EncodeToString(raw)

	got := loadCSRFSecret(config.SecurityConfig{CSRFSecret: hexStr, Env: "production"})
	if len(got) != 32 {
		t.Errorf("got %d bytes, want 32", len(got))
	}
	for i, b := range got {
		if b != raw[i] {
			t.Errorf("byte[%d] = %d, want %d", i, b, raw[i])
		}
	}
}

func TestLoadCSRFSecret_EmptyDevelopment(t *testing.T) {
	// Empty secret in development mode generates a random 32-byte key.
	secret := loadCSRFSecret(config.SecurityConfig{Env: "Development"})
	if len(secret) != 32 {
		t.Errorf("got %d bytes, want 32", len(secret))
	}
}

func TestLoadCSRFSecret_EmptyProductionPanics(t *testing.T) {
	// Empty secret in production is a programming error (config.Validate
	// prevents it at startup), so loadCSRFSecret panics.
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty secret in production, got none")
		}
	}()
	loadCSRFSecret(config.SecurityConfig{Env: "production"})
}

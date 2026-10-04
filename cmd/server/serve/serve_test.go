package serve

import (
	"bytes"
	"encoding/hex"
	"log/slog"
	"strings"
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

func TestNewLogger(t *testing.T) {
	for _, tc := range []struct {
		level, format, env string
		wantFormat         string
		wantLevel          slog.Level
		wantWarning        bool
	}{
		{"debug", "", "development", "text", slog.LevelDebug, false},
		{"INFO", "", "production", "json", slog.LevelInfo, false},
		{"bogus", "TEXT", "production", "text", slog.LevelInfo, true},
		{"warn", "", "Development", "text", slog.LevelWarn, false},
		{"WARN", "json", "production", "json", slog.LevelWarn, false},
		{"error", "", "production", "json", slog.LevelError, false},
		{" debug \n", " text ", "production", "text", slog.LevelDebug, false},
		{"", "", "production", "json", slog.LevelInfo, true},
	} {
		cfg := &config.Config{Log: config.LogConfig{Level: tc.level, Format: tc.format}, Security: config.SecurityConfig{Env: tc.env}}
		var buf bytes.Buffer
		l, format := newLogger(&buf, cfg)
		if format != tc.wantFormat {
			t.Errorf("%+v: format = %q, want %q", tc, format, tc.wantFormat)
		}
		if !l.Enabled(t.Context(), tc.wantLevel) || l.Enabled(t.Context(), tc.wantLevel-1) {
			t.Errorf("%+v: logger level is not %v", tc, tc.wantLevel)
		}
		if hasWarning := strings.Contains(buf.String(), "invalid log level"); hasWarning != tc.wantWarning {
			t.Errorf("%+v: warning logged = %v, want %v (output %q)", tc, hasWarning, tc.wantWarning, buf.String())
		}
		l.Error("x")
		if isJSON := strings.HasPrefix(buf.String(), "{"); isJSON != (tc.wantFormat == "json") {
			t.Errorf("%+v: output %q does not match format %q", tc, buf.String(), tc.wantFormat)
		}
	}
}

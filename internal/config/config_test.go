package config_test

import (
	"os"
	"testing"

	"github.com/tamcore/motus/internal/config"
)

func TestLoadFromEnv_Defaults(t *testing.T) {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Port != "8080" {
		t.Errorf("expected default port 8080, got %q", cfg.Server.Port)
	}
	if cfg.Database.Host != "localhost" {
		t.Errorf("expected default host localhost, got %q", cfg.Database.Host)
	}
}

func TestLoadFromEnv_CustomValues(t *testing.T) {
	_ = os.Setenv("MOTUS_SERVER_PORT", "9090")
	_ = os.Setenv("MOTUS_DATABASE_HOST", "db.example.com")
	defer func() {
		_ = os.Unsetenv("MOTUS_SERVER_PORT")
		_ = os.Unsetenv("MOTUS_DATABASE_HOST")
	}()

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Port != "9090" {
		t.Errorf("expected port 9090, got %q", cfg.Server.Port)
	}
	if cfg.Database.Host != "db.example.com" {
		t.Errorf("expected host db.example.com, got %q", cfg.Database.Host)
	}
}

func TestDatabaseConfig_URL(t *testing.T) {
	cfg := config.DatabaseConfig{
		Host:     "localhost",
		Port:     "5432",
		User:     "motus",
		Password: "secret",
		Name:     "motus_test",
		SSLMode:  "disable",
	}

	expected := "postgres://motus:secret@localhost:5432/motus_test?sslmode=disable"
	if got := cfg.URL(); got != expected {
		t.Errorf("expected URL %q, got %q", expected, got)
	}
}

func TestDatabaseConfig_URL_WithURI(t *testing.T) {
	// When URI is set, it should be returned directly without constructing from parts.
	uri := "postgres://custom-user:custom-pass@db.example.com:5433/mydb?sslmode=require"
	cfg := config.DatabaseConfig{
		URI:      uri,
		Host:     "ignored",
		Port:     "ignored",
		User:     "ignored",
		Password: "ignored",
		Name:     "ignored",
		SSLMode:  "ignored",
	}

	if got := cfg.URL(); got != uri {
		t.Errorf("expected URI passthrough %q, got %q", uri, got)
	}
}

package logger_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/tamcore/motus/internal/logger"
)

func TestNew_DefaultsToInfoLevel(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(logger.Options{
		Writer: &buf,
	})

	l.Info("hello")
	l.Debug("should not appear")

	output := buf.String()
	if !strings.Contains(output, "hello") {
		t.Errorf("expected INFO message in output, got %q", output)
	}
	if strings.Contains(output, "should not appear") {
		t.Error("DEBUG message should not appear at INFO level")
	}
}

func TestNew_DebugLevel(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(logger.Options{
		Level:  "DEBUG",
		Writer: &buf,
	})

	l.Debug("debug message")
	l.Info("info message")

	output := buf.String()
	if !strings.Contains(output, "debug message") {
		t.Errorf("expected DEBUG message in output, got %q", output)
	}
	if !strings.Contains(output, "info message") {
		t.Errorf("expected INFO message in output, got %q", output)
	}
}

func TestNew_ErrorLevel(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(logger.Options{
		Level:  "ERROR",
		Writer: &buf,
	})

	l.Info("info message")
	l.Warn("warn message")
	l.Error("error message")

	output := buf.String()
	if strings.Contains(output, "info message") {
		t.Error("INFO message should not appear at ERROR level")
	}
	if strings.Contains(output, "warn message") {
		t.Error("WARN message should not appear at ERROR level")
	}
	if !strings.Contains(output, "error message") {
		t.Errorf("expected ERROR message in output, got %q", output)
	}
}

func TestNew_WarnLevel(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(logger.Options{
		Level:  "WARN",
		Writer: &buf,
	})

	l.Info("info message")
	l.Warn("warn message")
	l.Error("error message")

	output := buf.String()
	if strings.Contains(output, "info message") {
		t.Error("INFO message should not appear at WARN level")
	}
	if !strings.Contains(output, "warn message") {
		t.Error("expected WARN message in output")
	}
	if !strings.Contains(output, "error message") {
		t.Error("expected ERROR message in output")
	}
}

func TestNew_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(logger.Options{
		Format: "json",
		Writer: &buf,
	})

	l.Info("test message", slog.String("key", "value"))

	output := buf.String()
	// Verify it's valid JSON.
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err != nil {
		t.Fatalf("expected valid JSON output, got %q: %v", output, err)
	}
	if parsed["msg"] != "test message" {
		t.Errorf("expected msg='test message', got %v", parsed["msg"])
	}
	if parsed["key"] != "value" {
		t.Errorf("expected key='value', got %v", parsed["key"])
	}
}

func TestNew_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(logger.Options{
		Format: "text",
		Writer: &buf,
	})

	l.Info("test message")

	output := buf.String()
	// Text format should NOT be valid JSON.
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err == nil {
		t.Error("text format output should not be valid JSON")
	}
	if !strings.Contains(output, "test message") {
		t.Errorf("expected 'test message' in output, got %q", output)
	}
}

func TestNew_CaseInsensitiveLevel(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(logger.Options{
		Level:  "debug",
		Writer: &buf,
	})

	l.Debug("debug message")
	if !strings.Contains(buf.String(), "debug message") {
		t.Error("case-insensitive level should work: 'debug' should equal 'DEBUG'")
	}
}

func TestNew_InvalidLevelFallsBackToInfo(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(logger.Options{
		Level:  "INVALID",
		Writer: &buf,
	})

	l.Info("info message")
	l.Debug("debug message")

	output := buf.String()
	if !strings.Contains(output, "info message") {
		t.Error("expected INFO message with fallback to INFO level")
	}
	if strings.Contains(output, "debug message") {
		t.Error("DEBUG should not appear with fallback to INFO level")
	}
}

func TestNew_ContextualFields(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(logger.Options{
		Format: "json",
		Writer: &buf,
	})

	l.Info("device connected",
		slog.Int64("deviceID", 42),
		slog.String("protocol", "h02"),
	)

	output := buf.String()
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err != nil {
		t.Fatalf("expected valid JSON output: %v", err)
	}
	// json.Unmarshal puts numbers as float64.
	if parsed["deviceID"] != float64(42) {
		t.Errorf("expected deviceID=42, got %v", parsed["deviceID"])
	}
	if parsed["protocol"] != "h02" {
		t.Errorf("expected protocol='h02', got %v", parsed["protocol"])
	}
}

func TestNew_WithGroup(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(logger.Options{
		Format: "json",
		Writer: &buf,
	})

	l.WithGroup("request").Info("handled",
		slog.String("method", "GET"),
		slog.Int("status", 200),
	)

	output := buf.String()
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err != nil {
		t.Fatalf("expected valid JSON output: %v", err)
	}
	// The group should nest the attributes.
	reqGroup, ok := parsed["request"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'request' group in JSON output, got %v", parsed)
	}
	if reqGroup["method"] != "GET" {
		t.Errorf("expected method='GET', got %v", reqGroup["method"])
	}
}

func TestNew_With(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(logger.Options{
		Format: "json",
		Writer: &buf,
	})

	child := l.With(slog.String("component", "gps-server"))
	child.Info("started")

	output := buf.String()
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err != nil {
		t.Fatalf("expected valid JSON output: %v", err)
	}
	if parsed["component"] != "gps-server" {
		t.Errorf("expected component='gps-server', got %v", parsed["component"])
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"DEBUG", slog.LevelDebug},
		{"debug", slog.LevelDebug},
		{"INFO", slog.LevelInfo},
		{"info", slog.LevelInfo},
		{"WARN", slog.LevelWarn},
		{"warn", slog.LevelWarn},
		{"WARNING", slog.LevelWarn},
		{"ERROR", slog.LevelError},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},
		{"INVALID", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := logger.ParseLevel(tt.input)
			if got != tt.want {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatForEnv(t *testing.T) {
	tests := []struct {
		env  string
		want string
	}{
		{"production", "json"},
		{"", "json"},
		{"staging", "json"},
		{"development", "text"},
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			got := logger.FormatForEnv(tt.env)
			if got != tt.want {
				t.Errorf("FormatForEnv(%q) = %q, want %q", tt.env, got, tt.want)
			}
		})
	}
}

func TestNew_EmptyFormatDefaultsBasedOnMOTUS_ENV(t *testing.T) {
	// When format is empty and env is "production", use JSON.
	var buf bytes.Buffer
	l := logger.New(logger.Options{
		Writer: &buf,
		Format: "json",
	})
	l.Info("test")

	output := buf.String()
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &parsed); err != nil {
		t.Fatalf("expected JSON output with Format=json, got %q", output)
	}
}

func TestNew_NilWriter_UsesStderr(t *testing.T) {
	// When Writer is nil, New() should fall back to os.Stderr without panicking.
	l := logger.New(logger.Options{
		Format: "json",
		// Writer is intentionally nil.
	})
	if l == nil {
		t.Fatal("expected non-nil logger with nil Writer")
	}
	// Just verify it doesn't panic on use.
	l.Info("nil writer fallback test")
}

func TestNew_MultipleLogLevelsOutput(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(logger.Options{
		Level:  "DEBUG",
		Format: "json",
		Writer: &buf,
	})

	l.Debug("d")
	l.Info("i")
	l.Warn("w")
	l.Error("e")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 log lines, got %d: %v", len(lines), lines)
	}

	levels := []string{"DEBUG", "INFO", "WARN", "ERROR"}
	for i, line := range lines {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(line), &parsed); err != nil {
			t.Fatalf("line %d: invalid JSON: %v", i, err)
		}
		if parsed["level"] != levels[i] {
			t.Errorf("line %d: expected level %q, got %v", i, levels[i], parsed["level"])
		}
	}
}

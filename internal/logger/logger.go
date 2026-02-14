// Package logger provides structured logging for the Motus application using
// Go's standard log/slog package. It supports JSON and text output formats,
// configurable log levels, and contextual fields.
//
// Usage:
//
//	l := logger.New(logger.Options{
//	    Level:  "INFO",
//	    Format: "json",
//	})
//	l.Info("device connected", slog.Int64("deviceID", 42))
package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Options configures the structured logger.
type Options struct {
	// Level is the minimum log level (DEBUG, INFO, WARN, ERROR).
	// Default: INFO.
	Level string

	// Format is the output format: "json" or "text".
	// Default: "json" (production) or "text" (development).
	Format string

	// Writer is the output destination. Default: os.Stderr.
	Writer io.Writer
}

// New creates a new *slog.Logger configured with the given options.
// It returns a standard slog.Logger so callers do not depend on a custom type.
func New(opts Options) *slog.Logger {
	w := opts.Writer
	if w == nil {
		w = os.Stderr
	}

	level := ParseLevel(opts.Level)
	handlerOpts := &slog.HandlerOptions{
		Level: level,
	}

	format := strings.ToLower(strings.TrimSpace(opts.Format))

	var handler slog.Handler
	switch format {
	case "text":
		handler = slog.NewTextHandler(w, handlerOpts)
	default:
		// Default to JSON for production safety.
		handler = slog.NewJSONHandler(w, handlerOpts)
	}

	return slog.New(handler)
}

// ParseLevel converts a string log level name to a slog.Level value.
// Supported values (case-insensitive): DEBUG, INFO, WARN, WARNING, ERROR.
// Returns slog.LevelInfo for unrecognized values.
func ParseLevel(s string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// FormatForEnv returns the default log format for the given MOTUS_ENV value.
// Returns "text" for "development", "json" for everything else.
func FormatForEnv(env string) string {
	if strings.ToLower(env) == "development" {
		return "text"
	}
	return "json"
}

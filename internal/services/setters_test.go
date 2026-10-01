package services

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestNewEventEmitter_DefaultsLogger(t *testing.T) {
	if e := newEventEmitter(nil, nil, nil, nil); e.logger != slog.Default() {
		t.Error("nil logger should default to slog.Default()")
	}
	custom := slog.New(slog.Default().Handler())
	if e := newEventEmitter(nil, nil, nil, custom); e.logger != custom {
		t.Error("custom logger should be kept")
	}
}

// TestSetGeocoder_IdleService verifies SetGeocoder stores the geocoder.
func TestSetGeocoder_IdleService(t *testing.T) {
	s := &IdleService{logger: slog.Default()}
	s.SetGeocoder(nil, nil)
	if s.geocoder != nil {
		t.Error("expected geocoder to be nil after SetGeocoder(nil, nil)")
	}
}

// TestIdleService_Start_ContextCancel verifies Start exits when the context
// is cancelled immediately.
func TestIdleService_Start_ContextCancel(t *testing.T) {
	s := &IdleService{logger: slog.Default()}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Start(ctx)
	}()

	select {
	case <-done:
		// Good.
	case <-time.After(5 * time.Second):
		t.Fatal("IdleService.Start did not exit after context cancellation")
	}
}

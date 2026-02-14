package services

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

// TestSetLogger_AlarmService verifies SetLogger replaces the logger (non-nil)
// and is a no-op for nil.
func TestSetLogger_AlarmService(t *testing.T) {
	s := &AlarmService{logger: slog.Default()}
	initial := s.logger
	s.SetLogger(nil)
	if s.logger != initial {
		t.Error("SetLogger(nil) should not change the logger")
	}
	custom := slog.New(slog.Default().Handler())
	s.SetLogger(custom)
	if s.logger != custom {
		t.Error("SetLogger(custom) should replace the logger")
	}
}

// TestSetLogger_IgnitionService verifies SetLogger behaviour.
func TestSetLogger_IgnitionService(t *testing.T) {
	s := &IgnitionService{logger: slog.Default()}
	initial := s.logger
	s.SetLogger(nil)
	if s.logger != initial {
		t.Error("SetLogger(nil) should not change the logger")
	}
	custom := slog.New(slog.Default().Handler())
	s.SetLogger(custom)
	if s.logger != custom {
		t.Error("SetLogger(custom) should replace the logger")
	}
}

// TestSetLogger_MotionService verifies SetLogger behaviour.
func TestSetLogger_MotionService(t *testing.T) {
	s := &MotionService{logger: slog.Default()}
	initial := s.logger
	s.SetLogger(nil)
	if s.logger != initial {
		t.Error("SetLogger(nil) should not change the logger")
	}
	custom := slog.New(slog.Default().Handler())
	s.SetLogger(custom)
	if s.logger != custom {
		t.Error("SetLogger(custom) should replace the logger")
	}
}

// TestSetLogger_OverspeedService verifies SetLogger behaviour.
func TestSetLogger_OverspeedService(t *testing.T) {
	s := &OverspeedService{logger: slog.Default()}
	initial := s.logger
	s.SetLogger(nil)
	if s.logger != initial {
		t.Error("SetLogger(nil) should not change the logger")
	}
	custom := slog.New(slog.Default().Handler())
	s.SetLogger(custom)
	if s.logger != custom {
		t.Error("SetLogger(custom) should replace the logger")
	}
}

// TestSetLogger_GeofenceEventService verifies SetLogger behaviour.
func TestSetLogger_GeofenceEventService(t *testing.T) {
	s := &GeofenceEventService{logger: slog.Default()}
	initial := s.logger
	s.SetLogger(nil)
	if s.logger != initial {
		t.Error("SetLogger(nil) should not change the logger")
	}
	custom := slog.New(slog.Default().Handler())
	s.SetLogger(custom)
	if s.logger != custom {
		t.Error("SetLogger(custom) should replace the logger")
	}
}

// TestSetLogger_IdleService verifies SetLogger behaviour.
func TestSetLogger_IdleService(t *testing.T) {
	s := &IdleService{logger: slog.Default()}
	initial := s.logger
	s.SetLogger(nil)
	if s.logger != initial {
		t.Error("SetLogger(nil) should not change the logger")
	}
	custom := slog.New(slog.Default().Handler())
	s.SetLogger(custom)
	if s.logger != custom {
		t.Error("SetLogger(custom) should replace the logger")
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

// TestSetLogger_CleanupService verifies SetLogger behaviour.
func TestSetLogger_CleanupService(t *testing.T) {
	s := &CleanupService{logger: slog.Default(), interval: time.Hour}
	initial := s.logger
	s.SetLogger(nil)
	if s.logger != initial {
		t.Error("SetLogger(nil) should not change the logger")
	}
	custom := slog.New(slog.Default().Handler())
	s.SetLogger(custom)
	if s.logger != custom {
		t.Error("SetLogger(custom) should replace the logger")
	}
}

// TestSetLogger_NotificationService verifies SetLogger behaviour.
func TestSetLogger_NotificationService(t *testing.T) {
	s := &NotificationService{logger: slog.Default()}
	initial := s.logger
	s.SetLogger(nil)
	if s.logger != initial {
		t.Error("SetLogger(nil) should not change the logger")
	}
	custom := slog.New(slog.Default().Handler())
	s.SetLogger(custom)
	if s.logger != custom {
		t.Error("SetLogger(custom) should replace the logger")
	}
}

// TestSetAuditLogger_NotificationService verifies SetAuditLogger stores the logger.
func TestSetAuditLogger_NotificationService(t *testing.T) {
	s := &NotificationService{logger: slog.Default()}
	s.SetAuditLogger(nil) // should not panic
	if s.audit != nil {
		t.Error("expected audit to be nil after SetAuditLogger(nil)")
	}
}

// TestSetLogger_DeviceTimeoutService verifies SetLogger behaviour.
func TestSetLogger_DeviceTimeoutService(t *testing.T) {
	s := &DeviceTimeoutService{logger: slog.Default()}
	initial := s.logger
	s.SetLogger(nil)
	if s.logger != initial {
		t.Error("SetLogger(nil) should not change the logger")
	}
	custom := slog.New(slog.Default().Handler())
	s.SetLogger(custom)
	if s.logger != custom {
		t.Error("SetLogger(custom) should replace the logger")
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

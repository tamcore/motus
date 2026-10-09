package handlers_test

import (
	"context"
	"slices"
	"testing"

	"github.com/tamcore/motus/internal/api/handlers"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/services"
	"github.com/tamcore/motus/internal/storage/repository"
)

// newNotificationDeviceTestHandler wires a device repo whose access check
// grants only the device IDs in allowed.
func newNotificationDeviceTestHandler(notifications repository.NotificationRepo, allowed ...int64) *handlers.Handler {
	devices := &mockDeviceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, id int64) bool {
			return slices.Contains(allowed, id)
		},
	}
	svc := services.NewNotificationService(notifications, devices, &auditMockGeofenceRepo{}, &auditMockPositionRepo{}, nil, nil)
	return handlers.NewHandler(handlers.HandlerConfig{
		Notifications:       notifications,
		Devices:             devices,
		NotificationService: svc,
		NotificationRules:   services.NewNotificationRuleService(notifications, nil, devices, nil),
		AuditLogger:         audit.NewLogger(nil),
	})
}

func TestCreateNotification_DeviceFilter(t *testing.T) {
	var created *model.NotificationRule
	h := newNotificationDeviceTestHandler(capturingNotifRepo(&created), 3, 4)

	in := validNotificationInput(t)
	in.DeviceIds = []int64{4, 3, 4}
	res, err := h.CreateNotification(ctxAs(1, model.RoleUser), in)
	if err != nil {
		t.Fatalf("CreateNotification: %v", err)
	}
	rule, ok := res.(*oas.NotificationRule)
	if !ok {
		t.Fatalf("expected *oas.NotificationRule, got %T", res)
	}
	if !slices.Equal(rule.DeviceIds, []int64{3, 4}) {
		t.Errorf("response deviceIds = %v, want [3 4]", rule.DeviceIds)
	}
	if created == nil || !slices.Equal(created.DeviceIDs, []int64{3, 4}) {
		t.Fatalf("stored deviceIds = %+v, want [3 4]", created)
	}
}

func TestCreateNotification_DeviceFilterDenied(t *testing.T) {
	h := newNotificationDeviceTestHandler(&auditMockNotificationRepo{}, 3)

	in := validNotificationInput(t)
	in.DeviceIds = []int64{3, 9}
	res, err := h.CreateNotification(ctxAs(1, model.RoleUser), in)
	if err != nil {
		t.Fatalf("CreateNotification: %v", err)
	}
	if _, ok := res.(*oas.CreateNotificationBadRequest); !ok {
		t.Fatalf("expected *oas.CreateNotificationBadRequest, got %T", res)
	}
}

func TestCreateNotification_NoDeviceFilterReturnsEmptyList(t *testing.T) {
	var created *model.NotificationRule
	h := newNotificationDeviceTestHandler(capturingNotifRepo(&created))

	res, err := h.CreateNotification(ctxAs(1, model.RoleUser), validNotificationInput(t))
	if err != nil {
		t.Fatalf("CreateNotification: %v", err)
	}
	rule := res.(*oas.NotificationRule)
	if rule.DeviceIds == nil || len(rule.DeviceIds) != 0 {
		t.Errorf("deviceIds = %#v, want empty list", rule.DeviceIds)
	}
}

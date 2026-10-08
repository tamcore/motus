package handlers_test

// Tests for attaching geofences to devices via the device geofenceIds field.

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/tamcore/motus/internal/api/handlers"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
)

// newDeviceGeofenceHandler builds a handler whose geofence access check
// allows exactly the given geofence IDs.
func newDeviceGeofenceHandler(devices *mockDeviceRepo, accessible ...int64) *handlers.Handler {
	return handlers.NewHandler(handlers.HandlerConfig{
		Devices: devices,
		Geofences: &auditMockGeofenceRepo{
			userHasAccessFn: func(_ context.Context, _ *model.User, id int64) bool {
				return slices.Contains(accessible, id)
			},
		},
		AuditLogger: audit.NewLogger(nil),
	})
}

// deviceGeofenceMock returns a device repo for device 10 whose attachments are
// held in *attached; set records the last SetGeofences call.
func deviceGeofenceMock(attached *[]int64, set *[]int64) *mockDeviceRepo {
	return &mockDeviceRepo{
		userHasAccessFn: func(context.Context, *model.User, int64) bool { return true },
		getByIDFn: func(_ context.Context, id int64) (*model.Device, error) {
			if id != 10 {
				return nil, errors.New("not found")
			}
			return &model.Device{ID: 10, UniqueID: "geo-10", Name: "Dog", Status: "online"}, nil
		},
		getGeofenceIDsFn: func(_ context.Context, ids []int64) (map[int64][]int64, error) {
			out := map[int64][]int64{}
			if slices.Contains(ids, 10) && len(*attached) > 0 {
				out[10] = slices.Clone(*attached)
			}
			return out, nil
		},
		setGeofencesFn: func(_ context.Context, deviceID int64, ids []int64) error {
			if deviceID != 10 {
				return errors.New("unexpected device")
			}
			*set = slices.Clone(ids)
			*attached = slices.Clone(ids)
			return nil
		},
	}
}

func TestListDevices_IncludesGeofenceIDs_OAS(t *testing.T) {
	mock := &mockDeviceRepo{
		getByUserFn: func(context.Context, int64) ([]*model.Device, error) {
			return []*model.Device{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}}, nil
		},
		getGeofenceIDsFn: func(context.Context, []int64) (map[int64][]int64, error) {
			return map[int64][]int64{1: {5, 6}}, nil
		},
	}
	h := newDeviceTestHandler(mock)

	res, err := h.ListDevices(ctxAs(1, model.RoleUser))
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	list := *res.(*oas.ListDevicesOKApplicationJSON)
	if !slices.Equal(list[0].GeofenceIds, []int64{5, 6}) {
		t.Errorf("device 1 geofenceIds = %v, want [5 6]", list[0].GeofenceIds)
	}
	// Devices without attachments report an empty list, not an absent field.
	if list[1].GeofenceIds == nil || len(list[1].GeofenceIds) != 0 {
		t.Errorf("device 2 geofenceIds = %#v, want empty non-nil", list[1].GeofenceIds)
	}
}

func TestUpdateDevice_SetsGeofences_OAS(t *testing.T) {
	var attached, set []int64
	h := newDeviceGeofenceHandler(deviceGeofenceMock(&attached, &set), 5, 6)

	res, err := h.UpdateDevice(ctxAs(1, model.RoleUser), &oas.DeviceInput{
		Name: "Dog", UniqueId: "geo-10", GeofenceIds: []int64{6, 5, 6},
	}, oas.UpdateDeviceParams{ID: 10})
	if err != nil {
		t.Fatalf("UpdateDevice: %v", err)
	}
	device, ok := res.(*oas.Device)
	if !ok {
		t.Fatalf("expected *oas.Device, got %T", res)
	}
	if !slices.Equal(set, []int64{5, 6}) {
		t.Errorf("SetGeofences = %v, want [5 6]", set)
	}
	if !slices.Equal(device.GeofenceIds, []int64{5, 6}) {
		t.Errorf("response geofenceIds = %v, want [5 6]", device.GeofenceIds)
	}
}

func TestUpdateDevice_EmptyGeofencesClears_OAS(t *testing.T) {
	attached := []int64{5}
	set := []int64{99}
	h := newDeviceGeofenceHandler(deviceGeofenceMock(&attached, &set), 5)

	if _, err := h.UpdateDevice(ctxAs(1, model.RoleUser), &oas.DeviceInput{
		Name: "Dog", UniqueId: "geo-10", GeofenceIds: []int64{},
	}, oas.UpdateDeviceParams{ID: 10}); err != nil {
		t.Fatalf("UpdateDevice: %v", err)
	}
	if set == nil || len(set) != 0 {
		t.Errorf("SetGeofences = %#v, want empty", set)
	}
}

func TestUpdateDevice_OmittedGeofencesUnchanged_OAS(t *testing.T) {
	attached := []int64{5}
	var set []int64
	called := false
	mock := deviceGeofenceMock(&attached, &set)
	mock.setGeofencesFn = func(context.Context, int64, []int64) error {
		called = true
		return nil
	}
	h := newDeviceGeofenceHandler(mock, 5)

	res, err := h.UpdateDevice(ctxAs(1, model.RoleUser), &oas.DeviceInput{Name: "Dog", UniqueId: "geo-10"},
		oas.UpdateDeviceParams{ID: 10})
	if err != nil {
		t.Fatalf("UpdateDevice: %v", err)
	}
	if called {
		t.Error("SetGeofences must not be called when geofenceIds is absent")
	}
	if got := res.(*oas.Device).GeofenceIds; !slices.Equal(got, []int64{5}) {
		t.Errorf("response geofenceIds = %v, want [5]", got)
	}
}

func TestUpdateDevice_InaccessibleGeofenceRejected_OAS(t *testing.T) {
	var attached, set []int64
	called := false
	mock := deviceGeofenceMock(&attached, &set)
	mock.setGeofencesFn = func(context.Context, int64, []int64) error {
		called = true
		return nil
	}
	h := newDeviceGeofenceHandler(mock, 5)

	res, err := h.UpdateDevice(ctxAs(1, model.RoleUser), &oas.DeviceInput{
		Name: "Dog", UniqueId: "geo-10", GeofenceIds: []int64{5, 7},
	}, oas.UpdateDeviceParams{ID: 10})
	if err != nil {
		t.Fatalf("UpdateDevice: %v", err)
	}
	if _, ok := res.(*oas.UpdateDeviceBadRequest); !ok {
		t.Fatalf("expected *oas.UpdateDeviceBadRequest, got %T", res)
	}
	if called {
		t.Error("SetGeofences must not be called for an inaccessible geofence")
	}
}

// A shared device can carry geofences of another owner. Saving the device
// with the caller's own selection must not drop those hidden attachments.
func TestUpdateDevice_PreservesHiddenGeofences_OAS(t *testing.T) {
	attached := []int64{4, 5}
	var set []int64
	h := newDeviceGeofenceHandler(deviceGeofenceMock(&attached, &set), 5, 6)

	if _, err := h.UpdateDevice(ctxAs(1, model.RoleUser), &oas.DeviceInput{
		Name: "Dog", UniqueId: "geo-10", GeofenceIds: []int64{6},
	}, oas.UpdateDeviceParams{ID: 10}); err != nil {
		t.Fatalf("UpdateDevice: %v", err)
	}
	if !slices.Equal(set, []int64{4, 6}) {
		t.Errorf("SetGeofences = %v, want [4 6] (hidden 4 kept, 5 removed, 6 added)", set)
	}
}

func TestCreateDevice_SetsGeofences_OAS(t *testing.T) {
	var set []int64
	mock := &mockDeviceRepo{
		setGeofencesFn: func(_ context.Context, deviceID int64, ids []int64) error {
			if deviceID != 42 {
				return errors.New("unexpected device")
			}
			set = slices.Clone(ids)
			return nil
		},
		getGeofenceIDsFn: func(context.Context, []int64) (map[int64][]int64, error) {
			return map[int64][]int64{42: slices.Clone(set)}, nil
		},
	}
	h := newDeviceGeofenceHandler(mock, 5)

	res, err := h.CreateDevice(ctxAs(1, model.RoleUser), &oas.DeviceInput{
		Name: "Cat", UniqueId: "geo-new", GeofenceIds: []int64{5},
	})
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	device, ok := res.(*oas.Device)
	if !ok {
		t.Fatalf("expected *oas.Device, got %T", res)
	}
	if !slices.Equal(set, []int64{5}) || !slices.Equal(device.GeofenceIds, []int64{5}) {
		t.Errorf("set = %v, response = %v, want [5]", set, device.GeofenceIds)
	}
}

func TestCreateDevice_InaccessibleGeofenceRejected_OAS(t *testing.T) {
	created := false
	mock := &mockDeviceRepo{
		createFn: func(_ context.Context, d *model.Device, _ int64) error {
			created = true
			d.ID = 42
			return nil
		},
	}
	h := newDeviceGeofenceHandler(mock)

	res, err := h.CreateDevice(ctxAs(1, model.RoleUser), &oas.DeviceInput{
		Name: "Cat", UniqueId: "geo-new", GeofenceIds: []int64{7},
	})
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	if _, ok := res.(*oas.CreateDeviceBadRequest); !ok {
		t.Fatalf("expected *oas.CreateDeviceBadRequest, got %T", res)
	}
	if created {
		t.Error("device must not be created when a geofence is inaccessible")
	}
}

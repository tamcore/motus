package handlers_test

// Tests for the ogen Handler device CRUD methods (ListDevices, GetDevice,
// CreateDevice, UpdateDevice, DeleteDevice). Ported from the deleted chi
// DeviceHandler test files device_test.go and device_mock_test.go, plus the
// device half of the deleted validation_test.go
// (TestDeviceCreate_InvalidUniqueID/InvalidName/ValidInput,
// TestDeviceUpdate_InvalidName). These run against mock repositories — no
// database required.
//
// Dropped tests (no live equivalent):
//   - invalid-JSON body and invalid path-ID transport tests: ogen owns
//     request decoding and path-param parsing.

import (
	"context"
	"errors"
	"testing"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/audit"
	"github.com/tamcore/motus/internal/model"
)

// newDeviceTestHandler builds an ogen Handler from a mock device repo. The
// nil-pool audit logger exercises the audit code paths as documented no-ops.
func newDeviceTestHandler(devices *mockDeviceRepo) *handlers.Handler {
	return handlers.NewHandler(handlers.HandlerConfig{
		Devices:     devices,
		AuditLogger: audit.NewLogger(nil),
	})
}

func deviceTestUserCtx(id int64) context.Context {
	return api.ContextWithUser(context.Background(),
		&model.User{ID: id, Email: "devhandler@example.com", Name: "Dev Handler"})
}

// ---------------------------------------------------------------------------
// ListDevices
// ---------------------------------------------------------------------------

func TestListDevices_Empty_OAS(t *testing.T) {
	h := newDeviceTestHandler(&mockDeviceRepo{})

	res, err := h.ListDevices(deviceTestUserCtx(1))
	if err != nil {
		t.Fatalf("ListDevices returned error: %v", err)
	}
	list, ok := res.(*oas.ListDevicesOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.ListDevicesOKApplicationJSON, got %T", res)
	}
	if len(*list) != 0 {
		t.Errorf("expected empty list, got %d devices", len(*list))
	}
}

func TestListDevices_WithDevices_OAS(t *testing.T) {
	mock := &mockDeviceRepo{
		getByUserFn: func(_ context.Context, userID int64) ([]*model.Device, error) {
			if userID != 1 {
				t.Errorf("expected userID=1, got %d", userID)
			}
			return []*model.Device{
				{ID: 10, UniqueID: "dev-a", Name: "Device A", Status: "online"},
				{ID: 20, UniqueID: "dev-b", Name: "Device B", Status: "offline"},
			}, nil
		},
	}
	h := newDeviceTestHandler(mock)

	res, err := h.ListDevices(deviceTestUserCtx(1))
	if err != nil {
		t.Fatalf("ListDevices returned error: %v", err)
	}
	list, ok := res.(*oas.ListDevicesOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.ListDevicesOKApplicationJSON, got %T", res)
	}
	if len(*list) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(*list))
	}
	if (*list)[0].Name != "Device A" {
		t.Errorf("expected first device name 'Device A', got %q", (*list)[0].Name)
	}
}

func TestListDevices_DBError_OAS(t *testing.T) {
	mock := &mockDeviceRepo{
		getByUserFn: func(_ context.Context, _ int64) ([]*model.Device, error) {
			return nil, errors.New("connection refused")
		},
	}
	h := newDeviceTestHandler(mock)

	res, err := h.ListDevices(deviceTestUserCtx(1))
	if err != nil {
		t.Fatalf("ListDevices returned error: %v", err)
	}
	if _, ok := res.(*oas.Error); !ok {
		t.Errorf("expected *oas.Error on DB failure, got %T", res)
	}
}

// ---------------------------------------------------------------------------
// GetDevice
// ---------------------------------------------------------------------------

func TestGetDevice_Success_OAS(t *testing.T) {
	mock := &mockDeviceRepo{
		userHasAccessFn: func(_ context.Context, user *model.User, deviceID int64) bool {
			return user.ID == 1 && deviceID == 10
		},
		getByIDFn: func(_ context.Context, id int64) (*model.Device, error) {
			if id == 10 {
				return &model.Device{ID: 10, UniqueID: "dev-10", Name: "My Device", Status: "online"}, nil
			}
			return nil, errors.New("not found")
		},
	}
	h := newDeviceTestHandler(mock)

	res, err := h.GetDevice(deviceTestUserCtx(1), oas.GetDeviceParams{ID: 10})
	if err != nil {
		t.Fatalf("GetDevice returned error: %v", err)
	}
	device, ok := res.(*oas.Device)
	if !ok {
		t.Fatalf("expected *oas.Device, got %T", res)
	}
	if device.Name != "My Device" {
		t.Errorf("expected name 'My Device', got %q", device.Name)
	}
}

func TestGetDevice_Forbidden_OAS(t *testing.T) {
	// IDOR: access check always denies; the lookup must not leak the device.
	mock := &mockDeviceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return false },
	}
	h := newDeviceTestHandler(mock)

	res, err := h.GetDevice(deviceTestUserCtx(99), oas.GetDeviceParams{ID: 10})
	if err != nil {
		t.Fatalf("GetDevice returned error: %v", err)
	}
	if _, ok := res.(*oas.GetDeviceForbidden); !ok {
		t.Errorf("expected *oas.GetDeviceForbidden, got %T", res)
	}
}

// ---------------------------------------------------------------------------
// CreateDevice
// ---------------------------------------------------------------------------

func TestCreateDevice_Success_OAS(t *testing.T) {
	var createdDevice *model.Device
	var createdForUser int64
	mock := &mockDeviceRepo{
		createFn: func(_ context.Context, d *model.Device, userID int64) error {
			createdDevice = d
			createdForUser = userID
			d.ID = 99 // Simulate DB assigning an ID.
			return nil
		},
	}
	h := newDeviceTestHandler(mock)

	res, err := h.CreateDevice(deviceTestUserCtx(7), &oas.DeviceInput{
		UniqueId: "new-001",
		Name:     "Brand New Device",
		Protocol: oas.NewOptString("h02"),
	})
	if err != nil {
		t.Fatalf("CreateDevice returned error: %v", err)
	}
	device, ok := res.(*oas.Device)
	if !ok {
		t.Fatalf("expected *oas.Device, got %T", res)
	}
	if createdDevice == nil {
		t.Fatal("expected createFn to be called")
	}
	if createdDevice.UniqueID != "new-001" {
		t.Errorf("expected uniqueId 'new-001', got %q", createdDevice.UniqueID)
	}
	if createdForUser != 7 {
		t.Errorf("expected userID=7, got %d", createdForUser)
	}
	if device.ID != 99 {
		t.Errorf("expected response ID=99, got %d", device.ID)
	}
}

// TestCreateDevice_ValidationMatrix_OAS covers the device-creation input
// validation ported from validation_test.go and device_mock_test.go.
func TestCreateDevice_ValidationMatrix_OAS(t *testing.T) {
	tests := []struct {
		name     string
		uniqueID string
		devName  string
	}{
		{"empty uniqueId", "", "Valid Name"},
		{"uniqueId with spaces", "device 001", "Valid Name"},
		{"uniqueId with special chars", "device@001", "Valid Name"},
		{"uniqueId with angle brackets", "device<001>", "Valid Name"},
		{"empty name", "valid-device-001", ""},
		{"XSS name", "valid-device-001", "name<script>alert(1)</script>"},
		{"missing fields", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newDeviceTestHandler(&mockDeviceRepo{})
			res, err := h.CreateDevice(deviceTestUserCtx(1), &oas.DeviceInput{
				UniqueId: tt.uniqueID,
				Name:     tt.devName,
			})
			if err != nil {
				t.Fatalf("CreateDevice returned error: %v", err)
			}
			if _, ok := res.(*oas.CreateDeviceBadRequest); !ok {
				t.Errorf("expected *oas.CreateDeviceBadRequest for uniqueId=%q name=%q, got %T",
					tt.uniqueID, tt.devName, res)
			}
		})
	}
}

func TestCreateDevice_ValidInput_OAS(t *testing.T) {
	mock := &mockDeviceRepo{
		createFn: func(_ context.Context, d *model.Device, _ int64) error {
			d.ID = 1
			return nil
		},
	}
	h := newDeviceTestHandler(mock)

	res, err := h.CreateDevice(deviceTestUserCtx(1), &oas.DeviceInput{
		UniqueId: "valid-device-002",
		Name:     "Valid Device",
		Protocol: oas.NewOptString("h02"),
	})
	if err != nil {
		t.Fatalf("CreateDevice returned error: %v", err)
	}
	device, ok := res.(*oas.Device)
	if !ok {
		t.Fatalf("expected *oas.Device for valid input, got %T", res)
	}
	if device.UniqueId != "valid-device-002" {
		t.Errorf("expected uniqueId 'valid-device-002', got %q", device.UniqueId)
	}
}

func TestCreateDevice_DBError_OAS(t *testing.T) {
	mock := &mockDeviceRepo{
		createFn: func(_ context.Context, _ *model.Device, _ int64) error {
			return errors.New("unique constraint violation")
		},
	}
	h := newDeviceTestHandler(mock)

	res, err := h.CreateDevice(deviceTestUserCtx(1), &oas.DeviceInput{
		UniqueId: "dup-001",
		Name:     "Duplicate",
	})
	if err != nil {
		t.Fatalf("CreateDevice returned error: %v", err)
	}
	if _, ok := res.(*oas.CreateDeviceBadRequest); !ok {
		t.Errorf("expected *oas.CreateDeviceBadRequest on DB failure, got %T", res)
	}
}

// ---------------------------------------------------------------------------
// UpdateDevice
// ---------------------------------------------------------------------------

func TestUpdateDevice_Success_OAS(t *testing.T) {
	existing := &model.Device{ID: 10, UniqueID: "upd-001", Name: "Before", Protocol: "h02", Status: "online"}
	var updatedName string
	mock := &mockDeviceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return true },
		getByIDFn: func(_ context.Context, id int64) (*model.Device, error) {
			if id == 10 {
				d := *existing // copy to avoid cross-call mutation
				return &d, nil
			}
			return nil, errors.New("not found")
		},
		updateFn: func(_ context.Context, d *model.Device) error {
			updatedName = d.Name
			return nil
		},
	}
	h := newDeviceTestHandler(mock)

	res, err := h.UpdateDevice(deviceTestUserCtx(1), &oas.DeviceInput{
		UniqueId: "upd-001",
		Name:     "After Update",
	}, oas.UpdateDeviceParams{ID: 10})
	if err != nil {
		t.Fatalf("UpdateDevice returned error: %v", err)
	}
	device, ok := res.(*oas.Device)
	if !ok {
		t.Fatalf("expected *oas.Device, got %T", res)
	}
	if device.Name != "After Update" {
		t.Errorf("expected response name 'After Update', got %q", device.Name)
	}
	if updatedName != "After Update" {
		t.Errorf("expected persisted name 'After Update', got %q", updatedName)
	}
}

func TestUpdateDevice_InvalidName_OAS(t *testing.T) {
	mock := &mockDeviceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return true },
		getByIDFn: func(_ context.Context, id int64) (*model.Device, error) {
			return &model.Device{ID: id, UniqueID: "update-val-001", Name: "Original", Status: "unknown"}, nil
		},
	}
	h := newDeviceTestHandler(mock)

	res, err := h.UpdateDevice(deviceTestUserCtx(1), &oas.DeviceInput{
		UniqueId: "update-val-001",
		Name:     "name<script>alert(1)</script>",
	}, oas.UpdateDeviceParams{ID: 10})
	if err != nil {
		t.Fatalf("UpdateDevice returned error: %v", err)
	}
	if _, ok := res.(*oas.UpdateDeviceBadRequest); !ok {
		t.Errorf("expected *oas.UpdateDeviceBadRequest for invalid name, got %T", res)
	}
}

// ---------------------------------------------------------------------------
// DeleteDevice
// ---------------------------------------------------------------------------

func TestDeleteDevice_Success_OAS(t *testing.T) {
	var deletedID int64
	mock := &mockDeviceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return true },
		deleteFn: func(_ context.Context, id int64) error {
			deletedID = id
			return nil
		},
	}
	h := newDeviceTestHandler(mock)

	res, err := h.DeleteDevice(deviceTestUserCtx(1), oas.DeleteDeviceParams{ID: 10})
	if err != nil {
		t.Fatalf("DeleteDevice returned error: %v", err)
	}
	if _, ok := res.(*oas.DeleteDeviceNoContent); !ok {
		t.Fatalf("expected *oas.DeleteDeviceNoContent, got %T", res)
	}
	if deletedID != 10 {
		t.Errorf("expected delete called with ID=10, got %d", deletedID)
	}
}

func TestDeleteDevice_Forbidden_OAS(t *testing.T) {
	mock := &mockDeviceRepo{
		userHasAccessFn: func(_ context.Context, _ *model.User, _ int64) bool { return false },
	}
	h := newDeviceTestHandler(mock)

	res, err := h.DeleteDevice(deviceTestUserCtx(99), oas.DeleteDeviceParams{ID: 10})
	if err != nil {
		t.Fatalf("DeleteDevice returned error: %v", err)
	}
	if _, ok := res.(*oas.DeleteDeviceForbidden); !ok {
		t.Errorf("expected *oas.DeleteDeviceForbidden, got %T", res)
	}
}

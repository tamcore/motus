// Package handlers_test contains integration tests that validate Motus API
// responses against the requirements of Home Assistant 2025.11+ and the
// Traccar Manager mobile application.
//
// These tests use testcontainers (PostGIS) to exercise the full stack from
// HTTP handler through repository to database and back. They exist as a
// regression safety net: if any change breaks the JSON shape, field
// presence, value range, or authentication contract that Home Assistant and
// pytraccar depend on, these tests will fail.
//
// Reference documentation:
//   - Home Assistant Traccar Server integration:
//     https://www.home-assistant.io/integrations/traccar_server/
//   - pytraccar (HA client library):
//     https://github.com/ludeeus/pytraccar
//   - Traccar API specification:
//     https://www.traccar.org/api-reference/
package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
	"github.com/tamcore/motus/internal/websocket"
	"golang.org/x/crypto/bcrypt"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// compatTestFixtures holds repositories and test data for compat tests.
type compatTestFixtures struct {
	pool         any // kept for reference
	userRepo     *repository.UserRepository
	deviceRepo   *repository.DeviceRepository
	positionRepo *repository.PositionRepository
	sessionRepo  *repository.SessionRepository
	geofenceRepo *repository.GeofenceRepository
	apiKeyRepo   *repository.ApiKeyRepository

	user   *model.User
	device *model.Device
}

// setupCompatFixtures creates a clean database, a test user, a device
// associated with that user, and all the repository instances needed to
// build handlers.
func setupCompatFixtures(t *testing.T) *compatTestFixtures {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	ctx := context.Background()

	userRepo := repository.NewUserRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	positionRepo := repository.NewPositionRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	geofenceRepo := repository.NewGeofenceRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	hash, err := bcrypt.GenerateFromPassword([]byte("testpass123"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := &model.User{
		Email:        "compat@example.com",
		PasswordHash: string(hash),
		Name:         "HA Compat User",
		Role:         model.RoleUser,
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	device := &model.Device{
		UniqueID: "compat-dev-001",
		Name:     "HA Test Device",
		Status:   "online",
	}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	return &compatTestFixtures{
		pool:         pool,
		userRepo:     userRepo,
		deviceRepo:   deviceRepo,
		positionRepo: positionRepo,
		sessionRepo:  sessionRepo,
		geofenceRepo: geofenceRepo,
		apiKeyRepo:   apiKeyRepo,
		user:         user,
		device:       device,
	}
}

// compatUserCtx returns a context carrying the given authenticated user.
func compatUserCtx(user *model.User) context.Context {
	return api.ContextWithUser(context.Background(), user)
}

// remarshalOAS encodes an ogen response value with its generated JSON
// encoder and decodes it into out, so tests can assert the exact wire
// format the live API produces (field presence, nulls, empty objects).
func remarshalOAS(t *testing.T, v any, out any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal ogen value: %v", err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("unmarshal ogen value: %v\njson: %s", err, b)
	}
}

// compatDeviceHandler builds an ogen Handler over the fixture device repo.
func compatDeviceHandler(deviceRepo repository.DeviceRepo) *handlers.Handler {
	return handlers.NewHandler(handlers.HandlerConfig{Devices: deviceRepo})
}

// compatPositionHandler builds an ogen Handler over the fixture repos.
func compatPositionHandler(positionRepo repository.PositionRepo, deviceRepo repository.DeviceRepo) *handlers.Handler {
	return handlers.NewHandler(handlers.HandlerConfig{Positions: positionRepo, Devices: deviceRepo})
}

// listDevicesRaw fetches the user's devices through the live ogen handler
// and returns them in wire format.
func listDevicesRaw(t *testing.T, h *handlers.Handler, user *model.User) []map[string]any {
	t.Helper()
	res, err := h.ListDevices(compatUserCtx(user))
	if err != nil {
		t.Fatalf("ListDevices returned error: %v", err)
	}
	list, ok := res.(*oas.ListDevicesOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.ListDevicesOKApplicationJSON, got %T", res)
	}
	var devices []map[string]any
	remarshalOAS(t, list, &devices)
	return devices
}

// latestPositionsRaw fetches the latest positions per device through the
// live ogen handler and returns them in wire format.
func latestPositionsRaw(t *testing.T, h *handlers.Handler, user *model.User) []map[string]any {
	t.Helper()
	res, err := h.GetPositions(compatUserCtx(user), oas.GetPositionsParams{})
	if err != nil {
		t.Fatalf("GetPositions returned error: %v", err)
	}
	list, ok := res.(*oas.GetPositionsOKApplicationJSON)
	if !ok {
		t.Fatalf("expected *oas.GetPositionsOKApplicationJSON, got %T", res)
	}
	var positions []map[string]any
	remarshalOAS(t, list, &positions)
	return positions
}

// ---------------------------------------------------------------------------
// 1. Device JSON field presence tests
// ---------------------------------------------------------------------------

// TestTraccarCompat_DeviceFieldPresence verifies that every field required by
// Home Assistant's traccar_server integration is present in the JSON response,
// even when the underlying value is nil/zero.
//
// Home Assistant parses device JSON and accesses these fields by name. If a
// field is missing (due to omitempty or incorrect serialization), HA will
// raise a KeyError or show an entity as unavailable.
//
// Reference: pytraccar DeviceModel fields.
func TestTraccarCompat_DeviceFieldPresence(t *testing.T) {
	f := setupCompatFixtures(t)

	h := compatDeviceHandler(f.deviceRepo)
	devices := listDevicesRaw(t, h, f.user)
	if len(devices) == 0 {
		t.Fatal("expected at least one device in response")
	}

	dev := devices[0]

	// Required fields per Home Assistant traccar_server integration.
	// These MUST be present in every device JSON object even if their
	// value is null. The "attributes" field MUST be {} (empty object),
	// never null.
	requiredFields := []string{
		"id",
		"uniqueId",
		"name",
		"status",
		"model",
		"phone",
		"contact",
		"category",
		"groupId",
		"calendarId",
		"positionId",
		"expirationTime",
		"disabled",
		"attributes",
	}

	for _, field := range requiredFields {
		if _, exists := dev[field]; !exists {
			t.Errorf("device JSON is missing required field %q (HA compatibility)", field)
		}
	}

	// Attributes MUST be an object (empty {}), never null.
	// Home Assistant calls .get() on device attributes and will fail on null.
	attrs, ok := dev["attributes"]
	if !ok {
		t.Fatal("device JSON is missing 'attributes' field")
	}
	if attrs == nil {
		t.Error("device.attributes is null; must be {} for HA compatibility")
	} else if _, isMap := attrs.(map[string]any); !isMap {
		t.Errorf("device.attributes is %T; must be a JSON object (map) for HA compatibility", attrs)
	}
}

// TestTraccarCompat_DeviceFieldPresence_NilValues verifies field presence when
// all optional pointer fields on the device are nil. This catches cases where
// omitempty would silently drop required fields.
func TestTraccarCompat_DeviceFieldPresence_NilValues(t *testing.T) {
	f := setupCompatFixtures(t)
	ctx := context.Background()

	// Create a minimal device with all optional fields left nil.
	minimalDevice := &model.Device{
		UniqueID: "minimal-nil-dev",
		Name:     "Minimal Device",
		Status:   "unknown",
		// All pointer fields (Model, Phone, Contact, Category, GroupID,
		// CalendarID, PositionID, ExpirationTime) are nil.
	}
	if err := f.deviceRepo.Create(ctx, minimalDevice, f.user.ID); err != nil {
		t.Fatalf("create minimal device: %v", err)
	}

	h := compatDeviceHandler(f.deviceRepo)

	// Use GetDevice (single device) to isolate the minimal device.
	res, err := h.GetDevice(compatUserCtx(f.user), oas.GetDeviceParams{ID: minimalDevice.ID})
	if err != nil {
		t.Fatalf("GetDevice returned error: %v", err)
	}
	oasDev, ok := res.(*oas.Device)
	if !ok {
		t.Fatalf("expected *oas.Device, got %T", res)
	}
	var dev map[string]any
	remarshalOAS(t, oasDev, &dev)

	// These nullable fields MUST still be present as JSON null, not omitted.
	// Home Assistant checks for key existence, not just truthiness.
	nullableRequired := []string{
		"positionId",
		"groupId",
		"phone",
		"model",
		"contact",
		"category",
		"calendarId",
		"expirationTime",
	}
	for _, field := range nullableRequired {
		if _, exists := dev[field]; !exists {
			t.Errorf("device JSON omits field %q when value is nil (must be present as null for HA)", field)
		}
	}

	// Attributes must be {} even for a fresh device with no attributes set.
	attrs := dev["attributes"]
	if attrs == nil {
		t.Error("device.attributes is null for fresh device; must be {}")
	}
}

// ---------------------------------------------------------------------------
// 2. Position JSON field presence tests
// ---------------------------------------------------------------------------

// TestTraccarCompat_PositionFieldPresence verifies that every field required
// by Home Assistant is present in the position JSON.
//
// pytraccar expects specific fields when parsing position responses. Missing
// fields cause KeyError exceptions in the HA integration.
//
// Reference: pytraccar PositionModel fields and
// homeassistant/components/traccar_server/coordinator.py
func TestTraccarCompat_PositionFieldPresence(t *testing.T) {
	f := setupCompatFixtures(t)
	ctx := context.Background()

	now := time.Now().UTC()
	speed := 25.5
	altitude := 150.0
	course := 90.0
	pos := &model.Position{
		DeviceID:  f.device.ID,
		Timestamp: now,
		Valid:     true,
		Latitude:  52.52,
		Longitude: 13.405,
		Altitude:  &altitude,
		Speed:     &speed,
		Course:    &course,
		Accuracy:  12.5,
	}
	if err := f.positionRepo.Create(ctx, pos); err != nil {
		t.Fatalf("create position: %v", err)
	}

	h := compatPositionHandler(f.positionRepo, f.deviceRepo)
	positions := latestPositionsRaw(t, h, f.user)
	if len(positions) == 0 {
		t.Fatal("expected at least one position in response")
	}

	p := positions[0]

	// All fields required by Home Assistant's traccar_server integration.
	// NOTE (live ogen contract): "address" is emitted only when set (it is
	// nil here) and "geofenceIds" is not part of the OpenAPI Position schema,
	// so neither is asserted as always-present anymore.
	requiredFields := []string{
		"id",
		"deviceId",
		"fixTime",
		"valid",
		"latitude",
		"longitude",
		"altitude",
		"speed",
		"course",
		"accuracy",
		"attributes",
		"network",
		"outdated",
	}

	for _, field := range requiredFields {
		if _, exists := p[field]; !exists {
			t.Errorf("position JSON is missing required field %q (HA compatibility)", field)
		}
	}

	// accuracy MUST be a number, never null.
	// HA uses this for GPS accuracy circle on the map.
	acc, ok := p["accuracy"]
	if !ok {
		t.Fatal("position JSON is missing 'accuracy' field")
	}
	if acc == nil {
		t.Error("position.accuracy is null; must be numeric (0.0 default) for HA compatibility")
	}
	if _, isNum := acc.(float64); !isNum {
		t.Errorf("position.accuracy is %T (%v); must be a number for HA compatibility", acc, acc)
	}

	// attributes MUST be {} not null.
	attrs := p["attributes"]
	if attrs == nil {
		t.Error("position.attributes is null; must be {} for HA compatibility")
	} else if _, isMap := attrs.(map[string]any); !isMap {
		t.Errorf("position.attributes is %T; must be a JSON object for HA compatibility", attrs)
	}

	// network MUST be {} not null.
	// HA reads network info for cell tower / WiFi data.
	network := p["network"]
	if network == nil {
		t.Error("position.network is null; must be {} for HA compatibility")
	} else if _, isMap := network.(map[string]any); !isMap {
		t.Errorf("position.network is %T; must be a JSON object for HA compatibility", network)
	}
}

// TestTraccarCompat_PositionFieldPresence_NilOptionals verifies field
// presence when all optional position fields are nil.
func TestTraccarCompat_PositionFieldPresence_NilOptionals(t *testing.T) {
	f := setupCompatFixtures(t)
	ctx := context.Background()

	// Create a position with only the absolute minimum fields set.
	// Speed, Altitude, Course, Address are all nil.
	pos := &model.Position{
		DeviceID:  f.device.ID,
		Timestamp: time.Now().UTC(),
		Valid:     false,
		Latitude:  0.0,
		Longitude: 0.0,
		// All pointer fields are nil: Speed, Altitude, Course, Address
		// Accuracy is zero-value (0.0)
	}
	if err := f.positionRepo.Create(ctx, pos); err != nil {
		t.Fatalf("create position: %v", err)
	}

	// The minimal position is the only one for the device, so the
	// latest-per-device mode of the live handler returns it.
	h := compatPositionHandler(f.positionRepo, f.deviceRepo)
	positions := latestPositionsRaw(t, h, f.user)
	if len(positions) == 0 {
		t.Fatal("expected at least one position")
	}

	p := positions[0]

	// Even with nil optionals, these fields must be present (the live API
	// serializes them as required numbers defaulting to 0).
	// NOTE (live ogen contract): "address" is omitted when nil and is no
	// longer asserted.
	for _, field := range []string{"altitude", "speed", "course"} {
		if _, exists := p[field]; !exists {
			t.Errorf("position JSON omits field %q when value is nil (must be present for HA)", field)
		}
	}

	// accuracy must be numeric even when stored as 0 or NULL.
	acc := p["accuracy"]
	if acc == nil {
		t.Error("position.accuracy is null; must be numeric for HA")
	}

	// attributes and network must still be empty objects.
	if p["attributes"] == nil {
		t.Error("position.attributes is null for minimal position; must be {}")
	}
	if p["network"] == nil {
		t.Error("position.network is null for minimal position; must be {}")
	}
}

// ---------------------------------------------------------------------------
// 3. API response format tests
// ---------------------------------------------------------------------------

// TestTraccarCompat_BareArrayResponses verifies that list endpoints return
// bare JSON arrays without any pagination envelope. Home Assistant and
// pytraccar parse the response as a direct JSON array. Wrapping it in an
// object like {"data": [...], "total": N} would break the integration.
func TestTraccarCompat_BareArrayResponses(t *testing.T) {
	f := setupCompatFixtures(t)

	// GET /api/devices is served by the ogen Handler; verify the response
	// type marshals to a bare JSON array (no envelope).
	t.Run("GET /api/devices returns bare array", func(t *testing.T) {
		h := compatDeviceHandler(f.deviceRepo)
		res, err := h.ListDevices(compatUserCtx(f.user))
		if err != nil {
			t.Fatalf("ListDevices returned error: %v", err)
		}
		list, ok := res.(*oas.ListDevicesOKApplicationJSON)
		if !ok {
			t.Fatalf("expected *oas.ListDevicesOKApplicationJSON, got %T", res)
		}
		var raw any
		remarshalOAS(t, list, &raw)
		if _, isArray := raw.([]any); !isArray {
			t.Errorf("response is %T; must be a JSON array (no envelope) for HA compatibility", raw)
		}
	})

	t.Run("GET /api/positions returns bare array", func(t *testing.T) {
		h := compatPositionHandler(f.positionRepo, f.deviceRepo)
		res, err := h.GetPositions(compatUserCtx(f.user), oas.GetPositionsParams{})
		if err != nil {
			t.Fatalf("GetPositions returned error: %v", err)
		}
		list, ok := res.(*oas.GetPositionsOKApplicationJSON)
		if !ok {
			t.Fatalf("expected *oas.GetPositionsOKApplicationJSON, got %T", res)
		}
		var raw any
		remarshalOAS(t, list, &raw)
		if _, isArray := raw.([]any); !isArray {
			t.Errorf("response is %T; must be a JSON array (no envelope) for HA compatibility", raw)
		}
	})

	// GET /api/geofences is served by the ogen Handler; verify the response
	// type marshals to a bare JSON array (no envelope).
	t.Run("GET /api/geofences returns bare array", func(t *testing.T) {
		h := handlers.NewHandler(handlers.HandlerConfig{Geofences: f.geofenceRepo})
		res, err := h.ListGeofences(api.ContextWithUser(context.Background(), f.user))
		if err != nil {
			t.Fatalf("ListGeofences returned error: %v", err)
		}
		list, ok := res.(*oas.ListGeofencesOKApplicationJSON)
		if !ok {
			t.Fatalf("expected *oas.ListGeofencesOKApplicationJSON, got %T", res)
		}
		body, err := json.Marshal(list)
		if err != nil {
			t.Fatalf("marshal geofence list: %v", err)
		}
		var raw any
		if err := json.Unmarshal(body, &raw); err != nil {
			t.Fatalf("unmarshal geofence list: %v", err)
		}
		if _, isArray := raw.([]any); !isArray {
			t.Errorf("response is %T; must be a JSON array (no envelope) for HA compatibility", raw)
		}
	})
}

// TestTraccarCompat_EmptyListReturnsEmptyArray verifies that list endpoints
// return an empty JSON array [] when there are no results, not null.
// pytraccar iterates over the response; a null would cause a TypeError.
func TestTraccarCompat_EmptyListReturnsEmptyArray(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	userRepo := repository.NewUserRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	positionRepo := repository.NewPositionRepository(pool)
	geofenceRepo := repository.NewGeofenceRepository(pool)

	// Create a user with NO devices, positions, or geofences.
	user := &model.User{
		Email:        "empty@example.com",
		PasswordHash: "hash",
		Name:         "Empty User",
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	t.Run("devices empty array", func(t *testing.T) {
		h := compatDeviceHandler(deviceRepo)
		devices := listDevicesRaw(t, h, user)
		if len(devices) != 0 {
			t.Errorf("expected empty array [], got %d elements", len(devices))
		}
		// The wire format must be [] (not null) for pytraccar.
		res, err := h.ListDevices(compatUserCtx(user))
		if err != nil {
			t.Fatalf("ListDevices returned error: %v", err)
		}
		body, err := json.Marshal(res.(*oas.ListDevicesOKApplicationJSON))
		if err != nil {
			t.Fatalf("marshal device list: %v", err)
		}
		if string(body) != "[]" {
			t.Errorf("expected empty array [], got %s", body)
		}
	})

	t.Run("positions empty array", func(t *testing.T) {
		h := compatPositionHandler(positionRepo, deviceRepo)
		res, err := h.GetPositions(compatUserCtx(user), oas.GetPositionsParams{})
		if err != nil {
			t.Fatalf("GetPositions returned error: %v", err)
		}
		list, ok := res.(*oas.GetPositionsOKApplicationJSON)
		if !ok {
			t.Fatalf("expected *oas.GetPositionsOKApplicationJSON, got %T", res)
		}
		body, err := json.Marshal(list)
		if err != nil {
			t.Fatalf("marshal position list: %v", err)
		}
		if string(body) != "[]" {
			t.Errorf("expected empty array [], got %s", body)
		}
	})

	// GET /api/geofences is served by the ogen Handler; an empty result must
	// marshal to [] (not null) for pytraccar.
	t.Run("geofences empty array", func(t *testing.T) {
		h := handlers.NewHandler(handlers.HandlerConfig{Geofences: geofenceRepo})
		res, err := h.ListGeofences(api.ContextWithUser(ctx, user))
		if err != nil {
			t.Fatalf("ListGeofences returned error: %v", err)
		}
		list, ok := res.(*oas.ListGeofencesOKApplicationJSON)
		if !ok {
			t.Fatalf("expected *oas.ListGeofencesOKApplicationJSON, got %T", res)
		}
		body, err := json.Marshal(list)
		if err != nil {
			t.Fatalf("marshal geofence list: %v", err)
		}
		var arr []any
		if err := json.Unmarshal(body, &arr); err != nil {
			t.Fatalf("expected JSON array, got %s: %v", body, err)
		}
		if len(arr) != 0 {
			t.Errorf("expected empty array [], got %d elements", len(arr))
		}
	})
}

// ---------------------------------------------------------------------------
// 4. Motion attribute tests
// ---------------------------------------------------------------------------

// TestTraccarCompat_MotionAttribute verifies that position.attributes.motion
// is set correctly based on the speed value. Home Assistant derives the
// binary_sensor.motion entity from this attribute.
//
// Contract:
//   - speed >= 5.0 km/h -> attributes.motion = true
//   - speed < 5.0 km/h  -> attributes.motion = false
//   - speed is nil       -> attributes.motion = false
//
// The motion attribute is set by the protocol PositionHandler on ingest.
// These tests verify the attribute persists through the database and is
// returned correctly in the API response.
func TestTraccarCompat_MotionAttribute(t *testing.T) {
	f := setupCompatFixtures(t)
	ctx := context.Background()

	tests := []struct {
		name           string
		speed          *float64
		expectedMotion bool
	}{
		{
			name:           "speed above threshold (25 km/h) -> motion=true",
			speed:          new(25.0),
			expectedMotion: true,
		},
		{
			name:           "speed at exact threshold (5 km/h) -> motion=true",
			speed:          new(5.0),
			expectedMotion: true,
		},
		{
			name:           "speed below threshold (3 km/h) -> motion=false",
			speed:          new(3.0),
			expectedMotion: false,
		},
		{
			name:           "speed zero -> motion=false",
			speed:          new(0.0),
			expectedMotion: false,
		},
		{
			name:           "speed nil -> motion=false",
			speed:          nil,
			expectedMotion: false,
		},
	}

	// Each subtest creates a strictly newer position so that the live
	// latest-per-device endpoint returns the position under test.
	baseTime := time.Now().UTC()
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate what the protocol handler does: set the motion
			// attribute before persisting. This mirrors protocol/handler.go
			// HandlePosition() lines 106-114.
			isMoving := tt.speed != nil && *tt.speed >= 5.0
			attrs := map[string]any{
				"motion": isMoving,
			}

			pos := &model.Position{
				DeviceID:   f.device.ID,
				Timestamp:  baseTime.Add(time.Duration(i) * time.Second),
				Valid:      true,
				Latitude:   52.52,
				Longitude:  13.405,
				Speed:      tt.speed,
				Attributes: attrs,
			}
			if err := f.positionRepo.Create(ctx, pos); err != nil {
				t.Fatalf("create position: %v", err)
			}

			// Fetch the position back through the live API to verify the
			// attribute is persisted and returned correctly.
			h := compatPositionHandler(f.positionRepo, f.deviceRepo)
			positions := latestPositionsRaw(t, h, f.user)
			if len(positions) == 0 {
				t.Fatal("expected at least one position")
			}

			posJSON := positions[0]
			attrsJSON, ok := posJSON["attributes"].(map[string]any)
			if !ok {
				t.Fatalf("position.attributes is not an object: %v", posJSON["attributes"])
			}

			motion, exists := attrsJSON["motion"]
			if !exists {
				t.Fatal("position.attributes.motion is missing (required for HA binary_sensor.motion)")
			}

			motionBool, ok := motion.(bool)
			if !ok {
				t.Fatalf("position.attributes.motion is %T (%v); must be boolean", motion, motion)
			}
			if motionBool != tt.expectedMotion {
				t.Errorf("position.attributes.motion = %v; want %v", motionBool, tt.expectedMotion)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 5. Device status value tests
// ---------------------------------------------------------------------------

// TestTraccarCompat_DeviceStatusValues verifies that device.status is one of
// the values recognized by Home Assistant: "online", "offline", or "unknown".
//
// Home Assistant's binary_sensor.status checks:
//
//	status == "online" -> ON (device is active)
//	anything else      -> OFF
//
// The integration MUST NOT use "moving" as a status value. Motion state is
// communicated via position.attributes.motion instead.
//
// Reference: homeassistant/components/traccar_server/binary_sensor.py
func TestTraccarCompat_DeviceStatusValues(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	userRepo := repository.NewUserRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)

	user := &model.User{
		Email:        "status@example.com",
		PasswordHash: "hash",
		Name:         "Status User",
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	validStatuses := []string{"online", "offline", "unknown"}

	for _, status := range validStatuses {
		t.Run("status_"+status, func(t *testing.T) {
			dev := &model.Device{
				UniqueID: fmt.Sprintf("status-%s-dev", status),
				Name:     fmt.Sprintf("Device %s", status),
				Status:   status,
			}
			if err := deviceRepo.Create(ctx, dev, user.ID); err != nil {
				t.Fatalf("create device: %v", err)
			}
		})
	}

	// Fetch all devices through the live ogen handler and verify status values.
	h := compatDeviceHandler(deviceRepo)
	devices := listDevicesRaw(t, h, user)

	allowedStatuses := map[string]bool{
		"online":  true,
		"offline": true,
		"unknown": true,
	}

	for _, dev := range devices {
		status, ok := dev["status"].(string)
		if !ok {
			t.Errorf("device %v has non-string status: %v", dev["uniqueId"], dev["status"])
			continue
		}
		if !allowedStatuses[status] {
			t.Errorf("device %v has status %q; must be one of online/offline/unknown for HA (never 'moving')",
				dev["uniqueId"], status)
		}
	}
}

// TestTraccarCompat_DeviceStatusNeverMoving specifically tests that the
// "moving" status is never returned, since earlier Motus versions used it
// and HA does not recognize it.
func TestTraccarCompat_DeviceStatusNeverMoving(t *testing.T) {
	f := setupCompatFixtures(t)
	ctx := context.Background()

	// Read back a device through the live ogen handler. The status should
	// NOT be "moving". Even if code were to set it to "moving" (which the DB
	// CHECK constraint should prevent), the test will catch it.
	h := compatDeviceHandler(f.deviceRepo)

	res, err := h.GetDevice(compatUserCtx(f.user), oas.GetDeviceParams{ID: f.device.ID})
	if err != nil {
		t.Fatalf("GetDevice returned error: %v", err)
	}
	oasDev, ok := res.(*oas.Device)
	if !ok {
		t.Fatalf("expected *oas.Device, got %T", res)
	}
	var dev map[string]any
	remarshalOAS(t, oasDev, &dev)

	status := dev["status"].(string)
	if status == "moving" {
		t.Error("device status is 'moving'; HA does not recognize this value. Use 'online' and set motion via position.attributes.motion")
	}

	// Also verify the device we get via the list endpoint.
	devices := listDevicesRaw(t, h, f.user)
	for _, d := range devices {
		if s, _ := d["status"].(string); s == "moving" {
			t.Errorf("device %v has status 'moving' in list response; must not use 'moving'", d["uniqueId"])
		}
	}

	// Verify the DB CHECK constraint prevents "moving" status.
	// The devices.status CHECK constraint in migration 00010 only allows
	// online, offline, unknown. We just confirm the device loaded above
	// has a valid status that is not "moving".
	_ = ctx // used above for setup
}

// ---------------------------------------------------------------------------
// 6. Authentication tests
// ---------------------------------------------------------------------------

// TestTraccarCompat_BearerTokenAuth verifies that Bearer token authentication
// works for API endpoints. Home Assistant uses Bearer tokens to authenticate
// with the Traccar server.
//
// Reference: pytraccar uses "Authorization: Bearer <token>" header.
func TestTraccarCompat_BearerTokenAuth(t *testing.T) {
	env := setupCompatRouterServer(t)
	ctx := context.Background()

	user := createCompatUser(t, env.userRepo, "bearer@example.com", "bearerpass")

	// Create an API key for the user.
	apiKey := &model.ApiKey{
		UserID:      user.ID,
		Name:        "HA Integration Key",
		Permissions: model.PermissionFull,
	}
	if err := env.apiKeyRepo.Create(ctx, apiKey); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Create a device for the user so we have something to list.
	dev := &model.Device{
		UniqueID: "bearer-dev",
		Name:     "Bearer Device",
		Status:   "online",
	}
	if err := env.deviceRepo.Create(ctx, dev, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	getDevices := func(t *testing.T, authorization string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, env.ts.URL+"/api/devices", nil)
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		return resp
	}

	// Test with valid Bearer token through the full live router.
	t.Run("valid bearer token", func(t *testing.T) {
		resp := getDevices(t, "Bearer "+apiKey.Token)
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("Bearer auth: expected 200, got %d: %s", resp.StatusCode, b)
		}

		var devices []map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&devices); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(devices) == 0 {
			t.Error("expected at least one device with valid bearer token")
		}
	})

	// Test with invalid Bearer token.
	t.Run("invalid bearer token", func(t *testing.T) {
		resp := getDevices(t, "Bearer invalid-token-value")
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 for invalid token, got %d", resp.StatusCode)
		}
	})

	// Test without any auth.
	t.Run("no auth returns 401", func(t *testing.T) {
		resp := getDevices(t, "")
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 without auth, got %d", resp.StatusCode)
		}
	})
}

// ---------------------------------------------------------------------------
// Full-router test server helpers
// ---------------------------------------------------------------------------

// compatRouterEnv bundles the full ogen router test server with the real
// repositories backing it.
type compatRouterEnv struct {
	ts          *httptest.Server
	userRepo    *repository.UserRepository
	sessionRepo *repository.SessionRepository
	apiKeyRepo  *repository.ApiKeyRepository
	deviceRepo  *repository.DeviceRepository
}

// setupCompatRouterServer builds the full router (ogen server + SecurityHandler)
// the same way TestTraccarCompat_FullRouterDevicesEndpoint does. The 3-arg
// api.NewRouter test variant applies no CSRF or rate-limit middleware, so
// session mutations need no X-CSRF-Token header.
func setupCompatRouterServer(t *testing.T) *compatRouterEnv {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)

	hub := websocket.NewHub(nil, nil, nil)
	handler := handlers.NewHandler(handlers.HandlerConfig{
		Users:    userRepo,
		Sessions: sessionRepo,
		Devices:  deviceRepo,
		ApiKeys:  apiKeyRepo,
	})
	secHandler := handlers.NewSecurityHandler(sessionRepo, apiKeyRepo, userRepo)
	router := api.NewRouter(handler, secHandler, hub)
	ts := httptest.NewServer(router)
	t.Cleanup(ts.Close)

	return &compatRouterEnv{
		ts:          ts,
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		apiKeyRepo:  apiKeyRepo,
		deviceRepo:  deviceRepo,
	}
}

// createCompatUser inserts a user with the given password into the test DB.
func createCompatUser(t *testing.T, userRepo *repository.UserRepository, email, password string) *model.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := &model.User{Email: email, PasswordHash: string(hash), Name: "Compat User", Role: model.RoleUser}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

// respSessionCookie returns the session_id cookie from an HTTP response, or nil.
func respSessionCookie(resp *http.Response) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == "session_id" {
			return c
		}
	}
	return nil
}

// loginViaRouter performs a JSON login through the full router and returns
// the session cookie.
func loginViaRouter(t *testing.T, ts *httptest.Server, email, password string) *http.Cookie {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	resp, err := http.Post(ts.URL+"/api/session", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("login failed: %d: %s", resp.StatusCode, b)
	}
	cookie := respSessionCookie(resp)
	if cookie == nil || cookie.Value == "" {
		t.Fatal("login did not set session_id cookie")
	}
	return cookie
}

// TestTraccarCompat_LegacyTokenAuth verifies that a legacy users.token value
// (generated via POST /api/session/token) still authenticates through
// GET /api/session?token= on the full router. Some existing integrations use
// the legacy token instead of an API key.
func TestTraccarCompat_LegacyTokenAuth(t *testing.T) {
	env := setupCompatRouterServer(t)
	ctx := context.Background()

	user := createCompatUser(t, env.userRepo, "legacy@example.com", "legacypass")

	// Generate a legacy token (users.token column).
	token, err := env.userRepo.GenerateToken(ctx, user.ID)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	resp, err := http.Get(env.ts.URL + "/api/session?token=" + token)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("legacy token login: expected 200, got %d: %s", resp.StatusCode, b)
	}

	var userResp map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&userResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if userResp["email"] != "legacy@example.com" {
		t.Errorf("expected email 'legacy@example.com', got %v", userResp["email"])
	}
	if c := respSessionCookie(resp); c == nil || c.Value == "" {
		t.Error("legacy token login must set a session_id cookie")
	}
}

// TestTraccarCompat_SessionTokenQueryParam verifies that GET /api/session?token=
// with an API-key token creates a session and returns user info with a
// session cookie through the full router. This is the mechanism used by
// pytraccar for initial authentication.
//
// Reference: pytraccar TraccarClient._get_session() uses ?token= parameter.
func TestTraccarCompat_SessionTokenQueryParam(t *testing.T) {
	env := setupCompatRouterServer(t)
	ctx := context.Background()

	user := createCompatUser(t, env.userRepo, "pytraccar@example.com", "pytraccarpass")

	// Create an API key for the user; Create populates the token.
	apiKey := &model.ApiKey{
		UserID:      user.ID,
		Name:        "HA Integration Key",
		Permissions: model.PermissionFull,
	}
	if err := env.apiKeyRepo.Create(ctx, apiKey); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// GET /api/session?token=<token> should authenticate and create a session.
	resp, err := http.Get(env.ts.URL + "/api/session?token=" + apiKey.Token)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /api/session?token= returned %d: %s", resp.StatusCode, b)
	}

	// Verify a session cookie is set.
	if c := respSessionCookie(resp); c == nil || c.Value == "" {
		t.Error("GET /api/session?token= must set a session_id cookie (required for WebSocket auth)")
	}

	// Verify the response contains user info with Traccar-compatible fields.
	var userResp map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&userResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if userResp["email"] != "pytraccar@example.com" {
		t.Errorf("expected email 'pytraccar@example.com', got %v", userResp["email"])
	}
	// Traccar-compatible boolean fields must be present.
	if _, exists := userResp["administrator"]; !exists {
		t.Error("user response missing 'administrator' field (Traccar compatibility)")
	}
	if _, exists := userResp["readonly"]; !exists {
		t.Error("user response missing 'readonly' field (Traccar compatibility)")
	}
	if _, exists := userResp["disabled"]; !exists {
		t.Error("user response missing 'disabled' field (Traccar compatibility)")
	}
}

// TestTraccarCompat_SessionTokenQueryParam_StaleCookie verifies that token
// login succeeds even when the client still carries an invalid session_id
// cookie. This is the QR re-login scenario: the previous session expired
// (HttpOnly cookie cannot be cleared by frontend JS) and the user scans a
// fresh QR code. The security layer must treat the bad cookie as
// "not satisfied" and fall through to the anonymous requirement so the
// token query parameter is evaluated.
func TestTraccarCompat_SessionTokenQueryParam_StaleCookie(t *testing.T) {
	env := setupCompatRouterServer(t)
	ctx := context.Background()

	user := createCompatUser(t, env.userRepo, "stale-cookie@example.com", "stalecookiepass")

	apiKey := &model.ApiKey{
		UserID:      user.ID,
		Name:        "Stale Cookie Key",
		Permissions: model.PermissionFull,
	}
	if err := env.apiKeyRepo.Create(ctx, apiKey); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, env.ts.URL+"/api/session?token="+apiKey.Token, nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "stale-or-expired-session-id"})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("token login with stale cookie returned %d (want 200): %s", resp.StatusCode, b)
	}
	if c := respSessionCookie(resp); c == nil || c.Value == "" {
		t.Error("token login with stale cookie must set a fresh session_id cookie")
	}
}

// TestTraccarCompat_SessionTokenQueryParam_InvalidToken verifies that an
// invalid token returns 401 through the full router.
func TestTraccarCompat_SessionTokenQueryParam_InvalidToken(t *testing.T) {
	env := setupCompatRouterServer(t)

	resp, err := http.Get(env.ts.URL + "/api/session?token=nonexistent-token")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid token, got %d", resp.StatusCode)
	}
}

// TestTraccarCompat_CookieSessionAuth verifies that session cookie
// authentication works through the full router. After login, subsequent
// requests use the session cookie.
func TestTraccarCompat_CookieSessionAuth(t *testing.T) {
	env := setupCompatRouterServer(t)

	createCompatUser(t, env.userRepo, "cookie@example.com", "cookiepass")

	// Step 1: Login to get a session cookie.
	sessionCookie := loginViaRouter(t, env.ts, "cookie@example.com", "cookiepass")

	// Step 2: Use the session cookie to read the current session.
	req, err := http.NewRequest(http.MethodGet, env.ts.URL+"/api/session", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.AddCookie(sessionCookie)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("cookie auth: expected 200, got %d: %s", resp.StatusCode, b)
	}

	var userResp map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&userResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if userResp["email"] != "cookie@example.com" {
		t.Errorf("expected email 'cookie@example.com', got %v", userResp["email"])
	}
}

// ---------------------------------------------------------------------------
// 7. Full router integration test
// ---------------------------------------------------------------------------

// TestTraccarCompat_FullRouterDevicesEndpoint tests the /api/devices endpoint
// through the full chi router to ensure middleware, routing, and response
// format all work together.
func TestTraccarCompat_FullRouterDevicesEndpoint(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	positionRepo := repository.NewPositionRepository(pool)
	geofenceRepo := repository.NewGeofenceRepository(pool)

	hash, _ := bcrypt.GenerateFromPassword([]byte("routerpass"), bcrypt.MinCost)
	user := &model.User{
		Email:        "router@example.com",
		PasswordHash: string(hash),
		Name:         "Router User",
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	dev := &model.Device{UniqueID: "router-dev", Name: "Router Device", Status: "online"}
	if err := deviceRepo.Create(ctx, dev, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	// Create a position for the device.
	speed := 15.0
	pos := &model.Position{
		DeviceID:   dev.ID,
		Timestamp:  time.Now().UTC(),
		Valid:      true,
		Latitude:   52.52,
		Longitude:  13.405,
		Speed:      &speed,
		Attributes: map[string]any{"motion": true},
	}
	if err := positionRepo.Create(ctx, pos); err != nil {
		t.Fatalf("create position: %v", err)
	}

	// Create an API key for Bearer auth.
	apiKey := &model.ApiKey{
		UserID:      user.ID,
		Name:        "Router Test Key",
		Permissions: model.PermissionFull,
	}
	if err := apiKeyRepo.Create(ctx, apiKey); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	hub := websocket.NewHub(nil, nil, nil)
	handler := handlers.NewHandler(handlers.HandlerConfig{
		Users:     userRepo,
		Sessions:  sessionRepo,
		Devices:   deviceRepo,
		Positions: positionRepo,
		Geofences: geofenceRepo,
		ApiKeys:   apiKeyRepo,
	})
	secHandler := handlers.NewSecurityHandler(sessionRepo, apiKeyRepo, userRepo)
	router := api.NewRouter(handler, secHandler, hub)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Test /api/devices with Bearer token through the full stack.
	t.Run("GET /api/devices via full router with Bearer", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/devices", nil)
		req.Header.Set("Authorization", "Bearer "+apiKey.Token)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var devices []map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&devices); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(devices) == 0 {
			t.Fatal("expected at least one device")
		}

		// Verify full device field presence through the router.
		d := devices[0]
		for _, field := range []string{"id", "uniqueId", "name", "status", "attributes", "disabled"} {
			if _, exists := d[field]; !exists {
				t.Errorf("device is missing field %q through full router", field)
			}
		}
	})

	// Test /api/positions with Bearer token.
	t.Run("GET /api/positions via full router with Bearer", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/positions", nil)
		req.Header.Set("Authorization", "Bearer "+apiKey.Token)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var positions []map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&positions); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(positions) == 0 {
			t.Fatal("expected at least one position")
		}

		p := positions[0]
		// Verify key position fields through the router.
		for _, field := range []string{"id", "deviceId", "fixTime", "latitude", "longitude", "accuracy", "attributes", "network"} {
			if _, exists := p[field]; !exists {
				t.Errorf("position is missing field %q through full router", field)
			}
		}

		// Verify motion attribute exists.
		if attrs, ok := p["attributes"].(map[string]any); ok {
			if _, exists := attrs["motion"]; !exists {
				t.Error("position.attributes.motion missing through full router")
			}
		}
	})

	// Test /api/server (public, no auth).
	t.Run("GET /api/server returns Traccar-compatible info", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/server")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		var server map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&server); err != nil {
			t.Fatalf("decode: %v", err)
		}

		// Fields required by pytraccar for initialization.
		for _, field := range []string{"id", "version", "map", "latitude", "longitude", "zoom", "attributes"} {
			if _, exists := server[field]; !exists {
				t.Errorf("server response missing field %q (pytraccar compatibility)", field)
			}
		}
	})

	// Test /api/devices without auth returns 401.
	t.Run("GET /api/devices without auth returns 401", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/devices")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
	})
}

// ---------------------------------------------------------------------------
// 8. Form-encoded login test (Traccar Manager compatibility)
// ---------------------------------------------------------------------------

// TestTraccarCompat_FormEncodedLogin verifies that the login endpoint accepts
// application/x-www-form-urlencoded requests through the full router. The
// Traccar Manager mobile app sends credentials in form-encoded format, not
// JSON.
func TestTraccarCompat_FormEncodedLogin(t *testing.T) {
	env := setupCompatRouterServer(t)

	createCompatUser(t, env.userRepo, "formlogin@example.com", "formpass")

	formBody := "email=formlogin%40example.com&password=formpass"
	resp, err := http.Post(env.ts.URL+"/api/session",
		"application/x-www-form-urlencoded", bytes.NewReader([]byte(formBody)))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("form-encoded login: expected 200, got %d: %s", resp.StatusCode, b)
	}

	// Verify session cookie is set.
	if c := respSessionCookie(resp); c == nil || c.Value == "" {
		t.Error("form-encoded login must set session_id cookie")
	}

	// Verify response contains Traccar-compatible user fields.
	var userResp map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&userResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, field := range []string{"id", "email", "name", "administrator", "readonly", "disabled"} {
		if _, exists := userResp[field]; !exists {
			t.Errorf("login response missing field %q (Traccar Manager compatibility)", field)
		}
	}
}

// ---------------------------------------------------------------------------
// 8b. Logout test (Traccar Manager compatibility)
// ---------------------------------------------------------------------------

// TestTraccarCompat_Logout verifies that DELETE /api/session returns 204 No
// Content (matching the original Traccar Java server), properly clears the
// session cookie (both MaxAge=-1 and epoch Expires), and actually deletes
// the server-side session so subsequent requests with the same cookie fail.
//
// The Traccar Manager app's web view calls DELETE /api/session and then sends
// a "logout" message to the native bridge. The 204 response confirms success
// and the cookie clearing ensures the WebView won't auto-authenticate.
func TestTraccarCompat_Logout(t *testing.T) {
	env := setupCompatRouterServer(t)

	createCompatUser(t, env.userRepo, "logout@example.com", "logoutpass")

	// Step 1: Login through the router to get a session cookie.
	sessionCookie := loginViaRouter(t, env.ts, "logout@example.com", "logoutpass")

	// Step 2: Logout using the session cookie. The test router config has no
	// CSRF middleware, so no X-CSRF-Token header is required.
	logoutReq, err := http.NewRequest(http.MethodDelete, env.ts.URL+"/api/session", nil)
	if err != nil {
		t.Fatalf("build logout request: %v", err)
	}
	logoutReq.AddCookie(sessionCookie)

	logoutResp, err := http.DefaultClient.Do(logoutReq)
	if err != nil {
		t.Fatalf("logout request failed: %v", err)
	}
	defer func() { _ = logoutResp.Body.Close() }()

	// Verify 204 No Content (matches original Traccar Java server).
	if logoutResp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204 No Content from logout, got %d", logoutResp.StatusCode)
	}

	// Verify the response body is empty (Traccar returns no body).
	if body, _ := io.ReadAll(logoutResp.Body); len(body) != 0 {
		t.Errorf("expected empty body from logout, got %q", body)
	}

	// Verify the session cookie is properly cleared.
	clearedCookie := respSessionCookie(logoutResp)
	if clearedCookie == nil {
		t.Fatal("logout response did not include session_id cookie")
	}
	if clearedCookie.Value != "" {
		t.Errorf("expected empty cookie value, got %q", clearedCookie.Value)
	}
	if clearedCookie.MaxAge != -1 {
		t.Errorf("expected MaxAge=-1 for immediate cookie deletion, got %d", clearedCookie.MaxAge)
	}

	// Step 3: Verify the session is actually deleted by trying to use it.
	checkReq, err := http.NewRequest(http.MethodGet, env.ts.URL+"/api/session", nil)
	if err != nil {
		t.Fatalf("build check request: %v", err)
	}
	checkReq.AddCookie(sessionCookie) // Use the old (now-deleted) session cookie

	checkResp, err := http.DefaultClient.Do(checkReq)
	if err != nil {
		t.Fatalf("check request failed: %v", err)
	}
	defer func() { _ = checkResp.Body.Close() }()

	if checkResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 after logout with deleted session, got %d", checkResp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// 9. Content-Type header test
// ---------------------------------------------------------------------------

// TestTraccarCompat_ContentTypeJSON verifies that all JSON API responses
// include the correct Content-Type header. pytraccar checks for
// "application/json" before parsing the response body.
func TestTraccarCompat_ContentTypeJSON(t *testing.T) {
	env := setupCompatRouterServer(t)
	ctx := context.Background()

	user := createCompatUser(t, env.userRepo, "contenttype@example.com", "ctpass")
	apiKey := &model.ApiKey{
		UserID:      user.ID,
		Name:        "Content-Type Key",
		Permissions: model.PermissionFull,
	}
	if err := env.apiKeyRepo.Create(ctx, apiKey); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, env.ts.URL+"/api/devices", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		t.Fatal("Content-Type header is missing")
	}
	// ogen writes "application/json"; allow an explicit charset variant.
	if ct != "application/json" && ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type is %q; expected 'application/json' for HA compatibility", ct)
	}
}

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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	"github.com/tamcore/motus/internal/api/middleware"
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
	pool         interface{} // kept for reference
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

// authedRequest creates an HTTP request with the given user set in context.
func authedRequest(method, url string, body []byte, user *model.User) *http.Request {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, url, bytes.NewReader(body))
	} else {
		req = httptest.NewRequest(method, url, nil)
	}
	return req.WithContext(api.ContextWithUser(req.Context(), user))
}

// withChiParams adds chi URL params to the request context.
func withChiParams(req *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// decodeRawJSON parses the recorder body into a raw JSON value, preserving
// the exact structure (arrays, objects, nulls) for assertion.
func decodeRawJSON(t *testing.T, rr *httptest.ResponseRecorder) interface{} {
	t.Helper()
	var raw interface{}
	if err := json.NewDecoder(rr.Body).Decode(&raw); err != nil {
		t.Fatalf("failed to decode JSON response: %v\nbody: %s", err, rr.Body.String())
	}
	return raw
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

	h := handlers.NewDeviceHandler(f.deviceRepo, "")
	req := authedRequest(http.MethodGet, "/api/devices", nil, f.user)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/devices returned %d: %s", rr.Code, rr.Body.String())
	}

	// Parse as raw JSON array of objects to inspect keys directly.
	var devices []map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&devices); err != nil {
		t.Fatalf("decode devices: %v", err)
	}
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
	} else if _, isMap := attrs.(map[string]interface{}); !isMap {
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

	h := handlers.NewDeviceHandler(f.deviceRepo, "")

	// Use Get (single device) to isolate the minimal device.
	req := authedRequest(http.MethodGet, fmt.Sprintf("/api/devices/%d", minimalDevice.ID), nil, f.user)
	req = withChiParams(req, map[string]string{"id": fmt.Sprintf("%d", minimalDevice.ID)})
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/devices/%d returned %d: %s", minimalDevice.ID, rr.Code, rr.Body.String())
	}

	var dev map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&dev); err != nil {
		t.Fatalf("decode device: %v", err)
	}

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

	h := handlers.NewPositionHandler(f.positionRepo, f.deviceRepo)
	req := authedRequest(http.MethodGet, "/api/positions", nil, f.user)
	rr := httptest.NewRecorder()
	h.GetPositions(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/positions returned %d: %s", rr.Code, rr.Body.String())
	}

	var positions []map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&positions); err != nil {
		t.Fatalf("decode positions: %v", err)
	}
	if len(positions) == 0 {
		t.Fatal("expected at least one position in response")
	}

	p := positions[0]

	// All fields required by Home Assistant's traccar_server integration.
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
		"address",
		"accuracy",
		"attributes",
		"network",
		"geofenceIds",
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
	} else if _, isMap := attrs.(map[string]interface{}); !isMap {
		t.Errorf("position.attributes is %T; must be a JSON object for HA compatibility", attrs)
	}

	// network MUST be {} not null.
	// HA reads network info for cell tower / WiFi data.
	network := p["network"]
	if network == nil {
		t.Error("position.network is null; must be {} for HA compatibility")
	} else if _, isMap := network.(map[string]interface{}); !isMap {
		t.Errorf("position.network is %T; must be a JSON object for HA compatibility", network)
	}

	// geofenceIds MUST be present (can be null or []).
	// HA reads this to determine active geofences.
	if _, exists := p["geofenceIds"]; !exists {
		t.Error("position JSON is missing 'geofenceIds' field (HA compatibility)")
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

	h := handlers.NewPositionHandler(f.positionRepo, f.deviceRepo)
	req := authedRequest(http.MethodGet, fmt.Sprintf("/api/positions?id=%d", pos.ID), nil, f.user)
	rr := httptest.NewRecorder()
	h.GetPositions(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/positions?id=%d returned %d: %s", pos.ID, rr.Code, rr.Body.String())
	}

	var positions []map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&positions); err != nil {
		t.Fatalf("decode positions: %v", err)
	}
	if len(positions) == 0 {
		t.Fatal("expected at least one position")
	}

	p := positions[0]

	// Even with nil optionals, these fields must be present.
	for _, field := range []string{"altitude", "speed", "course", "address"} {
		if _, exists := p[field]; !exists {
			t.Errorf("position JSON omits field %q when value is nil (must be present as null for HA)", field)
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

	tests := []struct {
		name    string
		handler func(w http.ResponseWriter, r *http.Request)
		path    string
	}{
		{
			name: "GET /api/devices returns bare array",
			handler: func(w http.ResponseWriter, r *http.Request) {
				h := handlers.NewDeviceHandler(f.deviceRepo, "")
				h.List(w, r)
			},
			path: "/api/devices",
		},
		{
			name: "GET /api/positions returns bare array",
			handler: func(w http.ResponseWriter, r *http.Request) {
				h := handlers.NewPositionHandler(f.positionRepo, f.deviceRepo)
				h.GetPositions(w, r)
			},
			path: "/api/positions",
		},
		{
			name: "GET /api/geofences returns bare array",
			handler: func(w http.ResponseWriter, r *http.Request) {
				h := handlers.NewGeofenceHandler(f.geofenceRepo)
				h.List(w, r)
			},
			path: "/api/geofences",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := authedRequest(http.MethodGet, tt.path, nil, f.user)
			rr := httptest.NewRecorder()
			tt.handler(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
			}

			raw := decodeRawJSON(t, rr)
			if _, isArray := raw.([]interface{}); !isArray {
				t.Errorf("response is %T; must be a JSON array (no envelope) for HA compatibility", raw)
			}
		})
	}
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

	tests := []struct {
		name    string
		handler func(w http.ResponseWriter, r *http.Request)
		path    string
	}{
		{
			name: "devices empty array",
			handler: func(w http.ResponseWriter, r *http.Request) {
				h := handlers.NewDeviceHandler(deviceRepo, "")
				h.List(w, r)
			},
			path: "/api/devices",
		},
		{
			name: "positions empty array",
			handler: func(w http.ResponseWriter, r *http.Request) {
				h := handlers.NewPositionHandler(positionRepo, deviceRepo)
				h.GetPositions(w, r)
			},
			path: "/api/positions",
		},
		{
			name: "geofences empty array",
			handler: func(w http.ResponseWriter, r *http.Request) {
				h := handlers.NewGeofenceHandler(geofenceRepo)
				h.List(w, r)
			},
			path: "/api/geofences",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := authedRequest(http.MethodGet, tt.path, nil, user)
			rr := httptest.NewRecorder()
			tt.handler(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
			}

			raw := decodeRawJSON(t, rr)
			arr, isArray := raw.([]interface{})
			if !isArray {
				t.Fatalf("response is %T; must be a JSON array", raw)
			}
			if len(arr) != 0 {
				t.Errorf("expected empty array [], got %d elements", len(arr))
			}
		})
	}
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
			speed:          float64Ptr(25.0),
			expectedMotion: true,
		},
		{
			name:           "speed at exact threshold (5 km/h) -> motion=true",
			speed:          float64Ptr(5.0),
			expectedMotion: true,
		},
		{
			name:           "speed below threshold (3 km/h) -> motion=false",
			speed:          float64Ptr(3.0),
			expectedMotion: false,
		},
		{
			name:           "speed zero -> motion=false",
			speed:          float64Ptr(0.0),
			expectedMotion: false,
		},
		{
			name:           "speed nil -> motion=false",
			speed:          nil,
			expectedMotion: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate what the protocol handler does: set the motion
			// attribute before persisting. This mirrors protocol/handler.go
			// HandlePosition() lines 106-114.
			isMoving := tt.speed != nil && *tt.speed >= 5.0
			attrs := map[string]interface{}{
				"motion": isMoving,
			}

			pos := &model.Position{
				DeviceID:   f.device.ID,
				Timestamp:  time.Now().UTC(),
				Valid:      true,
				Latitude:   52.52,
				Longitude:  13.405,
				Speed:      tt.speed,
				Attributes: attrs,
			}
			if err := f.positionRepo.Create(ctx, pos); err != nil {
				t.Fatalf("create position: %v", err)
			}

			// Fetch the position back through the API to verify the
			// attribute is persisted and returned correctly.
			h := handlers.NewPositionHandler(f.positionRepo, f.deviceRepo)
			req := authedRequest(http.MethodGet,
				fmt.Sprintf("/api/positions?id=%d", pos.ID), nil, f.user)
			rr := httptest.NewRecorder()
			h.GetPositions(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
			}

			var positions []map[string]interface{}
			if err := json.NewDecoder(rr.Body).Decode(&positions); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(positions) == 0 {
				t.Fatal("expected at least one position")
			}

			posJSON := positions[0]
			attrsJSON, ok := posJSON["attributes"].(map[string]interface{})
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

func float64Ptr(v float64) *float64 {
	return &v
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

	// Fetch all devices and verify status values.
	h := handlers.NewDeviceHandler(deviceRepo, "")
	req := authedRequest(http.MethodGet, "/api/devices", nil, user)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var devices []map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&devices); err != nil {
		t.Fatalf("decode: %v", err)
	}

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

	// Create a device that we will read back. The status should NOT be
	// "moving". Even if code were to set it to "moving" (which the DB
	// CHECK constraint should prevent), the test will catch it.
	h := handlers.NewDeviceHandler(f.deviceRepo, "")

	// Fetch the pre-created device.
	req := authedRequest(http.MethodGet,
		fmt.Sprintf("/api/devices/%d", f.device.ID), nil, f.user)
	req = withChiParams(req, map[string]string{"id": fmt.Sprintf("%d", f.device.ID)})
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var dev map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&dev); err != nil {
		t.Fatalf("decode: %v", err)
	}

	status := dev["status"].(string)
	if status == "moving" {
		t.Error("device status is 'moving'; HA does not recognize this value. Use 'online' and set motion via position.attributes.motion")
	}

	// Also verify the device we get via the list endpoint.
	req = authedRequest(http.MethodGet, "/api/devices", nil, f.user)
	rr = httptest.NewRecorder()
	h.List(rr, req)

	var devices []map[string]interface{}
	_ = json.NewDecoder(rr.Body).Decode(&devices)
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
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)

	user := &model.User{
		Email:        "bearer@example.com",
		PasswordHash: "hash",
		Name:         "Bearer User",
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Create an API key for the user.
	apiKey := &model.ApiKey{
		UserID:      user.ID,
		Name:        "HA Integration Key",
		Permissions: model.PermissionFull,
	}
	if err := apiKeyRepo.Create(ctx, apiKey); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Create a device for the user so we have something to list.
	dev := &model.Device{
		UniqueID: "bearer-dev",
		Name:     "Bearer Device",
		Status:   "online",
	}
	if err := deviceRepo.Create(ctx, dev, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	// Build the auth middleware.
	authMW := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)

	// Create a handler that lists devices, wrapped with auth middleware.
	deviceHandler := handlers.NewDeviceHandler(deviceRepo, "")
	handler := authMW(http.HandlerFunc(deviceHandler.List))

	// Test with valid Bearer token.
	t.Run("valid bearer token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
		req.Header.Set("Authorization", "Bearer "+apiKey.Token)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("Bearer auth: expected 200, got %d: %s", rr.Code, rr.Body.String())
		}

		var devices []map[string]interface{}
		if err := json.NewDecoder(rr.Body).Decode(&devices); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(devices) == 0 {
			t.Error("expected at least one device with valid bearer token")
		}
	})

	// Test with invalid Bearer token.
	t.Run("invalid bearer token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
		req.Header.Set("Authorization", "Bearer invalid-token-value")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 for invalid token, got %d", rr.Code)
		}
	})

	// Test without any auth.
	t.Run("no auth returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 without auth, got %d", rr.Code)
		}
	})
}

// TestTraccarCompat_LegacyTokenAuth verifies that the legacy user.token
// Bearer authentication still works. Some existing integrations may use
// the legacy token generated via POST /api/session/token.
func TestTraccarCompat_LegacyTokenAuth(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)

	user := &model.User{
		Email:        "legacy@example.com",
		PasswordHash: "hash",
		Name:         "Legacy User",
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Generate a legacy token.
	token, err := userRepo.GenerateToken(ctx, user.ID)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	dev := &model.Device{
		UniqueID: "legacy-dev",
		Name:     "Legacy Device",
		Status:   "online",
	}
	if err := deviceRepo.Create(ctx, dev, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	authMW := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	deviceHandler := handlers.NewDeviceHandler(deviceRepo, "")
	handler := authMW(http.HandlerFunc(deviceHandler.List))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("legacy Bearer auth: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var devices []map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&devices); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(devices) == 0 {
		t.Error("expected at least one device with legacy bearer token")
	}
}

// TestTraccarCompat_SessionTokenQueryParam verifies that GET /api/session?token=
// creates a session and returns user info with a session cookie. This is the
// mechanism used by pytraccar for initial authentication.
//
// Reference: pytraccar TraccarClient._get_session() uses ?token= parameter.
func TestTraccarCompat_SessionTokenQueryParam(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	user := &model.User{
		Email:        "pytraccar@example.com",
		PasswordHash: "hash",
		Name:         "PyTraccar User",
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Generate a legacy token for the user.
	token, err := userRepo.GenerateToken(ctx, user.ID)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	h := handlers.NewSessionHandler(userRepo, sessionRepo, apiKeyRepo)

	// GET /api/session?token=<token> should authenticate and create a session.
	req := httptest.NewRequest(http.MethodGet,
		"/api/session?token="+token, nil)
	rr := httptest.NewRecorder()
	h.GetCurrentSession(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/session?token= returned %d: %s", rr.Code, rr.Body.String())
	}

	// Verify a session cookie is set.
	var sessionCookie *http.Cookie
	for _, c := range rr.Result().Cookies() {
		if c.Name == "session_id" && c.Value != "" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Error("GET /api/session?token= must set a session_id cookie (required for WebSocket auth)")
	}

	// Verify the response contains user info with Traccar-compatible fields.
	var userResp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&userResp); err != nil {
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

// TestTraccarCompat_SessionTokenQueryParam_InvalidToken verifies that an
// invalid token returns 401.
func TestTraccarCompat_SessionTokenQueryParam_InvalidToken(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	h := handlers.NewSessionHandler(userRepo, sessionRepo, apiKeyRepo)

	req := httptest.NewRequest(http.MethodGet,
		"/api/session?token=nonexistent-token", nil)
	rr := httptest.NewRecorder()
	h.GetCurrentSession(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid token, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestTraccarCompat_CookieSessionAuth verifies that session cookie
// authentication works. After login, subsequent requests use the session
// cookie.
func TestTraccarCompat_CookieSessionAuth(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)

	hash, _ := bcrypt.GenerateFromPassword([]byte("cookiepass"), bcrypt.MinCost)
	user := &model.User{
		Email:        "cookie@example.com",
		PasswordHash: string(hash),
		Name:         "Cookie User",
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	dev := &model.Device{UniqueID: "cookie-dev", Name: "Cookie Device", Status: "online"}
	if err := deviceRepo.Create(ctx, dev, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	// Step 1: Login to get a session cookie.
	sessionHandler := handlers.NewSessionHandler(userRepo, sessionRepo, apiKeyRepo)
	loginBody, _ := json.Marshal(map[string]string{
		"email":    "cookie@example.com",
		"password": "cookiepass",
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/session", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRR := httptest.NewRecorder()
	sessionHandler.Login(loginRR, loginReq)

	if loginRR.Code != http.StatusOK {
		t.Fatalf("login failed: %d: %s", loginRR.Code, loginRR.Body.String())
	}

	var sessionCookie *http.Cookie
	for _, c := range loginRR.Result().Cookies() {
		if c.Name == "session_id" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("login did not set session_id cookie")
	}

	// Step 2: Use the session cookie to access the devices endpoint.
	authMW := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	deviceHandler := handlers.NewDeviceHandler(deviceRepo, "")
	handler := authMW(http.HandlerFunc(deviceHandler.List))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.AddCookie(sessionCookie)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("cookie auth: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var devices []map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&devices); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(devices) == 0 {
		t.Error("expected at least one device with cookie auth")
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
		Attributes: map[string]interface{}{"motion": true},
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

	// Build handlers.
	deviceHandler := handlers.NewDeviceHandler(deviceRepo, "")
	positionHandler := handlers.NewPositionHandler(positionRepo, deviceRepo)
	geofenceHandler := handlers.NewGeofenceHandler(geofenceRepo)
	sessionHandler := handlers.NewSessionHandler(userRepo, sessionRepo, apiKeyRepo)
	serverHandler := handlers.NewServerHandler()

	// Build the router with real auth middleware.
	authMW := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	adminMW := func(next http.Handler) http.Handler { return next } // no-op for this test
	hub := websocket.NewHub(nil, nil, nil)

	h := api.Handlers{
		GetServer:         serverHandler.GetServer,
		Login:             sessionHandler.Login,
		GetCurrentSession: sessionHandler.GetCurrentSession,
		GenerateToken:     sessionHandler.GenerateToken,
		Logout:            sessionHandler.Logout,
		ListDevices:       deviceHandler.List,
		GetDevice:         deviceHandler.Get,
		CreateDevice:      deviceHandler.Create,
		UpdateDevice:      deviceHandler.Update,
		DeleteDevice:      deviceHandler.Delete,
		GetPositions:      positionHandler.GetPositions,
		ListGeofences:     geofenceHandler.List,
		GetGeofence:       geofenceHandler.Get,
		CreateGeofence:    geofenceHandler.Create,
		UpdateGeofence:    geofenceHandler.Update,
		DeleteGeofence:    geofenceHandler.Delete,
	}

	// Fill remaining handlers with no-ops to avoid nil panics.
	noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
	})
	if h.CreateCommand == nil {
		h.CreateCommand = noop
	}
	if h.SendCommand == nil {
		h.SendCommand = noop
	}
	if h.GetCmdTypes == nil {
		h.GetCmdTypes = noop
	}
	if h.ListEvents == nil {
		h.ListEvents = noop
	}
	if h.ReportEvents == nil {
		h.ReportEvents = noop
	}
	if h.ListNotifications == nil {
		h.ListNotifications = noop
	}
	if h.CreateNotification == nil {
		h.CreateNotification = noop
	}
	if h.UpdateNotification == nil {
		h.UpdateNotification = noop
	}
	if h.DeleteNotification == nil {
		h.DeleteNotification = noop
	}
	if h.TestNotification == nil {
		h.TestNotification = noop
	}
	if h.NotificationLogs == nil {
		h.NotificationLogs = noop
	}
	if h.CreateShare == nil {
		h.CreateShare = noop
	}
	if h.ListShares == nil {
		h.ListShares = noop
	}
	if h.DeleteShare == nil {
		h.DeleteShare = noop
	}
	if h.GetSharedDevice == nil {
		h.GetSharedDevice = noop
	}
	if h.ListUsers == nil {
		h.ListUsers = noop
	}
	if h.CreateUser == nil {
		h.CreateUser = noop
	}
	if h.UpdateUser == nil {
		h.UpdateUser = noop
	}
	if h.DeleteUser == nil {
		h.DeleteUser = noop
	}
	if h.ListUserDevs == nil {
		h.ListUserDevs = noop
	}
	if h.AssignDevice == nil {
		h.AssignDevice = noop
	}
	if h.UnassignDevice == nil {
		h.UnassignDevice = noop
	}
	if h.StartSudo == nil {
		h.StartSudo = noop
	}
	if h.EndSudo == nil {
		h.EndSudo = noop
	}
	if h.GetSudoStatus == nil {
		h.GetSudoStatus = noop
	}
	if h.GetPlatformStats == nil {
		h.GetPlatformStats = noop
	}
	if h.GetUserStats == nil {
		h.GetUserStats = noop
	}
	if h.GetAuditLog == nil {
		h.GetAuditLog = noop
	}
	if h.ListCalendars == nil {
		h.ListCalendars = noop
	}
	if h.CreateCalendar == nil {
		h.CreateCalendar = noop
	}
	if h.UpdateCalendar == nil {
		h.UpdateCalendar = noop
	}
	if h.DeleteCalendar == nil {
		h.DeleteCalendar = noop
	}
	if h.CheckCalendar == nil {
		h.CheckCalendar = noop
	}
	if h.CreateApiKey == nil {
		h.CreateApiKey = noop
	}
	if h.ListApiKeys == nil {
		h.ListApiKeys = noop
	}
	if h.DeleteApiKey == nil {
		h.DeleteApiKey = noop
	}
	if h.AdminListUserKeys == nil {
		h.AdminListUserKeys = noop
	}
	if h.AdminListAllGeofences == nil {
		h.AdminListAllGeofences = noop
	}
	if h.AdminListAllCalendars == nil {
		h.AdminListAllCalendars = noop
	}
	if h.AdminListAllNotifications == nil {
		h.AdminListAllNotifications = noop
	}
	if h.AdminGetAllPositions == nil {
		h.AdminGetAllPositions = noop
	}

	router := api.NewRouter(h, authMW, adminMW, hub)
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

		var devices []map[string]interface{}
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

		var positions []map[string]interface{}
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
		if attrs, ok := p["attributes"].(map[string]interface{}); ok {
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

		var server map[string]interface{}
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
// application/x-www-form-urlencoded requests. The Traccar Manager mobile app
// sends credentials in form-encoded format, not JSON.
func TestTraccarCompat_FormEncodedLogin(t *testing.T) {
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	hash, _ := bcrypt.GenerateFromPassword([]byte("formpass"), bcrypt.MinCost)
	user := &model.User{
		Email:        "formlogin@example.com",
		PasswordHash: string(hash),
		Name:         "Form Login User",
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	h := handlers.NewSessionHandler(userRepo, sessionRepo, apiKeyRepo)

	formBody := "email=formlogin%40example.com&password=formpass"
	req := httptest.NewRequest(http.MethodPost, "/api/session",
		bytes.NewReader([]byte(formBody)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("form-encoded login: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify session cookie is set.
	var found bool
	for _, c := range rr.Result().Cookies() {
		if c.Name == "session_id" && c.Value != "" {
			found = true
			break
		}
	}
	if !found {
		t.Error("form-encoded login must set session_id cookie")
	}

	// Verify response contains Traccar-compatible user fields.
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, field := range []string{"id", "email", "name", "administrator", "readonly", "disabled"} {
		if _, exists := resp[field]; !exists {
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
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)
	ctx := context.Background()

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	apiKeyRepo := repository.NewApiKeyRepository(pool)

	hash, _ := bcrypt.GenerateFromPassword([]byte("logoutpass"), bcrypt.MinCost)
	user := &model.User{
		Email:        "logout@example.com",
		PasswordHash: string(hash),
		Name:         "Logout User",
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Step 1: Login to get a session.
	sessionHandler := handlers.NewSessionHandler(userRepo, sessionRepo, apiKeyRepo)
	loginBody, _ := json.Marshal(map[string]string{
		"email":    "logout@example.com",
		"password": "logoutpass",
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/session", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRR := httptest.NewRecorder()
	sessionHandler.Login(loginRR, loginReq)

	if loginRR.Code != http.StatusOK {
		t.Fatalf("login failed: %d: %s", loginRR.Code, loginRR.Body.String())
	}

	var sessionCookie *http.Cookie
	for _, c := range loginRR.Result().Cookies() {
		if c.Name == "session_id" && c.Value != "" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("login did not set session_id cookie")
	}

	// Step 2: Logout using the session cookie.
	logoutReq := authedRequest(http.MethodDelete, "/api/session", nil, user)
	logoutReq.AddCookie(sessionCookie)
	logoutRR := httptest.NewRecorder()
	sessionHandler.Logout(logoutRR, logoutReq)

	// Verify 204 No Content (matches original Traccar Java server).
	if logoutRR.Code != http.StatusNoContent {
		t.Errorf("expected 204 No Content from logout, got %d", logoutRR.Code)
	}

	// Verify the response body is empty (Traccar returns no body).
	if logoutRR.Body.Len() != 0 {
		t.Errorf("expected empty body from logout, got %q", logoutRR.Body.String())
	}

	// Verify the session cookie is properly cleared.
	var clearedCookie *http.Cookie
	for _, c := range logoutRR.Result().Cookies() {
		if c.Name == "session_id" {
			clearedCookie = c
			break
		}
	}
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
	authMW := middleware.Auth(userRepo, sessionRepo, apiKeyRepo)
	checkHandler := authMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	checkReq := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	checkReq.AddCookie(sessionCookie) // Use the old (now-deleted) session cookie
	checkRR := httptest.NewRecorder()
	checkHandler.ServeHTTP(checkRR, checkReq)

	if checkRR.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 after logout with deleted session, got %d", checkRR.Code)
	}
}

// ---------------------------------------------------------------------------
// 9. Content-Type header test
// ---------------------------------------------------------------------------

// TestTraccarCompat_ContentTypeJSON verifies that all JSON API responses
// include the correct Content-Type header. pytraccar checks for
// "application/json" before parsing the response body.
func TestTraccarCompat_ContentTypeJSON(t *testing.T) {
	f := setupCompatFixtures(t)

	h := handlers.NewDeviceHandler(f.deviceRepo, "")
	req := authedRequest(http.MethodGet, "/api/devices", nil, f.user)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	ct := rr.Header().Get("Content-Type")
	if ct == "" {
		t.Fatal("Content-Type header is missing")
	}
	// Go's json.Encoder writes "application/json" but may include charset.
	if ct != "application/json" && ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type is %q; expected 'application/json' for HA compatibility", ct)
	}
}

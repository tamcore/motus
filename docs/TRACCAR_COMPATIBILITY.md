# Traccar API Compatibility Guide

**Tested With:** Home Assistant 2026.2.2, pytraccar 3.0.0

This document details the requirements for maintaining compatibility with Traccar-based integrations, specifically Home Assistant's `traccar_server` integration and Traccar mobile apps.

---

## Table of Contents

1. [Overview](#overview)
2. [Critical Compatibility Rules](#critical-compatibility-rules)
3. [Device Model Requirements](#device-model-requirements)
4. [Position Model Requirements](#position-model-requirements)
5. [API Endpoint Requirements](#api-endpoint-requirements)
6. [Home Assistant Specific Issues](#home-assistant-specific-issues)
7. [Traccar Apps Specific Issues](#traccar-apps-specific-issues)
8. [Common Pitfalls](#common-pitfalls)
9. [Testing Guidelines](#testing-guidelines)

---

## Overview

**The Problem:** Home Assistant and Traccar apps use the `pytraccar` Python library which makes **unsafe assumptions** about API responses. The code uses Python bracket notation (`data["field"]`) without `.get()` safety checks, causing `KeyError` exceptions when fields are missing.

**The Solution:** Ensure ALL Traccar-compatible fields are ALWAYS present in JSON responses, even when null/empty.

**Key Principle:** **"Always present, never omitted"** - Remove `omitempty` JSON tags from all Traccar-compatible fields.

---

## Critical Compatibility Rules

### Rule 1: Never Use `omitempty` on Traccar Fields

**❌ WRONG:**
```go
Accuracy *float64 `json:"accuracy,omitempty"`  // Omitted when nil
```

**✅ CORRECT:**
```go
Accuracy float64 `json:"accuracy"`  // Always present (0.0 when unknown)
```

**Why:** pytraccar uses `position["accuracy"]` which raises `KeyError` if the key doesn't exist.

---

### Rule 2: Attributes Must Be Objects, Never Null

**❌ WRONG:**
```go
// Scanner that allows attributes to be nil
if len(attrs) > 0 {
    json.Unmarshal(attrs, &d.Attributes)
}
// d.Attributes may be nil → serializes as "attributes": null
```

**✅ CORRECT:**
```go
if len(attrs) > 0 {
    json.Unmarshal(attrs, &d.Attributes)
} else {
    d.Attributes = make(map[string]interface{})  // Always {}
}
```

**Why:** Home Assistant calls `device["attributes"].get(key)` - calling `.get()` on `None` raises `AttributeError`.

---

### Rule 3: Response Format Must Be Bare JSON Arrays

**❌ WRONG:**
```json
{
  "data": [...],
  "total": 42,
  "page": 1
}
```

**✅ CORRECT:**
```json
[{...}, {...}, ...]
```

**Why:** pytraccar directly casts responses to `list[DeviceModel]`. Envelopes cause deserialization crashes.

---

### Rule 4: Status Values Are Limited

**Device Status Values Recognized by Home Assistant:**
- `"online"` → binary_sensor.status = **ON** (active)
- `"offline"` → binary_sensor.status = **OFF** (inactive)
- `"unknown"` → binary_sensor.status = **UNKNOWN**
- **ANY OTHER VALUE** (including `"moving"`) → binary_sensor.status = **OFF**

**Critical:** Use `"online"` for active devices, communicate motion via `position.attributes.motion` boolean.

---

## Device Model Requirements

### Required Fields (Always Present)

| Field | Type | JSON Tag | Notes |
|-------|------|----------|-------|
| `id` | `int64` | `json:"id"` | Primary key |
| `uniqueId` | `string` | `json:"uniqueId"` | Device identifier |
| `name` | `string` | `json:"name"` | Device name |
| `status` | `string` | `json:"status"` | Must be "online", "offline", or "unknown" |
| `disabled` | `bool` | `json:"disabled"` | Account status |
| `attributes` | `map[string]interface{}` | `json:"attributes"` | **NEVER null, always {}** |

### Fields That Must Be Present (Can Be Null)

| Field | Type | JSON Tag | Default When Null |
|-------|------|----------|-------------------|
| `model` | `*string` | `json:"model"` | null ← OK |
| `phone` | `*string` | `json:"phone"` | null ← OK |
| `contact` | `*string` | `json:"contact"` | null ← OK |
| `category` | `*string` | `json:"category"` | null ← OK |
| `groupId` | `*int64` | `json:"groupId"` | null ← OK (pytraccar allows) |
| `calendarId` | `*int64` | `json:"calendarId"` | null ← OK |
| `positionId` | `*int64` | `json:"positionId"` | null ← OK |
| `expirationTime` | `*time.Time` | `json:"expirationTime"` | null ← OK |

**Critical:** Do NOT use `omitempty` on any of these fields. Home Assistant entity initialization uses bracket notation: `model=device["model"]`.

### Example Correct Device JSON

```json
{
  "id": 1,
  "uniqueId": "9000000000001",
  "name": "Demo Car 1",
  "status": "online",
  "model": "Sinotrack ST-901L 4G",
  "phone": "+1234567890",
  "contact": null,
  "category": "car",
  "groupId": null,
  "calendarId": null,
  "positionId": 12345,
  "expirationTime": null,
  "disabled": false,
  "attributes": {}
}
```

---

## Position Model Requirements

### Required Fields (Always Present)

| Field | Type | JSON Tag | Notes |
|-------|------|----------|-------|
| `id` | `int64` | `json:"id"` | Primary key |
| `deviceId` | `int64` | `json:"deviceId"` | Foreign key |
| `fixTime` | `time.Time` | `json:"fixTime"` | GPS timestamp (note: "fixTime" not "timestamp") |
| `valid` | `bool` | `json:"valid"` | GPS fix validity |
| `latitude` | `float64` | `json:"latitude"` | Required for tracking |
| `longitude` | `float64` | `json:"longitude"` | Required for tracking |
| `accuracy` | `float64` | `json:"accuracy"` | **MUST be numeric, default 0.0** |
| `outdated` | `bool` | `json:"outdated"` | Position staleness |
| `attributes` | `map[string]interface{}` | `json:"attributes"` | **NEVER null, always {}** |
| `network` | `map[string]interface{}` | `json:"network"` | **NEVER null, always {}** |
| `geofenceIds` | `[]int64` | `json:"geofenceIds"` | **Can be null or []** |

### Fields That Must Be Present (Can Be Null)

| Field | Type | JSON Tag | Notes |
|-------|------|----------|-------|
| `altitude` | `*float64` | `json:"altitude"` | Meters above sea level |
| `speed` | `*float64` | `json:"speed"` | km/h or knots |
| `course` | `*float64` | `json:"course"` | Degrees (0-360) |
| `address` | `*string` | `json:"address"` | Reverse geocoded address |
| `protocol` | `string` | `json:"protocol,omitempty"` | h02, watch, etc. |

### Critical Accuracy Field

**Issue:** Home Assistant uses accuracy in zone calculations:
```python
zone_dist - zone_radius < radius  # Fails if radius is None!
```

**Solution:** accuracy CANNOT be null. Must be `float64` (non-pointer) defaulting to `0.0`.

**Scanner Implementation:**
```go
var accuracy *float64  // Scan from nullable DB column
// ... scan ...
if accuracy != nil {
    p.Accuracy = *accuracy
} else {
    p.Accuracy = 0.0  // Critical for HA
}
```

### Critical Motion Attribute

**Issue:** Home Assistant binary_sensor.motion reads:
```python
motion = position["attributes"]["motion"]
```

**Solution:** Set `position.Attributes["motion"]` boolean based on speed:
```go
isMoving := pos.Speed != nil && *pos.Speed >= 5.0  // 5 km/h threshold
pos.Attributes["motion"] = isMoving
```

**Where to Set:** In the protocol handler BEFORE storing the position, so it persists.

### Example Correct Position JSON

```json
{
  "id": 12345,
  "deviceId": 1,
  "fixTime": "2026-02-17T10:30:00Z",
  "valid": true,
  "latitude": 52.5200,
  "longitude": 13.4050,
  "altitude": 100.0,
  "speed": 45.5,
  "course": 90.0,
  "address": "Main Street 123, Berlin",
  "accuracy": 10.0,
  "attributes": {
    "motion": true,
    "ignition": true
  },
  "network": {},
  "geofenceIds": [1, 5],
  "outdated": false
}
```

---

## API Endpoint Requirements

### Mandatory Endpoints for Home Assistant

| Endpoint | Method | Response Type | Notes |
|----------|--------|---------------|-------|
| `/api/server` | GET | Object | Server info (public, no auth) |
| `/api/session` | POST | User Object | Login (CSRF exempt) |
| `/api/session` | GET | User Object | Get current user |
| `/api/session?token=<token>` | GET | User Object | Create session from API token (for WebSocket) |
| `/api/session/token` | POST | `{"token": "..."}` | Generate API token (authenticated) |
| `/api/devices` | GET | Device[] | List all devices |
| `/api/positions` | GET | Position[] | List positions (latest per device when no params) |
| `/api/geofences` | GET | Geofence[] | List geofences |
| `/api/socket` | WS | - | WebSocket for real-time updates |

### Authentication Methods

**1. Bearer Token (API Clients)**
```http
GET /api/devices
Authorization: Bearer <token>
```

**2. Session Cookie (Browsers)**
```http
GET /api/devices
Cookie: session_id=<session_id>
```

**3. CSRF Exemption**
- Bearer token requests: **Exempt** from CSRF (not vulnerable)
- Session requests: **Require** CSRF token (vulnerable to CSRF)

---

## Home Assistant Specific Issues

### Issue 1: Unsafe Bracket Notation Everywhere

**Problem:** pytraccar uses `data["field"]` without checking existence.

**Affected Code Locations:**
- `coordinator.py`: `accuracy = position["accuracy"] or 0.0`
- `entity.py`: `model=device["model"]`
- `sensor.py`: `value_fn=lambda x: x["address"]`

**Solution:** Ensure EVERY field pytraccar accesses is always present in JSON.

---

### Issue 2: Attributes Dictionary Access

**Problem:**
```python
# HA code
custom_attrs = device["attributes"].get("customAttr")
```

When `attributes` is `null`, calling `.get()` on `None` raises `AttributeError`.

**Solution:** Attributes must ALWAYS be an object `{}`, never `null`.

**Implementation:**
```go
// In scanner
if len(attrs) > 0 {
    json.Unmarshal(attrs, &d.Attributes)
} else {
    d.Attributes = make(map[string]interface{})  // Critical!
}
```

---

### Issue 3: Zone Calculations Require Numeric Accuracy

**Problem:**
```python
# HA zone/__init__.py:150
zone_dist - zone_radius < radius
```

When `radius` (from `location_accuracy`) is `None`, this raises `TypeError`.

**Solution:** accuracy must be `float64` (non-nullable), default to `0.0`.

---

### Issue 4: Status Sensor Only Recognizes "online"

**Problem:**
```python
# HA binary_sensor.py
s == "online"  # Only "online" returns True
```

Any other status value (including `"moving"`) is treated as False (off/stopped).

**Solution:**
- Device status: Use `"online"` for active devices
- Motion detection: Use `position.attributes.motion` boolean
- Two separate sensors:
  - `binary_sensor.status` → device connectivity
  - `binary_sensor.motion` → actual movement

---

### Issue 5: Geofence Sensor Requires geofenceIds

**Problem:** HA expects `position.geofenceIds` to be populated with current geofence containment.

**Solution:**
```go
// In geofence detection service
currentGeofences := geofenceRepo.CheckContainment(ctx, userID, lat, lon)
position.GeofenceIDs = currentGeofences  // Set the IDs!
```

**Format:** `geofenceIds` can be `null` (no geofences) or `[1, 5, 7]` (array of IDs).

---

### Issue 6: WebSocket Message Format

**Required Format:**
```json
{
  "devices": [{...}],
  "positions": [{...}],
  "events": [{...}]
}
```

**Critical:** All three fields are optional, but when present must be arrays (not null).

---

## Traccar Apps Specific Issues

### Mobile Apps (Traccar Manager, Traccar Client)

**Platform:** iOS and Android apps use WebView with Traccar Web frontend.

**Key Differences from Home Assistant:**
- Apps use the web frontend (React), not pytraccar
- More forgiving of missing fields (JavaScript `?.` operator)
- Same authentication (Bearer tokens)
- Same WebSocket format

**Testing:** Open your Motus server URL in Traccar Manager app → Should load and work.

---

## Common Pitfalls

### Pitfall 1: Adding `omitempty` to Simplify JSON

**Temptation:**
```go
Model *string `json:"model,omitempty"`  // Cleaner JSON when null?
```

**Reality:** Breaks Home Assistant when it calls `device["model"]`.

**Rule:** For Traccar-compatible fields, **NEVER use `omitempty`**. Accept verbosity for compatibility.

---

### Pitfall 2: Returning Null for Empty Collections

**Temptation:**
```go
var geofences []*model.Geofence  // nil slice
api.RespondJSON(w, http.StatusOK, geofences)  // Serializes as null
```

**Reality:** pytraccar expects `[]` not `null`.

**Solution:**
```go
geofences := []*model.Geofence{}  // Empty slice
// or
if geofences == nil {
    geofences = []*model.Geofence{}
}
api.RespondJSON(w, http.StatusOK, geofences)
```

---

### Pitfall 3: Using Database NULLs Without Defaults

**Problem:** Database columns are nullable, but Go scanners must provide defaults.

**Example:**
```go
// DB column: accuracy NUMERIC NULL
// Model field: Accuracy float64 (non-nullable)

// Scanner must handle NULL:
var accuracy *float64
scanner.Scan(..., &accuracy, ...)
if accuracy != nil {
    p.Accuracy = *accuracy
} else {
    p.Accuracy = 0.0  // Default for HA compatibility
}
```

---

### Pitfall 4: Adding Pagination Without Backward Compatibility

**Temptation:** Add required `?limit=20&offset=0` parameters.

**Reality:** pytraccar calls endpoints with ZERO parameters.

**Solution:** Pagination must be OPTIONAL with default = return all data.

---

## Device Model Field Reference

### Complete Field List

```go
type Device struct {
    // Core fields (always non-null)
    ID         int64     `json:"id"`
    UniqueID   string    `json:"uniqueId"`
    Name       string    `json:"name"`
    Status     string    `json:"status"`           // "online", "offline", "unknown"
    Disabled   bool      `json:"disabled"`
    Attributes map[string]interface{} `json:"attributes"`  // NEVER null
    CreatedAt  time.Time `json:"createdAt"`
    UpdatedAt  time.Time `json:"updatedAt"`

    // Nullable fields (always present, can be null)
    Protocol       string     `json:"protocol,omitempty"`  // Exception: Motus-specific
    LastUpdate     *time.Time `json:"lastUpdate,omitempty"`  // Exception: Can omit
    PositionID     *int64     `json:"positionId"`        // No omitempty!
    GroupID        *int64     `json:"groupId"`           // No omitempty!
    Phone          *string    `json:"phone"`             // No omitempty!
    Model          *string    `json:"model"`             // No omitempty!
    Contact        *string    `json:"contact"`           // No omitempty!
    Category       *string    `json:"category"`          // No omitempty!
    CalendarID     *int64     `json:"calendarId"`        // No omitempty!
    ExpirationTime *time.Time `json:"expirationTime"`    // No omitempty!
    SpeedLimit     *float64   `json:"speedLimit,omitempty"`  // Motus extension
}
```

**pytraccar DeviceModel Fields Used by HA:**
- `id`, `name`, `uniqueId`, `status`, `disabled`
- `model` (for device info display)
- `category` (for icon selection)
- `positionId` (for current position lookup)
- `attributes` (for custom attributes)

---

## Position Model Field Reference

### Complete Field List

```go
type Position struct {
    // Core fields (always non-null)
    ID          int64                  `json:"id"`
    DeviceID    int64                  `json:"deviceId"`
    Timestamp   time.Time              `json:"fixTime"`     // Note: "fixTime" not "timestamp"
    Valid       bool                   `json:"valid"`
    Latitude    float64                `json:"latitude"`
    Longitude   float64                `json:"longitude"`
    Accuracy    float64                `json:"accuracy"`    // NEVER null, default 0.0
    Outdated    bool                   `json:"outdated"`
    Attributes  map[string]interface{} `json:"attributes"`  // NEVER null, always {}
    Network     map[string]interface{} `json:"network"`     // NEVER null, always {}
    GeofenceIDs []int64                `json:"geofenceIds"` // Can be null or []

    // Nullable fields (always present)
    Protocol    string     `json:"protocol,omitempty"`  // Exception: can omit
    ServerTime  *time.Time `json:"serverTime,omitempty"` // Exception: can omit
    DeviceTime  *time.Time `json:"deviceTime,omitempty"` // Exception: can omit
    Altitude    *float64   `json:"altitude"`         // No omitempty!
    Speed       *float64   `json:"speed"`            // No omitempty!
    Course      *float64   `json:"course"`           // No omitempty!
    Address     *string    `json:"address"`          // No omitempty!
}
```

**Critical Attributes:**

```go
// Set these in protocol handler:
position.Attributes["motion"] = speed >= 5.0  // For binary_sensor.motion
```

**pytraccar PositionModel Fields Used by HA:**
- `id`, `deviceId`, `fixTime`, `valid`, `latitude`, `longitude`
- `altitude`, `speed`, `course`, `address` (for sensors)
- `accuracy` (for zone calculations - MUST be numeric!)
- `attributes.motion` (for binary_sensor.motion)
- `geofenceIds` (for sensor.geofence)

---

## API Endpoint Behavior

### GET /api/devices

**Request:** No parameters (pytraccar doesn't send any)

**Response:** Bare JSON array of all user's devices
```json
[
  { "id": 1, "name": "Car", ... },
  { "id": 2, "name": "Truck", ... }
]
```

**NOT:**
```json
{
  "data": [...],
  "total": 2
}
```

---

### GET /api/positions

**Request Modes:**

1. **No parameters** (pytraccar default):
   - Returns latest position for EACH device user has access to
   - Used by HA for initial state

2. **With deviceId + time range**:
   - Returns position history for analytics
   - Used by charts/replay features

**Response:** Always bare JSON array

---

### GET /api/session?token=<token>

**Critical for WebSocket:**

Home Assistant calls this BEFORE connecting to WebSocket to establish a session cookie. The endpoint must:
1. Validate the API token
2. Create a session with cookie
3. Return the User object

**Implementation:**
```go
// Extract token from query parameter
token := r.URL.Query().Get("token")
user, err := users.GetByToken(ctx, token)
// ... create session, set cookie, return user
```

---

### WebSocket /api/socket

**Message Format:**
```json
{
  "devices": [{ "id": 1, "status": "online", ... }],
  "positions": [{ "id": 123, "deviceId": 1, ... }],
  "events": [{ "id": 456, "type": "geofenceEnter", ... }]
}
```

**Fields are optional but must be arrays when present.**

**Authentication:** Uses session cookie (established via `GET /api/session?token=`)

---

## Testing Guidelines

### Manual Testing with Home Assistant

**1. Setup:**
```bash
# Generate API token in Motus Settings
# Or use demo token: "demo"
```

**2. Add Integration:**
- Settings → Integrations → Add → Traccar Server
- Host: your-motus-server.com
- Port: 443 (HTTPS) or 80 (HTTP)
- SSL: Enable if HTTPS
- Verify SSL: Disable if self-signed
- Token: <your_token>

**3. Verify Entities Created:**
```
device_tracker.device_name
binary_sensor.device_name_status
binary_sensor.device_name_motion
sensor.device_name_speed
sensor.device_name_altitude
sensor.device_name_address
sensor.device_name_geofence
sensor.device_name_battery  (if battery in attributes)
```

**4. Check for Errors:**
```bash
# In Home Assistant logs:
grep "KeyError\|AttributeError\|TypeError" home-assistant.log
```

**Common Errors:**
- `KeyError: 'model'` → model field missing (add without omitempty)
- `AttributeError: 'NoneType' object has no attribute 'get'` → attributes is null (initialize to {})
- `TypeError: '<' not supported between ... 'NoneType'` → accuracy is null (must be numeric)

---

### Automated Testing

**Create a test that validates all required fields:**

```go
func TestDeviceJSON_HomeAssistantCompatibility(t *testing.T) {
    device := &model.Device{
        ID: 1, UniqueID: "TEST", Name: "Test", Status: "online",
    }

    data, _ := json.Marshal(device)
    var result map[string]interface{}
    json.Unmarshal(data, &result)

    // All these fields MUST be present (not omitted)
    required := []string{
        "id", "uniqueId", "name", "status", "disabled",
        "model", "phone", "contact", "category",
        "groupId", "calendarId", "positionId", "expirationTime",
        "attributes",
    }

    for _, field := range required {
        if _, exists := result[field]; !exists {
            t.Errorf("Required field %q missing from JSON", field)
        }
    }

    // Attributes must be object not null
    if result["attributes"] == nil {
        t.Error("attributes must be {} not null")
    }
}
```

---

### Integration Test Checklist

**Before deploying changes that affect API responses:**

- [ ] Device JSON includes all required fields
- [ ] Position JSON includes all required fields
- [ ] `attributes` is always `{}` never `null`
- [ ] `network` is always `{}` never `null`
- [ ] `accuracy` is always numeric (never `null`)
- [ ] `geofenceIds` is populated correctly
- [ ] `position.attributes.motion` reflects actual movement
- [ ] Device status is `"online"` not `"moving"`
- [ ] List endpoints return `[]` not `null` for empty results
- [ ] WebSocket messages use correct envelope format
- [ ] Bearer token authentication works
- [ ] CSRF exemption for Bearer requests
- [ ] No `omitempty` on Traccar-compatible fields

---

## Migration Impact Analysis

**When modifying Position/Device models:**

1. **Adding new fields:** Safe (HA ignores unknown fields)
2. **Removing fields:** **DANGEROUS** - Check if pytraccar uses it
3. **Renaming fields:** **BREAKING** - Don't do this
4. **Changing nullability:** Review scanner and ensure proper defaults
5. **Adding `omitempty`:** **BREAKING** for Traccar fields

**Safe Extensions:**
- New endpoints (HA doesn't call them)
- New fields with data (additive)
- New attributes in `attributes` map (HA uses `.get()` with defaults)

**Breaking Changes:**
- Changing response envelope format
- Removing required fields
- Changing status values
- Making attributes nullable
- Changing accuracy to nullable

---

## Debugging Guide

### Symptom: "Failed setup, will retry: 'field_name'"

**Cause:** Field missing from JSON (pytraccar `KeyError`)

**Solution:** Remove `omitempty` from that field's JSON tag

---

### Symptom: "AttributeError: 'NoneType' object has no attribute 'get'"

**Cause:** `attributes` or `network` is null

**Solution:** Initialize empty maps in scanner

---

### Symptom: "TypeError: '<' not supported between instances of 'float' and 'NoneType'"

**Cause:** accuracy is null (used in zone math)

**Solution:** Make accuracy non-nullable with default 0.0

---

### Symptom: Entities created but binary_sensor.status always "off"

**Cause:** Device status is not "online"

**Solution:** Set `device.Status = "online"` (not "moving" or other values)

---

### Symptom: binary_sensor.motion always shows "off"

**Cause:** `position.attributes.motion` not set

**Solution:** Set `position.Attributes["motion"] = speed >= threshold` in protocol handler

---

### Symptom: sensor.geofence shows "unknown"

**Cause:** `position.geofenceIds` is empty or not populated

**Solution:** Populate `position.GeofenceIDs` with current geofence containment

---

## Compatibility Matrix

| Client | Tested Version | Status |
|--------|----------------|--------|
| Home Assistant `traccar_server` | 2026.2.2 | ✅ Fully Compatible |
| pytraccar | 3.0.0 | ✅ Fully Compatible |
| Traccar Server (for reference) | 6.11.1 | ✅ API Parity |

---

## Summary: Required Changes for Compatibility

**From initial implementation to full compatibility, these changes were required:**

1. ✅ Remove `omitempty` from: accuracy, model, phone, contact, category, groupId, calendarId, positionId, expirationTime, altitude, speed, course, address, network, geofenceIds, attributes

2. ✅ Make accuracy non-nullable (float64 not *float64) with 0.0 default

3. ✅ Initialize attributes/network to `{}` in scanners when DB is NULL

4. ✅ Set `position.Attributes["motion"]` boolean based on speed

5. ✅ Keep device.status as `"online"` (not `"moving"`)

6. ✅ Populate `position.GeofenceIDs` with current geofence containment

7. ✅ Return `[]` not `null` for empty lists

8. ✅ CSRF exemption for Bearer token requests

9. ✅ Support `GET /api/session?token=` for WebSocket auth

10. ✅ Add all missing Traccar-compatible device fields (expirationTime, etc.)

---

## References

- [Home Assistant Traccar Server Integration](https://github.com/home-assistant/core/tree/dev/homeassistant/components/traccar_server)
- [pytraccar Library](https://github.com/ludeeus/pytraccar)
- [Traccar API Documentation](https://www.traccar.org/api-reference/)
- [Traccar OpenAPI Spec](https://www.traccar.org/api-reference/openapi.yaml)

---

## Maintenance Notes

**When updating dependencies or refactoring:**

1. Run Home Assistant integration test
2. Check for new pytraccar versions (may have new requirements)
3. Verify all fields still present in JSON
4. Test with demo account (token: "demo")
5. Monitor HA logs for KeyError/AttributeError

**Contact:** For compatibility issues, check pytraccar source code at [github.com/ludeeus/pytraccar](https://github.com/ludeeus/pytraccar)

---

**Status:** ✅ Production Ready

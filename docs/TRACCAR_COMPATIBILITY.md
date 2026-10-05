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

**Key Principle:** **"Always present, never omitted"**.

Field presence and nullability are set by `required` and `nullable` in
[`docs/openapi.yaml`](openapi.yaml) (the ogen-generated response types follow them). They are
checked by [`internal/api/handlers/traccar_compat_test.go`](../internal/api/handlers/traccar_compat_test.go).

---

## Critical Compatibility Rules

### Rule 1: Response Format Must Be Bare JSON Arrays

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

### Rule 2: Status Values Are Limited

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

**Critical:** These fields must never be omitted. Home Assistant entity initialization uses bracket notation: `model=device["model"]`.

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
currentGeofences, _ := geofenceRepo.CheckContainmentForDevice(ctx, deviceID, lat, lon)
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

### Pitfall: Adding Pagination Without Backward Compatibility

**Temptation:** Add required `?limit=20&offset=0` parameters.

**Reality:** pytraccar calls endpoints with ZERO parameters.

**Solution:** Pagination must be OPTIONAL with default = return all data.

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
- `KeyError: 'model'` → model field missing (mark it `required` in openapi.yaml)
- `AttributeError: 'NoneType' object has no attribute 'get'` → attributes is null (initialize to {})
- `TypeError: '<' not supported between ... 'NoneType'` → accuracy is null (must be numeric)

---

### Automated Testing

```bash
go test ./internal/api/handlers -run TestTraccarCompat
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

---

## Migration Impact Analysis

**When modifying Position/Device models:**

1. **Adding new fields:** Safe (HA ignores unknown fields)
2. **Removing fields:** **DANGEROUS** - Check if pytraccar uses it
3. **Renaming fields:** **BREAKING** - Don't do this
4. **Changing nullability or dropping `required`:** **BREAKING** for Traccar fields

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

## Compatibility Matrix

| Client | Tested Version | Status |
|--------|----------------|--------|
| Home Assistant `traccar_server` | 2026.2.2 | ✅ Fully Compatible |
| pytraccar | 3.0.0 | ✅ Fully Compatible |
| Traccar Server (for reference) | 6.11.1 | ✅ API Parity |

---

## References

- [Home Assistant Traccar Server Integration](https://github.com/home-assistant/core/tree/dev/homeassistant/components/traccar_server)
- [pytraccar Library](https://github.com/ludeeus/pytraccar)
- [Traccar API Documentation](https://www.traccar.org/api-reference/)
- [Traccar OpenAPI Spec](https://www.traccar.org/api-reference/openapi.yaml)

# MCP Tool Reference

The AI assistant exposes 16 tools to the language model via the
Model Context Protocol (MCP). Each tool maps to a handler in
`internal/ai/mcp/tools.go` and is automatically converted to an
OpenAI function schema at startup (`internal/ai/chat/service.go`).

For architecture, configuration, and the SSE event protocol see
[`docs/ai-assistant.md`](ai-assistant.md).

**Access levels:**
- **read** — available to all authenticated users, including readonly API keys
- **write** — requires a non-readonly API key or a session login

---

## Common Tools

#### `get_server_time`

> Returns the current server time. Call this first so you can resolve relative
> dates like "last year" or "this month" correctly.

**Access:** read
**Handler:** `handleGetServerTime` (`internal/ai/mcp/tools.go:178`)

**Parameters:** none

**Returns:**
```json
{ "now": "2026-06-03T14:22:00Z", "year": "2026", "today": "2026-06-03" }
```

**Typical trigger:**
> "How far did my car travel last month?"
> — model calls `get_server_time` first to resolve "last month" to a date range.

---

#### `list_devices`

> Lists all GPS devices accessible to the current user.

**Access:** read (admin users see all users' devices)
**Handler:** `handleListDevices` (`internal/ai/mcp/tools.go:191`)

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `name_contains` | string | no | Case-insensitive substring filter on device name |

**Returns:** array of device summaries:
```json
[
  { "id": 42, "uniqueId": "123456789", "name": "Car", "model": "TK103",
    "lastUpdate": "2026-06-03T14:00:00Z", "ignitionOn": false }
]
```

**Typical trigger:**
> "What devices do I have?"

---

#### `get_latest_position`

> Returns the latest GPS position for a device.

**Access:** read
**Handler:** `handleGetLatestPosition` (`internal/ai/mcp/tools.go:233`)

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `device_id` | string (int) | one of these | Numeric device ID |
| `device_name` | string | one of these | Exact or partial device name |

At least one of `device_id` or `device_name` must be provided.

**Returns:**
```json
{
  "deviceId": 42,
  "latitude": 52.5200,
  "longitude": 13.4050,
  "address": "Pariser Platz, Berlin",
  "speed": 0,
  "timestamp": "2026-06-03T14:00:00Z"
}
```

**Typical trigger:**
> "Where is my car right now?"

---

#### `get_distance_traveled`

> Returns total trip distance (km) per device and grand total for the given
> time window.

**Access:** read
**Handler:** `handleGetDistanceTraveled` (`internal/ai/mcp/tools.go:268`)

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `from` | string (RFC3339) | yes | Start of time window |
| `to` | string (RFC3339) | yes | End of time window |
| `device_id` | string (int) | no | Restrict to one device |
| `device_name` | string | no | Restrict to one device by name |

When neither `device_id` nor `device_name` is given, all the user's devices
are included.

**Returns:**
```json
{
  "devices": [
    { "id": 42, "name": "Car", "distanceKm": 123.4, "tripCount": 7 }
  ],
  "totalKm": 123.4
}
```

Distances are rounded to one decimal place.

**Typical trigger:**
> "How many kilometres did I drive this week?"

---

#### `list_geofences`

> Lists all geofences accessible to the current user.

**Access:** read
**Handler:** `handleListGeofences` (`internal/ai/mcp/tools.go:339`)

**Parameters:** none

**Returns:**
```json
[ { "id": 5, "name": "Home", "area": "CIRCLE(...)", "calendarId": null } ]
```

**Typical trigger:**
> "What geofences do I have?"

---

#### `create_geofence`

> Creates a circular geofence around an address or coordinates.

**Access:** write
**Handler:** `handleCreateGeofence` (`internal/ai/mcp/tools.go:366`)

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Display name |
| `address` | string | no | Free-text address (calls `geocode_address` internally if lat/lon not given) |
| `latitude` | number | no | Centre latitude (overrides address) |
| `longitude` | number | no | Centre longitude (overrides address) |
| `radius_m` | number | no | Radius in metres. Default: `200` |
| `calendar_id` | string (int) | no | Attach a calendar for time-based activation |

Either `address` or (`latitude` + `longitude`) must be supplied.

**Returns:**
```json
{ "id": 12, "name": "Home", "latitude": 52.52, "longitude": 13.40, "radiusM": 200 }
```

**Typical trigger:**
> "Create a 500-metre geofence around Brandenburg Gate."

---

#### `geocode_address`

> Converts a free-text address into latitude/longitude coordinates. Read-only.

**Access:** read
**Handler:** `handleGeocodeAddress` (`internal/ai/mcp/tools.go:429`)

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `address` | string | yes | Address to geocode |

Calls the Nominatim `/search` endpoint (1 req/sec rate limit, shared with
reverse geocoding). Returns the top-ranked result only.

**Returns:**
```json
{ "latitude": 52.5163, "longitude": 13.3777, "displayName": "Brandenburg Gate, ..." }
```

**Typical trigger:**
> The model calls this internally before `create_geofence` when an address is
> given but coordinates are not.

---

## Calendars

#### `list_calendars`

> Lists all time-window calendars accessible to the current user. Calendars
> can be attached to geofences to make them time-conditional.

**Access:** read
**Handler:** `handleListCalendars` (`internal/ai/mcp/tools.go:453`)

**Parameters:** none

**Returns:**
```json
[ { "id": 3, "name": "Weekdays 9-17" } ]
```

---

#### `create_calendar`

> Creates a new calendar that can be attached to geofences for time-based
> activation.
>
> Two modes:
> - **One-shot:** provide `start_time` and `end_time` (RFC3339).
> - **Weekly recurring:** provide `weekdays` (comma-separated
>   MO/TU/WE/TH/FR/SA/SU) and `daily_start_time`/`daily_end_time` (HH:MM UTC).

**Access:** write
**Handler:** `handleCreateCalendar` (`internal/ai/mcp/tools.go:480`)

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Calendar display name |
| `start_time` | string (RFC3339) | one-shot | One-shot start datetime |
| `end_time` | string (RFC3339) | one-shot | One-shot end datetime |
| `weekdays` | string | recurring | Comma-separated weekday codes, e.g. `MO,WE,FR` |
| `daily_start_time` | string (HH:MM UTC) | recurring | Daily window start |
| `daily_end_time` | string (HH:MM UTC) | recurring | Daily window end |

Either (`start_time` + `end_time`) or (`weekdays` + `daily_start_time` +
`daily_end_time`) is required; providing neither or mixing the two returns an
error.

**Returns:**
```json
{ "id": 3, "name": "Weekdays 9-17" }
```

**Typical trigger:**
> "Create a calendar active every weekday from 09:00 to 17:00."

---

## Geofences (update / delete)

#### `update_geofence`

> Updates a geofence's name and/or calendar attachment. Use `calendar_id` to
> attach a calendar, or `"clear"` to detach one.

**Access:** write
**Handler:** `handleUpdateGeofence` (`internal/ai/mcp/tools.go:548`)

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `id` | string (int) | yes | Geofence ID |
| `name` | string | no | New display name |
| `calendar_id` | string (int or `"clear"`) | no | Calendar ID to attach, or `"clear"` / `"0"` / `"none"` to detach |

**Returns:**
```json
{ "id": 12, "name": "Home", "calendarId": 3 }
```

**Typical trigger:**
> "Attach the weekday calendar to my Home geofence."

---

#### `delete_geofence`

> Permanently deletes a geofence. This cannot be undone.

**Access:** write
**Handler:** `handleDeleteGeofence` (`internal/ai/mcp/tools.go:601`)

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `id` | string (int) | yes | Geofence ID |

**Returns:**
```json
{ "deleted": 12 }
```

---

## Notifications

#### `list_notification_rules`

> Lists all notification rules for the current user.

**Access:** read
**Handler:** `handleListNotificationRules` (`internal/ai/mcp/tools.go:633`)

**Parameters:** none

**Returns:**
```json
[
  { "id": 7, "name": "Geofence alerts",
    "eventTypes": ["geofenceEnter", "geofenceExit"],
    "channel": "webhook", "enabled": true }
]
```

---

#### `create_notification_rule`

> Creates a new notification rule. Supported event types: `geofenceEnter`,
> `geofenceExit`, `deviceOnline`, `deviceOffline`, `motion`, `deviceIdle`,
> `ignitionOn`, `ignitionOff`, `alarm`, `tripCompleted`. Only channel
> supported currently: `webhook`.

**Access:** write
**Handler:** `handleCreateNotificationRule` (`internal/ai/mcp/tools.go:665`)

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Rule display name |
| `event_types` | string | yes | Comma-separated event types |
| `channel` | string | yes | Delivery channel — currently only `"webhook"` |
| `webhook_url` | string | webhook | Destination URL (required for `webhook` channel) |
| `template` | string | no | Message template with `{{.Field}}` placeholders |
| `enabled` | string (`"true"`/`"false"`) | no | Default `"true"` |

**Returns:**
```json
{ "id": 7, "name": "Geofence alerts" }
```

**Typical trigger:**
> "Send a webhook to https://example.com/hook when my car enters a geofence."

---

#### `update_notification_rule`

> Updates an existing notification rule. Only the provided fields are changed.

**Access:** write
**Handler:** `handleUpdateNotificationRule` (`internal/ai/mcp/tools.go:726`)

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `id` | string (int) | yes | Rule ID |
| `name` | string | no | New display name |
| `event_types` | string | no | Comma-separated event types |
| `webhook_url` | string | no | New webhook URL |
| `template` | string | no | New message template |
| `enabled` | string (`"true"`/`"false"`) | no | Enable or disable the rule |

**Returns:**
```json
{ "id": 7, "name": "Geofence alerts", "enabled": false }
```

---

#### `delete_notification_rule`

> Permanently deletes a notification rule.

**Access:** write
**Handler:** `handleDeleteNotificationRule` (`internal/ai/mcp/tools.go:800`)

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `id` | string (int) | yes | Rule ID |

**Returns:**
```json
{ "deleted": 7 }
```

---

## Events

#### `list_events`

> Lists device events (geofence enter/exit, alarm, ignition, motion, trip
> completed, etc.) within a time window.

**Access:** read
**Handler:** `handleListEvents` (`internal/ai/mcp/tools.go:852`)

**Parameters:**

| Name | Type | Required | Description |
|---|---|---|---|
| `from` | string (RFC3339) | yes | Start of time window |
| `to` | string (RFC3339) | yes | End of time window |
| `device_id` | string (int) | no | Restrict to one device |
| `device_name` | string | no | Restrict to one device by name |
| `event_types` | string | no | Comma-separated filter, e.g. `geofenceEnter,alarm` |
| `limit` | number | no | Max results, 1–500. Default: `100` |

When no device filter is given, events for all the user's devices are
returned.

Supported event type values: `geofenceEnter`, `geofenceExit`,
`deviceOnline`, `deviceOffline`, `motion`, `deviceIdle`, `ignitionOn`,
`ignitionOff`, `alarm`, `tripCompleted`.

**Returns:**
```json
[
  {
    "id": 1001,
    "deviceId": 42,
    "type": "geofenceEnter",
    "timestamp": "2026-06-03T08:00:00Z",
    "geofenceId": 5,
    "attributes": {}
  }
]
```

**Typical trigger:**
> "Show me all geofence events from this morning."

---

## Tool count verification

```bash
grep -c 'mcp.NewTool(' internal/ai/mcp/tools.go
# Expected: 16
```

When adding a new tool, update this document in the matching domain section
and keep the count in sync.

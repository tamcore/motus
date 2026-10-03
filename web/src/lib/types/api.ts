/**
 * TypeScript type definitions for the Motus API.
 *
 * These interfaces mirror the backend Go models and are used throughout
 * the frontend for type-safe API interactions.
 */

// ---------------------------------------------------------------------------
// User
// ---------------------------------------------------------------------------

/** User roles supported by the backend. */

/** A system user as returned by the API. */
export interface User {
  id: number;
  email: string;
  name: string;
  role: "admin" | "user" | "readonly";
  token?: string | null;
  createdAt: string;

  /** Traccar-compatible fields (computed from role on the backend). */
  administrator: boolean;
  readonly: boolean;
  disabled: boolean;
  map?: string | null;
  latitude?: number | null;
  longitude?: number | null;
  zoom?: number | null;
  coordinateFormat?: string | null;
  attributes?: Record<string, unknown>;

  /** Index signature for Svelte store compatibility */
  [key: string]: unknown;
}

/** Payload for creating a new user. */
export interface CreateUserPayload {
  email: string;
  password: string;
  name: string;
  role: string;
}

/** Payload for updating an existing user. */
export interface UpdateUserPayload {
  email?: string;
  password?: string;
  name?: string;
  role?: string;
  disabled?: boolean;
}

/** Payload for updating the authenticated user's own profile. */
export interface UpdateProfilePayload {
  name?: string;
  email?: string;
  currentPassword?: string;
  password?: string;
}

// ---------------------------------------------------------------------------
// Device
// ---------------------------------------------------------------------------

/** A GPS tracking device. */
export interface Device {
  id: number;
  uniqueId: string;
  name: string;
  protocol?: string;
  status: string;
  speedLimit?: number | null;
  lastUpdate?: string | null;
  positionId?: number | null;
  groupId?: number | null;
  phone?: string | null;
  model?: string | null;
  contact?: string | null;
  category?: string | null;
  disabled: boolean;
  mileage?: number | null;
  attributes?: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
  /** Present only in admin list-all responses. */
  ownerName?: string;
}

/** Payload for creating a new device. */
export interface CreateDevicePayload {
  uniqueId: string;
  name: string;
  phone?: string;
  model?: string;
  contact?: string;
  category?: string;
  protocol?: string;
  disabled?: boolean;
  mileage?: number | null;
  attributes?: Record<string, unknown>;
}

/** Payload for updating an existing device. */
export interface UpdateDevicePayload {
  uniqueId?: string;
  name?: string;
  phone?: string;
  model?: string;
  contact?: string;
  category?: string;
  protocol?: string;
  disabled?: boolean;
  speedLimit?: number | null;
  mileage?: number | null;
  attributes?: Record<string, unknown>;
}

// ---------------------------------------------------------------------------
// Position
// ---------------------------------------------------------------------------

/** A GPS position report from a device. */
export interface Position {
  id: number;
  deviceId: number;
  protocol?: string;
  serverTime?: string | null;
  deviceTime?: string | null;
  /** The GPS fix time. Mapped from Go field `Timestamp` with JSON tag `fixTime`. */
  fixTime: string;
  valid: boolean;
  latitude: number;
  longitude: number;
  altitude?: number | null;
  speed?: number | null;
  course?: number | null;
  address?: string | null;
  accuracy?: number | null;
  geofenceIds?: number[];
  outdated: boolean;
  attributes?: Record<string, unknown>;
  network?: Record<string, unknown>;
}

/** Compact position from GET /api/positions/points. Speed is in km/h after client normalization. */
export interface PositionPoint {
  lat: number;
  lon: number;
  speed: number;
  fixTime: string;
  /** Degrees; absent when unknown. */
  course?: number;
  /** Meters; absent when unknown. */
  altitude?: number;
}

// ---------------------------------------------------------------------------
// Calendar
// ---------------------------------------------------------------------------

/** A time-based schedule stored in iCalendar (RFC 5545) format. */
export interface Calendar {
  id: number;
  userId?: number;
  name: string;
  /** iCalendar (RFC 5545) data string. */
  data: string;
  createdAt: string;
  updatedAt: string;
  /** Present only in admin list-all responses. */
  ownerName?: string;
}

/** Payload for creating a new calendar. */
export interface CreateCalendarPayload {
  name: string;
  data: string;
}

/** Payload for updating an existing calendar. */
export interface UpdateCalendarPayload {
  name?: string;
  data?: string;
}

/** Response from checking if a calendar is currently active. */
export interface CalendarCheckResponse {
  calendarId: number;
  name: string;
  active: boolean;
  checkedAt: string;
}

// ---------------------------------------------------------------------------
// Trail bookmark
// ---------------------------------------------------------------------------

/** A named, saved time range of a device's trail (e.g. a hike). */
export interface TrailBookmark {
  id: number;
  userId?: number;
  deviceId: number;
  deviceName?: string;
  name: string;
  description: string;
  /** ISO 8601 start of the range. */
  from: string;
  /** ISO 8601 end of the range. */
  to: string;
  createdAt: string;
  updatedAt: string;
}

/** Payload for creating or (fully) updating a trail bookmark. */
export interface TrailBookmarkPayload {
  deviceId: number;
  name: string;
  description: string;
  from: string;
  to: string;
}

// ---------------------------------------------------------------------------
// Geofence
// ---------------------------------------------------------------------------

/** A geographic boundary with GeoJSON geometry. */
export interface Geofence {
  id: number;
  name: string;
  description?: string;
  area: string;
  /** GeoJSON string of the geofence geometry. */
  geometry?: string;
  calendarId?: number | null;
  attributes?: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
  /** Present only in admin list-all responses. */
  ownerName?: string;
}

/** Payload for creating a new geofence. */
export interface CreateGeofencePayload {
  name: string;
  description?: string;
  geometry: string;
  calendarId?: number | null;
}

/** Payload for updating an existing geofence. */
export interface UpdateGeofencePayload {
  name?: string;
  description?: string;
  geometry?: string;
  calendarId?: number | null;
}

// ---------------------------------------------------------------------------
// Event
// ---------------------------------------------------------------------------

/** A system event (geofence enter/exit, alarm, etc). */
export interface Event {
  id: number;
  deviceId: number;
  geofenceId?: number | null;
  type: string;
  positionId?: number | null;
  /** The event time. Mapped from Go field `Timestamp` with JSON tag `eventTime`. */
  eventTime: string;
  attributes?: Record<string, unknown>;
}

// ---------------------------------------------------------------------------
// Command
// ---------------------------------------------------------------------------

/** A control command sent to a device. */
export interface Command {
  id: number;
  deviceId: number;
  type: string;
  attributes?: Record<string, unknown>;
  status: string;
  result?: string | null;
  createdAt: string;
  executedAt?: string | null;
}

// ---------------------------------------------------------------------------
// Notification
// ---------------------------------------------------------------------------

export interface NotificationConfigWebhook {
  channel: "webhook";
  webhookUrl: string;
  headers?: Record<string, string>;
}
export type NotificationConfig = NotificationConfigWebhook;

/** A notification rule defining when and how to send notifications. */
export interface NotificationRule {
  id: number;
  userId: number;
  name: string;
  eventTypes: string[];
  channel: "webhook";
  config: NotificationConfig;
  template: string;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
  /** Present only in admin list-all responses. */
  ownerName?: string;
}

/** Payload for creating a notification rule. */
export interface CreateNotificationPayload {
  name: string;
  eventTypes: string[];
  channel: string;
  config: NotificationConfig;
  template?: string;
  enabled?: boolean;
}

/** Payload for updating a notification rule. */
export interface UpdateNotificationPayload {
  name?: string;
  eventTypes?: string[];
  channel?: string;
  config?: NotificationConfig;
  template?: string;
  enabled?: boolean;
}

/** A notification delivery log entry. */
export interface NotificationLog {
  id: number;
  ruleId: number;
  eventId?: number;
  status: string;
  sentAt?: string;
  error?: string;
  responseCode?: number;
  createdAt: string;
}

// ---------------------------------------------------------------------------
// Device Share
// ---------------------------------------------------------------------------

/** A shareable link for public device tracking. */
export interface DeviceShare {
  id: number;
  deviceId: number;
  token: string;
  shareUrl: string;
  createdBy: number;
  expiresAt: string | null;
  createdAt: string;
}

// ---------------------------------------------------------------------------
// Session / Auth
// ---------------------------------------------------------------------------

/** An active user session. */
export interface Session {
  id: string;
  userId: number;
  rememberMe: boolean;
  apiKeyId?: number | null;
  apiKeyName?: string | null;
  isCurrent?: boolean;
  createdAt: string;
  expiresAt: string;
  lastSeenAt?: string | null;
  lastSeenIp?: string | null;
  lastSeenUserAgent?: string | null;
}

/** Response from token generation (POST /api/session/token). */
export interface TokenResponse {
  token: string;
}

/** Sudo status response. */
export interface SudoStatusResponse {
  isSudo: boolean;
  originalUserId?: number;
  originalUser?: string;
  targetUserId?: number;
  user?: string;
  expiresAt?: string;
}

// ---------------------------------------------------------------------------
// Admin Statistics
// ---------------------------------------------------------------------------

/** Platform-wide aggregate statistics. */
export interface PlatformStats {
  totalUsers: number;
  totalDevices: number;
  totalPositions: number;
  totalEvents: number;
  notificationsSent: number;
  devicesByStatus: Record<string, number>;
  positionsToday: number;
  activeUsers: number;
}

/** Statistics for a specific user. */
export interface UserStats {
  userId: number;
  devicesOwned: number;
  totalPositions: number;
  lastLogin?: string | null;
  eventsTriggered: number;
  geofencesOwned: number;
}

// ---------------------------------------------------------------------------
// Audit Log
// ---------------------------------------------------------------------------

/** A single audit log entry. */
export interface AuditEntry {
  id: number;
  timestamp: string;
  userId?: number | null;
  action: string;
  resourceType?: string | null;
  resourceId?: number | null;
  details?: Record<string, unknown>;
  ipAddress?: string | null;
  userAgent?: string | null;
}

/** Paginated response from the audit log endpoint. */
export interface AuditLogResponse {
  entries: AuditEntry[];
  total: number;
  limit: number;
  offset: number;
}

// ---------------------------------------------------------------------------
// API Keys
// ---------------------------------------------------------------------------

/** An API key for external integrations. */
export interface ApiKey {
  id: number;
  userId: number;
  /** Full token on creation, redacted (first 8 chars + "...") on list. */
  token: string;
  name: string;
  /** "full" or "readonly" */
  permissions: string;
  /** ISO 8601 expiration date, or null/undefined for never-expiring keys. */
  expiresAt?: string | null;
  createdAt: string;
  lastUsedAt?: string | null;
}

/** Payload for creating a new API key. */
export interface CreateApiKeyPayload {
  name: string;
  permissions: string;
  /** Number of hours from now until the key expires. Mutually exclusive with expiresAt. */
  expiresInHours?: number | null;
  /** RFC 3339 timestamp for custom expiration. Mutually exclusive with expiresInHours. */
  expiresAt?: string | null;
}

// ---------------------------------------------------------------------------
// Passkeys (WebAuthn)
// ---------------------------------------------------------------------------

/** A registered WebAuthn passkey credential (metadata only, no secrets). */
export interface PasskeyCredentialInfo {
  id: number;
  name: string;
  createdAt: string;
  lastUsedAt?: string | null;
}

// ---------------------------------------------------------------------------
// Calendar
// ---------------------------------------------------------------------------

/** A time-based schedule stored in iCalendar (RFC 5545) format. */
export interface Calendar {
  id: number;
  userId?: number;
  name: string;
  /** iCalendar (RFC 5545) data containing VEVENT components. */
  data: string;
  createdAt: string;
  updatedAt: string;
  /** Present only in admin list-all responses. */
  ownerName?: string;
}

/** Payload for creating a new calendar. */
export interface CreateCalendarPayload {
  name: string;
  data: string;
}

/** Payload for updating an existing calendar. */
export interface UpdateCalendarPayload {
  name?: string;
  data?: string;
}

/** Response from the calendar check endpoint. */
export interface CalendarCheckResponse {
  calendarId: number;
  name: string;
  active: boolean;
  checkedAt: string;
}

// ---------------------------------------------------------------------------
// WebSocket Messages
// ---------------------------------------------------------------------------

/**
 * A message received over the WebSocket connection.
 *
 * The server sends Traccar-compatible messages containing updated
 * devices, positions, and/or events.
 */
export interface WebSocketMessage {
  devices?: Device[];
  positions?: Position[];
  events?: Event[];
}

// ---------------------------------------------------------------------------
// Server Info
// ---------------------------------------------------------------------------

export interface ServerInfo {
  id: number;
  registration: boolean;
  readonly: boolean;
  deviceReadonly: boolean;
  limitCommands: boolean;
  version: string;
  map?: string;
  latitude?: number;
  longitude?: number;
  zoom?: number;
  openIdEnabled?: boolean;
  openIdForce?: boolean;
  aiEnabled?: boolean;
}

// ---------------------------------------------------------------------------
// AI Chat
// ---------------------------------------------------------------------------

export type ChatMessage =
  | { role: "user"; content: string }
  | { role: "assistant"; content: string; toolCalls?: { id: string; name: string; arguments: unknown }[] }
  | { role: "tool"; toolCallId: string; name: string; content: string };

export type ChatEvent =
  | { type: "token"; delta: string }
  | { type: "tool_call"; id: string; name: string; arguments?: unknown }
  | { type: "tool_result"; id: string; name: string; result?: unknown; error?: string }
  | { type: "done" }
  | { type: "error"; message: string };

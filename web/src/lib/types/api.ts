export interface User {
  id: number;
  email: string;
  name: string;
  createdAt: string;

  administrator: boolean;
  readonly: boolean;
  disabled: boolean;
  attributes?: Record<string, unknown>;
}

export interface UserPayload {
  email: string;
  name: string;
  password?: string;
  role?: string;
}

export interface UpdateProfilePayload {
  name?: string;
  email?: string;
  currentPassword?: string;
  password?: string;
}

/** Device connection state; "unknown" means it never connected. */
export type DeviceStatus = "online" | "offline" | "unknown";

export interface Device {
  id: number;
  uniqueId: string;
  name: string;
  protocol?: string;
  status: DeviceStatus;
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
  batteryLevel?: number | null;
  attributes?: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
  /** Present only in admin list-all responses. */
  ownerName?: string;
}

export interface DevicePayload {
  uniqueId: string;
  name: string;
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

export interface Position {
  id: number;
  deviceId: number;
  protocol?: string;
  serverTime?: string | null;
  deviceTime?: string | null;
  fixTime: string;
  valid: boolean;
  latitude: number;
  longitude: number;
  altitude?: number | null;
  speed?: number | null;
  course?: number | null;
  address?: string | null;
  accuracy?: number | null;
  outdated?: boolean;
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

/** A time-based schedule stored in iCalendar (RFC 5545) format. */
export interface Calendar {
  id: number;
  name: string;
  data: string;
  createdAt: string;
  updatedAt: string;
  /** Present only in admin list-all responses. */
  ownerName?: string;
}

export interface CalendarPayload {
  name: string;
  data: string;
}

export interface CalendarCheckResponse {
  active: boolean;
  nextTrigger?: string | null;
}

/** A named, saved time range of a device's trail (e.g. a hike). */
export interface TrailBookmark {
  id: number;
  userId?: number;
  deviceId: number;
  deviceName?: string;
  name: string;
  description: string;
  from: string;
  to: string;
  createdAt: string;
  updatedAt: string;
}

export interface TrailBookmarkPayload {
  deviceId: number;
  name: string;
  description: string;
  from: string;
  to: string;
}

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

export interface CreateGeofencePayload {
  name: string;
  description?: string;
  geometry: string;
  calendarId?: number | null;
}

export interface UpdateGeofencePayload {
  name?: string;
  description?: string;
  geometry?: string;
  calendarId?: number | null;
}

export interface Event {
  id: number;
  deviceId: number;
  geofenceId?: number | null;
  type: string;
  positionId?: number | null;
  eventTime: string;
  attributes?: Record<string, unknown>;
}

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

export interface NotificationConfigWebhook {
  channel: "webhook";
  webhookUrl: string;
  headers?: Record<string, string>;
}
/**
 * Sends a device command to the device that triggered the event. Attributes
 * use the same discriminated shape as POST /api/commands/send.
 */
export interface NotificationConfigCommand {
  channel: "command";
  commandType: string;
  attributes?: Record<string, unknown>;
}
export type NotificationConfig = NotificationConfigWebhook | NotificationConfigCommand;
export type NotificationChannel = NotificationConfig["channel"];

export interface NotificationRule {
  id: number;
  userId: number;
  name: string;
  eventTypes: string[];
  channel: NotificationChannel;
  config: NotificationConfig;
  template: string;
  enabled: boolean;
  /** Geofence filter for geofence enter/exit events; empty = all geofences. */
  geofenceIds: number[];
  createdAt: string;
  updatedAt: string;
  /** Present only in admin list-all responses. */
  ownerName?: string;
}

export interface NotificationPayload {
  name: string;
  eventTypes: string[];
  channel: string;
  config: NotificationConfig;
  template?: string;
  enabled?: boolean;
  geofenceIds?: number[];
}

export interface NotificationLog {
  id: number;
  ruleId: number;
  eventId?: number;
  status: "sent" | "queued" | "failed";
  sentAt?: string | null;
  error?: string;
  responseCode?: number;
  createdAt: string;
  /** Context of the triggering event; absent when the event was deleted. */
  eventType?: string;
  eventTime?: string | null;
  eventAttributes?: Record<string, unknown>;
  deviceId?: number | null;
  deviceName?: string;
  geofenceName?: string;
}

export interface DeviceShare {
  id: number;
  deviceId: number;
  token: string;
  createdBy: number;
  expiresAt?: string | null;
  createdAt: string;
}

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

export interface TokenResponse {
  token: string;
}

export interface SudoStatusResponse {
  active: boolean;
  originalUserId?: number | null;
  targetUserId?: number | null;
}

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

export interface UserStats {
  userId: number;
  devicesOwned: number;
  totalPositions: number;
  lastLogin?: string | null;
  eventsTriggered: number;
  geofencesOwned: number;
}

export interface AuditEntry {
  id: number;
  action: string;
  userId: number;
  userEmail?: string;
  resourceType?: string;
  resourceId?: string;
  metadata?: Record<string, unknown>;
  ipAddress?: string;
  createdAt: string;
}

export interface AuditLogResponse {
  entries: AuditEntry[];
  total: number;
}

export interface ApiKey {
  id: number;
  userId: number;
  /** Full token on creation, redacted (first 8 chars + "...") on list. */
  token: string;
  name: string;
  /** "full" or "readonly" */
  permissions: string;
  expiresAt?: string | null;
  createdAt: string;
  lastUsedAt?: string | null;
}

export interface CreateApiKeyPayload {
  name: string;
  permissions: string;
  /** RFC 3339 expiry; omit for a never-expiring key. */
  expiresAt?: string | null;
}

export interface PasskeyCredentialInfo {
  id: number;
  name: string;
  createdAt: string;
  lastUsedAt?: string | null;
}

export interface WebSocketMessage {
  devices?: Device[];
  positions?: Position[];
  events?: Event[];
}

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

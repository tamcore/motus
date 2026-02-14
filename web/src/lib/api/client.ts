import type {
  ApiKey,
  AuditLogResponse,
  Calendar,
  CalendarCheckResponse,
  Command,
  CreateApiKeyPayload,
  CreateCalendarPayload,
  CreateDevicePayload,
  CreateGeofencePayload,
  CreateNotificationPayload,
  CreateUserPayload,
  Device,
  DeviceShare,
  Geofence,
  NotificationLog,
  NotificationRule,
  PlatformStats,
  Position,
  Session,
  SharedDeviceResponse,
  SudoStatusResponse,
  TestNotificationResponse,
  TokenResponse,
  UpdateCalendarPayload,
  UpdateDevicePayload,
  UpdateGeofencePayload,
  UpdateNotificationPayload,
  UpdateUserPayload,
  User,
  UserStats,
} from "$lib/types/api";
import { currentUser } from "$lib/stores/auth";
import * as svelteStore from "svelte/store";

const API_BASE = "/api";

export class APIError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

// Store CSRF token in memory (set by server in X-CSRF-Token response header)
let csrfToken: string | null = null;

async function request<T>(
  endpoint: string,
  options: RequestInit = {},
): Promise<T> {
  const headers: HeadersInit = {
    "Content-Type": "application/json",
    ...options.headers,
  };

  // Include CSRF token for state-changing requests
  const method = (options.method || "GET").toUpperCase();
  if (csrfToken && ["POST", "PUT", "DELETE", "PATCH"].includes(method)) {
    (headers as Record<string, string>)["X-CSRF-Token"] = csrfToken;
  }

  const response = await fetch(`${API_BASE}${endpoint}`, {
    ...options,
    credentials: "include",
    headers,
  });

  // Extract and store CSRF token from response
  const token = response.headers.get("X-CSRF-Token");
  if (token) {
    csrfToken = token;
  }

  if (!response.ok) {
    throw new APIError(response.status, await response.text());
  }

  if (
    response.status === 204 ||
    response.headers.get("content-length") === "0"
  ) {
    return undefined as T;
  }

  return response.json();
}

export const api = {
  // ---------------------------------------------------------------------------
  // Auth
  // ---------------------------------------------------------------------------

  /** Authenticate with email and password. Returns the logged-in user. */
  login: (email: string, password: string, remember: boolean = false) =>
    request<User>("/session", {
      method: "POST",
      body: JSON.stringify({ email, password, remember }),
    }),

  /** End the current session. */
  logout: () => request<void>("/session", { method: "DELETE" }),

  /** Get the currently authenticated user. */
  getCurrentUser: () => request<User>("/session"),

  /** Generate a new API token for the current user. */
  generateToken: () =>
    request<TokenResponse>("/session/token", { method: "POST" }),

  // ---------------------------------------------------------------------------
  // Devices
  // ---------------------------------------------------------------------------

  /** List all devices accessible to the current user. */
  getDevices: () => request<Device[]>("/devices"),

  /** Get a single device by ID. */
  getDevice: (id: number) => request<Device>(`/devices/${id}`),

  /** Create a new device. */
  createDevice: (device: CreateDevicePayload) =>
    request<Device>("/devices", {
      method: "POST",
      body: JSON.stringify(device),
    }),

  /** Update an existing device. */
  updateDevice: (id: number, device: UpdateDevicePayload) =>
    request<Device>(`/devices/${id}`, {
      method: "PUT",
      body: JSON.stringify(device),
    }),

  /** Delete a device by ID. */
  deleteDevice: (id: number) =>
    request<void>(`/devices/${id}`, { method: "DELETE" }),

  /** Import a GPX track file into a device's position history. */
  importGPX: async (
    deviceId: number,
    file: File,
  ): Promise<{ imported: number; skipped: number }> => {
    const formData = new FormData();
    formData.append("file", file);

    const headers: Record<string, string> = {};
    if (csrfToken) {
      headers["X-CSRF-Token"] = csrfToken;
    }

    const response = await fetch(`${API_BASE}/devices/${deviceId}/gpx`, {
      method: "POST",
      body: formData,
      credentials: "include",
      headers,
    });

    const token = response.headers.get("X-CSRF-Token");
    if (token) csrfToken = token;

    if (!response.ok) {
      throw new APIError(response.status, await response.text());
    }
    return response.json();
  },

  // ---------------------------------------------------------------------------
  // Positions
  // ---------------------------------------------------------------------------

  /** Query positions with optional filters. */
  getPositions: (params?: {
    deviceId?: number;
    from?: string;
    to?: string;
    limit?: number;
  }) => {
    const query = new URLSearchParams();
    if (params?.deviceId) query.set("deviceId", String(params.deviceId));
    if (params?.from) query.set("from", params.from);
    if (params?.to) query.set("to", params.to);
    if (params?.limit) query.set("limit", String(params.limit));

    // Normalize speed from knots (Traccar API) to km/h (internal UI unit).
    return request<Position[]>(`/positions?${query}`).then((positions) =>
      positions.map((pos) =>
        pos.speed != null ? { ...pos, speed: pos.speed * 1.852 } : pos,
      ),
    );
  },

  // ---------------------------------------------------------------------------
  // Commands
  // ---------------------------------------------------------------------------

  /** Get supported command types for devices. */
  getCommandTypes: () => request<string[]>("/commands/types"),

  /** Send a command to a device. */
  sendCommand: (command: {
    deviceId: number;
    type: string;
    attributes?: Record<string, unknown>;
  }) =>
    request<Command>("/commands/send", {
      method: "POST",
      body: JSON.stringify(command),
    }),

  /** List recent commands for a device. */
  listCommands: (deviceId: number, limit: number = 10) =>
    request<Command[]>(`/commands?deviceId=${deviceId}&limit=${limit}`),

  // ---------------------------------------------------------------------------
  // Geofences
  // ---------------------------------------------------------------------------

  /** List all geofences for the current user. */
  getGeofences: () => request<Geofence[]>("/geofences"),

  /** Create a new geofence. */
  createGeofence: (geofence: CreateGeofencePayload) =>
    request<Geofence>("/geofences", {
      method: "POST",
      body: JSON.stringify(geofence),
    }),

  /** Update an existing geofence. */
  updateGeofence: (id: number, geofence: UpdateGeofencePayload) =>
    request<Geofence>(`/geofences/${id}`, {
      method: "PUT",
      body: JSON.stringify(geofence),
    }),

  /** Delete a geofence by ID. */
  deleteGeofence: (id: number) =>
    request<void>(`/geofences/${id}`, { method: "DELETE" }),

  // ---------------------------------------------------------------------------
  // Notifications
  // ---------------------------------------------------------------------------

  /** List all notification rules for the current user. */
  getNotifications: () => request<NotificationRule[]>("/notifications"),

  /** Create a new notification rule. */
  createNotification: (rule: CreateNotificationPayload) =>
    request<NotificationRule>("/notifications", {
      method: "POST",
      body: JSON.stringify(rule),
    }),

  /** Update an existing notification rule. */
  updateNotification: (id: number, rule: UpdateNotificationPayload) =>
    request<NotificationRule>(`/notifications/${id}`, {
      method: "PUT",
      body: JSON.stringify(rule),
    }),

  /** Delete a notification rule by ID. */
  deleteNotification: (id: number) =>
    request<void>(`/notifications/${id}`, { method: "DELETE" }),

  /** Send a test notification for the given rule. */
  testNotification: (id: number) =>
    request<TestNotificationResponse>(`/notifications/${id}/test`, {
      method: "POST",
    }),

  /** Get delivery logs for a notification rule. */
  getNotificationLogs: (id: number) =>
    request<NotificationLog[]>(`/notifications/${id}/logs`),

  // ---------------------------------------------------------------------------
  // Users (admin only)
  // ---------------------------------------------------------------------------

  /** List all users (admin only). */
  getUsers: () => request<User[]>("/users"),

  /** Create a new user (admin only). */
  createUser: (user: CreateUserPayload) =>
    request<User>("/users", {
      method: "POST",
      body: JSON.stringify(user),
    }),

  /** Update an existing user (admin only). */
  updateUser: (id: number, user: UpdateUserPayload) =>
    request<User>(`/users/${id}`, {
      method: "PUT",
      body: JSON.stringify(user),
    }),

  /** Delete a user by ID (admin only). */
  deleteUser: (id: number) =>
    request<void>(`/users/${id}`, { method: "DELETE" }),

  /** List all devices in the system (admin only). */
  getAllDevices: () => request<Device[]>("/admin/devices"),

  /** List all geofences in the system (admin only). */
  getAllGeofences: () => request<Geofence[]>("/admin/geofences"),

  /** List all calendars in the system (admin only). */
  getAllCalendars: () => request<Calendar[]>("/admin/calendars"),

  /** List all notification rules in the system (admin only). */
  getAllNotifications: () => request<NotificationRule[]>("/admin/notifications"),

  /** Get latest positions for all devices in the system (admin only). */
  getAllPositions: () =>
    request<Position[]>("/admin/positions").then((positions) =>
      positions.map((pos) =>
        pos.speed != null ? { ...pos, speed: pos.speed * 1.852 } : pos,
      ),
    ),

  /** Get devices assigned to a user (admin only). */
  getUserDevices: (id: number) => request<Device[]>(`/users/${id}/devices`),

  /** Assign a device to a user (admin only). */
  assignDevice: (userId: number, deviceId: number) =>
    request<void>(`/users/${userId}/devices/${deviceId}`, { method: "POST" }),

  /** Unassign a device from a user (admin only). */
  unassignDevice: (userId: number, deviceId: number) =>
    request<void>(`/users/${userId}/devices/${deviceId}`, {
      method: "DELETE",
    }),

  // ---------------------------------------------------------------------------
  // Admin: Sudo
  // ---------------------------------------------------------------------------

  /** Start impersonating a user (admin only). */
  startSudo: (userId: number) =>
    request<SudoStatusResponse>(`/admin/sudo/${userId}`, { method: "POST" }),

  /** End impersonation session. */
  endSudo: () =>
    request<SudoStatusResponse>("/admin/sudo", { method: "DELETE" }),

  /** Get current sudo/impersonation status. */
  getSudoStatus: () => request<SudoStatusResponse>("/admin/sudo"),

  // ---------------------------------------------------------------------------
  // Admin: Statistics
  // ---------------------------------------------------------------------------

  /** Get platform-wide aggregate statistics (admin only). */
  getPlatformStatistics: () => request<PlatformStats>("/admin/statistics"),

  /** Get statistics for a specific user (admin only). */
  getUserStatistics: (userId: number) =>
    request<UserStats>(`/admin/statistics/users/${userId}`),

  // ---------------------------------------------------------------------------
  // Admin: Audit log
  // ---------------------------------------------------------------------------

  /** Query paginated audit log entries (admin only). */
  getAuditLog: (filters?: {
    action?: string;
    userId?: string;
    resourceType?: string;
    limit?: number;
    offset?: number;
  }) => {
    const params = new URLSearchParams();
    if (filters?.action) params.set("action", filters.action);
    if (filters?.userId) params.set("userId", filters.userId);
    if (filters?.resourceType) params.set("resourceType", filters.resourceType);
    params.set("limit", String(filters?.limit || 50));
    if (filters?.offset) params.set("offset", String(filters.offset));
    return request<AuditLogResponse>(`/admin/audit?${params}`);
  },

  // ---------------------------------------------------------------------------
  // API Keys
  // ---------------------------------------------------------------------------

  /** List all API keys for the current user. Tokens are redacted. */
  getApiKeys: () => request<ApiKey[]>("/keys"),

  /** Create a new API key. Returns the full token (shown once). */
  createApiKey: (payload: CreateApiKeyPayload) =>
    request<ApiKey>("/keys", {
      method: "POST",
      body: JSON.stringify(payload),
    }),

  /** Revoke (delete) an API key by ID. */
  deleteApiKey: (id: number) =>
    request<void>(`/keys/${id}`, { method: "DELETE" }),

  // ---------------------------------------------------------------------------
  // Sessions
  // ---------------------------------------------------------------------------

  /** List all active sessions for the current user. */
  getSessions: () => request<Session[]>("/sessions"),

  /** Revoke (delete) a session by ID. */
  revokeSession: (id: string) =>
    request<void>(`/sessions/${id}`, { method: "DELETE" }),

  // ---------------------------------------------------------------------------
  // Device sharing
  // ---------------------------------------------------------------------------

  /** Create a new share link for a device. */
  createDeviceShare: (deviceId: number, expiresInHours?: number | null) =>
    request<DeviceShare>(`/devices/${deviceId}/share`, {
      method: "POST",
      body: JSON.stringify(expiresInHours ? { expiresInHours } : {}),
    }),

  /** List all active shares for a device. */
  listDeviceShares: (deviceId: number) =>
    request<DeviceShare[]>(`/devices/${deviceId}/shares`),

  /** Revoke a share link by ID. */
  deleteShare: (shareId: number) =>
    request<void>(`/shares/${shareId}`, { method: "DELETE" }),

  /** Get the shared device and its latest positions by share token. */
  getSharedDevice: (token: string) =>
    request<SharedDeviceResponse>(`/share/${token}`).then((data) => ({
      ...data,
      positions: data.positions?.map((pos) =>
        pos.speed != null ? { ...pos, speed: pos.speed * 1.852 } : pos,
      ),
    })),

  // ---------------------------------------------------------------------------
  // Calendars
  // ---------------------------------------------------------------------------

  /** List all calendars for the current user. */
  getCalendars: () => request<Calendar[]>("/calendars"),

  /** Create a new calendar. */
  createCalendar: (payload: CreateCalendarPayload) =>
    request<Calendar>("/calendars", {
      method: "POST",
      body: JSON.stringify(payload),
    }),

  /** Update an existing calendar. */
  updateCalendar: (id: number, payload: UpdateCalendarPayload) =>
    request<Calendar>(`/calendars/${id}`, {
      method: "PUT",
      body: JSON.stringify(payload),
    }),

  /** Delete a calendar by ID. */
  deleteCalendar: (id: number) =>
    request<void>(`/calendars/${id}`, { method: "DELETE" }),

  /** Check if a calendar is currently active. */
  checkCalendar: (id: number) =>
    request<CalendarCheckResponse>(`/calendars/${id}/check`),
};

/**
 * Fetch devices respecting the admin "show all" setting.
 * Admins with the toggle enabled get all devices in the instance;
 * everyone else gets only their assigned devices.
 */
export async function fetchDevices(isAdmin: boolean): Promise<Device[]> {
  const { getSettings } = await import("$lib/stores/settings");
  if (isAdmin && getSettings().showAllDevices) {
    const devices = await (api.getAllDevices() as Promise<Device[]>);
    return stripOwnOwnerName(devices);
  }
  return api.getDevices() as Promise<Device[]>;
}

/**
 * Fetch latest positions respecting the admin "show all" setting.
 * Admins with the toggle enabled get positions for all devices;
 * everyone else gets only their assigned devices' positions.
 */
export async function fetchPositions(isAdmin: boolean): Promise<Position[]> {
  const { getSettings } = await import("$lib/stores/settings");
  if (isAdmin && getSettings().showAllDevices) {
    return api.getAllPositions() as Promise<Position[]>;
  }
  return api.getPositions() as Promise<Position[]>;
}

/** Fetch geofences respecting the admin "show all" setting. */
export async function fetchGeofences(isAdmin: boolean): Promise<Geofence[]> {
  const { getSettings } = await import("$lib/stores/settings");
  if (isAdmin && getSettings().showAllDevices) {
    const geofences = await (api.getAllGeofences() as Promise<Geofence[]>);
    return stripOwnOwnerName(geofences);
  }
  return api.getGeofences() as Promise<Geofence[]>;
}

/** Fetch calendars respecting the admin "show all" setting. */
export async function fetchCalendars(isAdmin: boolean): Promise<Calendar[]> {
  const { getSettings } = await import("$lib/stores/settings");
  if (isAdmin && getSettings().showAllDevices) {
    const calendars = await (api.getAllCalendars() as Promise<Calendar[]>);
    return stripOwnOwnerName(calendars);
  }
  return api.getCalendars() as Promise<Calendar[]>;
}

/** Fetch notifications respecting the admin "show all" setting. */
export async function fetchNotifications(isAdmin: boolean): Promise<NotificationRule[]> {
  const { getSettings } = await import("$lib/stores/settings");
  if (isAdmin && getSettings().showAllDevices) {
    const rules = await (api.getAllNotifications() as Promise<NotificationRule[]>);
    return stripOwnOwnerName(rules);
  }
  return api.getNotifications() as Promise<NotificationRule[]>;
}

/** Clear ownerName on items that belong to the current user so they don't get highlighted. */
function stripOwnOwnerName<T extends { ownerName?: string }>(items: T[]): T[] {
  const { get } = svelteStore;
  const user = get(currentUser) as Record<string, unknown> | null;
  const myName = (user?.name as string) || "";
  if (!myName) return items;
  return items.map((item) =>
    item.ownerName === myName ? { ...item, ownerName: undefined } : item
  );
}

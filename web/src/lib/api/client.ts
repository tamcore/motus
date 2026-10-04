import type {
  AuthenticationResponseJSON,
  PublicKeyCredentialCreationOptionsJSON,
  PublicKeyCredentialRequestOptionsJSON,
  RegistrationResponseJSON,
} from "@simplewebauthn/browser";
import type {
  ApiKey,
  AuditLogResponse,
  Calendar,
  CalendarCheckResponse,
  Command,
  CreateApiKeyPayload,
  CalendarPayload,
  CreateGeofencePayload,
  Device,
  DevicePayload,
  DeviceShare,
  Geofence,
  NotificationLog,
  NotificationPayload,
  NotificationRule,
  PasskeyCredentialInfo,
  PlatformStats,
  Position,
  PositionPoint,
  ServerInfo,
  Session,
  SudoStatusResponse,
  TokenResponse,
  TrailBookmark,
  TrailBookmarkPayload,
  UpdateGeofencePayload,
  UpdateProfilePayload,
  User,
  UserPayload,
  UserStats,
} from "$lib/types/api";
import type { Trip } from "$lib/utils/trips";
import type { Stop } from "$lib/utils/stops";
import { currentUser, isAdmin } from "$lib/stores/auth";
import { settings } from "$lib/stores/settings";
import { getStoredAuthToken } from "$lib/auth-token-store";
import { get } from "svelte/store";

const API_BASE = "/api";
const KNOTS_TO_KMH = 1.852;
const CSRF_METHODS = ["POST", "PUT", "DELETE", "PATCH"];

/** Converts a position's speed from knots (Traccar API) to km/h (internal UI unit). */
export function speedToKmh<T extends { speed?: number | null }>(pos: T): T {
  return pos.speed != null ? { ...pos, speed: pos.speed * KNOTS_TO_KMH } : pos;
}

export class APIError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

let csrfToken: string | null = null;

/** The `error` field of a JSON error body, else the raw body. */
function errorMessage(body: string, status: number): string {
  try {
    const parsed = JSON.parse(body) as { error?: unknown } | null;
    if (typeof parsed?.error === "string" && parsed.error !== "") return parsed.error;
  } catch {
    // Plain-text body.
  }
  return body || `Request failed (${status})`;
}

/** Fetches an API endpoint with auth/CSRF headers; throws APIError on a non-2xx status. */
export async function apiFetch(endpoint: string, options: RequestInit = {}): Promise<Response> {
  const method = (options.method || "GET").toUpperCase();
  const authToken = await getStoredAuthToken();
  const headers: HeadersInit = {
    // The browser sets the multipart boundary for FormData bodies.
    ...(options.body instanceof FormData ? {} : { "Content-Type": "application/json" }),
    ...(csrfToken && CSRF_METHODS.includes(method) ? { "X-CSRF-Token": csrfToken } : {}),
    ...(authToken ? { "X-Auth-Token": authToken } : {}),
    ...options.headers,
  };

  const response = await fetch(`${API_BASE}${endpoint}`, {
    ...options,
    credentials: "include",
    headers,
  });

  const token = response.headers.get("X-CSRF-Token");
  if (token) csrfToken = token;

  if (!response.ok) {
    throw new APIError(response.status, errorMessage(await response.text(), response.status));
  }
  return response;
}

export async function request<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
  const response = await apiFetch(endpoint, options);
  if (response.status === 204 || response.headers.get("content-length") === "0") {
    return undefined as T;
  }
  return response.json();
}

const send = (method: string, body: unknown): RequestInit => ({ method, body: JSON.stringify(body) });

type QueryValue = string | number | boolean | null | undefined;

/** `?a=1&b=2` from the truthy values (arrays repeat the key); "" when none. */
function query(params: Record<string, QueryValue | QueryValue[]>): string {
  const q = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    for (const v of Array.isArray(value) ? value : [value]) {
      if (v) q.append(key, String(v));
    }
  }
  const qs = q.toString();
  return qs ? `?${qs}` : "";
}

interface ReportParams {
  deviceIds: number[];
  from: string;
  to: string;
}

export const api = {
  getServerInfo: () => request<ServerInfo>("/server"),

  login: (email: string, password: string, remember: boolean = false) =>
    request<User>("/session", send("POST", { email, password, remember })),
  loginWithToken: (token: string) => request<User>(`/session${query({ token })}`),
  logout: () => request<void>("/session", { method: "DELETE" }),
  getCurrentUser: () => request<User>("/session"),
  generateToken: () => request<TokenResponse>("/session/token", { method: "POST" }),

  /** Returns the raw options JSON for @simplewebauthn/browser's startRegistration. */
  passkeyRegisterBegin: () =>
    request<PublicKeyCredentialCreationOptionsJSON>("/session/passkey/register/begin", { method: "POST" }),
  passkeyRegisterFinish: (attestationJSON: RegistrationResponseJSON, name: string) =>
    request<PasskeyCredentialInfo>(
      `/session/passkey/register/finish${query({ name })}`,
      send("POST", attestationJSON),
    ),
  /** Public. Returns the raw options JSON for @simplewebauthn/browser's startAuthentication. */
  passkeyLoginBegin: () =>
    request<PublicKeyCredentialRequestOptionsJSON>("/session/passkey/login/begin", { method: "POST" }),
  /** Public. On success the session cookie is set. */
  passkeyLoginFinish: (assertionJSON: AuthenticationResponseJSON) =>
    request<User>("/session/passkey/login/finish", send("POST", assertionJSON)),
  listPasskeys: () => request<PasskeyCredentialInfo[]>("/session/passkey/credentials"),
  deletePasskey: (id: number) => request<void>(`/session/passkey/credentials/${id}`, { method: "DELETE" }),

  getDevices: () => request<Device[]>("/devices"),
  createDevice: (device: DevicePayload) => request<Device>("/devices", send("POST", device)),
  updateDevice: (id: number, device: DevicePayload) => request<Device>(`/devices/${id}`, send("PUT", device)),
  deleteDevice: (id: number) => request<void>(`/devices/${id}`, { method: "DELETE" }),
  importGPX: (deviceId: number, file: File) => {
    const body = new FormData();
    body.append("file", file);
    return request<{ imported: number }>(`/devices/${deviceId}/gpx`, { method: "POST", body });
  },

  /** `all` counts every device (admin only). */
  countPositions: (params: { from: string; to: string; all?: boolean }) =>
    request<{ count: number }>(`/positions/count${query(params)}`).then((r) => r.count),
  getPositions: (params: { deviceId?: number; from?: string; to?: string; limit?: number } = {}) =>
    request<Position[]>(`/positions${query(params)}`).then((positions) => positions.map(speedToKmh)),
  /** Server-side trips and stops in one pass; trip speeds are converted from knots to km/h. */
  getActivityReport: ({ deviceIds, from, to }: ReportParams) =>
    request<{ trips: Omit<Trip, "id">[]; stops: Omit<Stop, "id">[] }>(
      `/reports/activity${query({ from, to, deviceId: deviceIds })}`,
    ).then(({ trips, stops }) => ({
      trips: trips.map((t, i): Trip => ({
        ...t,
        id: `trip-${t.deviceId}-${i}`,
        avgSpeed: t.avgSpeed * KNOTS_TO_KMH,
        maxSpeed: t.maxSpeed * KNOTS_TO_KMH,
      })),
      stops: stops.map((s, i): Stop => ({ ...s, id: `stop-${s.deviceId}-${i}` })),
    })),
  /** Compact range points, sampled to `limit` by the server. */
  getPositionPoints: (params: { deviceId: number; from: string; to: string; limit?: number }) =>
    request<PositionPoint[]>(`/positions/points${query(params)}`).then((points) => points.map(speedToKmh)),

  /** With deviceId, only the types the device protocol supports. */
  getCommandTypes: (deviceId?: number) => request<{ type: string }[]>(`/commands/types${query({ deviceId })}`),
  sendCommand: (command: { deviceId: number; type: string; attributes?: Record<string, unknown> }) =>
    request<Command>("/commands/send", send("POST", command)),
  listCommands: (deviceId: number, limit: number = 10) =>
    request<Command[]>(`/commands${query({ deviceId, limit })}`),

  getGeofences: () => request<Geofence[]>("/geofences"),
  createGeofence: (geofence: CreateGeofencePayload) => request<Geofence>("/geofences", send("POST", geofence)),
  updateGeofence: (id: number, geofence: UpdateGeofencePayload) =>
    request<Geofence>(`/geofences/${id}`, send("PUT", geofence)),
  deleteGeofence: (id: number) => request<void>(`/geofences/${id}`, { method: "DELETE" }),

  getNotifications: () => request<NotificationRule[]>("/notifications"),
  createNotification: (rule: NotificationPayload) =>
    request<NotificationRule>("/notifications", send("POST", rule)),
  updateNotification: (id: number, rule: NotificationPayload) =>
    request<NotificationRule>(`/notifications/${id}`, send("PUT", rule)),
  deleteNotification: (id: number) => request<void>(`/notifications/${id}`, { method: "DELETE" }),
  testNotification: (id: number) => request<void>(`/notifications/${id}/test`, { method: "POST" }),
  getNotificationLogs: (id: number) => request<NotificationLog[]>(`/notifications/${id}/logs`),

  updateProfile: (data: UpdateProfilePayload) => request<User>("/profile", send("PUT", data)),

  getUsers: () => request<User[]>("/users"),
  createUser: (user: UserPayload) => request<User>("/users", send("POST", user)),
  updateUser: (id: number, user: UserPayload) => request<User>(`/users/${id}`, send("PUT", user)),
  deleteUser: (id: number) => request<void>(`/users/${id}`, { method: "DELETE" }),
  getAllDevices: () => request<Device[]>("/admin/devices"),
  getAllGeofences: () => request<Geofence[]>("/admin/geofences"),
  getAllCalendars: () => request<Calendar[]>("/admin/calendars"),
  getAllNotifications: () => request<NotificationRule[]>("/admin/notifications"),
  /** Without params: latest position per device; with from/to/limit: all devices in the window. */
  getAllPositions: (params: { from?: string; to?: string; limit?: number } = {}) =>
    request<Position[]>(`/admin/positions${query(params)}`).then((positions) => positions.map(speedToKmh)),
  getUserDevices: (id: number) => request<Device[]>(`/users/${id}/devices`),
  assignDevice: (userId: number, deviceId: number) =>
    request<void>(`/users/${userId}/devices/${deviceId}`, { method: "POST" }),
  unassignDevice: (userId: number, deviceId: number) =>
    request<void>(`/users/${userId}/devices/${deviceId}`, { method: "DELETE" }),

  startSudo: (userId: number) => request<void>(`/admin/sudo/${userId}`, { method: "POST" }),
  endSudo: () => request<void>("/admin/sudo", { method: "DELETE" }),
  getSudoStatus: () => request<SudoStatusResponse>("/admin/sudo"),

  getPlatformStatistics: () => request<PlatformStats>("/admin/statistics"),
  getUserStatistics: (userId: number) => request<UserStats>(`/admin/statistics/users/${userId}`),

  getAuditLog: (filters: {
    action?: string;
    userId?: string;
    resourceType?: string;
    limit?: number;
    offset?: number;
  } = {}) => request<AuditLogResponse>(`/admin/audit${query({ ...filters, limit: filters.limit || 50 })}`),

  /** Tokens are redacted. */
  getApiKeys: () => request<ApiKey[]>("/keys"),
  /** Returns the full token (shown once). */
  createApiKey: (payload: CreateApiKeyPayload) => request<ApiKey>("/keys", send("POST", payload)),
  deleteApiKey: (id: number) => request<void>(`/keys/${id}`, { method: "DELETE" }),

  getSessions: () => request<Session[]>("/sessions"),
  revokeSession: (id: string) => request<void>(`/sessions/${id}`, { method: "DELETE" }),
  /** Revokes all sessions except the active one. */
  revokeAllOtherSessions: () => request<void>("/sessions", { method: "DELETE" }),

  createDeviceShare: (deviceId: number, expiresAt?: string | null) =>
    request<DeviceShare>(`/devices/${deviceId}/share`, send("POST", expiresAt ? { expiresAt } : {})),
  listDeviceShares: (deviceId: number) => request<DeviceShare[]>(`/devices/${deviceId}/shares`),
  deleteShare: (shareId: number) => request<void>(`/shares/${shareId}`, { method: "DELETE" }),

  getCalendars: () => request<Calendar[]>("/calendars"),
  createCalendar: (payload: CalendarPayload) => request<Calendar>("/calendars", send("POST", payload)),
  updateCalendar: (id: number, payload: CalendarPayload) =>
    request<Calendar>(`/calendars/${id}`, send("PUT", payload)),
  deleteCalendar: (id: number) => request<void>(`/calendars/${id}`, { method: "DELETE" }),
  checkCalendar: (id: number) => request<CalendarCheckResponse>(`/calendars/${id}/check`),

  getTrailBookmarks: (deviceId?: number) => request<TrailBookmark[]>(`/trail-bookmarks${query({ deviceId })}`),
  createTrailBookmark: (payload: TrailBookmarkPayload) =>
    request<TrailBookmark>("/trail-bookmarks", send("POST", payload)),
  updateTrailBookmark: (id: number, payload: TrailBookmarkPayload) =>
    request<TrailBookmark>(`/trail-bookmarks/${id}`, send("PUT", payload)),
  deleteTrailBookmark: (id: number) => request<void>(`/trail-bookmarks/${id}`, { method: "DELETE" }),
};

/**
 * Fetch items respecting the admin "show all" setting.
 * Admins with the toggle enabled get all items in the instance;
 * everyone else gets only their own.
 */
async function fetchScoped<T extends object>(
  all: () => Promise<T[]>,
  own: () => Promise<T[]>,
): Promise<T[]> {
  if (get(isAdmin) && get(settings).showAllDevices) {
    return stripOwnOwnerName(await all());
  }
  return own();
}

export const fetchDevices = () =>
  fetchScoped<Device>(api.getAllDevices, api.getDevices);
export const fetchPositions = () =>
  fetchScoped<Position>(() => api.getAllPositions(), () => api.getPositions());
export const fetchGeofences = () =>
  fetchScoped<Geofence>(api.getAllGeofences, api.getGeofences);
export const fetchCalendars = () =>
  fetchScoped<Calendar>(api.getAllCalendars, api.getCalendars);
export const fetchNotifications = () =>
  fetchScoped<NotificationRule>(api.getAllNotifications, api.getNotifications);

/** Clear ownerName on items that belong to the current user so they don't get highlighted. */
export function stripOwnOwnerName<T extends object>(items: T[]): T[] {
  const myName = get(currentUser)?.name || "";
  if (!myName) return items;
  return items.map((item) =>
    "ownerName" in item && item.ownerName === myName ? { ...item, ownerName: undefined } : item
  );
}

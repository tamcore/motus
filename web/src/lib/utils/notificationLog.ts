import type { NotificationLog } from "$lib/types/api";
import { getEventLabel } from "$lib/utils/notificationRules";
import { formatDate, formatDistance, formatDuration, formatMileage, formatSpeed } from "$lib/utils/formatting";

/** A single attribute of the triggering event, optionally with its prior value. */
export interface LogChange {
  label: string;
  from?: string;
  to: string;
}

const ATTRIBUTE_LABELS: Record<string, string> = {
  speed: "Speed",
  distance: "Distance",
  mileage: "Mileage",
  idleDuration: "Idle duration",
  ignition: "Ignition",
  alarm: "Alarm",
};

/** Format a log timestamp; missing or unparseable values render as "--". */
export function formatLogTime(value: string | undefined | null): string {
  if (!value || isNaN(new Date(value).getTime())) return "--";
  return formatDate(value);
}

/** One-line description of what triggered the delivery, e.g. "Car · Geofence Enter · Home". */
export function logSubject(log: NotificationLog): string {
  return [log.deviceName, log.eventType && getEventLabel(log.eventType), log.geofenceName]
    .filter(Boolean)
    .join(" · ");
}

function labelFor(key: string): string {
  if (ATTRIBUTE_LABELS[key]) return ATTRIBUTE_LABELS[key];
  const words = key.replace(/([a-z0-9])([A-Z])/g, "$1 $2").toLowerCase();
  return words.charAt(0).toUpperCase() + words.slice(1);
}

function formatValue(key: string, value: unknown): string {
  if (typeof value === "number") {
    switch (key) {
      case "speed":
        return formatSpeed(value);
      case "distance":
        return formatDistance(value);
      case "mileage":
        return formatMileage(value);
      case "idleDuration":
        return formatDuration(value * 60); // stored in minutes
    }
  }
  if (typeof value === "boolean") return value ? "on" : "off";
  if (value === null || value === undefined) return "--";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

/**
 * Before/after view of the triggering event. Attributes stored as
 * `previousX` + `X` become a from → to change; state-transition events
 * (online/offline, ignition, geofence enter/exit) imply their prior state.
 */
export function logChanges(log: NotificationLog): LogChange[] {
  const changes: LogChange[] = [];

  switch (log.eventType) {
    case "deviceOnline":
      changes.push({ label: "Status", from: "offline", to: "online" });
      break;
    case "deviceOffline":
      changes.push({ label: "Status", from: "online", to: "offline" });
      break;
    case "geofenceEnter":
      changes.push({ label: log.geofenceName || "Geofence", from: "outside", to: "inside" });
      break;
    case "geofenceExit":
      changes.push({ label: log.geofenceName || "Geofence", from: "inside", to: "outside" });
      break;
  }

  const attrs = log.eventAttributes ?? {};
  for (const [key, value] of Object.entries(attrs)) {
    if (/^previous[A-Z]/.test(key)) {
      const base = key.charAt(8).toLowerCase() + key.slice(9);
      if (base in attrs) continue; // rendered together with its current value
    }
    const prevKey = `previous${key.charAt(0).toUpperCase()}${key.slice(1)}`;
    if (prevKey in attrs) {
      changes.push({ label: labelFor(key), from: formatValue(key, attrs[prevKey]), to: formatValue(key, value) });
    } else if (key === "ignition" && typeof value === "boolean") {
      changes.push({ label: labelFor(key), from: formatValue(key, !value), to: formatValue(key, value) });
    } else {
      changes.push({ label: labelFor(key), to: formatValue(key, value) });
    }
  }

  return changes;
}

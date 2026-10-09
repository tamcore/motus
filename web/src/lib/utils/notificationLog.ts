import type { NotificationLog } from "$lib/types/api";
import { getEventLabel } from "$lib/utils/notificationRules";
import { formatDistance, formatDuration, formatMileage, formatSpeed } from "$lib/utils/formatting";

/** A single attribute of the triggering event, optionally with its prior value. */
export interface LogChange {
  label: string;
  from?: string;
  to: string;
}

/** One-line description of what triggered the delivery, e.g. "Car · Geofence Enter · Home". */
export function logSubject(log: NotificationLog): string {
  return [log.deviceName, log.eventType && getEventLabel(log.eventType), log.geofenceName]
    .filter(Boolean)
    .join(" · ");
}

/**
 * Before/after view of the triggering event, one case per event type of the
 * EventAttributes schema (docs/openapi.yaml). State-transition events imply
 * their prior state.
 */
export function logChanges(log: NotificationLog): LogChange[] {
  const a = log.eventAttributes ?? {};
  const num = (v: unknown) => (typeof v === "number" ? v : 0);
  switch (log.eventType) {
    case "deviceOnline":
      return [{ label: "Status", from: "offline", to: "online" }];
    case "deviceOffline":
      return [{ label: "Status", from: "online", to: "offline" }];
    case "geofenceEnter":
      return [{ label: log.geofenceName || "Geofence", from: "outside", to: "inside" }];
    case "geofenceExit":
      return [{ label: log.geofenceName || "Geofence", from: "inside", to: "outside" }];
    case "ignitionOn":
      return [{ label: "Ignition", from: "off", to: "on" }];
    case "ignitionOff":
      return [{ label: "Ignition", from: "on", to: "off" }];
    case "motion":
      return [{ label: "Speed", from: formatSpeed(num(a.previousSpeed)), to: formatSpeed(num(a.speed)) }];
    case "tripCompleted":
      return [
        { label: "Distance", to: formatDistance(num(a.distance)) },
        { label: "Mileage", to: formatMileage(num(a.mileage)) },
      ];
    case "deviceIdle":
      return [{ label: "Idle duration", to: formatDuration(num(a.idleDuration) * 60) }]; // stored in minutes
    case "alarm":
      return typeof a.alarm === "string" ? [{ label: "Alarm", to: a.alarm }] : [];
    default:
      return [];
  }
}

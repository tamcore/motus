import type { NotificationConfigCommand } from "$lib/types/api";
import {
  commandAttributesPayload,
  COMMAND_TYPE_LABELS,
  formatInterval,
  MAX_REPORTING_INTERVAL_SECONDS,
} from "./commands";

/**
 * Command types a notification rule may send automatically, in display
 * order. factoryReset is rejected by the API for automated rules.
 */
export const NOTIFICATION_COMMAND_TYPES: readonly string[] = [
  "positionPeriodic",
  "positionSingle",
  "rebootDevice",
  "sosNumber",
  "setSpeedAlarm",
  "custom",
];

/** Raw form inputs of the command parameters. */
export interface CommandFormValues {
  frequency?: string;
  phoneNumber?: string;
  speed?: string;
  text?: string;
}

/** True when the event types include a geofence transition. */
export function hasGeofenceEvent(eventTypes: string[]): boolean {
  return eventTypes.includes("geofenceEnter") || eventTypes.includes("geofenceExit");
}

function parsePositiveInt(raw: string | undefined): number | null {
  const s = (raw ?? "").trim();
  if (!/^\d+$/.test(s)) return null;
  const n = Number(s);
  return n > 0 ? n : null;
}

/**
 * Validates the command form and builds the rule config. Returns either the
 * config or a user-facing error.
 */
export function buildCommandConfig(
  commandType: string,
  values: CommandFormValues,
): { config?: NotificationConfigCommand; error?: string } {
  if (!commandType) return { error: "Select a command" };

  const attrs: Record<string, unknown> = {};
  switch (commandType) {
    case "positionPeriodic": {
      const freq = parsePositiveInt(values.frequency);
      if (freq === null) return { error: "Interval must be a positive whole number of seconds" };
      if (freq > MAX_REPORTING_INTERVAL_SECONDS) {
        return { error: `Interval must be at most ${MAX_REPORTING_INTERVAL_SECONDS} seconds (1 day)` };
      }
      attrs.frequency = freq;
      break;
    }
    case "sosNumber": {
      const phone = (values.phoneNumber ?? "").trim();
      if (!phone) return { error: "SOS number is required" };
      attrs.phoneNumber = phone;
      break;
    }
    case "setSpeedAlarm": {
      const s = (values.speed ?? "").trim();
      const speed = Number(s);
      if (s === "" || !Number.isFinite(speed) || speed < 0) {
        return { error: "Speed must be 0 or a positive number" };
      }
      attrs.speed = speed;
      break;
    }
    case "custom": {
      const text = (values.text ?? "").trim();
      if (!text) return { error: "Command text is required" };
      attrs.text = text;
      break;
    }
  }

  const config: NotificationConfigCommand = { channel: "command", commandType };
  const attributes = commandAttributesPayload(commandType, attrs);
  if (attributes) config.attributes = attributes;
  return { config };
}

const RECONNECT_EVENTS: Readonly<Record<string, string>> = {
  deviceOnline: "Device Online",
  deviceOffline: "Device Offline",
};

/**
 * A reboot (or a custom command, which may reboot the device) sent on
 * deviceOnline/deviceOffline makes the device reconnect and fire the rule
 * again, without end. The API rejects these combinations; returns a
 * user-facing error, or null when the combination is fine.
 */
export function commandEventConflict(commandType: string, eventTypes: string[]): string | null {
  if (commandType !== "rebootDevice" && commandType !== "custom") return null;
  const event = eventTypes.find((et) => et in RECONNECT_EVENTS);
  if (!event) return null;
  const label = COMMAND_TYPE_LABELS[commandType] ?? commandType;
  return `${label} cannot be triggered by ${RECONNECT_EVENTS[event]}: the device would reconnect and trigger the rule again without end.`;
}

/** Restores the command form inputs from a stored config. */
export function commandFormValues(config: NotificationConfigCommand): Required<CommandFormValues> {
  const a = config.attributes ?? {};
  const str = (v: unknown) => (v === undefined || v === null ? "" : String(v));
  return {
    frequency: str(a.frequency),
    phoneNumber: str(a.phoneNumber),
    speed: str(a.speed),
    text: str(a.text),
  };
}

/** Human-readable summary of a command action, e.g. "Set Reporting Interval: 20 s". */
export function describeCommandAction(config: NotificationConfigCommand): string {
  const label = COMMAND_TYPE_LABELS[config.commandType] ?? config.commandType;
  const a = config.attributes ?? {};
  switch (config.commandType) {
    case "positionPeriodic":
      return typeof a.frequency === "number" ? `${label}: ${formatInterval(a.frequency)}` : label;
    case "sosNumber":
      return a.phoneNumber ? `${label}: ${a.phoneNumber}` : label;
    case "setSpeedAlarm":
      return a.speed !== undefined ? `${label}: ${a.speed} km/h` : label;
    case "custom":
      return a.text ? `${label}: ${a.text}` : label;
    default:
      return label;
  }
}

function unavailableGeofenceLabel(id: number): string {
  return `Geofence #${id} (unavailable)`;
}

/**
 * Summary of a rule's geofence filter ("All geofences" when empty). IDs not
 * in the lookup (deleted, or not accessible) are flagged as unavailable.
 */
export function describeGeofenceFilter(
  ids: number[] | undefined,
  geofences: ReadonlyArray<{ id: number; name: string }>,
): string {
  if (!ids || ids.length === 0) return "All geofences";
  return ids
    .map((id) => geofences.find((g) => g.id === id)?.name ?? unavailableGeofenceLabel(id))
    .join(", ");
}

/** A checkbox of the geofence filter in the rule editor. */
export interface GeofenceFilterOption {
  id: number;
  label: string;
  /** Selected in the rule but not in the lookup (deleted or inaccessible). */
  unavailable: boolean;
}

/**
 * Checkboxes of the geofence filter: every available geofence (with its owner
 * in admin all-users lists), followed by geofences that are no longer
 * available but are selected or stored in the rule being edited. Those stay
 * selected until the user explicitly unticks them; silently dropping them
 * could turn the filter into "all geofences". Stored ones stay listed after
 * unticking so the user can tick them again before saving.
 */
export function geofenceFilterOptions(
  selected: number[],
  geofences: ReadonlyArray<{ id: number; name: string; ownerName?: string }>,
  stored: number[] = [],
): GeofenceFilterOption[] {
  const options: GeofenceFilterOption[] = geofences.map((g) => ({
    id: g.id,
    label: g.ownerName ? `${g.name} (${g.ownerName})` : g.name,
    unavailable: false,
  }));
  for (const id of new Set([...stored, ...selected])) {
    if (!geofences.some((g) => g.id === id)) {
      options.push({ id, label: unavailableGeofenceLabel(id), unavailable: true });
    }
  }
  return options;
}

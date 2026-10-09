import type { NotificationConfigCommand } from "$lib/types/api";
import { buildCommandAttributes, COMMAND_TYPE_LABELS, type CommandFormValues, formatInterval } from "./commands";

export const EVENT_TYPES = [
  { value: "geofenceEnter", label: "Geofence Enter" },
  { value: "geofenceExit", label: "Geofence Exit" },
  { value: "deviceOnline", label: "Device Online" },
  { value: "deviceOffline", label: "Device Offline" },
  { value: "motion", label: "Motion Started" },
  { value: "deviceIdle", label: "Device Idle" },
  { value: "ignitionOn", label: "Ignition On" },
  { value: "ignitionOff", label: "Ignition Off" },
  { value: "alarm", label: "Alarm (SOS / Power Cut)" },
  { value: "tripCompleted", label: "Trip Completed" },
];

export function getEventLabel(eventType: string): string {
  return EVENT_TYPES.find((e) => e.value === eventType)?.label || eventType;
}

export const CHANNELS = [
  { value: "webhook", label: "Webhook" },
  { value: "command", label: "Device Command" },
];

export const TEMPLATE_VARIABLES = [
  "{{device.id}}",
  "{{device.name}}",
  "{{device.uniqueId}}",
  "{{event.type}}",
  "{{event.timestamp}}",
  "{{position.latitude}}",
  "{{position.longitude}}",
  "{{position.speed}}",
];

export const DEFAULT_TEMPLATE =
  '{"device": "{{device.name}}", "event": "{{event.type}}"}';

/**
 * Command types a notification rule may send automatically, in display
 * order. factoryReset is rejected by the API for automated rules.
 */
export const NOTIFICATION_COMMAND_TYPES: readonly string[] = Object.keys(COMMAND_TYPE_LABELS).filter(
  (t) => t !== "factoryReset",
);

/** True when the event types include a geofence transition. */
export function hasGeofenceEvent(eventTypes: string[]): boolean {
  return eventTypes.includes("geofenceEnter") || eventTypes.includes("geofenceExit");
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
  const { attributes, error } = buildCommandAttributes(commandType, values);
  if (error) return { error };
  const config: NotificationConfigCommand = { channel: "command", commandType };
  if (attributes) config.attributes = attributes;
  return { config };
}

const RECONNECT_EVENTS = EVENT_TYPES.filter((e) => e.value === "deviceOnline" || e.value === "deviceOffline");

/**
 * A reboot (or a custom command, which may reboot the device) sent on
 * deviceOnline/deviceOffline makes the device reconnect and fire the rule
 * again, without end. The API rejects these combinations; returns a
 * user-facing error, or null when the combination is fine.
 */
export function commandEventConflict(commandType: string, eventTypes: string[]): string | null {
  if (commandType !== "rebootDevice" && commandType !== "custom") return null;
  const event = RECONNECT_EVENTS.find((e) => eventTypes.includes(e.value));
  if (!event) return null;
  const label = COMMAND_TYPE_LABELS[commandType] ?? commandType;
  return `${label} cannot be triggered by ${event.label}: the device would reconnect and trigger the rule again without end.`;
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

type FilterKind = "Geofence" | "Device";
type FilterItem = { id: number; name: string; ownerName?: string };

function unavailableLabel(kind: FilterKind, id: number): string {
  return `${kind} #${id} (unavailable)`;
}

function describeFilter(kind: FilterKind, ids: number[] | undefined, items: ReadonlyArray<FilterItem>): string {
  if (!ids || ids.length === 0) return `All ${kind.toLowerCase()}s`;
  return ids.map((id) => items.find((i) => i.id === id)?.name ?? unavailableLabel(kind, id)).join(", ");
}

/** A checkbox of the geofence or device filter in the rule editor. */
interface FilterOption {
  id: number;
  label: string;
  /** Selected in the rule but not in the lookup (deleted or inaccessible). */
  unavailable: boolean;
}

function filterOptions(
  kind: FilterKind,
  selected: number[],
  items: ReadonlyArray<FilterItem>,
  stored: number[],
): FilterOption[] {
  const options: FilterOption[] = items.map((i) => ({
    id: i.id,
    label: i.ownerName ? `${i.name} (${i.ownerName})` : i.name,
    unavailable: false,
  }));
  for (const id of new Set([...stored, ...selected])) {
    if (!items.some((i) => i.id === id)) {
      options.push({ id, label: unavailableLabel(kind, id), unavailable: true });
    }
  }
  return options;
}

/**
 * Summary of a rule's geofence filter ("All geofences" when empty). IDs not
 * in the lookup (deleted, or not accessible) are flagged as unavailable.
 */
export function describeGeofenceFilter(ids: number[] | undefined, geofences: ReadonlyArray<FilterItem>): string {
  return describeFilter("Geofence", ids, geofences);
}

/** Summary of a rule's device filter ("All devices" when empty). */
export function describeDeviceFilter(ids: number[] | undefined, devices: ReadonlyArray<FilterItem>): string {
  return describeFilter("Device", ids, devices);
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
  geofences: ReadonlyArray<FilterItem>,
  stored: number[] = [],
): FilterOption[] {
  return filterOptions("Geofence", selected, geofences, stored);
}

/** Checkboxes of the device filter; same rules as geofenceFilterOptions. */
export function deviceFilterOptions(
  selected: number[],
  devices: ReadonlyArray<FilterItem>,
  stored: number[] = [],
): FilterOption[] {
  return filterOptions("Device", selected, devices, stored);
}

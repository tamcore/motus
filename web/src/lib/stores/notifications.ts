import { writable, derived } from "svelte/store";
import type { NotificationRule } from "$lib/types/api";

export type { NotificationRule };

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

export const notificationRules = writable<NotificationRule[]>([]);
export const enabledRuleCount = derived(
  notificationRules,
  ($rules) => $rules.filter((r) => r.enabled).length,
);

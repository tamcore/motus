import { writable, derived } from "svelte/store";

export interface NotificationRule {
  id: number;
  userId: number;
  name: string;
  eventTypes: string[];
  channel: "webhook";
  config: Record<string, any>;
  template: string;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
  ownerName?: string;
}

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
  { value: "overspeed", label: "Overspeed" },
  { value: "motion", label: "Motion Started" },
  { value: "deviceIdle", label: "Device Idle" },
  { value: "ignitionOn", label: "Ignition On" },
  { value: "ignitionOff", label: "Ignition Off" },
  { value: "alarm", label: "Alarm (SOS / Power Cut)" },
  { value: "tripCompleted", label: "Trip Completed" },
];

export const CHANNELS = [{ value: "webhook", label: "Webhook" }];

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

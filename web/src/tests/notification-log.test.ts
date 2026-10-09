import { describe, it, expect, vi } from "vitest";

vi.mock("$app/environment", () => ({ browser: false }));
vi.mock("$lib/stores/settings", async () => {
  const { writable, get } = await import("svelte/store");
  const store = writable({ dateFormat: "iso", timezone: "UTC", units: "metric" });
  return { settings: store, getSettings: () => get(store) };
});

import { logChanges, logSubject } from "$lib/utils/notificationLog";
import type { NotificationLog } from "$lib/types/api";

function log(overrides: Partial<NotificationLog> = {}): NotificationLog {
  return { id: 1, ruleId: 1, status: "sent", createdAt: "2026-10-08T12:30:00Z", ...overrides };
}

describe("logSubject", () => {
  it("names the device, event and geofence", () => {
    expect(
      logSubject(log({ eventType: "geofenceEnter", deviceName: "Family Car", geofenceName: "Home" })),
    ).toBe("Family Car · Geofence Enter · Home");
  });

  it("omits missing parts", () => {
    expect(logSubject(log({ eventType: "alarm" }))).toBe("Alarm (SOS / Power Cut)");
  });

  it("returns empty string without event context", () => {
    expect(logSubject(log())).toBe("");
  });
});

// Attributes as sent by the API: the EventAttributes union carries its
// discriminator in `type`, which must not render as a change.
describe("logChanges", () => {
  it("shows the speed change of motion events", () => {
    const changes = logChanges(
      log({ eventType: "motion", eventAttributes: { type: "motion", speed: 42, previousSpeed: 0 } }),
    );
    expect(changes).toEqual([{ label: "Speed", from: "0.0 km/h", to: "42.0 km/h" }]);
  });

  it("derives the previous ignition state", () => {
    expect(
      logChanges(log({ eventType: "ignitionOn", eventAttributes: { type: "ignitionOn", ignition: true } })),
    ).toEqual([{ label: "Ignition", from: "off", to: "on" }]);
    expect(
      logChanges(log({ eventType: "ignitionOff", eventAttributes: { type: "ignitionOff", ignition: false } })),
    ).toEqual([{ label: "Ignition", from: "on", to: "off" }]);
  });

  it("shows device status transitions for online/offline events", () => {
    expect(logChanges(log({ eventType: "deviceOnline" }))).toEqual([
      { label: "Status", from: "offline", to: "online" },
    ]);
    expect(logChanges(log({ eventType: "deviceOffline" }))).toEqual([
      { label: "Status", from: "online", to: "offline" },
    ]);
  });

  it("shows geofence transitions", () => {
    expect(
      logChanges(log({ eventType: "geofenceExit", geofenceName: "Home", eventAttributes: { type: "geofenceExit" } })),
    ).toEqual([{ label: "Home", from: "inside", to: "outside" }]);
  });

  it("formats trip, idle and alarm values with units", () => {
    expect(
      logChanges(
        log({ eventType: "tripCompleted", eventAttributes: { type: "tripCompleted", distance: 12.34, mileage: 1000 } }),
      ),
    ).toEqual([
      { label: "Distance", to: "12.34 km" },
      { label: "Mileage", to: expect.stringContaining("1") },
    ]);
    expect(
      logChanges(log({ eventType: "deviceIdle", eventAttributes: { type: "deviceIdle", idleDuration: 15 } })),
    ).toEqual([{ label: "Idle duration", to: "15m" }]);
    expect(logChanges(log({ eventType: "alarm", eventAttributes: { type: "alarm", alarm: "sos" } }))).toEqual([
      { label: "Alarm", to: "sos" },
    ]);
  });

  it("returns nothing without event context", () => {
    expect(logChanges(log())).toEqual([]);
  });
});

import { describe, it, expect, vi } from "vitest";
import {
  DEFAULT_AWAY_INTERVAL,
  DEFAULT_HOME_INTERVAL,
  NOTIFICATION_COMMAND_TYPES,
  buildIntervalAutomationRules,
  createAllOrNone,
  buildCommandConfig,
  commandEventConflict,
  commandFormValues,
  describeCommandAction,
  describeDeviceFilter,
  describeGeofenceFilter,
  deviceFilterOptions,
  geofenceFilterOptions,
  hasGeofenceEvent,
} from "./notificationRules";

describe("NOTIFICATION_COMMAND_TYPES", () => {
  it("offers the reporting interval first and never factory reset", () => {
    expect(NOTIFICATION_COMMAND_TYPES).toEqual([
      "positionPeriodic",
      "positionSingle",
      "rebootDevice",
      "sosNumber",
      "setSpeedAlarm",
      "custom",
    ]);
    expect(NOTIFICATION_COMMAND_TYPES).not.toContain("factoryReset");
  });
});

describe("hasGeofenceEvent", () => {
  it("detects geofence enter/exit", () => {
    expect(hasGeofenceEvent(["geofenceEnter"])).toBe(true);
    expect(hasGeofenceEvent(["alarm", "geofenceExit"])).toBe(true);
    expect(hasGeofenceEvent(["alarm", "deviceOnline"])).toBe(false);
    expect(hasGeofenceEvent([])).toBe(false);
  });
});

// Parameter validation is covered by buildCommandAttributes in commands.test.ts.
describe("buildCommandConfig", () => {
  it("wraps the validated attributes in a command config", () => {
    expect(buildCommandConfig("positionPeriodic", { frequency: "20" })).toEqual({
      config: {
        channel: "command",
        commandType: "positionPeriodic",
        attributes: { type: "positionPeriodic", frequency: 20 },
      },
    });
    expect(buildCommandConfig("positionSingle", {})).toEqual({
      config: { channel: "command", commandType: "positionSingle" },
    });
  });

  it("returns validation errors", () => {
    expect(buildCommandConfig("positionPeriodic", { frequency: "0" }).error).toMatch(/interval/i);
    expect(buildCommandConfig("", {}).error).toBeTruthy();
  });
});

describe("commandEventConflict", () => {
  it("rejects reboot and custom commands on device online/offline (reconnect loop)", () => {
    expect(commandEventConflict("rebootDevice", ["deviceOnline"])).toMatch(/Device Online/);
    expect(commandEventConflict("rebootDevice", ["alarm", "deviceOffline"])).toMatch(/Device Offline/);
    expect(commandEventConflict("custom", ["deviceOnline"])).toBeTruthy();
    expect(commandEventConflict("custom", ["deviceOffline"])).toBeTruthy();
  });

  it("allows other combinations", () => {
    expect(commandEventConflict("rebootDevice", ["geofenceExit", "alarm"])).toBeNull();
    expect(commandEventConflict("positionPeriodic", ["deviceOnline", "deviceOffline"])).toBeNull();
    expect(commandEventConflict("positionSingle", ["deviceOnline"])).toBeNull();
  });
});

describe("commandFormValues", () => {
  it("restores form values from a stored command config", () => {
    expect(
      commandFormValues({
        channel: "command",
        commandType: "positionPeriodic",
        attributes: { type: "positionPeriodic", frequency: 300 },
      }),
    ).toEqual({ frequency: "300", phoneNumber: "", speed: "", text: "" });
    expect(commandFormValues({ channel: "command", commandType: "positionSingle" })).toEqual({
      frequency: "",
      phoneNumber: "",
      speed: "",
      text: "",
    });
  });
});

describe("describeCommandAction", () => {
  it("formats the reporting interval like the device command dialog", () => {
    expect(
      describeCommandAction({
        channel: "command",
        commandType: "positionPeriodic",
        attributes: { type: "positionPeriodic", frequency: 20 },
      }),
    ).toBe("Set Reporting Interval: 20 s");
    expect(
      describeCommandAction({
        channel: "command",
        commandType: "positionPeriodic",
        attributes: { type: "positionPeriodic", frequency: 300 },
      }),
    ).toBe("Set Reporting Interval: 300 s (5 min)");
  });

  it("falls back to the command label", () => {
    expect(describeCommandAction({ channel: "command", commandType: "positionSingle" })).toBe(
      "Request Position",
    );
  });
});

describe("describeGeofenceFilter", () => {
  const geofences = [
    { id: 1, name: "Home" },
    { id: 2, name: "Park" },
  ];

  it("treats an empty filter as all geofences", () => {
    expect(describeGeofenceFilter([], geofences)).toBe("All geofences");
    expect(describeGeofenceFilter(undefined, geofences)).toBe("All geofences");
  });

  it("counts geofences the user cannot see as 'N other' in count mode", () => {
    // Device attachments may include another owner's geofences on a shared device.
    expect(describeGeofenceFilter([1, 7, 8], geofences, "count")).toBe("Home, 2 other");
    expect(describeGeofenceFilter([9], geofences, "count")).toBe("1 other");
    expect(describeGeofenceFilter([2, 1], geofences, "count")).toBe("Park, Home");
  });

  it("lists geofence names and flags unavailable ones", () => {
    expect(describeGeofenceFilter([1, 2], geofences)).toBe("Home, Park");
    expect(describeGeofenceFilter([1, 9], geofences)).toBe("Home, Geofence #9 (unavailable)");
  });
});

describe("geofenceFilterOptions", () => {
  const geofences = [
    { id: 1, name: "Home" },
    { id: 2, name: "Park", ownerName: "Alice" },
  ];

  it("lists available geofences with owner hints", () => {
    expect(geofenceFilterOptions([], geofences)).toEqual([
      { id: 1, label: "Home", unavailable: false },
      { id: 2, label: "Park (Alice)", unavailable: false },
    ]);
  });

  it("appends selected geofences that are no longer available so they can be removed", () => {
    expect(geofenceFilterOptions([9, 1], geofences)).toEqual([
      { id: 1, label: "Home", unavailable: false },
      { id: 2, label: "Park (Alice)", unavailable: false },
      { id: 9, label: "Geofence #9 (unavailable)", unavailable: true },
    ]);
  });

  it("keeps an unavailable stored geofence listed after it is unticked", () => {
    // Unticking must not remove the checkbox, so the user can tick it again.
    expect(geofenceFilterOptions([1], geofences, [9])).toEqual([
      { id: 1, label: "Home", unavailable: false },
      { id: 2, label: "Park (Alice)", unavailable: false },
      { id: 9, label: "Geofence #9 (unavailable)", unavailable: true },
    ]);
  });

  it("lists each unavailable geofence once when it is both stored and selected", () => {
    expect(geofenceFilterOptions([9], geofences, [9, 1])).toEqual([
      { id: 1, label: "Home", unavailable: false },
      { id: 2, label: "Park (Alice)", unavailable: false },
      { id: 9, label: "Geofence #9 (unavailable)", unavailable: true },
    ]);
  });
});

describe("device filter", () => {
  const devices = [
    { id: 2, name: "Rex" },
    { id: 4, name: "Car", ownerName: "Alice" },
  ];

  it("summarizes the filter", () => {
    expect(describeDeviceFilter([], devices)).toBe("All devices");
    expect(describeDeviceFilter(undefined, devices)).toBe("All devices");
    expect(describeDeviceFilter([2, 9], devices)).toBe("Rex, Device #9 (unavailable)");
  });

  it("lists available devices plus unavailable selected or stored ones", () => {
    expect(deviceFilterOptions([9], devices, [8])).toEqual([
      { id: 2, label: "Rex", unavailable: false },
      { id: 4, label: "Car (Alice)", unavailable: false },
      { id: 8, label: "Device #8 (unavailable)", unavailable: true },
      { id: 9, label: "Device #9 (unavailable)", unavailable: true },
    ]);
  });
});

describe("buildIntervalAutomationRules", () => {
  const build = (deviceIds: number[], awayInterval: string, homeInterval: string) =>
    buildIntervalAutomationRules({ geofenceId: 7, geofenceName: "Home", deviceIds, awayInterval, homeInterval });

  it("creates a fast-interval exit rule and a slow-interval enter rule for the geofence and devices", () => {
    expect(build([4, 2, 4], String(DEFAULT_AWAY_INTERVAL), String(DEFAULT_HOME_INTERVAL))).toEqual([
      {
        name: "Left Home: report every 20 s",
        eventTypes: ["geofenceExit"],
        channel: "command",
        geofenceIds: [7],
        deviceIds: [2, 4],
        config: {
          channel: "command",
          commandType: "positionPeriodic",
          attributes: { type: "positionPeriodic", frequency: 20 },
        },
        enabled: true,
      },
      {
        name: "Entered Home: report every 300 s (5 min)",
        eventTypes: ["geofenceEnter"],
        channel: "command",
        geofenceIds: [7],
        deviceIds: [2, 4],
        config: {
          channel: "command",
          commandType: "positionPeriodic",
          attributes: { type: "positionPeriodic", frequency: 300 },
        },
        enabled: true,
      },
    ]);
  });

  it("requires at least one device", () => {
    expect(() => build([], "20", "300")).toThrow(/device/i);
  });

  it("rejects intervals the reporting interval command does not accept", () => {
    expect(() => build([2], "0", "300")).toThrow(/interval/i);
    expect(() => build([2], "20", "1.5")).toThrow(/interval/i);
    expect(() => build([2], "20", "86401")).toThrow(/1 day/);
  });
});

describe("createAllOrNone", () => {
  it("creates every rule in order", async () => {
    const create = vi.fn().mockResolvedValueOnce({ id: 1 }).mockResolvedValueOnce({ id: 2 });
    const remove = vi.fn();
    await expect(createAllOrNone(["a", "b"], create, remove)).resolves.toEqual([1, 2]);
    expect(create.mock.calls).toEqual([["a"], ["b"]]);
    expect(remove).not.toHaveBeenCalled();
  });

  it("deletes the rules already created when a later one fails", async () => {
    const create = vi.fn().mockResolvedValueOnce({ id: 1 }).mockRejectedValueOnce(new Error("boom"));
    const remove = vi.fn().mockResolvedValue(undefined);
    await expect(createAllOrNone(["a", "b"], create, remove)).rejects.toThrow("boom");
    expect(remove.mock.calls).toEqual([[1]]);
  });

  it("still reports the create error when the cleanup fails", async () => {
    const create = vi.fn().mockResolvedValueOnce({ id: 1 }).mockRejectedValueOnce(new Error("boom"));
    const remove = vi.fn().mockRejectedValue(new Error("gone"));
    await expect(createAllOrNone(["a", "b"], create, remove)).rejects.toThrow("boom");
  });
});

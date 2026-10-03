import { describe, it, expect } from "vitest";
import {
  NOTIFICATION_COMMAND_TYPES,
  buildCommandConfig,
  commandEventConflict,
  commandFormValues,
  describeCommandAction,
  describeGeofenceFilter,
  geofenceFilterOptions,
  hasGeofenceEvent,
} from "./notificationRules";
import { MAX_REPORTING_INTERVAL_SECONDS } from "./commands";

describe("NOTIFICATION_COMMAND_TYPES", () => {
  it("offers the reporting interval first and never factory reset", () => {
    expect(NOTIFICATION_COMMAND_TYPES[0]).toBe("positionPeriodic");
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

describe("buildCommandConfig", () => {
  it("builds a reporting interval command with discriminated attributes", () => {
    expect(buildCommandConfig("positionPeriodic", { frequency: "20" })).toEqual({
      config: {
        channel: "command",
        commandType: "positionPeriodic",
        attributes: { type: "positionPeriodic", frequency: 20 },
      },
    });
  });

  it("rejects a missing or non-positive interval", () => {
    expect(buildCommandConfig("positionPeriodic", { frequency: "" }).error).toMatch(/interval/i);
    expect(buildCommandConfig("positionPeriodic", { frequency: "0" }).error).toMatch(/interval/i);
    expect(buildCommandConfig("positionPeriodic", { frequency: "1.5" }).error).toMatch(/interval/i);
  });

  it("enforces the maximum interval of one day", () => {
    expect(MAX_REPORTING_INTERVAL_SECONDS).toBe(86400);
    expect(buildCommandConfig("positionPeriodic", { frequency: "86400" }).config?.attributes).toEqual({
      type: "positionPeriodic",
      frequency: 86400,
    });
    expect(buildCommandConfig("positionPeriodic", { frequency: "86401" }).error).toMatch(/86400/);
  });

  it("omits attributes for parameterless commands", () => {
    expect(buildCommandConfig("positionSingle", {})).toEqual({
      config: { channel: "command", commandType: "positionSingle" },
    });
  });

  it("validates the other parameterised commands", () => {
    expect(buildCommandConfig("sosNumber", { phoneNumber: " " }).error).toBeTruthy();
    expect(buildCommandConfig("sosNumber", { phoneNumber: "+49123" }).config?.attributes).toEqual({
      type: "sosNumber",
      phoneNumber: "+49123",
    });
    expect(buildCommandConfig("setSpeedAlarm", { speed: "-1" }).error).toBeTruthy();
    expect(buildCommandConfig("setSpeedAlarm", { speed: "0" }).config?.attributes).toEqual({
      type: "setSpeedAlarm",
      speed: 0,
    });
    expect(buildCommandConfig("custom", { text: "" }).error).toBeTruthy();
    expect(buildCommandConfig("custom", { text: "UPLOAD,60" }).config?.attributes).toEqual({
      type: "custom",
      text: "UPLOAD,60",
    });
  });

  it("rejects missing command type", () => {
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

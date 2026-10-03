import { describe, it, expect } from "vitest";
import {
  buildCommandAttributes,
  commandIntervalLabel,
  commandSentMessage,
  COMMAND_TYPE_LABELS,
  DEFAULT_REPORTING_INTERVAL_SECONDS,
  formatInterval,
  MAX_REPORTING_INTERVAL_SECONDS,
  REPORTING_INTERVAL_PRESETS,
} from "./commands";

describe("REPORTING_INTERVAL_PRESETS", () => {
  it("offers 5 s, 20 s, 1 min, 5 min and 10 min", () => {
    expect(REPORTING_INTERVAL_PRESETS.map((p) => p.seconds)).toEqual([5, 20, 60, 300, 600]);
    expect(REPORTING_INTERVAL_PRESETS.map((p) => p.label)).toEqual([
      "5 sec",
      "20 sec",
      "1 min",
      "5 min",
      "10 min",
    ]);
  });

  it("includes the 60 s default", () => {
    expect(DEFAULT_REPORTING_INTERVAL_SECONDS).toBe(60);
    expect(REPORTING_INTERVAL_PRESETS.some((p) => p.seconds === DEFAULT_REPORTING_INTERVAL_SECONDS)).toBe(true);
  });
});

describe("formatInterval", () => {
  it("shows seconds and, for whole minutes, the minute value", () => {
    expect(formatInterval(5)).toBe("5 s");
    expect(formatInterval(20)).toBe("20 s");
    expect(formatInterval(60)).toBe("60 s (1 min)");
    expect(formatInterval(300)).toBe("300 s (5 min)");
    expect(formatInterval(600)).toBe("600 s (10 min)");
    expect(formatInterval(90)).toBe("90 s");
  });
});

describe("commandSentMessage", () => {
  it("says the interval was requested, not set, when the command is sent", () => {
    const msg = commandSentMessage("positionPeriodic", { frequency: 60 }, "sent");
    expect(msg).toBe("Reporting interval of 60 s (1 min) requested; command sent to device.");
    expect(msg).not.toMatch(/\bset\b/i);
  });

  it("says the interval request is queued for an offline device", () => {
    const msg = commandSentMessage("positionPeriodic", { frequency: 5 }, "pending");
    expect(msg).toBe("Reporting interval of 5 s requested; device is offline, command queued.");
    expect(msg).not.toMatch(/\bset\b/i);
  });

  it("uses the command label for other commands", () => {
    expect(commandSentMessage("rebootDevice", {}, "sent")).toBe("Reboot Device: command sent to device.");
    expect(commandSentMessage("rebootDevice", {}, "pending")).toBe(
      "Reboot Device: device is offline, command queued.",
    );
  });
});

describe("commandIntervalLabel", () => {
  it("shows the requested interval of a positionPeriodic command", () => {
    expect(commandIntervalLabel({ type: "positionPeriodic", attributes: { frequency: 60 } })).toBe(
      "Interval: 60 s (1 min)",
    );
    expect(commandIntervalLabel({ type: "positionPeriodic", attributes: { frequency: 45 } })).toBe(
      "Interval: 45 s",
    );
  });

  it("returns null for other commands or a missing/invalid frequency", () => {
    expect(commandIntervalLabel({ type: "rebootDevice", attributes: { frequency: 60 } })).toBeNull();
    expect(commandIntervalLabel({ type: "positionPeriodic" })).toBeNull();
    expect(commandIntervalLabel({ type: "positionPeriodic", attributes: {} })).toBeNull();
    expect(commandIntervalLabel({ type: "positionPeriodic", attributes: { frequency: "abc" } })).toBeNull();
    expect(commandIntervalLabel({ type: "positionPeriodic", attributes: { frequency: 0 } })).toBeNull();
  });
});

// The API decodes command attributes as a oneOf discriminated by "type"
// (docs/openapi.yaml CommandAttributes). Attributes without "type" or an
// empty object are rejected with "unable to detect sum type variant".
describe("buildCommandAttributes", () => {
  it("validates parameters and adds the command type as discriminator", () => {
    expect(buildCommandAttributes("positionPeriodic", { frequency: "20" })).toEqual({
      attributes: { type: "positionPeriodic", frequency: 20 },
    });
    expect(buildCommandAttributes("sosNumber", { phoneNumber: " +49123 " })).toEqual({
      attributes: { type: "sosNumber", phoneNumber: "+49123" },
    });
    expect(buildCommandAttributes("setSpeedAlarm", { speed: "0" })).toEqual({
      attributes: { type: "setSpeedAlarm", speed: 0 },
    });
    expect(buildCommandAttributes("custom", { text: "UPLOAD,60" })).toEqual({
      attributes: { type: "custom", text: "UPLOAD,60" },
    });
  });

  it("omits attributes for commands without parameters", () => {
    expect(buildCommandAttributes("rebootDevice", {})).toEqual({});
    expect(buildCommandAttributes("positionSingle", { frequency: "20" })).toEqual({});
    expect(buildCommandAttributes("factoryReset", {})).toEqual({});
  });

  it("rejects a missing, non-positive or too large interval", () => {
    expect(MAX_REPORTING_INTERVAL_SECONDS).toBe(86400);
    for (const frequency of ["", "0", "1.5"]) {
      expect(buildCommandAttributes("positionPeriodic", { frequency }).error).toMatch(/interval/i);
    }
    expect(buildCommandAttributes("positionPeriodic", { frequency: "86400" }).attributes).toEqual({
      type: "positionPeriodic",
      frequency: 86400,
    });
    expect(buildCommandAttributes("positionPeriodic", { frequency: "86401" }).error).toMatch(/86400/);
  });

  it("rejects empty or invalid parameters", () => {
    expect(buildCommandAttributes("sosNumber", { phoneNumber: " " }).error).toBeTruthy();
    expect(buildCommandAttributes("setSpeedAlarm", { speed: "-1" }).error).toBeTruthy();
    expect(buildCommandAttributes("setSpeedAlarm", { speed: "" }).error).toBeTruthy();
    expect(buildCommandAttributes("custom", { text: "" }).error).toBeTruthy();
  });
});

describe("COMMAND_TYPE_LABELS", () => {
  it("has a label for every command type", () => {
    for (const type of ["rebootDevice", "positionPeriodic", "positionSingle", "sosNumber", "custom", "setSpeedAlarm", "factoryReset"]) {
      expect(COMMAND_TYPE_LABELS[type]).toBeTruthy();
    }
  });
});

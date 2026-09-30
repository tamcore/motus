import { describe, it, expect } from "vitest";
import { commandAttributesPayload, commandTypeOptions, COMMAND_TYPE_LABELS } from "./commands";

// The API decodes command attributes as a oneOf discriminated by "type"
// (docs/openapi.yaml CommandAttributes). Attributes without "type" or an
// empty object are rejected with "unable to detect sum type variant".
describe("commandAttributesPayload", () => {
  it("adds the command type as discriminator", () => {
    expect(commandAttributesPayload("positionPeriodic", { frequency: 60 })).toEqual({
      type: "positionPeriodic",
      frequency: 60,
    });
    expect(commandAttributesPayload("sosNumber", { phoneNumber: "+49123" })).toEqual({
      type: "sosNumber",
      phoneNumber: "+49123",
    });
    expect(commandAttributesPayload("setSpeedAlarm", { speed: 0 })).toEqual({
      type: "setSpeedAlarm",
      speed: 0,
    });
    expect(commandAttributesPayload("custom", { text: "UPLOAD,60" })).toEqual({
      type: "custom",
      text: "UPLOAD,60",
    });
  });

  it("omits attributes for commands without parameters", () => {
    expect(commandAttributesPayload("rebootDevice", {})).toBeUndefined();
    expect(commandAttributesPayload("positionSingle", {})).toBeUndefined();
    expect(commandAttributesPayload("factoryReset", {})).toBeUndefined();
  });

  it("does not let a caller override the discriminator", () => {
    expect(commandAttributesPayload("custom", { type: "other", text: "x" })).toEqual({
      type: "custom",
      text: "x",
    });
  });
});

describe("commandTypeOptions", () => {
  it("labels the command types in the order the server returns them", () => {
    expect(commandTypeOptions(["rebootDevice", "positionPeriodic", "custom"])).toEqual([
      { value: "rebootDevice", label: "Reboot Device" },
      { value: "positionPeriodic", label: "Set Reporting Interval" },
      { value: "custom", label: "Custom (raw text)" },
    ]);
  });

  it("falls back to the raw type for unknown command types", () => {
    expect(commandTypeOptions(["powerOff"])).toEqual([{ value: "powerOff", label: "powerOff" }]);
  });

  it("returns no options for protocols without commands", () => {
    expect(commandTypeOptions([])).toEqual([]);
  });

  it("has a label for every command type", () => {
    for (const type of ["rebootDevice", "positionPeriodic", "positionSingle", "sosNumber", "custom", "setSpeedAlarm", "factoryReset"]) {
      expect(COMMAND_TYPE_LABELS[type]).toBeTruthy();
    }
  });
});

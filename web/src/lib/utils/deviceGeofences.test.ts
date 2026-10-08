import { describe, it, expect } from "vitest";
import { describeDeviceGeofences, deviceGeofencePayload, toggleId } from "./deviceGeofences";

const geofences = [
  { id: 1, name: "Home" },
  { id: 2, name: "Park" },
];

describe("describeDeviceGeofences", () => {
  it("is empty without attachments (all geofences apply)", () => {
    expect(describeDeviceGeofences(undefined, geofences)).toBe("");
    expect(describeDeviceGeofences([], geofences)).toBe("");
  });

  it("lists names of visible geofences", () => {
    expect(describeDeviceGeofences([2, 1], geofences)).toBe("Park, Home");
  });

  it("summarizes geofences the user cannot see", () => {
    expect(describeDeviceGeofences([1, 7, 8], geofences)).toBe("Home, 2 other");
    expect(describeDeviceGeofences([9], geofences)).toBe("1 other");
  });
});

describe("deviceGeofencePayload", () => {
  it("sends only visible selected geofences, sorted and unique", () => {
    // Hidden attachments (id 7) are kept by the server, not sent back.
    expect(deviceGeofencePayload([2, 7, 1, 2], geofences)).toEqual([1, 2]);
  });

  it("sends [] to clear", () => {
    expect(deviceGeofencePayload([], geofences)).toEqual([]);
  });
});

describe("toggleId", () => {
  it("adds and removes an id", () => {
    expect(toggleId([1], 2, true)).toEqual([1, 2]);
    expect(toggleId([1, 2], 1, false)).toEqual([2]);
    expect(toggleId([1], 1, true)).toEqual([1]);
  });
});

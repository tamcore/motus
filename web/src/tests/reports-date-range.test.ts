import { describe, it, expect } from "vitest";
import { ALL_TIME_START, resolveDatePreset } from "$lib/utils/date-range";

// The "All time" start is shared by the map trail, heatmap and all /reports
// pages through ALL_TIME_START; this guards its value.
describe("All time preset", () => {
  it("starts at 2020-01-01 UTC", () => {
    expect(ALL_TIME_START.toISOString()).toBe("2020-01-01T00:00:00.000Z");
  });
});

describe("resolveDatePreset", () => {
  const now = new Date(2026, 9, 7, 15, 30); // Wednesday

  it("resolves relative and calendar presets", () => {
    expect(resolveDatePreset("week", "", "", now)).toEqual({
      from: new Date(2026, 8, 30, 15, 30).toISOString(),
      to: now.toISOString(),
    });
    expect(resolveDatePreset("yesterday", "", "", now)).toEqual({
      from: new Date(2026, 9, 6).toISOString(),
      to: new Date(2026, 9, 7).toISOString(),
    });
    expect(resolveDatePreset("prevWeek", "", "", now)).toEqual({
      from: new Date(2026, 8, 27).toISOString(),
      to: new Date(2026, 9, 4).toISOString(),
    });
    expect(resolveDatePreset("prevMonth", "", "", now)).toEqual({
      from: new Date(2026, 8, 1).toISOString(),
      to: new Date(2026, 9, 1).toISOString(),
    });
    expect(resolveDatePreset("all", "", "", now).from).toBe(ALL_TIME_START.toISOString());
  });

  it("covers whole local days for a custom range", () => {
    expect(resolveDatePreset("custom", "2026-09-01", "2026-09-02", now)).toEqual({
      from: new Date(2026, 8, 1, 0, 0, 0).toISOString(),
      to: new Date(2026, 8, 2, 23, 59, 59).toISOString(),
    });
  });
});

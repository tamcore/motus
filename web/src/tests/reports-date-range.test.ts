import { describe, it, expect } from "vitest";
import { ALL_TIME_START } from "$lib/utils/date-range";

// The "All time" start is shared by the map trail, heatmap and all /reports
// pages through ALL_TIME_START; this guards its value.
describe("All time preset", () => {
  it("starts at 2020-01-01 UTC", () => {
    expect(ALL_TIME_START.toISOString()).toBe("2020-01-01T00:00:00.000Z");
  });
});

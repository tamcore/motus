import { describe, it, expect } from "vitest";
import { pointStats } from "./point-stats";

describe("pointStats", () => {
  it("returns null for no points", () => {
    expect(pointStats([])).toBeNull();
  });

  it("computes time range and speed stats, skipping invalid times", () => {
    const stats = pointStats([
      { lat: 0, lon: 0, speed: 10, fixTime: "2026-10-02T00:00:00Z" },
      { lat: 0, lon: 0, speed: 30, fixTime: "not a date" },
      { lat: 0, lon: 0, speed: 20, fixTime: "2026-10-01T00:00:00Z" },
    ]);

    expect(stats).toEqual({
      earliest: Date.parse("2026-10-01T00:00:00Z"),
      latest: Date.parse("2026-10-02T00:00:00Z"),
      avgSpeed: 20,
      maxSpeed: 30,
    });
  });

  it("handles more points than the call stack allows as spread arguments", () => {
    const points = Array.from({ length: 500_000 }, (_, i) => ({
      lat: 0,
      lon: 0,
      speed: i,
      fixTime: "2026-10-01T00:00:00Z",
    }));

    expect(pointStats(points)?.maxSpeed).toBe(499_999);
  });
});

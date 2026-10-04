import { describe, it, expect } from "vitest";
import { haversineDistance, pathDistance } from "$lib/utils/trips";
import { interpolatePosition } from "$lib/utils/replay";
import type { RoutePosition } from "$lib/utils/route-points";

function makePosition(overrides: Partial<RoutePosition> = {}): RoutePosition {
  return {
    fixTime: "2024-01-15T10:00:00Z",
    latitude: 49.79,
    longitude: 9.95,
    altitude: 150,
    speed: 60,
    course: 180,
    ...overrides,
  };
}

function makeRoute(count: number): RoutePosition[] {
  return Array.from({ length: count }, (_, i) =>
    makePosition({ latitude: 49.79 + i * 0.001, longitude: 9.95 + i * 0.001 }),
  );
}

describe("pathDistance", () => {
  it("returns 0 for an empty array", () => {
    expect(pathDistance([])).toBe(0);
  });

  it("returns 0 for a single position", () => {
    expect(pathDistance([makePosition()])).toBe(0);
  });

  it("returns positive distance for two different positions", () => {
    const positions = [
      makePosition({ latitude: 49.0, longitude: 9.0 }),
      makePosition({ latitude: 49.1, longitude: 9.1 }),
    ];
    const dist = pathDistance(positions);
    expect(dist).toBeGreaterThan(0);
  });

  it("accumulates distance over multiple positions", () => {
    const route = makeRoute(5);
    const totalDist = pathDistance(route);
    const partialDist = pathDistance(route.slice(0, 3));
    expect(totalDist).toBeGreaterThan(partialDist);
  });

  it("returns 0 for identical positions", () => {
    const pos = makePosition();
    const dist = pathDistance([pos, pos, pos]);
    expect(dist).toBe(0);
  });

  it("matches haversineDistance for two positions", () => {
    const p1 = makePosition({ latitude: 48.0, longitude: 8.0 });
    const p2 = makePosition({ latitude: 49.0, longitude: 9.0 });
    const expected = haversineDistance(48.0, 8.0, 49.0, 9.0);
    expect(pathDistance([p1, p2])).toBeCloseTo(expected, 10);
  });
});

describe("interpolatePosition", () => {
  it("returns the start position at fraction 0", () => {
    const p1 = makePosition({ latitude: 49.0, longitude: 9.0, course: 90 });
    const p2 = makePosition({
      latitude: 50.0,
      longitude: 10.0,
      course: 180,
    });
    const result = interpolatePosition(p1, p2, 0);
    expect(result.lat).toBe(49.0);
    expect(result.lng).toBe(9.0);
    expect(result.course).toBe(90);
  });

  it("returns the end position at fraction 1", () => {
    const p1 = makePosition({ latitude: 49.0, longitude: 9.0, course: 90 });
    const p2 = makePosition({
      latitude: 50.0,
      longitude: 10.0,
      course: 180,
    });
    const result = interpolatePosition(p1, p2, 1);
    expect(result.lat).toBe(50.0);
    expect(result.lng).toBe(10.0);
    expect(result.course).toBe(180);
  });

  it("returns the midpoint at fraction 0.5", () => {
    const p1 = makePosition({ latitude: 48.0, longitude: 8.0, course: 0 });
    const p2 = makePosition({
      latitude: 50.0,
      longitude: 10.0,
      course: 100,
    });
    const result = interpolatePosition(p1, p2, 0.5);
    expect(result.lat).toBeCloseTo(49.0, 10);
    expect(result.lng).toBeCloseTo(9.0, 10);
    expect(result.course).toBeCloseTo(50, 10);
  });

  it("handles course wrapping around 360 to 0 (clockwise short path)", () => {
    // Going from 350 to 10 should wrap through 0, not go 350 -> 10 via 180
    const p1 = makePosition({ latitude: 49.0, longitude: 9.0, course: 350 });
    const p2 = makePosition({ latitude: 49.0, longitude: 9.0, course: 10 });
    const result = interpolatePosition(p1, p2, 0.5);
    // 350 + 20*0.5 = 360 which is equivalent to 0 degrees
    expect(result.course % 360).toBeCloseTo(0, 10);
  });

  it("handles course wrapping around 0 to 360 (counter-clockwise short path)", () => {
    // Going from 10 to 350 should wrap through 0 backwards
    const p1 = makePosition({ latitude: 49.0, longitude: 9.0, course: 10 });
    const p2 = makePosition({
      latitude: 49.0,
      longitude: 9.0,
      course: 350,
    });
    const result = interpolatePosition(p1, p2, 0.5);
    expect(result.course).toBeCloseTo(0, 10);
  });

  it("handles null course values (defaults to 0)", () => {
    const p1 = makePosition({ latitude: 49.0, longitude: 9.0, course: null });
    const p2 = makePosition({
      latitude: 50.0,
      longitude: 10.0,
      course: null,
    });
    const result = interpolatePosition(p1, p2, 0.5);
    expect(result.course).toBe(0);
  });

  it("interpolates correctly at fraction 0.25", () => {
    const p1 = makePosition({ latitude: 40.0, longitude: 0.0, course: 0 });
    const p2 = makePosition({
      latitude: 44.0,
      longitude: 4.0,
      course: 120,
    });
    const result = interpolatePosition(p1, p2, 0.25);
    expect(result.lat).toBeCloseTo(41.0, 10);
    expect(result.lng).toBeCloseTo(1.0, 10);
    expect(result.course).toBeCloseTo(30, 10);
  });
});

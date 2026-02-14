import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Mock $app/environment before any imports that depend on it
vi.mock("$app/environment", () => ({
  browser: true,
}));

// Mock window.matchMedia for theme store
Object.defineProperty(window, "matchMedia", {
  writable: true,
  value: vi.fn().mockImplementation((query: string) => ({
    matches: query === "(prefers-color-scheme: dark)",
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
});

import type { Position } from "$lib/types/api";
import { haversineDistance } from "$lib/utils/trips";

// ---------------------------------------------------------------------------
// Replicate the pure functions from the replay page for unit testing.
// These mirror the logic in +page.svelte but are importable here.
// ---------------------------------------------------------------------------

function getTime(pos: Position): string {
  return pos.fixTime;
}

function calcTotalDistance(positions: Position[]): number {
  let total = 0;
  for (let i = 1; i < positions.length; i++) {
    total += haversineDistance(
      positions[i - 1].latitude,
      positions[i - 1].longitude,
      positions[i].latitude,
      positions[i].longitude,
    );
  }
  return total;
}

function getInterpolatedPosition(
  p1: Position,
  p2: Position,
  fraction: number,
): { lat: number; lng: number; course: number } {
  const lat = p1.latitude + (p2.latitude - p1.latitude) * fraction;
  const lng = p1.longitude + (p2.longitude - p1.longitude) * fraction;
  const c1 = p1.course ?? 0;
  const c2 = p2.course ?? 0;
  let diff = c2 - c1;
  if (diff > 180) diff -= 360;
  if (diff < -180) diff += 360;
  const course = c1 + diff * fraction;
  return { lat, lng, course };
}

// ---------------------------------------------------------------------------
// Test data factory
// ---------------------------------------------------------------------------

function makePosition(overrides: Partial<Position> = {}): Position {
  return {
    id: 1,
    deviceId: 100,
    fixTime: "2024-01-15T10:00:00Z",
    valid: true,
    latitude: 49.79,
    longitude: 9.95,
    altitude: 150,
    speed: 60,
    course: 180,
    accuracy: 5,
    outdated: false,
    ...overrides,
  };
}

function makeRoute(count: number): Position[] {
  const positions: Position[] = [];
  for (let i = 0; i < count; i++) {
    positions.push(
      makePosition({
        id: i + 1,
        fixTime: new Date(Date.UTC(2024, 0, 15, 10, 0, i * 10)).toISOString(),
        latitude: 49.79 + i * 0.001,
        longitude: 9.95 + i * 0.001,
        speed: 30 + i * 5,
        altitude: 150 + i * 10,
        course: (i * 30) % 360,
      }),
    );
  }
  return positions;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("Drive Replay", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  // -------------------------------------------------------------------------
  // getTime
  // -------------------------------------------------------------------------
  describe("getTime", () => {
    it("returns the fixTime from a position", () => {
      const pos = makePosition({ fixTime: "2024-06-01T12:00:00Z" });
      expect(getTime(pos)).toBe("2024-06-01T12:00:00Z");
    });
  });

  // -------------------------------------------------------------------------
  // calcTotalDistance
  // -------------------------------------------------------------------------
  describe("calcTotalDistance", () => {
    it("returns 0 for an empty array", () => {
      expect(calcTotalDistance([])).toBe(0);
    });

    it("returns 0 for a single position", () => {
      expect(calcTotalDistance([makePosition()])).toBe(0);
    });

    it("returns positive distance for two different positions", () => {
      const positions = [
        makePosition({ latitude: 49.0, longitude: 9.0 }),
        makePosition({ latitude: 49.1, longitude: 9.1 }),
      ];
      const dist = calcTotalDistance(positions);
      expect(dist).toBeGreaterThan(0);
    });

    it("accumulates distance over multiple positions", () => {
      const route = makeRoute(5);
      const totalDist = calcTotalDistance(route);
      const partialDist = calcTotalDistance(route.slice(0, 3));
      expect(totalDist).toBeGreaterThan(partialDist);
    });

    it("returns 0 for identical positions", () => {
      const pos = makePosition();
      const dist = calcTotalDistance([pos, pos, pos]);
      expect(dist).toBe(0);
    });

    it("matches haversineDistance for two positions", () => {
      const p1 = makePosition({ latitude: 48.0, longitude: 8.0 });
      const p2 = makePosition({ latitude: 49.0, longitude: 9.0 });
      const expected = haversineDistance(48.0, 8.0, 49.0, 9.0);
      expect(calcTotalDistance([p1, p2])).toBeCloseTo(expected, 10);
    });
  });

  // -------------------------------------------------------------------------
  // getInterpolatedPosition
  // -------------------------------------------------------------------------
  describe("getInterpolatedPosition", () => {
    it("returns the start position at fraction 0", () => {
      const p1 = makePosition({ latitude: 49.0, longitude: 9.0, course: 90 });
      const p2 = makePosition({
        latitude: 50.0,
        longitude: 10.0,
        course: 180,
      });
      const result = getInterpolatedPosition(p1, p2, 0);
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
      const result = getInterpolatedPosition(p1, p2, 1);
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
      const result = getInterpolatedPosition(p1, p2, 0.5);
      expect(result.lat).toBeCloseTo(49.0, 10);
      expect(result.lng).toBeCloseTo(9.0, 10);
      expect(result.course).toBeCloseTo(50, 10);
    });

    it("handles course wrapping around 360 to 0 (clockwise short path)", () => {
      // Going from 350 to 10 should wrap through 0, not go 350 -> 10 via 180
      const p1 = makePosition({ latitude: 49.0, longitude: 9.0, course: 350 });
      const p2 = makePosition({ latitude: 49.0, longitude: 9.0, course: 10 });
      const result = getInterpolatedPosition(p1, p2, 0.5);
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
      const result = getInterpolatedPosition(p1, p2, 0.5);
      expect(result.course).toBeCloseTo(0, 10);
    });

    it("handles null course values (defaults to 0)", () => {
      const p1 = makePosition({ latitude: 49.0, longitude: 9.0, course: null });
      const p2 = makePosition({
        latitude: 50.0,
        longitude: 10.0,
        course: null,
      });
      const result = getInterpolatedPosition(p1, p2, 0.5);
      expect(result.course).toBe(0);
    });

    it("interpolates correctly at fraction 0.25", () => {
      const p1 = makePosition({ latitude: 40.0, longitude: 0.0, course: 0 });
      const p2 = makePosition({
        latitude: 44.0,
        longitude: 4.0,
        course: 120,
      });
      const result = getInterpolatedPosition(p1, p2, 0.25);
      expect(result.lat).toBeCloseTo(41.0, 10);
      expect(result.lng).toBeCloseTo(1.0, 10);
      expect(result.course).toBeCloseTo(30, 10);
    });
  });

  // -------------------------------------------------------------------------
  // Playback time calculations
  // -------------------------------------------------------------------------
  describe("Playback time calculations", () => {
    it("computes total duration from first to last position", () => {
      const route = makeRoute(10);
      const firstTime = new Date(getTime(route[0])).getTime();
      const lastTime = new Date(getTime(route[route.length - 1])).getTime();
      const duration = (lastTime - firstTime) / 1000;
      expect(duration).toBeGreaterThan(0);
      // 10 positions, 10s apart = 90s total
      expect(duration).toBe(90);
    });

    it("computes elapsed duration at a given index", () => {
      const route = makeRoute(10);
      const firstTime = new Date(getTime(route[0])).getTime();
      const midTime = new Date(getTime(route[5])).getTime();
      const elapsed = (midTime - firstTime) / 1000;
      // 5 positions at 10s each = 50s
      expect(elapsed).toBe(50);
    });

    it("computes progress percentage", () => {
      const route = makeRoute(11);
      const currentIndex = 5;
      const progress = (currentIndex / (route.length - 1)) * 100;
      expect(progress).toBe(50);
    });

    it("computes GPS time delta between consecutive positions", () => {
      const route = makeRoute(5);
      const t1 = new Date(getTime(route[0])).getTime();
      const t2 = new Date(getTime(route[1])).getTime();
      expect(t2 - t1).toBe(10000); // 10 seconds in ms
    });
  });

  // -------------------------------------------------------------------------
  // Route statistics
  // -------------------------------------------------------------------------
  describe("Route statistics", () => {
    it("computes max speed from positions", () => {
      const route = makeRoute(5);
      const maxSpeed = Math.max(...route.map((p) => p.speed ?? 0));
      // speeds are 30, 35, 40, 45, 50
      expect(maxSpeed).toBe(50);
    });

    it("computes average speed from total distance and duration", () => {
      const route = makeRoute(5);
      const totalDist = calcTotalDistance(route);
      const firstTime = new Date(getTime(route[0])).getTime();
      const lastTime = new Date(getTime(route[route.length - 1])).getTime();
      const durationHours = (lastTime - firstTime) / 1000 / 3600;
      const avgSpeed = durationHours > 0 ? totalDist / durationHours : 0;
      expect(avgSpeed).toBeGreaterThan(0);
      expect(Number.isFinite(avgSpeed)).toBe(true);
    });

    it("handles zero duration gracefully", () => {
      const pos = makePosition();
      const route = [pos];
      const totalDist = calcTotalDistance(route);
      const durationSec = 0;
      const avgSpeed = durationSec > 0 ? totalDist / (durationSec / 3600) : 0;
      expect(avgSpeed).toBe(0);
    });
  });

  // -------------------------------------------------------------------------
  // Position sorting
  // -------------------------------------------------------------------------
  describe("Position sorting", () => {
    it("sorts positions by fixTime ascending", () => {
      const positions = [
        makePosition({ fixTime: "2024-01-15T10:05:00Z" }),
        makePosition({ fixTime: "2024-01-15T10:00:00Z" }),
        makePosition({ fixTime: "2024-01-15T10:10:00Z" }),
        makePosition({ fixTime: "2024-01-15T10:02:00Z" }),
      ];
      const sorted = [...positions].sort(
        (a, b) =>
          new Date(getTime(a)).getTime() - new Date(getTime(b)).getTime(),
      );
      expect(getTime(sorted[0])).toBe("2024-01-15T10:00:00Z");
      expect(getTime(sorted[1])).toBe("2024-01-15T10:02:00Z");
      expect(getTime(sorted[2])).toBe("2024-01-15T10:05:00Z");
      expect(getTime(sorted[3])).toBe("2024-01-15T10:10:00Z");
    });
  });

  // -------------------------------------------------------------------------
  // Chart data generation
  // -------------------------------------------------------------------------
  describe("Chart data generation", () => {
    it("extracts speed data from positions", () => {
      const route = makeRoute(5);
      const data = route.map((p) => p.speed ?? 0);
      expect(data).toEqual([30, 35, 40, 45, 50]);
    });

    it("extracts altitude data from positions", () => {
      const route = makeRoute(5);
      const data = route.map((p) => p.altitude ?? 0);
      expect(data).toEqual([150, 160, 170, 180, 190]);
    });

    it("extracts course data from positions", () => {
      const route = makeRoute(5);
      const data = route.map((p) => p.course ?? 0);
      expect(data).toEqual([0, 30, 60, 90, 120]);
    });

    it("generates time labels from positions", () => {
      const route = makeRoute(3);
      const labels = route.map((p) => {
        const d = new Date(getTime(p));
        return d.toLocaleTimeString([], {
          hour: "2-digit",
          minute: "2-digit",
          second: "2-digit",
        });
      });
      expect(labels).toHaveLength(3);
      labels.forEach((label) => {
        expect(typeof label).toBe("string");
        expect(label.length).toBeGreaterThan(0);
      });
    });

    it("handles null speed values as 0", () => {
      const positions = [
        makePosition({ speed: null }),
        makePosition({ speed: 50 }),
        makePosition({ speed: null }),
      ];
      const data = positions.map((p) => p.speed ?? 0);
      expect(data).toEqual([0, 50, 0]);
    });

    it("handles null altitude values as 0", () => {
      const positions = [
        makePosition({ altitude: null }),
        makePosition({ altitude: 200 }),
      ];
      const data = positions.map((p) => p.altitude ?? 0);
      expect(data).toEqual([0, 200]);
    });
  });

  // -------------------------------------------------------------------------
  // Traveled distance tracking
  // -------------------------------------------------------------------------
  describe("Traveled distance tracking", () => {
    it("traveled distance at index 0 is 0", () => {
      const route = makeRoute(5);
      const traveled = calcTotalDistance(route.slice(0, 1));
      expect(traveled).toBe(0);
    });

    it("traveled distance at last index equals total distance", () => {
      const route = makeRoute(5);
      const total = calcTotalDistance(route);
      const traveled = calcTotalDistance(route.slice(0, route.length));
      expect(traveled).toBeCloseTo(total, 10);
    });

    it("traveled distance increases monotonically", () => {
      const route = makeRoute(10);
      let prev = 0;
      for (let i = 1; i <= route.length; i++) {
        const dist = calcTotalDistance(route.slice(0, i));
        expect(dist).toBeGreaterThanOrEqual(prev);
        prev = dist;
      }
    });
  });
});

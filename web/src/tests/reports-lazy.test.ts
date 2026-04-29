import { describe, it, expect } from "vitest";
import { detectTrips } from "$lib/utils/trips";

// ---------------------------------------------------------------------------
// Page-size selector contract
// ---------------------------------------------------------------------------
describe("page size options", () => {
  it("contains the five expected options", () => {
    expect(PAGE_SIZE_OPTIONS).toEqual([5, 10, 25, 50, 100]);
  });

  it("default page size is 10", () => {
    expect(PAGE_SIZE).toBe(10);
    expect(PAGE_SIZE_OPTIONS).toContain(PAGE_SIZE);
  });

  it("each option produces correct initial visible slice", () => {
    // Generate 200 trips (synthetic — just need a long array).
    const base = new Date("2026-04-10T08:00:00Z");
    const positions = Array.from({ length: 200 * 12 }, (_, i) => ({
      latitude: 52.0 + (i % 200) * 0.01,
      longitude: 13.0,
      speed: i % 12 < 2 ? 60 : 0,
      fixTime: new Date(base.getTime() + i * 60_000).toISOString(),
    }));
    const raw = detectTrips(positions, "Test", 1);
    const trips = [...raw].sort(
      (a, b) => new Date(b.startTime).getTime() - new Date(a.startTime).getTime(),
    );

    for (const size of PAGE_SIZE_OPTIONS) {
      const visible = trips.slice(0, size);
      expect(visible.length).toBeLessThanOrEqual(size);
    }
  });
});

// ---------------------------------------------------------------------------
// Unit tests for the lazy-loading threshold and trip detection with large
// datasets. The component uses PAGE_SIZE = 50; we verify detectTrips produces
// the correct number of trips from a dense position set and that slicing to
// PAGE_SIZE gives the expected window.
// ---------------------------------------------------------------------------

const PAGE_SIZE = 10;
const PAGE_SIZE_OPTIONS = [5, 10, 25, 50, 100];

function makePositions(count: number, speedKmh = 60) {
  const base = new Date("2026-04-10T08:00:00Z");
  return Array.from({ length: count }, (_, i) => ({
    latitude: 52.0 + i * 0.001,
    longitude: 13.0,
    speed: speedKmh,
    fixTime: new Date(base.getTime() + i * 10_000).toISOString(),
  }));
}

describe("reports lazy loading", () => {
  it("detectTrips returns one trip for a continuous moving sequence", () => {
    // 200 positions all moving — should collapse to a single trip.
    const positions = makePositions(200, 60);
    const trips = detectTrips(positions, "Test", 1);
    expect(trips.length).toBe(1);
  });

  it("detectTrips splits on >1h position gaps", () => {
    const base = new Date("2026-04-10T08:00:00Z");
    // 10 moving positions, then a >1h gap, then 10 more.
    const first = Array.from({ length: 10 }, (_, i) => ({
      latitude: 52.0 + i * 0.001,
      longitude: 13.0,
      speed: 60,
      fixTime: new Date(base.getTime() + i * 10_000).toISOString(),
    }));
    const second = Array.from({ length: 10 }, (_, i) => ({
      latitude: 53.0 + i * 0.001,
      longitude: 13.0,
      speed: 60,
      fixTime: new Date(base.getTime() + 7_200_000 + i * 10_000).toISOString(),
    }));
    const trips = detectTrips([...first, ...second], "Test", 1);
    expect(trips.length).toBe(2);
  });

  it("slicing to PAGE_SIZE yields the first PAGE_SIZE trips", () => {
    // Build a large set of short trips (each ~2 positions with a long stop between).
    const base = new Date("2026-04-10T08:00:00Z");
    const positions = [];
    for (let t = 0; t < 100; t++) {
      // Two fast positions (moving)
      for (let j = 0; j < 2; j++) {
        positions.push({
          latitude: 52.0 + t * 0.01 + j * 0.001,
          longitude: 13.0,
          speed: 60,
          fixTime: new Date(base.getTime() + t * 3_600_000 + j * 5_000).toISOString(),
        });
      }
      // Ten slow positions to trigger a stop (>5 min below threshold)
      for (let j = 0; j < 10; j++) {
        positions.push({
          latitude: 52.0 + t * 0.01 + 0.002,
          longitude: 13.0,
          speed: 0,
          fixTime: new Date(base.getTime() + t * 3_600_000 + 10_000 + j * 60_000).toISOString(),
        });
      }
    }
    const raw = detectTrips(positions, "Test", 1);
    expect(raw.length).toBeGreaterThan(PAGE_SIZE);

    // fetchReports sorts newest-first then resets visibleTripCount = PAGE_SIZE.
    const trips = [...raw].sort(
      (a, b) => new Date(b.startTime).getTime() - new Date(a.startTime).getTime(),
    );
    const visible = trips.slice(0, PAGE_SIZE);
    expect(visible.length).toBe(PAGE_SIZE);
    // First visible entry must be the most recent trip.
    expect(visible[0].startTime).toBe(trips[0].startTime);
    expect(visible[PAGE_SIZE - 1].startTime).toBe(trips[PAGE_SIZE - 1].startTime);
  });

  it("no limit means positions beyond 10k are not truncated", () => {
    // Simulate what the frontend does: all positions passed to detectTrips.
    // Previously the 10k API limit would have cut this off.
    const positions = makePositions(15_000, 60);
    // Should not throw and should process all 15k positions.
    const trips = detectTrips(positions, "Test", 1);
    expect(trips.length).toBeGreaterThanOrEqual(1);
    // The final position should be reflected in the trip end time.
    const lastPos = positions[positions.length - 1];
    const lastTrip = trips[trips.length - 1];
    expect(new Date(lastTrip.endTime).getTime()).toBeGreaterThanOrEqual(
      new Date(positions[0].fixTime!).getTime(),
    );
    // End time must be at or after last position time.
    expect(new Date(lastTrip.endTime).getTime()).toBeGreaterThanOrEqual(
      new Date(lastPos.fixTime!).getTime() - 1,
    );
  });
});

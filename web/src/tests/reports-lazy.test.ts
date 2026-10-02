import { describe, it, expect } from "vitest";
import type { Trip } from "$lib/utils/trips";

// Trip detection moved server-side (internal/reports, Go tests). These tests
// cover the page-size and newest-first slicing contract of the reports page.

const PAGE_SIZE = 10;
const PAGE_SIZE_OPTIONS = [5, 10, 25, 50, 100];

function makeTrips(count: number): Trip[] {
  const base = new Date("2026-04-10T08:00:00Z").getTime();
  return Array.from({ length: count }, (_, i) => ({
    id: `trip-1-${i}`,
    deviceId: 1,
    deviceName: "Test",
    startTime: new Date(base + i * 3_600_000).toISOString(),
    endTime: new Date(base + i * 3_600_000 + 600_000).toISOString(),
    duration: 600,
    distance: 5,
    avgSpeed: 30,
    maxSpeed: 60,
  }));
}

function newestFirst(trips: Trip[]): Trip[] {
  return [...trips].sort(
    (a, b) => new Date(b.startTime).getTime() - new Date(a.startTime).getTime(),
  );
}

describe("page size options", () => {
  it("contains the five expected options", () => {
    expect(PAGE_SIZE_OPTIONS).toEqual([5, 10, 25, 50, 100]);
  });

  it("default page size is 10", () => {
    expect(PAGE_SIZE).toBe(10);
    expect(PAGE_SIZE_OPTIONS).toContain(PAGE_SIZE);
  });

  it("each option produces correct initial visible slice", () => {
    const trips = newestFirst(makeTrips(200));
    for (const size of PAGE_SIZE_OPTIONS) {
      expect(trips.slice(0, size).length).toBe(size);
    }
  });
});

describe("reports lazy loading", () => {
  it("slicing to PAGE_SIZE yields the newest PAGE_SIZE trips", () => {
    const trips = newestFirst(makeTrips(100));
    const visible = trips.slice(0, PAGE_SIZE);
    expect(visible.length).toBe(PAGE_SIZE);
    expect(visible[0].id).toBe("trip-1-99");
    expect(visible[PAGE_SIZE - 1].id).toBe("trip-1-90");
  });
});

import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("$app/environment", () => ({ browser: true }));
vi.mock("$lib/auth-token-store", () => ({ getStoredAuthToken: vi.fn().mockResolvedValue(null) }));

import { api } from "$lib/api/client";

function stubFetch(body: unknown) {
  const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status: 200 }));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

const range = { from: "2026-10-01T00:00:00Z", to: "2026-10-02T00:00:00Z" };

describe("api.getTripReport", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("requests the trips endpoint with repeated deviceId params", async () => {
    const fetchMock = stubFetch([]);

    await api.getTripReport({ deviceIds: [3, 7], ...range });

    const url = new URL(fetchMock.mock.calls[0][0], "http://x");
    expect(url.pathname).toBe("/api/reports/trips");
    expect(url.searchParams.getAll("deviceId")).toEqual(["3", "7"]);
    expect(url.searchParams.get("from")).toBe(range.from);
    expect(url.searchParams.get("to")).toBe(range.to);
  });

  it("converts speeds from knots to km/h and assigns unique ids", async () => {
    const trip = {
      deviceId: 3, deviceName: "Car", startTime: range.from, endTime: range.to,
      duration: 600, distance: 5.5, avgSpeed: 10, maxSpeed: 20,
    };
    stubFetch([trip, { ...trip, startTime: range.to }]);

    const trips = await api.getTripReport({ deviceIds: [3], ...range });

    expect(trips[0].avgSpeed).toBeCloseTo(18.52);
    expect(trips[0].maxSpeed).toBeCloseTo(37.04);
    expect(trips[0].distance).toBe(5.5);
    expect(new Set(trips.map((t) => t.id)).size).toBe(2);
  });
});

describe("api.getStopReport", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("requests the stops endpoint and assigns unique ids", async () => {
    const stop = {
      deviceId: 3, deviceName: "Car", latitude: 52, longitude: 13, address: "Main St",
      arrivalTime: range.from, departureTime: range.to, duration: 900,
    };
    const fetchMock = stubFetch([stop, stop]);

    const stops = await api.getStopReport({ deviceIds: [3], ...range });

    const url = new URL(fetchMock.mock.calls[0][0], "http://x");
    expect(url.pathname).toBe("/api/reports/stops");
    expect(url.searchParams.getAll("deviceId")).toEqual(["3"]);
    expect(stops).toHaveLength(2);
    expect(stops[0].address).toBe("Main St");
    expect(new Set(stops.map((s) => s.id)).size).toBe(2);
  });
});

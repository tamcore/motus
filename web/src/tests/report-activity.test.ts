import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("$lib/auth-token-store", () => ({ getStoredAuthToken: vi.fn().mockResolvedValue(null) }));

import { api } from "$lib/api/client";

function stubFetch(body: unknown) {
  const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status: 200 }));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

const range = { from: "2026-10-01T00:00:00Z", to: "2026-10-02T00:00:00Z" };

const trip = {
  deviceId: 3, deviceName: "Car", startTime: range.from, endTime: range.to,
  duration: 600, distance: 5.5, avgSpeed: 10, maxSpeed: 20,
};
const stop = {
  deviceId: 3, deviceName: "Car", latitude: 52, longitude: 13, address: "Main St",
  arrivalTime: range.from, departureTime: range.to, duration: 900,
};

describe("api.getActivityReport", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("requests the activity endpoint once with repeated deviceId params", async () => {
    const fetchMock = stubFetch({ trips: [], stops: [] });

    await api.getActivityReport({ deviceIds: [3, 7], ...range });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const url = new URL(fetchMock.mock.calls[0][0], "http://x");
    expect(url.pathname).toBe("/api/reports/activity");
    expect(url.searchParams.getAll("deviceId")).toEqual(["3", "7"]);
    expect(url.searchParams.get("from")).toBe(range.from);
    expect(url.searchParams.get("to")).toBe(range.to);
  });

  it("converts trip speeds from knots to km/h and assigns unique ids", async () => {
    stubFetch({ trips: [trip, { ...trip, startTime: range.to }], stops: [stop, stop] });

    const { trips, stops } = await api.getActivityReport({ deviceIds: [3], ...range });

    expect(trips[0].avgSpeed).toBeCloseTo(18.52);
    expect(trips[0].maxSpeed).toBeCloseTo(37.04);
    expect(trips[0].distance).toBe(5.5);
    expect(new Set(trips.map((t) => t.id)).size).toBe(2);
    expect(stops).toHaveLength(2);
    expect(stops[0].address).toBe("Main St");
    expect(new Set(stops.map((s) => s.id)).size).toBe(2);
  });
});

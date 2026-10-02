import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("$app/environment", () => ({ browser: true }));
vi.mock("$lib/auth-token-store", () => ({ getStoredAuthToken: vi.fn().mockResolvedValue(null) }));

import { api } from "$lib/api/client";

function stubFetch(body: unknown) {
  const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status: 200 }));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("api.getPositionPoints", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("requests the points endpoint and converts speed from knots to km/h", async () => {
    const fetchMock = stubFetch([
      { lat: 48, lon: 11, speed: 10, fixTime: "2026-10-02T00:00:00Z" },
      { lat: 48.1, lon: 11.1, speed: 0, fixTime: "2026-10-02T00:01:00Z" },
    ]);

    const points = await api.getPositionPoints({ deviceId: 3, from: "a", to: "b", limit: 10000 });

    expect(points).toHaveLength(2);
    expect(points[0]).toMatchObject({ lat: 48, lon: 11, fixTime: "2026-10-02T00:00:00Z" });
    expect(points[0].speed).toBeCloseTo(18.52, 5);
    expect(points[1].speed).toBe(0);
    const url = new URL(fetchMock.mock.calls[0][0], "http://x");
    expect(url.pathname).toBe("/api/positions/points");
    expect(url.searchParams.get("deviceId")).toBe("3");
    expect(url.searchParams.get("from")).toBe("a");
    expect(url.searchParams.get("to")).toBe("b");
    expect(url.searchParams.get("limit")).toBe("10000");
  });

  it("omits unset parameters", async () => {
    const fetchMock = stubFetch([]);

    expect(await api.getPositionPoints({ from: "a", to: "b" })).toEqual([]);

    const url = new URL(fetchMock.mock.calls[0][0], "http://x");
    expect(url.searchParams.has("deviceId")).toBe(false);
    expect(url.searchParams.has("limit")).toBe(false);
  });
});

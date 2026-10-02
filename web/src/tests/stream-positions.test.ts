import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("$app/environment", () => ({ browser: true }));
vi.mock("$lib/auth-token-store", () => ({ getStoredAuthToken: vi.fn().mockResolvedValue(null) }));

import { streamPositions } from "$lib/api/stream";

function makePositionJSON(id: number) {
  return {
    id,
    deviceId: 1,
    fixTime: "2024-01-01T00:00:00Z",
    valid: true,
    latitude: 48.0 + id * 0.001,
    longitude: 11.0 + id * 0.001,
    speed: 10,
  };
}

describe("streamPositions", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("returns all positions and normalizes speed from knots to km/h", async () => {
    const raw = [makePositionJSON(1), makePositionJSON(2)];
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response(JSON.stringify(raw))),
    );

    const deltas: number[] = [];
    const result = await streamPositions({ deviceId: 1 }, (delta) => deltas.push(delta));

    expect(result).toHaveLength(2);
    expect(result[0].speed).toBeCloseTo(10 * 1.852, 5);
    expect(result[1].speed).toBeCloseTo(10 * 1.852, 5);
    expect(deltas.reduce((a, b) => a + b, 0)).toBe(2);
  });

  it("sends limit so the server samples the range", async () => {
    const mockFetch = vi.fn().mockResolvedValue(new Response(JSON.stringify([makePositionJSON(1)])));
    vi.stubGlobal("fetch", mockFetch);

    await streamPositions({ deviceId: 1, limit: 10000 }, () => {});

    expect(new URL(mockFetch.mock.calls[0][0], "http://x").searchParams.get("limit")).toBe("10000");
  });

  it("sends deviceId, from, to as query params", async () => {
    const mockFetch = vi.fn().mockResolvedValue(new Response(JSON.stringify([makePositionJSON(1)])));
    vi.stubGlobal("fetch", mockFetch);

    await streamPositions(
      { deviceId: 42, from: "2024-01-01T00:00:00Z", to: "2024-12-31T23:59:59Z" },
      () => {},
    );

    const url: string = mockFetch.mock.calls[0][0];
    expect(url).toContain("deviceId=42");
    expect(url).toContain("from=");
    expect(url).toContain("to=");
  });

  it("parses the compact JSON array the API sends", async () => {
    const raw = Array.from({ length: 3 }, (_, i) => makePositionJSON(i + 1));
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify(raw))));

    const result = await streamPositions({ deviceId: 1 }, () => {});

    expect(result.map((p) => p.id)).toEqual([1, 2, 3]);
  });

  it("throws on non-ok response", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: false,
        status: 502,
        text: vi.fn().mockResolvedValue("Bad Gateway"),
      }),
    );

    await expect(streamPositions({}, () => {})).rejects.toThrow("HTTP 502");
  });

  it("leaves null speed as null without crashing", async () => {
    const raw = [{ ...makePositionJSON(1), speed: null }];
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response(JSON.stringify(raw))),
    );

    const result = await streamPositions({}, () => {});
    expect(result[0].speed).toBeNull();
  });
});

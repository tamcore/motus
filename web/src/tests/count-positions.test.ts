import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("$app/environment", () => ({ browser: true }));
vi.mock("$lib/auth-token-store", () => ({ getStoredAuthToken: vi.fn().mockResolvedValue(null) }));

import { api } from "$lib/api/client";

function stubFetch(body: unknown) {
  const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status: 200 }));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

describe("api.countPositions", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("requests the count endpoint and returns the number", async () => {
    const fetchMock = stubFetch({ count: 42 });

    const n = await api.countPositions({ from: "2026-10-02T00:00:00Z", to: "2026-10-02T12:00:00Z" });

    expect(n).toBe(42);
    const url = new URL(fetchMock.mock.calls[0][0], "http://x");
    expect(url.pathname).toBe("/api/positions/count");
    expect(url.searchParams.get("from")).toBe("2026-10-02T00:00:00Z");
    expect(url.searchParams.has("all")).toBe(false);
  });

  it("adds all=true for the admin view", async () => {
    const fetchMock = stubFetch({ count: 1 });

    await api.countPositions({ from: "a", to: "b", all: true });

    expect(new URL(fetchMock.mock.calls[0][0], "http://x").searchParams.get("all")).toBe("true");
  });
});

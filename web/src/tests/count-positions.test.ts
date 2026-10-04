import { describe, it, expect, vi, beforeEach } from "vitest";

import { api } from "$lib/api/client";
import { stubFetch } from "./helpers/stub-fetch";

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

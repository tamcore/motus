import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("$lib/auth-token-store", () => ({ getStoredAuthToken: vi.fn().mockResolvedValue(null) }));

import { api } from "$lib/api/client";

function stubFetch(body: unknown, status = 200) {
  const fetchMock = vi
    .fn()
    .mockResolvedValue(
      status === 204 ? new Response(null, { status }) : new Response(JSON.stringify(body), { status }),
    );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

const payload = {
  deviceId: 4,
  name: "Zugspitze",
  description: "",
  from: "2026-06-06T06:00:00.000Z",
  to: "2026-06-06T14:30:00.000Z",
};

describe("trail bookmark API client", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("lists all bookmarks", async () => {
    const fetchMock = stubFetch([{ id: 1 }]);
    expect(await api.getTrailBookmarks()).toEqual([{ id: 1 }]);
    const url = new URL(fetchMock.mock.calls[0][0], "http://x");
    expect(url.pathname).toBe("/api/trail-bookmarks");
    expect(url.search).toBe("");
  });

  it("lists bookmarks of one device", async () => {
    const fetchMock = stubFetch([]);
    await api.getTrailBookmarks(7);
    const url = new URL(fetchMock.mock.calls[0][0], "http://x");
    expect(url.pathname).toBe("/api/trail-bookmarks");
    expect(url.searchParams.get("deviceId")).toBe("7");
  });

  it("creates a bookmark with POST", async () => {
    const fetchMock = stubFetch({ id: 9, ...payload });
    const created = await api.createTrailBookmark(payload);
    expect(created.id).toBe(9);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/trail-bookmarks");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body)).toEqual(payload);
  });

  it("updates a bookmark with PUT", async () => {
    const fetchMock = stubFetch({ id: 9, ...payload });
    await api.updateTrailBookmark(9, payload);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/trail-bookmarks/9");
    expect(init.method).toBe("PUT");
    expect(JSON.parse(init.body)).toEqual(payload);
  });

  it("deletes a bookmark", async () => {
    const fetchMock = stubFetch(null, 204);
    await api.deleteTrailBookmark(9);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/trail-bookmarks/9");
    expect(init.method).toBe("DELETE");
  });

  it("rejects with the error field of a JSON error body", async () => {
    stubFetch({ error: "name is required" }, 400);
    await expect(api.createTrailBookmark(payload)).rejects.toMatchObject({
      status: 400,
      message: "name is required",
    });
  });

  it("sends FormData without a JSON content type", async () => {
    const fetchMock = stubFetch({ imported: 1, skipped: 0 });
    await api.importGPX(4, new File(["<gpx/>"], "t.gpx"));
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/devices/4/gpx");
    expect(init.body).toBeInstanceOf(FormData);
    expect(init.headers["Content-Type"]).toBeUndefined();
  });
});

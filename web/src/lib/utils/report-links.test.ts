import { describe, it, expect } from "vitest";
import { tripLink, normalizeTimeParam } from "./report-links";

describe("tripLink", () => {
  it("percent-encodes offset timestamps so '+' survives the query string", () => {
    const href = tripLink({
      deviceId: 1,
      startTime: "2026-10-03T07:48:24+02:00",
      endTime: "2026-10-03T08:17:34+02:00",
    });

    const url = new URL(href, "http://localhost");
    expect(url.pathname).toBe("/reports/replay");
    expect(url.searchParams.get("deviceId")).toBe("1");
    expect(url.searchParams.get("from")).toBe("2026-10-03T07:48:24+02:00");
    expect(url.searchParams.get("to")).toBe("2026-10-03T08:17:34+02:00");
  });

  it("keeps UTC timestamps readable", () => {
    const href = tripLink({
      deviceId: 7,
      startTime: "2025-01-01T00:00:00Z",
      endTime: "2025-01-02T00:00:00Z",
    });

    expect(new URL(href, "http://localhost").searchParams.get("from")).toBe("2025-01-01T00:00:00Z");
  });
});

describe("normalizeTimeParam", () => {
  it("returns null for missing values", () => {
    expect(normalizeTimeParam(null)).toBeNull();
    expect(normalizeTimeParam("")).toBeNull();
  });

  it("converts UTC and offset timestamps to UTC ISO strings", () => {
    expect(normalizeTimeParam("2025-01-01T00:00:00Z")).toBe("2025-01-01T00:00:00.000Z");
    expect(normalizeTimeParam("2026-10-03T07:48:24+02:00")).toBe("2026-10-03T05:48:24.000Z");
    expect(normalizeTimeParam("2026-10-03T07:48:24-05:30")).toBe("2026-10-03T13:18:24.000Z");
  });

  it("repairs a '+' offset that was decoded to a space by an unencoded link", () => {
    // new URL('...&from=2026-10-03T07:48:24+02:00').searchParams.get('from')
    expect(normalizeTimeParam("2026-10-03T07:48:24 02:00")).toBe("2026-10-03T05:48:24.000Z");
  });

  it("returns null for unparsable values", () => {
    expect(normalizeTimeParam("not-a-date")).toBeNull();
  });
});

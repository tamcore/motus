import { describe, it, expect } from "vitest";
import {
  BOOKMARK_DESCRIPTION_MAX,
  BOOKMARK_NAME_MAX,
  bookmarkDuration,
  bookmarkErrorMessage,
  bookmarkFormFromRange,
  bookmarkMapHref,
  bookmarkMatchesRange,
  bookmarkOriginalFromRange,
  bookmarkRangeLabel,
  bookmarkToTrailRange,
  buildBookmarkPayload,
  charCount,
  filterBookmarks,
  parseBookmarkBoundary,
  type BookmarkFormInput,
} from "$lib/utils/trail-bookmarks";
import { formatDate, formatDuration } from "$lib/utils/formatting";
import type { TrailBookmark } from "$lib/types/api";

function bookmark(overrides: Partial<TrailBookmark> = {}): TrailBookmark {
  return {
    id: 1,
    deviceId: 42,
    deviceName: "Backpack",
    name: "Zugspitze",
    description: "Via Höllental",
    from: new Date(2026, 5, 6, 8, 0).toISOString(),
    to: new Date(2026, 5, 6, 16, 30).toISOString(),
    createdAt: new Date(2026, 5, 7).toISOString(),
    updatedAt: new Date(2026, 5, 7).toISOString(),
    ...overrides,
  };
}

function form(overrides: Partial<BookmarkFormInput> = {}): BookmarkFormInput {
  return {
    deviceId: 42,
    name: "Zugspitze",
    description: "Via Höllental",
    fromDate: "2026-06-06",
    fromTime: "08:00",
    toDate: "2026-06-06",
    toTime: "16:30",
    ...overrides,
  };
}

describe("bookmarkToTrailRange", () => {
  it("turns a bookmark into an absolute custom trail range", () => {
    const b = bookmark();
    expect(bookmarkToTrailRange(b)).toEqual({ preset: "custom", from: b.from, to: b.to });
  });

  it("normalises server timestamps to ISO", () => {
    const range = bookmarkToTrailRange(
      bookmark({ from: "2026-06-06T08:00:00+02:00", to: "2026-06-06T16:30:00+02:00" }),
    );
    expect(range).toEqual({
      preset: "custom",
      from: "2026-06-06T06:00:00.000Z",
      to: "2026-06-06T14:30:00.000Z",
    });
  });
});

describe("bookmarkMapHref", () => {
  it("links to the map with device and range", () => {
    const b = bookmark();
    const url = new URL(bookmarkMapHref(b), "http://localhost");
    expect(url.pathname).toBe("/map");
    expect(url.searchParams.get("device")).toBe("42");
    expect(url.searchParams.get("from")).toBe(new Date(b.from).toISOString());
    expect(url.searchParams.get("to")).toBe(new Date(b.to).toISOString());
    expect(url.searchParams.has("trail")).toBe(false);
  });
});

describe("bookmarkRangeLabel / bookmarkDuration", () => {
  it("formats both boundaries in the user's date format", () => {
    const b = bookmark();
    expect(bookmarkRangeLabel(b)).toBe(`${formatDate(b.from)} – ${formatDate(b.to)}`);
  });

  it.each([
    [new Date(2026, 5, 6, 8, 0), new Date(2026, 5, 6, 8, 45), "45m"],
    [new Date(2026, 5, 6, 8, 0), new Date(2026, 5, 6, 16, 30), "8h 30m"],
    [new Date(2026, 5, 6, 8, 0), new Date(2026, 5, 6, 16, 30, 59, 999), "8h 31m"],
    [new Date(2026, 5, 6, 0, 0), new Date(2026, 5, 6, 23, 59, 59, 999), "24h 0m"],
    [new Date(2026, 5, 6, 0, 0), new Date(2026, 5, 7, 0, 0), "24h 0m"],
  ])("reuses formatDuration (%s – %s → %s)", (from, to, expected) => {
    const b = bookmark({ from: from.toISOString(), to: to.toISOString() });
    expect(bookmarkDuration(b)).toBe(expected);
    expect(bookmarkDuration(b)).toBe(
      formatDuration(Math.round((to.getTime() - from.getTime()) / 60_000) * 60),
    );
  });
});

describe("bookmarkMatchesRange", () => {
  it("matches the same absolute range only", () => {
    const b = bookmark();
    expect(bookmarkMatchesRange(b, bookmarkToTrailRange(b))).toBe(true);
    expect(bookmarkMatchesRange(b, { preset: "24h" })).toBe(false);
    expect(
      bookmarkMatchesRange(b, {
        preset: "custom",
        from: b.from,
        to: new Date(2026, 5, 6, 17, 0).toISOString(),
      }),
    ).toBe(false);
  });
});

describe("charCount", () => {
  it("counts Unicode code points, not UTF-16 units", () => {
    expect(charCount("abc")).toBe(3);
    expect(charCount("öäü")).toBe(3);
    expect("🥾🥾".length).toBe(4);
    expect(charCount("🥾🥾")).toBe(2);
  });
});

describe("bookmarkFormFromRange", () => {
  it("prefills native date and time values", () => {
    const values = bookmarkFormFromRange({
      preset: "custom",
      from: new Date(2026, 5, 6, 8, 5).toISOString(),
      to: new Date(2026, 5, 7, 18, 0).toISOString(),
    });
    expect(values).toEqual({
      fromDate: "2026-06-06",
      fromTime: "08:05",
      toDate: "2026-06-07",
      toTime: "18:00",
    });
  });

  it("leaves times empty for whole days", () => {
    const values = bookmarkFormFromRange({
      preset: "custom",
      from: new Date(2026, 5, 6, 0, 0).toISOString(),
      to: new Date(2026, 5, 8, 23, 59, 59, 999).toISOString(),
    });
    expect(values).toEqual({
      fromDate: "2026-06-06",
      fromTime: "",
      toDate: "2026-06-08",
      toTime: "",
    });
  });

  it("freezes a relative preset to concrete times ending now", () => {
    const now = new Date(2026, 5, 6, 12, 0);
    expect(bookmarkFormFromRange({ preset: "24h" }, now)).toEqual({
      fromDate: "2026-06-05",
      fromTime: "12:00",
      toDate: "2026-06-06",
      toTime: "12:00",
    });
  });
});

describe("bookmarkOriginalFromRange", () => {
  it("keeps custom boundaries exactly", () => {
    const from = new Date(2026, 5, 6, 8, 0, 12, 345).toISOString();
    const to = new Date(2026, 5, 6, 16, 30, 59, 999).toISOString();
    expect(bookmarkOriginalFromRange({ preset: "custom", from, to })).toEqual({ from, to });
  });

  it("freezes relative presets at now", () => {
    const now = new Date(2026, 5, 6, 12, 0, 30, 500);
    expect(bookmarkOriginalFromRange({ preset: "24h" }, now)).toEqual({
      from: new Date(now.getTime() - 24 * 3600_000).toISOString(),
      to: now.toISOString(),
    });
  });
});

describe("parseBookmarkBoundary", () => {
  it("parses start and end of a minute", () => {
    expect(parseBookmarkBoundary("2026-06-06", "08:00", "start")).toEqual(
      new Date(2026, 5, 6, 8, 0, 0, 0),
    );
    expect(parseBookmarkBoundary("2026-06-06", "16:30", "end")).toEqual(
      new Date(2026, 5, 6, 16, 30, 59, 999),
    );
  });

  it("uses the whole day without a time", () => {
    expect(parseBookmarkBoundary("2026-06-06", "", "start")).toEqual(new Date(2026, 5, 6, 0, 0));
    expect(parseBookmarkBoundary("2026-06-06", "", "end")).toEqual(
      new Date(2026, 5, 6, 23, 59, 59, 999),
    );
  });

  it.each([
    ["", ""],
    ["06.06.2026", ""],
    ["2026-02-31", ""],
    ["2026-06-06", "25:00"],
    ["2026-06-06", "8:00"],
  ])("rejects %s %s", (date, time) => {
    expect(parseBookmarkBoundary(date, time, "start")).toBeNull();
  });
});

describe("buildBookmarkPayload", () => {
  it("builds a trimmed payload with ISO timestamps", () => {
    const result = buildBookmarkPayload(form({ name: "  Zugspitze ", description: "  nice  " }));
    expect(result).toEqual({
      ok: true,
      payload: {
        deviceId: 42,
        name: "Zugspitze",
        description: "nice",
        from: new Date(2026, 5, 6, 8, 0, 0, 0).toISOString(),
        to: new Date(2026, 5, 6, 16, 30, 59, 999).toISOString(),
      },
    });
  });

  it("covers whole days when times are empty", () => {
    const result = buildBookmarkPayload(form({ fromTime: "", toTime: "", toDate: "2026-06-08" }));
    expect(result.ok).toBe(true);
    if (!result.ok) return;
    expect(result.payload.from).toBe(new Date(2026, 5, 6, 0, 0, 0, 0).toISOString());
    expect(result.payload.to).toBe(new Date(2026, 5, 8, 23, 59, 59, 999).toISOString());
  });

  describe("with the original boundaries", () => {
    // Seconds and milliseconds a minute-precision form cannot show, as for
    // bookmarks created through the API.
    const original = {
      from: "2026-06-06T06:00:12.345Z",
      to: "2026-06-06T14:30:00.000Z",
    };
    const fields = bookmarkFormFromRange({ preset: "custom", ...original });

    it("keeps both timestamps when only the name changes", () => {
      const result = buildBookmarkPayload(form({ ...fields, name: "Renamed" }), original);
      expect(result).toEqual({
        ok: true,
        payload: {
          deviceId: 42,
          name: "Renamed",
          description: "Via Höllental",
          from: original.from,
          to: original.to,
        },
      });
    });

    it("reparses only an edited start", () => {
      const result = buildBookmarkPayload(form({ ...fields, fromTime: "07:15" }), original);
      expect(result.ok && result.payload.from).toBe(
        parseBookmarkBoundary(fields.fromDate, "07:15", "start")!.toISOString(),
      );
      expect(result.ok && result.payload.to).toBe(original.to);
    });

    it("reparses only an edited end", () => {
      const result = buildBookmarkPayload(form({ ...fields, toDate: "2026-06-07" }), original);
      expect(result.ok && result.payload.from).toBe(original.from);
      expect(result.ok && result.payload.to).toBe(
        parseBookmarkBoundary("2026-06-07", fields.toTime, "end")!.toISOString(),
      );
    });

    it("still rejects an edited start after the kept end", () => {
      const result = buildBookmarkPayload(form({ ...fields, fromDate: "2026-06-09" }), original);
      expect(result).toEqual({ ok: false, error: "Start must be before end" });
    });
  });

  it.each([
    ["missing name", { name: "   " }, "Name is required"],
    ["name too long", { name: "x".repeat(BOOKMARK_NAME_MAX + 1) }, "Name is too long"],
    ["angle brackets in name", { name: "<b>hike</b>" }, "Name must not contain < or >"],
    [
      "description too long",
      { description: "x".repeat(BOOKMARK_DESCRIPTION_MAX + 1) },
      "Description is too long",
    ],
    ["angle brackets in description", { description: "a > b" }, "Description must not contain < or >"],
    ["invalid start", { fromDate: "06.06.2026" }, "Invalid start"],
    ["invalid end", { toTime: "25:00" }, "Invalid end"],
    ["inverted range", { fromDate: "2026-06-07" }, "Start must be before end"],
  ])("rejects %s", (_label, overrides, error) => {
    const result = buildBookmarkPayload(form(overrides));
    expect(result.ok).toBe(false);
    if (result.ok) return;
    expect(result.error).toContain(error);
  });

  it("counts the limits in characters like the server", () => {
    // 200 astral characters are 400 UTF-16 units but within the limit.
    const name = "🥾".repeat(BOOKMARK_NAME_MAX);
    expect(buildBookmarkPayload(form({ name })).ok).toBe(true);
    expect(buildBookmarkPayload(form({ name: name + "🥾" })).ok).toBe(false);
  });

  it("allows an empty description", () => {
    const result = buildBookmarkPayload(form({ description: "" }));
    expect(result.ok && result.payload.description).toBe("");
  });
});

describe("bookmarkErrorMessage", () => {
  it("unwraps JSON error bodies", () => {
    expect(bookmarkErrorMessage(new Error('{"error":"name is required"}'), "x")).toBe(
      "name is required",
    );
  });

  it("keeps plain messages and falls back for unknown errors", () => {
    expect(bookmarkErrorMessage(new Error("boom"), "x")).toBe("boom");
    expect(bookmarkErrorMessage("nope", "fallback")).toBe("fallback");
    expect(bookmarkErrorMessage(new Error(""), "fallback")).toBe("fallback");
  });
});

describe("filterBookmarks", () => {
  const list = [
    bookmark({ id: 1, name: "Zugspitze", description: "Höllental", deviceName: "Backpack" }),
    bookmark({ id: 2, name: "Watzmann", description: "", deviceName: "Watch" }),
  ];

  it("returns everything for an empty query", () => {
    expect(filterBookmarks(list, "  ")).toHaveLength(2);
  });

  it("matches name, description and device name case-insensitively", () => {
    expect(filterBookmarks(list, "watz").map((b) => b.id)).toEqual([2]);
    expect(filterBookmarks(list, "HÖLLEN").map((b) => b.id)).toEqual([1]);
    expect(filterBookmarks(list, "watch").map((b) => b.id)).toEqual([2]);
  });
});

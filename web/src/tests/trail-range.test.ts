import { describe, it, expect, beforeEach, vi } from "vitest";
import { get } from "svelte/store";
import {
  DEFAULT_TRAIL_RANGE,
  TRAIL_PRESETS,
  customTrailRange,
  isLiveRange,
  resolveTrailRange,
  trailRangeFromSearchParams,
  trailRangeToSearchParams,
  type TrailRange,
} from "$lib/utils/trail-range";
import { ALL_TIME_START } from "$lib/utils/date-range";

const HOUR = 60 * 60 * 1000;
const DAY = 24 * HOUR;

describe("trail range presets", () => {
  it("defaults to the last 24h (previous behaviour)", () => {
    expect(DEFAULT_TRAIL_RANGE).toEqual({ preset: "24h" });
  });

  it("offers 24h, 48h, 7d, 30d, all time and custom", () => {
    expect(TRAIL_PRESETS.map((p) => p.value)).toEqual([
      "24h",
      "48h",
      "7d",
      "30d",
      "all",
      "custom",
    ]);
  });

  it.each([
    ["24h", DAY],
    ["48h", 2 * DAY],
    ["7d", 7 * DAY],
    ["30d", 30 * DAY],
  ] as const)("resolves %s relative to now", (preset, span) => {
    const now = new Date("2026-10-03T12:00:00Z");
    const { from, to } = resolveTrailRange({ preset }, now);
    expect(to.getTime()).toBe(now.getTime());
    expect(now.getTime() - from.getTime()).toBe(span);
  });

  it("resolves all time from 2020-01-01 (same as heatmap/reports) to now", () => {
    const now = new Date("2026-10-03T12:00:00Z");
    const { from, to } = resolveTrailRange({ preset: "all" }, now);
    expect(from.toISOString()).toBe("2020-01-01T00:00:00.000Z");
    expect(from.getTime()).toBe(ALL_TIME_START.getTime());
    expect(to.getTime()).toBe(now.getTime());
  });

  it("resolves a custom range to its exact from/to", () => {
    const range: TrailRange = {
      preset: "custom",
      from: "2026-09-01T06:30:00.000Z",
      to: "2026-09-02T18:00:00.000Z",
    };
    const { from, to } = resolveTrailRange(range);
    expect(from.toISOString()).toBe(range.from);
    expect(to.toISOString()).toBe(range.to);
  });
});

describe("isLiveRange", () => {
  const now = new Date("2026-10-03T12:00:00Z");

  it("treats presets as live", () => {
    expect(isLiveRange({ preset: "7d" }, now)).toBe(true);
    expect(isLiveRange({ preset: "all" }, now)).toBe(true);
  });

  it("treats a custom range that ended in the past as not live", () => {
    expect(
      isLiveRange(
        { preset: "custom", from: "2026-09-01T00:00:00.000Z", to: "2026-09-02T00:00:00.000Z" },
        now,
      ),
    ).toBe(false);
  });

  it("treats a custom range ending in the future as live", () => {
    expect(
      isLiveRange(
        { preset: "custom", from: "2026-10-03T00:00:00.000Z", to: "2026-10-03T23:59:59.000Z" },
        now,
      ),
    ).toBe(true);
  });
});

describe("customTrailRange", () => {
  it("uses local start/end of day when no time is given", () => {
    expect(customTrailRange("2026-09-01", "", "2026-09-02", "")).toEqual({
      preset: "custom",
      from: new Date(2026, 8, 1, 0, 0, 0).toISOString(),
      to: new Date(2026, 8, 2, 23, 59, 59, 999).toISOString(),
    });
  });

  it("applies optional times in local time", () => {
    expect(customTrailRange("2026-09-01", "06:30", "2026-09-02", "18:15")).toEqual({
      preset: "custom",
      from: new Date(2026, 8, 1, 6, 30, 0).toISOString(),
      to: new Date(2026, 8, 2, 18, 15, 59, 999).toISOString(),
    });
  });

  it("accepts a single day", () => {
    expect(customTrailRange("2026-09-02", "", "2026-09-02", "")).not.toBeNull();
  });

  it("rejects a start after the end", () => {
    expect(customTrailRange("2026-09-03", "", "2026-09-02", "")).toBeNull();
    expect(customTrailRange("2026-09-02", "12:00", "2026-09-02", "11:00")).toBeNull();
  });

  it("rejects missing dates", () => {
    expect(customTrailRange("", "", "2026-09-02", "")).toBeNull();
    expect(customTrailRange("2026-09-01", "", "", "")).toBeNull();
  });
});

describe("URL search params", () => {
  it("reads a preset from ?trail=", () => {
    expect(trailRangeFromSearchParams(new URLSearchParams("trail=7d"))).toEqual({
      preset: "7d",
    });
  });

  it("reads an arbitrary range from ?from=&to=", () => {
    const params = new URLSearchParams(
      "from=2026-09-01T06:30:00.000Z&to=2026-09-02T18:00:00.000Z",
    );
    expect(trailRangeFromSearchParams(params)).toEqual({
      preset: "custom",
      from: "2026-09-01T06:30:00.000Z",
      to: "2026-09-02T18:00:00.000Z",
    });
  });

  it("from/to take precedence over trail=", () => {
    const params = new URLSearchParams(
      "trail=24h&from=2026-09-01T00:00:00Z&to=2026-09-02T00:00:00Z",
    );
    expect(trailRangeFromSearchParams(params)?.preset).toBe("custom");
  });

  it("returns null for absent or invalid params", () => {
    expect(trailRangeFromSearchParams(new URLSearchParams(""))).toBeNull();
    expect(trailRangeFromSearchParams(new URLSearchParams("trail=1y"))).toBeNull();
    expect(
      trailRangeFromSearchParams(new URLSearchParams("from=nope&to=2026-09-02T00:00:00Z")),
    ).toBeNull();
    expect(
      trailRangeFromSearchParams(
        new URLSearchParams("from=2026-09-03T00:00:00Z&to=2026-09-02T00:00:00Z"),
      ),
    ).toBeNull();
  });

  it("writes presets as trail= and custom ranges as from/to, keeping other params", () => {
    const base = new URLSearchParams("device=4&trail=7d&from=x&to=y");
    const preset = trailRangeToSearchParams({ preset: "48h" }, base);
    expect(preset.get("device")).toBe("4");
    expect(preset.get("trail")).toBe("48h");
    expect(preset.has("from")).toBe(false);
    expect(preset.has("to")).toBe(false);

    const custom = trailRangeToSearchParams(
      { preset: "custom", from: "2026-09-01T00:00:00.000Z", to: "2026-09-02T00:00:00.000Z" },
      base,
    );
    expect(custom.get("device")).toBe("4");
    expect(custom.has("trail")).toBe(false);
    expect(custom.get("from")).toBe("2026-09-01T00:00:00.000Z");
    expect(custom.get("to")).toBe("2026-09-02T00:00:00.000Z");
    // The input is not mutated.
    expect(base.get("trail")).toBe("7d");
  });

  it("round-trips through search params", () => {
    const range: TrailRange = {
      preset: "custom",
      from: "2026-09-01T00:00:00.000Z",
      to: "2026-09-02T00:00:00.000Z",
    };
    expect(trailRangeFromSearchParams(trailRangeToSearchParams(range))).toEqual(range);
  });
});

describe("trailRange store", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.resetModules();
  });

  it("starts at the 24h default", async () => {
    const { trailRange } = await import("$lib/stores/trailRange");
    expect(get(trailRange)).toEqual({ preset: "24h" });
  });

  it("exposes only subscribe and set", async () => {
    const { trailRange } = await import("$lib/stores/trailRange");
    expect(Object.keys(trailRange).sort()).toEqual(["set", "subscribe"]);
  });

  it("persists the selection to localStorage", async () => {
    const { trailRange } = await import("$lib/stores/trailRange");
    trailRange.set({ preset: "30d" });
    vi.resetModules();
    const reloaded = await import("$lib/stores/trailRange");
    expect(get(reloaded.trailRange)).toEqual({ preset: "30d" });
  });

  it("accepts an arbitrary custom range (e.g. from a bookmark)", async () => {
    const { trailRange } = await import("$lib/stores/trailRange");
    trailRange.set({
      preset: "custom",
      from: "2026-09-01T00:00:00Z",
      to: "2026-09-02T00:00:00Z",
    });
    expect(get(trailRange)).toEqual({
      preset: "custom",
      from: "2026-09-01T00:00:00.000Z",
      to: "2026-09-02T00:00:00.000Z",
    });
  });

  it("falls back to the default when set to an invalid range", async () => {
    const { trailRange } = await import("$lib/stores/trailRange");
    trailRange.set({ preset: "custom", from: "bad", to: "worse" });
    expect(get(trailRange)).toEqual({ preset: "24h" });
  });

  it("falls back to the default for corrupted storage", async () => {
    localStorage.setItem("motus_trail_range", "{not json");
    const { trailRange } = await import("$lib/stores/trailRange");
    expect(get(trailRange)).toEqual({ preset: "24h" });
  });

  it("falls back to the default for an invalid stored range", async () => {
    localStorage.setItem(
      "motus_trail_range",
      JSON.stringify({ preset: "custom", from: "bad", to: "worse" }),
    );
    const { trailRange } = await import("$lib/stores/trailRange");
    expect(get(trailRange)).toEqual({ preset: "24h" });
  });
});

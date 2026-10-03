/**
 * Time range model for the map view's device trail: a relative preset ending
 * "now", or an absolute custom range with ISO from/to timestamps (custom
 * input, URL params, saved bookmarks).
 */
import { ALL_TIME_START, dateValue, parseLocalBoundary, timeValue } from "./date-range";

export type TrailRelativePreset = "24h" | "48h" | "7d" | "30d" | "all";
export type TrailPreset = TrailRelativePreset | "custom";

export type TrailRange =
  | { preset: TrailRelativePreset }
  | { preset: "custom"; from: string; to: string };

export const DEFAULT_TRAIL_RANGE: TrailRange = { preset: "24h" };

const HOUR = 60 * 60 * 1000;
const DAY = 24 * HOUR;

const PRESET_SPANS: Record<Exclude<TrailRelativePreset, "all">, number> = {
  "24h": DAY,
  "48h": 2 * DAY,
  "7d": 7 * DAY,
  "30d": 30 * DAY,
};

export const TRAIL_PRESETS: { value: TrailPreset; label: string }[] = [
  { value: "24h", label: "Last 24h" },
  { value: "48h", label: "Last 48h" },
  { value: "7d", label: "Last 7 days" },
  { value: "30d", label: "Last 30 days" },
  { value: "all", label: "All time" },
  { value: "custom", label: "Custom range" },
];

const RELATIVE_PRESETS = new Set<string>(["24h", "48h", "7d", "30d", "all"]);

export function isRelativePreset(value: unknown): value is TrailRelativePreset {
  return typeof value === "string" && RELATIVE_PRESETS.has(value);
}

function validDate(iso: unknown): Date | null {
  if (typeof iso !== "string" || iso === "") return null;
  const d = new Date(iso);
  return isNaN(d.getTime()) ? null : d;
}

/** Validates an unknown value (e.g. from storage) as a TrailRange. */
export function normalizeTrailRange(value: unknown): TrailRange | null {
  if (!value || typeof value !== "object") return null;
  const v = value as Record<string, unknown>;
  if (isRelativePreset(v.preset)) return { preset: v.preset };
  if (v.preset === "custom") {
    const from = validDate(v.from);
    const to = validDate(v.to);
    if (!from || !to || from.getTime() > to.getTime()) return null;
    return { preset: "custom", from: from.toISOString(), to: to.toISOString() };
  }
  return null;
}

/** Concrete from/to dates for a range; relative presets end at `now`. */
export function resolveTrailRange(
  range: TrailRange,
  now: Date = new Date(),
): { from: Date; to: Date } {
  if (range.preset === "custom") {
    return { from: new Date(range.from), to: new Date(range.to) };
  }
  if (range.preset === "all") {
    return { from: new Date(ALL_TIME_START), to: now };
  }
  return { from: new Date(now.getTime() - PRESET_SPANS[range.preset]), to: now };
}

/** Whether live positions belong on the trail: the range has not ended yet. */
export function isLiveRange(range: TrailRange, now: Date = new Date()): boolean {
  return range.preset !== "custom" || new Date(range.to).getTime() >= now.getTime();
}

/** Native date/time input values: date `yyyy-mm-dd`, time `HH:mm` or empty (whole day). */
export interface RangeInputs {
  fromDate: string;
  fromTime: string;
  toDate: string;
  toTime: string;
}

/**
 * Input values for a range (relative presets end at `now`). A start at
 * 00:00 / an end at 23:59 leaves the time empty (whole day).
 */
export function rangeToInputs(range: TrailRange, now: Date = new Date()): RangeInputs {
  const { from, to } = resolveTrailRange(range, now);
  const fromIsDayStart = from.getHours() === 0 && from.getMinutes() === 0;
  const toIsDayEnd = to.getHours() === 23 && to.getMinutes() === 59;
  return {
    fromDate: dateValue(from),
    fromTime: fromIsDayStart ? "" : timeValue(from),
    toDate: dateValue(to),
    toTime: toIsDayEnd ? "" : timeValue(to),
  };
}

/**
 * Builds a custom range from native input values in local time. Returns null
 * when a value is missing/invalid or the start is after the end.
 */
export function customTrailRange(
  fromDate: string,
  fromTime: string,
  toDate: string,
  toTime: string,
): TrailRange | null {
  const from = parseLocalBoundary(fromDate, fromTime, "start");
  const to = parseLocalBoundary(toDate, toTime, "end");
  if (!from || !to || from > to) return null;
  return { preset: "custom", from: from.toISOString(), to: to.toISOString() };
}

/**
 * Reads a range from URL params: `from` + `to` (ISO) select an absolute range
 * and take precedence over `trail=<preset>`. Returns null when absent/invalid.
 */
export function trailRangeFromSearchParams(params: URLSearchParams): TrailRange | null {
  if (params.has("from") || params.has("to")) {
    return normalizeTrailRange({
      preset: "custom",
      from: params.get("from"),
      to: params.get("to"),
    });
  }
  const trail = params.get("trail");
  return isRelativePreset(trail) ? { preset: trail } : null;
}

/** Returns a copy of `base` with the range encoded (other params kept). */
export function trailRangeToSearchParams(
  range: TrailRange,
  base: URLSearchParams = new URLSearchParams(),
): URLSearchParams {
  const params = new URLSearchParams(base);
  params.delete("trail");
  params.delete("from");
  params.delete("to");
  if (range.preset === "custom") {
    params.set("from", range.from);
    params.set("to", range.to);
  } else {
    params.set("trail", range.preset);
  }
  return params;
}

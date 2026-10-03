/**
 * Helpers for trail bookmarks: named, saved absolute time ranges of a device
 * trail (e.g. hikes). A bookmark maps onto a `{ preset: "custom" }` TrailRange
 * so applying it goes through the regular trail range handling.
 */
import type { TrailBookmark, TrailBookmarkPayload } from "$lib/types/api";
import { formatDate, formatDuration } from "$lib/utils/formatting";
import {
  resolveTrailRange,
  trailRangeToSearchParams,
  type TrailRange,
} from "$lib/utils/trail-range";

/**
 * Length limits in characters (Unicode code points), the unit the server
 * counts in as well (see handlers/trail_bookmark.go).
 */
export const BOOKMARK_NAME_MAX = 200;
export const BOOKMARK_DESCRIPTION_MAX = 2000;

/** Length in characters (Unicode code points, not UTF-16 units or bytes). */
export function charCount(value: string): number {
  return Array.from(value).length;
}

/** The absolute trail range a bookmark stands for. */
export function bookmarkToTrailRange(b: Pick<TrailBookmark, "from" | "to">): TrailRange {
  return {
    preset: "custom",
    from: new Date(b.from).toISOString(),
    to: new Date(b.to).toISOString(),
  };
}

/** Map URL that opens the bookmarked device trail directly. */
export function bookmarkMapHref(b: Pick<TrailBookmark, "deviceId" | "from" | "to">): string {
  const params = trailRangeToSearchParams(
    bookmarkToTrailRange(b),
    new URLSearchParams({ device: String(b.deviceId) }),
  );
  return `/map?${params.toString()}`;
}

/** Start – end in the user's date format. */
export function bookmarkRangeLabel(b: Pick<TrailBookmark, "from" | "to">): string {
  return `${formatDate(b.from)} – ${formatDate(b.to)}`;
}

/** Human-readable length of the bookmarked range ("8h 30m"). */
export function bookmarkDuration(b: Pick<TrailBookmark, "from" | "to">): string {
  const ms = Math.max(0, new Date(b.to).getTime() - new Date(b.from).getTime());
  // Whole minutes, so an end at :59.999 (whole last minute) counts it fully.
  return formatDuration(Math.round(ms / 60_000) * 60);
}

/** True when `range` is exactly the bookmark's range (e.g. to highlight it). */
export function bookmarkMatchesRange(b: Pick<TrailBookmark, "from" | "to">, range: TrailRange): boolean {
  if (range.preset !== "custom") return false;
  const own = bookmarkToTrailRange(b);
  return (
    own.preset === "custom" &&
    new Date(range.from).getTime() === new Date(own.from).getTime() &&
    new Date(range.to).getTime() === new Date(own.to).getTime()
  );
}

/**
 * Native input values of a bookmark form: date `yyyy-mm-dd`, time `HH:mm` or
 * empty (start / end of the day), in local time.
 */
export interface BookmarkRangeFields {
  fromDate: string;
  fromTime: string;
  toDate: string;
  toTime: string;
}

/** The exact boundaries a bookmark form was prefilled with. */
export interface BookmarkRangeOriginal {
  from: string;
  to: string;
}

const pad2 = (n: number) => String(n).padStart(2, "0");
const dateValue = (d: Date) => `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
const timeValue = (d: Date) => `${pad2(d.getHours())}:${pad2(d.getMinutes())}`;

/**
 * Freezes a range to absolute ISO boundaries (relative presets end at `now`),
 * since a bookmark always stores an absolute window.
 */
export function bookmarkOriginalFromRange(
  range: TrailRange,
  now: Date = new Date(),
): BookmarkRangeOriginal {
  const { from, to } = resolveTrailRange(range, now);
  return { from: from.toISOString(), to: to.toISOString() };
}

/**
 * Form values for absolute boundaries. As in the trail range selector, a
 * start at 00:00 / an end at 23:59 leaves the time empty (whole day).
 */
export function bookmarkFormFromRange(range: TrailRange, now: Date = new Date()): BookmarkRangeFields {
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

const DATE_RE = /^\d{4}-\d{2}-\d{2}$/;
const TIME_RE = /^\d{2}:\d{2}$/;

/**
 * Parses native date + optional time input values as local time. A start
 * covers its whole minute from :00.000, an end up to :59.999; an empty time
 * means 00:00 / 23:59. Returns null for invalid input.
 */
export function parseBookmarkBoundary(date: string, time: string, edge: "start" | "end"): Date | null {
  if (!DATE_RE.test(date) || (time !== "" && !TIME_RE.test(time))) return null;
  const hm = time || (edge === "start" ? "00:00" : "23:59");
  const d = new Date(`${date}T${hm}:${edge === "start" ? "00.000" : "59.999"}`);
  // Reject overflowing values such as 2026-02-31 or 24:30.
  if (isNaN(d.getTime()) || dateValue(d) !== date || timeValue(d) !== hm) return null;
  return d;
}

export interface BookmarkFormInput extends BookmarkRangeFields {
  deviceId: number;
  name: string;
  description: string;
}

export type BookmarkPayloadResult =
  | { ok: true; payload: TrailBookmarkPayload }
  | { ok: false; error: string };

const ANGLE_BRACKETS = /[<>]/;

/**
 * Validates the bookmark form and builds the API payload. With `original`
 * (the boundaries the form was prefilled with), a boundary whose date and
 * time fields are unchanged keeps its exact original timestamp; only edited
 * boundaries are reparsed (at minute precision).
 */
export function buildBookmarkPayload(
  input: BookmarkFormInput,
  original?: BookmarkRangeOriginal | null,
): BookmarkPayloadResult {
  const name = input.name.trim();
  const description = input.description.trim();
  if (name === "") return { ok: false, error: "Name is required" };
  if (charCount(name) > BOOKMARK_NAME_MAX) {
    return { ok: false, error: `Name is too long (max ${BOOKMARK_NAME_MAX} characters)` };
  }
  if (ANGLE_BRACKETS.test(name)) return { ok: false, error: "Name must not contain < or >" };
  if (charCount(description) > BOOKMARK_DESCRIPTION_MAX) {
    return {
      ok: false,
      error: `Description is too long (max ${BOOKMARK_DESCRIPTION_MAX} characters)`,
    };
  }
  if (ANGLE_BRACKETS.test(description)) {
    return { ok: false, error: "Description must not contain < or >" };
  }

  const initial = original
    ? bookmarkFormFromRange({ preset: "custom", from: original.from, to: original.to })
    : null;
  // Untouched boundaries are sent verbatim (keeps seconds / sub-ms precision
  // of bookmarks created through the API).
  const from =
    original && initial && input.fromDate === initial.fromDate && input.fromTime === initial.fromTime
      ? original.from
      : parseBookmarkBoundary(input.fromDate, input.fromTime, "start")?.toISOString();
  if (!from) return { ok: false, error: "Invalid start date or time" };
  const to =
    original && initial && input.toDate === initial.toDate && input.toTime === initial.toTime
      ? original.to
      : parseBookmarkBoundary(input.toDate, input.toTime, "end")?.toISOString();
  if (!to) return { ok: false, error: "Invalid end date or time" };
  if (new Date(from).getTime() >= new Date(to).getTime()) {
    return { ok: false, error: "Start must be before end" };
  }

  return { ok: true, payload: { deviceId: input.deviceId, name, description, from, to } };
}

/** User-facing message for a failed request (unwraps `{"error": "..."}` bodies). */
export function bookmarkErrorMessage(err: unknown, fallback: string): string {
  if (!(err instanceof Error) || err.message === "") return fallback;
  try {
    const body = JSON.parse(err.message) as { error?: unknown } | null;
    if (body && typeof body.error === "string" && body.error !== "") return body.error;
  } catch {
    // Plain-text message.
  }
  return err.message;
}

/** Case-insensitive search over name, description and device name. */
export function filterBookmarks(list: TrailBookmark[], query: string): TrailBookmark[] {
  const q = query.trim().toLowerCase();
  if (q === "") return list;
  return list.filter((b) =>
    [b.name, b.description, b.deviceName ?? ""].some((v) => v.toLowerCase().includes(q)),
  );
}

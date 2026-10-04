/** Start of the "All time" range on every page (map trail, heatmap, reports). */
export const ALL_TIME_START = new Date("2020-01-01T00:00:00.000Z");

export type DatePreset =
  | "day"
  | "week"
  | "month"
  | "all"
  | "custom"
  | "today"
  | "yesterday"
  | "thisWeek"
  | "prevWeek"
  | "thisMonth"
  | "prevMonth";

/** Preset buttons shared by the reports and replay pages. */
export const RELATIVE_DATE_PRESETS: { value: DatePreset; label: string }[] = [
  { value: "day", label: "Last 24h" },
  { value: "week", label: "Last 7d" },
  { value: "month", label: "Last 30d" },
  { value: "all", label: "All time" },
  { value: "custom", label: "Custom" },
];

export const pad2 = (n: number) => String(n).padStart(2, "0");
/** Local `yyyy-mm-dd` of a date, as used by native date inputs. */
export const dateValue = (d: Date) => `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
export const timeValue = (d: Date) => `${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
/** RFC 3339 timestamp `hours` from now, for expiry presets. */
export const hoursFromNow = (hours: number) => new Date(Date.now() + hours * 3_600_000).toISOString();

const DATE_RE = /^\d{4}-\d{2}-\d{2}$/;
const TIME_RE = /^\d{2}:\d{2}$/;

/**
 * Parses native date (yyyy-mm-dd) + optional time (HH:mm) input values as
 * local time. A start covers its whole minute from :00.000, an end up to
 * :59.999; an empty time means 00:00 / 23:59. Returns null for invalid input.
 */
export function parseLocalBoundary(date: string, time: string, edge: "start" | "end"): Date | null {
  if (!DATE_RE.test(date) || (time !== "" && !TIME_RE.test(time))) return null;
  const hm = time || (edge === "start" ? "00:00" : "23:59");
  const d = new Date(`${date}T${hm}:${edge === "start" ? "00.000" : "59.999"}`);
  // Reject overflowing values such as 2026-02-31 or 24:30.
  if (isNaN(d.getTime()) || dateValue(d) !== date || timeValue(d) !== hm) return null;
  return d;
}

const DAYS_BACK: Partial<Record<DatePreset, number>> = { day: 1, week: 7, month: 30 };

/**
 * ISO from/to for a date preset. Custom dates (yyyy-mm-dd, local time) cover
 * whole days; a missing or invalid custom start falls back to `emptyFrom`, a
 * missing end to `now`.
 */
export function resolveDatePreset(
  preset: DatePreset,
  customFrom = "",
  customTo = "",
  now: Date = new Date(),
  emptyFrom: Date = now,
): { from: string; to: string } {
  const iso = (from: Date, to: Date = now) => ({ from: from.toISOString(), to: to.toISOString() });
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const dayOffset = (d: Date, days: number) =>
    new Date(d.getFullYear(), d.getMonth(), d.getDate() + days);
  const weekStart = dayOffset(today, -today.getDay());
  const monthStart = new Date(today.getFullYear(), today.getMonth(), 1);

  switch (preset) {
    case "custom":
      return iso(
        parseLocalBoundary(customFrom, "", "start") ?? emptyFrom,
        parseLocalBoundary(customTo, "", "end") ?? now,
      );
    case "all":
      return iso(ALL_TIME_START);
    case "today":
      return iso(today);
    case "yesterday":
      return iso(dayOffset(today, -1), today);
    case "thisWeek":
      return iso(weekStart);
    case "prevWeek":
      return iso(dayOffset(weekStart, -7), weekStart);
    case "thisMonth":
      return iso(monthStart);
    case "prevMonth":
      return iso(new Date(today.getFullYear(), today.getMonth() - 1, 1), monthStart);
    default: {
      const from = new Date(now);
      from.setDate(from.getDate() - (DAYS_BACK[preset] ?? 1));
      return iso(from);
    }
  }
}

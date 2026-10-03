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

const DAYS_BACK: Partial<Record<DatePreset, number>> = { day: 1, week: 7, month: 30 };

/**
 * ISO from/to for a date preset. Custom dates (yyyy-mm-dd, local time) cover
 * whole days; a missing custom date falls back to `now`.
 */
export function resolveDatePreset(
  preset: DatePreset,
  customFrom = "",
  customTo = "",
  now: Date = new Date(),
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
        customFrom ? new Date(`${customFrom}T00:00:00`) : now,
        customTo ? new Date(`${customTo}T23:59:59`) : now,
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

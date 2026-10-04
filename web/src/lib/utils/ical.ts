import { dateValue, pad2, parseLocalBoundary } from "./date-range";

export type TemplateId =
  | "business_hours"
  | "weekends"
  | "weeknights"
  | "always"
  | "custom";

export interface CalendarTemplate {
  id: TemplateId;
  label: string;
  description: string;
  data: string;
}

const ICAL_DAYS = ["SU", "MO", "TU", "WE", "TH", "FR", "SA"] as const;

const DAY_NAMES = [
  "Sunday",
  "Monday",
  "Tuesday",
  "Wednesday",
  "Thursday",
  "Friday",
  "Saturday",
];

const DAY_SHORT = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

function makeIcal(
  summary: string,
  dtstart: string,
  dtend: string,
  rrule?: string,
): string {
  return [
    "BEGIN:VCALENDAR",
    "VERSION:2.0",
    "PRODID:-//Motus//Calendar//EN",
    "BEGIN:VEVENT",
    `SUMMARY:${summary}`,
    `DTSTART:${dtstart}`,
    `DTEND:${dtend}`,
    ...(rrule ? [`RRULE:${rrule}`] : []),
    "END:VEVENT",
    "END:VCALENDAR",
  ].join("\r\n");
}

export const CALENDAR_TEMPLATES: readonly CalendarTemplate[] = [
  {
    id: "business_hours",
    label: "Business Hours",
    description: "Monday through Friday, 8:00 AM to 5:00 PM",
    data: makeIcal(
      "Business Hours",
      "20240101T080000",
      "20240101T170000",
      "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR",
    ),
  },
  {
    id: "weekends",
    label: "Weekends",
    description: "Saturday and Sunday, all day",
    data: makeIcal(
      "Weekends",
      "20240106T000000",
      "20240106T235959",
      "FREQ=WEEKLY;BYDAY=SA,SU",
    ),
  },
  {
    id: "weeknights",
    label: "Weeknights",
    description: "Monday through Friday, 6:00 PM to 8:00 AM next day",
    data: makeIcal(
      "Weeknights",
      "20240101T180000",
      "20240102T080000",
      "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR",
    ),
  },
  {
    id: "always",
    label: "24/7",
    description: "Always active, every day",
    data: makeIcal(
      "Always Active",
      "20240101T000000",
      "20240101T235959",
      "FREQ=DAILY",
    ),
  },
  {
    id: "custom",
    label: "Custom",
    description: "Write your own iCalendar schedule",
    data: "",
  },
] as const;

interface ParsedEvent {
  summary: string;
  dtstart: Date | null;
  dtend: Date | null;
  rrule: string | null;
  byDay: string[];
  freq: string | null;
  until: Date | null;
}

/** Lightweight parser for a minimal iCalendar subset; the backend performs full validation. */
function parseIcalEvents(icalData: string): ParsedEvent[] {
  const events: ParsedEvent[] = [];
  const eventBlocks = icalData.split("BEGIN:VEVENT");

  for (let i = 1; i < eventBlocks.length; i++) {
    const block = eventBlocks[i].split("END:VEVENT")[0];
    const lines = block.split(/\r?\n/);

    const event: ParsedEvent = {
      summary: "",
      dtstart: null,
      dtend: null,
      rrule: null,
      byDay: [],
      freq: null,
      until: null,
    };

    for (const line of lines) {
      const trimmed = line.trim();
      if (trimmed.startsWith("SUMMARY:")) {
        event.summary = trimmed.slice(8);
      } else if (trimmed.startsWith("DTSTART")) {
        event.dtstart = parseIcalDateTime(trimmed);
      } else if (trimmed.startsWith("DTEND")) {
        event.dtend = parseIcalDateTime(trimmed);
      } else if (trimmed.startsWith("RRULE:")) {
        event.rrule = trimmed.slice(6);
        const params = parseRruleParams(event.rrule);
        event.freq = params.FREQ || null;
        if (params.BYDAY) {
          // Strip numeric prefixes (e.g., "1MO" -> "MO").
          event.byDay = params.BYDAY.split(",").map((d) => d.trim().replace(/^\d+/, ""));
        }
        if (params.UNTIL) {
          event.until = parseIcalDateTime(`UNTIL:${params.UNTIL}`);
        }
      }
    }

    events.push(event);
  }

  return events;
}

/** Floating/UTC value of a line like "DTSTART;TZID=America/New_York:20240101T080000", as a UTC Date. */
function parseIcalDateTime(line: string): Date | null {
  const colonIdx = line.lastIndexOf(":");
  if (colonIdx === -1) return null;
  const val = line.slice(colonIdx + 1).trim();

  const match = val.match(/^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})Z?$/);
  if (match) {
    const [, y, m, d, h, min, s] = match;
    return new Date(
      Date.UTC(
        parseInt(y),
        parseInt(m) - 1,
        parseInt(d),
        parseInt(h),
        parseInt(min),
        parseInt(s),
      ),
    );
  }

  const dateMatch = val.match(/^(\d{4})(\d{2})(\d{2})$/);
  if (dateMatch) {
    const [, y, m, d] = dateMatch;
    return new Date(Date.UTC(parseInt(y), parseInt(m) - 1, parseInt(d)));
  }

  return null;
}

function parseRruleParams(rrule: string): Record<string, string> {
  const result: Record<string, string> = {};
  for (const part of rrule.split(";")) {
    const [key, value] = part.split("=", 2);
    if (key && value) {
      result[key] = value;
    }
  }
  return result;
}

function formatTime12h(date: Date): string {
  return date.toLocaleTimeString("en-US", { timeZone: "UTC", hour: "numeric", minute: "2-digit" });
}

/** Sorted weekday indices (Sun=0) of BYDAY codes. */
function dayIndices(days: string[]): number[] {
  return days
    .map((d) => ICAL_DAYS.indexOf(d as (typeof ICAL_DAYS)[number]))
    .filter((i) => i !== -1)
    .sort((a, b) => a - b);
}

/** Milliseconds since UTC midnight. */
function utcTimeOfDayMs(d: Date): number {
  return (d.getUTCHours() * 3600 + d.getUTCMinutes() * 60 + d.getUTCSeconds()) * 1000;
}

function formatDayList(days: string[]): string {
  const indices = dayIndices(days);

  if (indices.length === 0) return "";

  const weekdays = [1, 2, 3, 4, 5];
  const weekend = [0, 6];

  if (indices.length === 5 && weekdays.every((d) => indices.includes(d))) {
    return "Mon-Fri";
  }
  if (indices.length === 2 && weekend.every((d) => indices.includes(d))) {
    return "Sat-Sun";
  }
  if (indices.length === 7) {
    return "Every day";
  }

  return indices.map((i) => DAY_SHORT[i]).join(", ");
}

export function getScheduleSummary(icalData: string): string {
  if (!icalData || !icalData.trim()) return "No schedule defined";

  try {
    const events = parseIcalEvents(icalData);
    if (events.length === 0) return "No events defined";

    const summaries = events.map((event) => {
      const parts: string[] = [];

      if (event.freq === "DAILY") {
        parts.push("Daily");
      } else if (event.freq === "WEEKLY" && event.byDay.length > 0) {
        parts.push(formatDayList(event.byDay));
      } else if (event.freq === "WEEKLY") {
        parts.push("Weekly");
      } else if (event.freq === "MONTHLY") {
        parts.push("Monthly");
      } else if (event.freq === "YEARLY") {
        parts.push("Yearly");
      }

      if (event.dtstart && event.dtend) {
        const startTime = formatTime12h(event.dtstart);
        const endTime = formatTime12h(event.dtend);

        const diffMs = event.dtend.getTime() - event.dtstart.getTime();
        const diffHours = diffMs / (1000 * 60 * 60);

        if (diffHours >= 23.5) {
          parts.push("all day");
        } else {
          parts.push(`${startTime} - ${endTime}`);
        }
      }

      return parts.join(", ") || event.summary || "Custom schedule";
    });

    return summaries.join(" | ");
  } catch {
    return "Custom schedule";
  }
}

/**
 * Get a human-readable status string: "Active now" or "Next active: <description>".
 */
export function getActiveStatus(icalData: string): {
  active: boolean;
  label: string;
} {
  if (!icalData || !icalData.trim()) {
    return { active: false, label: "No schedule" };
  }

  try {
    const events = parseIcalEvents(icalData);
    const now = new Date();

    for (const event of events) {
      if (isEventActiveAt(event, now)) {
        return { active: true, label: "Active now" };
      }
    }

    // Check if all events are expired (past UNTIL)
    const allExpired = events.every(
      (event) => event.until && now > event.until,
    );
    if (allExpired) {
      return { active: false, label: "Expired" };
    }

    // Find next occurrence
    const nextLabel = getNextOccurrenceLabel(events, now);
    return { active: false, label: nextLabel };
  } catch {
    return { active: false, label: "Unknown" };
  }
}

/**
 * Check if a parsed event is active at the given time.
 * Handles recurring events with RRULE (WEEKLY with BYDAY, DAILY).
 */
function isEventActiveAt(event: ParsedEvent, t: Date): boolean {
  if (!event.dtstart || !event.dtend) return false;

  const eventDurationMs = event.dtend.getTime() - event.dtstart.getTime();

  if (!event.rrule) {
    // Single occurrence
    return (
      t.getTime() >= event.dtstart.getTime() &&
      t.getTime() < event.dtend.getTime()
    );
  }

  // Recurring event
  if (event.freq === "WEEKLY" && event.byDay.length > 0) {
    return isActiveInWeeklyByDay(event, eventDurationMs, t);
  }

  if (event.freq === "DAILY") {
    return isActiveInDailyRecurrence(event, eventDurationMs, t);
  }

  return false;
}

/** Whether the time of day of t falls within the window starting at the time of day of start. */
function inDailyWindow(start: Date, eventDurationMs: number, t: Date): boolean {
  const startMs = utcTimeOfDayMs(start);
  const nowMs = utcTimeOfDayMs(t);
  return nowMs >= startMs && nowMs < startMs + eventDurationMs;
}

/**
 * Check if time t falls within a WEEKLY BYDAY recurrence.
 */
function isActiveInWeeklyByDay(
  event: ParsedEvent,
  eventDurationMs: number,
  t: Date,
): boolean {
  if (!event.dtstart) return false;
  if (event.until && t > event.until) return false;
  if (!event.byDay.includes(ICAL_DAYS[t.getUTCDay()])) return false;
  return inDailyWindow(event.dtstart, eventDurationMs, t);
}

/**
 * Check if time t falls within a DAILY recurrence.
 */
function isActiveInDailyRecurrence(
  event: ParsedEvent,
  eventDurationMs: number,
  t: Date,
): boolean {
  if (!event.dtstart) return false;
  if (event.until && t > event.until) return false;

  const params = event.rrule ? parseRruleParams(event.rrule) : {};
  const interval = parseInt(params.INTERVAL || "1") || 1;
  const daysDiff = Math.floor(
    (t.getTime() - event.dtstart.getTime()) / (86400 * 1000),
  );
  if (daysDiff < 0 || daysDiff % interval !== 0) return false;
  return inDailyWindow(event.dtstart, eventDurationMs, t);
}

/**
 * Get a description of the next occurrence for display.
 */
function getNextOccurrenceLabel(events: ParsedEvent[], now: Date): string {
  for (const event of events) {
    if (!event.dtstart || !event.freq) continue;

    if (event.freq === "WEEKLY" && event.byDay.length > 0) {
      const currentDayIdx = now.getUTCDay();
      const days = dayIndices(event.byDay);

      // Find next matching day
      for (let offset = 0; offset <= 7; offset++) {
        const checkDay = (currentDayIdx + offset) % 7;
        if (days.includes(checkDay)) {
          if (offset === 0) {
            // Today - check if the event hasn't started yet
            if (utcTimeOfDayMs(now) < utcTimeOfDayMs(event.dtstart)) {
              return `Next: Today at ${formatTime12h(event.dtstart)}`;
            }
            continue; // Past today's window, check next day
          }

          const dayName = offset === 1 ? "Tomorrow" : DAY_NAMES[checkDay];
          return `Next: ${dayName} at ${formatTime12h(event.dtstart)}`;
        }
      }
    }

    if (event.freq === "DAILY") {
      if (utcTimeOfDayMs(now) < utcTimeOfDayMs(event.dtstart)) {
        return `Next: Today at ${formatTime12h(event.dtstart)}`;
      }
      return `Next: Tomorrow at ${formatTime12h(event.dtstart)}`;
    }
  }

  return "Inactive";
}

/** Returns null if valid, or an error message. */
export function validateIcalData(data: string): string | null {
  if (!data || !data.trim()) {
    return "iCalendar data is required";
  }

  if (!data.includes("BEGIN:VCALENDAR")) {
    return "Missing BEGIN:VCALENDAR";
  }
  if (!data.includes("END:VCALENDAR")) {
    return "Missing END:VCALENDAR";
  }
  if (!data.includes("BEGIN:VEVENT")) {
    return "Must contain at least one VEVENT";
  }
  if (!data.includes("END:VEVENT")) {
    return "VEVENT is not properly closed";
  }
  if (!data.includes("DTSTART")) {
    return "VEVENT must have a DTSTART";
  }

  return null;
}

export type RecurrenceType = "none" | "daily" | "weekly";

export interface DateRangeBuilderConfig {
  /** YYYY-MM-DD. */
  startDate: string;
  /** YYYY-MM-DD; the UNTIL date for recurrences. */
  endDate: string;
  startHour: number;
  startMinute: number;
  endHour: number;
  endMinute: number;
  recurrence: RecurrenceType;
  /** Weekly recurrence days, Sun=0 through Sat=6. */
  weeklyDays: boolean[];
}

/** Returns null if valid, or an error message. */
export function validateDateRangeConfig(
  config: DateRangeBuilderConfig,
): string | null {
  if (!config.startDate) {
    return "Start date is required";
  }
  if (!config.endDate) {
    return "End date is required";
  }

  const start = new Date(config.startDate);
  const end = new Date(config.endDate);

  if (isNaN(start.getTime())) {
    return "Invalid start date";
  }
  if (isNaN(end.getTime())) {
    return "Invalid end date";
  }
  if (end < start) {
    return "End date must be on or after start date";
  }

  const startMinutes = config.startHour * 60 + config.startMinute;
  const endMinutes = config.endHour * 60 + config.endMinute;
  if (endMinutes <= startMinutes) {
    return "End time must be after start time";
  }

  if (config.recurrence === "weekly" && !config.weeklyDays.some(Boolean)) {
    return "Select at least one day for weekly recurrence";
  }

  return null;
}

/** Empty string when the configuration is invalid. */
export function buildDateRangeIcal(config: DateRangeBuilderConfig): string {
  const validationError = validateDateRangeConfig(config);
  if (validationError) return "";

  const startDay = config.startDate.replaceAll("-", "");
  const endDay = config.endDate.replaceAll("-", "");
  const startTime = `${pad2(config.startHour)}${pad2(config.startMinute)}00`;
  const endTime = `${pad2(config.endHour)}${pad2(config.endMinute)}00`;
  const until = `${endDay}T235959`;
  const dtstart = `${startDay}T${startTime}`;

  if (config.recurrence === "none") {
    // A single event spanning from startDate to endDate.
    return makeIcal("Custom Schedule", dtstart, `${endDay}T${endTime}`);
  }
  // Recurring: each occurrence lasts within a single day, until endDate.
  const rrule =
    config.recurrence === "daily"
      ? `FREQ=DAILY;UNTIL=${until}`
      : `FREQ=WEEKLY;BYDAY=${ICAL_DAYS.filter((_, i) => config.weeklyDays[i]).join(",")};UNTIL=${until}`;
  return makeIcal("Custom Schedule", dtstart, `${startDay}T${endTime}`, rrule);
}

const isoDate = (d: Date) => d.toISOString().slice(0, 10);

/** Builder config of the first VEVENT, to edit an existing calendar; null when unparseable. */
export function parseIcalToDateRangeConfig(
  icalData: string,
): DateRangeBuilderConfig | null {
  const [event] = icalData ? parseIcalEvents(icalData) : [];
  if (!event) return null;

  const { dtstart, dtend, freq } = event;
  const endAt = event.rrule ? event.until : dtend;
  const config: DateRangeBuilderConfig = {
    startDate: dtstart ? isoDate(dtstart) : "",
    endDate: endAt ? isoDate(endAt) : "",
    startHour: dtstart ? dtstart.getUTCHours() : 8,
    startMinute: dtstart ? dtstart.getUTCMinutes() : 0,
    endHour: dtend ? dtend.getUTCHours() : 17,
    endMinute: dtend ? dtend.getUTCMinutes() : 0,
    recurrence: freq === "DAILY" ? "daily" : freq === "WEEKLY" ? "weekly" : "none",
    weeklyDays:
      freq === "WEEKLY" && event.byDay.length > 0
        ? ICAL_DAYS.map((code) => event.byDay.includes(code))
        : [false, true, true, true, true, true, false],
  };

  // No end date: default to start date + 30 days.
  const start = parseLocalBoundary(config.startDate, "", "start");
  if (!config.endDate && start) {
    start.setDate(start.getDate() + 30);
    config.endDate = dateValue(start);
  }
  return config;
}

/** DTSTART date and the end date: UNTIL for recurrences (null when ongoing), else DTEND. */
export function getDateRange(icalData: string): {
  start: string | null;
  end: string | null;
} {
  const [event] = icalData ? parseIcalEvents(icalData) : [];
  const end = event?.rrule ? event.until : event?.dtend;
  return {
    start: event?.dtstart ? isoDate(event.dtstart) : null,
    end: end ? isoDate(end) : null,
  };
}

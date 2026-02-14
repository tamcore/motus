/**
 * iCalendar (RFC 5545) utility functions for the frontend.
 *
 * Provides template generation, schedule summary parsing, and
 * active/next-active status determination for calendar schedules.
 */

// ---------------------------------------------------------------------------
// Template types and definitions
// ---------------------------------------------------------------------------

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

/** Day abbreviations used in BYDAY rules (iCal format). */
const ICAL_DAYS = ["SU", "MO", "TU", "WE", "TH", "FR", "SA"] as const;

/** Human-readable day names corresponding to ICAL_DAYS indices. */
const DAY_NAMES = [
  "Sunday",
  "Monday",
  "Tuesday",
  "Wednesday",
  "Thursday",
  "Friday",
  "Saturday",
];

/** Short day names for compact display. */
const DAY_SHORT = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

/**
 * Generate a valid iCalendar document wrapping a single VEVENT with RRULE.
 */
function makeIcal(
  summary: string,
  dtstart: string,
  dtend: string,
  rrule: string,
): string {
  const lines = [
    "BEGIN:VCALENDAR",
    "VERSION:2.0",
    "PRODID:-//Motus//Calendar//EN",
    "BEGIN:VEVENT",
    `SUMMARY:${summary}`,
    `DTSTART:${dtstart}`,
    `DTEND:${dtend}`,
    `RRULE:${rrule}`,
    "END:VEVENT",
    "END:VCALENDAR",
  ];
  return lines.join("\r\n");
}

/**
 * Pre-defined calendar templates for common scheduling patterns.
 */
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

// ---------------------------------------------------------------------------
// Schedule summary parsing
// ---------------------------------------------------------------------------

interface ParsedEvent {
  summary: string;
  dtstart: Date | null;
  dtend: Date | null;
  rrule: string | null;
  byDay: string[];
  freq: string | null;
  until: Date | null;
}

/**
 * Parse a minimal subset of iCalendar data to extract human-readable info.
 * This is a lightweight client-side parser -- the backend performs full validation.
 */
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
          event.byDay = params.BYDAY.split(",").map((d) => d.trim());
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

/**
 * Parse an iCalendar date-time value from a property line like
 * "DTSTART:20240101T080000" or "DTSTART;TZID=America/New_York:20240101T080000"
 */
function parseIcalDateTime(line: string): Date | null {
  // Extract the value after the last colon
  const colonIdx = line.lastIndexOf(":");
  if (colonIdx === -1) return null;
  const val = line.slice(colonIdx + 1).trim();

  // Try date-time format: YYYYMMDDTHHMMSS
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

  // Try date-only format: YYYYMMDD
  const dateMatch = val.match(/^(\d{4})(\d{2})(\d{2})$/);
  if (dateMatch) {
    const [, y, m, d] = dateMatch;
    return new Date(Date.UTC(parseInt(y), parseInt(m) - 1, parseInt(d)));
  }

  return null;
}

/**
 * Parse RRULE parameters into a key-value map.
 */
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

/**
 * Format a time from a Date in HH:MM AM/PM format.
 */
function formatTime12h(date: Date): string {
  const hours = date.getUTCHours();
  const minutes = date.getUTCMinutes();
  const ampm = hours >= 12 ? "PM" : "AM";
  const h = hours % 12 || 12;
  const m = minutes.toString().padStart(2, "0");
  return `${h}:${m} ${ampm}`;
}

/**
 * Convert iCal day abbreviations to human-readable day list.
 */
function formatDayList(days: string[]): string {
  // Strip numeric prefixes (e.g., "1MO" -> "MO")
  const cleaned = days.map((d) => d.replace(/^\d+/, ""));

  const indices = cleaned
    .map((d) => ICAL_DAYS.indexOf(d as (typeof ICAL_DAYS)[number]))
    .filter((i) => i !== -1)
    .sort((a, b) => a - b);

  if (indices.length === 0) return "";

  // Check for common patterns
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

/**
 * Generate a human-readable summary of iCalendar schedule data.
 */
export function getScheduleSummary(icalData: string): string {
  if (!icalData || !icalData.trim()) return "No schedule defined";

  try {
    const events = parseIcalEvents(icalData);
    if (events.length === 0) return "No events defined";

    const summaries = events.map((event) => {
      const parts: string[] = [];

      // Frequency / days
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

      // Time range
      if (event.dtstart && event.dtend) {
        const startTime = formatTime12h(event.dtstart);
        const endTime = formatTime12h(event.dtend);

        // Check if it spans nearly 24 hours (all day)
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

// ---------------------------------------------------------------------------
// Active status checking (client-side approximation)
// ---------------------------------------------------------------------------

/**
 * Check whether the current time falls within any VEVENT in the iCalendar data.
 * This is a client-side approximation; the authoritative check is via the
 * backend's GET /api/calendars/{id}/check endpoint.
 */
export function isActiveNow(icalData: string): boolean {
  if (!icalData || !icalData.trim()) return false;

  try {
    const events = parseIcalEvents(icalData);
    const now = new Date();

    for (const event of events) {
      if (isEventActiveAt(event, now)) {
        return true;
      }
    }

    return false;
  } catch {
    return false;
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

/**
 * Check if time t falls within a WEEKLY BYDAY recurrence.
 */
function isActiveInWeeklyByDay(
  event: ParsedEvent,
  eventDurationMs: number,
  t: Date,
): boolean {
  if (!event.dtstart) return false;

  // Check if past the recurrence end date (UNTIL boundary)
  if (event.until && t > event.until) return false;

  const currentDayIdx = t.getUTCDay();
  const currentDayIcal = ICAL_DAYS[currentDayIdx];

  const cleaned = event.byDay.map((d) => d.replace(/^\d+/, ""));
  if (!cleaned.includes(currentDayIcal)) return false;

  // Check if current time-of-day is within the event window
  const startHour = event.dtstart.getUTCHours();
  const startMin = event.dtstart.getUTCMinutes();
  const startSec = event.dtstart.getUTCSeconds();
  const startMs = (startHour * 3600 + startMin * 60 + startSec) * 1000;

  const nowHour = t.getUTCHours();
  const nowMin = t.getUTCMinutes();
  const nowSec = t.getUTCSeconds();
  const nowMs = (nowHour * 3600 + nowMin * 60 + nowSec) * 1000;

  return nowMs >= startMs && nowMs < startMs + eventDurationMs;
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

  // Check if past the recurrence end date (UNTIL boundary)
  if (event.until && t > event.until) return false;

  const params = event.rrule ? parseRruleParams(event.rrule) : {};
  const interval = parseInt(params.INTERVAL || "1") || 1;

  if (interval === 1) {
    // Every day - just check time of day
    const startHour = event.dtstart.getUTCHours();
    const startMin = event.dtstart.getUTCMinutes();
    const startSec = event.dtstart.getUTCSeconds();
    const startMs = (startHour * 3600 + startMin * 60 + startSec) * 1000;

    const nowHour = t.getUTCHours();
    const nowMin = t.getUTCMinutes();
    const nowSec = t.getUTCSeconds();
    const nowMs = (nowHour * 3600 + nowMin * 60 + nowSec) * 1000;

    return nowMs >= startMs && nowMs < startMs + eventDurationMs;
  }

  // For intervals > 1, check if the day matches
  const daysDiff = Math.floor(
    (t.getTime() - event.dtstart.getTime()) / (86400 * 1000),
  );
  if (daysDiff < 0 || daysDiff % interval !== 0) return false;

  const startHour = event.dtstart.getUTCHours();
  const startMin = event.dtstart.getUTCMinutes();
  const startSec = event.dtstart.getUTCSeconds();
  const startMs = (startHour * 3600 + startMin * 60 + startSec) * 1000;

  const nowHour = t.getUTCHours();
  const nowMin = t.getUTCMinutes();
  const nowSec = t.getUTCSeconds();
  const nowMs = (nowHour * 3600 + nowMin * 60 + nowSec) * 1000;

  return nowMs >= startMs && nowMs < startMs + eventDurationMs;
}

/**
 * Get a description of the next occurrence for display.
 */
function getNextOccurrenceLabel(events: ParsedEvent[], now: Date): string {
  for (const event of events) {
    if (!event.dtstart || !event.freq) continue;

    if (event.freq === "WEEKLY" && event.byDay.length > 0) {
      const currentDayIdx = now.getUTCDay();
      const cleaned = event.byDay.map((d) => d.replace(/^\d+/, ""));
      const dayIndices = cleaned
        .map((d) => ICAL_DAYS.indexOf(d as (typeof ICAL_DAYS)[number]))
        .filter((i) => i !== -1)
        .sort((a, b) => a - b);

      // Find next matching day
      for (let offset = 0; offset <= 7; offset++) {
        const checkDay = (currentDayIdx + offset) % 7;
        if (dayIndices.includes(checkDay)) {
          if (offset === 0) {
            // Today - check if the event hasn't started yet
            const startHour = event.dtstart.getUTCHours();
            const startMin = event.dtstart.getUTCMinutes();
            const nowTimeMs =
              (now.getUTCHours() * 3600 +
                now.getUTCMinutes() * 60 +
                now.getUTCSeconds()) *
              1000;
            const startTimeMs = (startHour * 3600 + startMin * 60) * 1000;

            if (nowTimeMs < startTimeMs) {
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
      const startHour = event.dtstart.getUTCHours();
      const startMin = event.dtstart.getUTCMinutes();
      const nowTimeMs =
        (now.getUTCHours() * 3600 +
          now.getUTCMinutes() * 60 +
          now.getUTCSeconds()) *
        1000;
      const startTimeMs = (startHour * 3600 + startMin * 60) * 1000;

      if (nowTimeMs < startTimeMs) {
        return `Next: Today at ${formatTime12h(event.dtstart)}`;
      }
      return `Next: Tomorrow at ${formatTime12h(event.dtstart)}`;
    }
  }

  return "Inactive";
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

/**
 * Client-side iCalendar data validation.
 * Returns null if valid, or an error message string.
 */
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

// ---------------------------------------------------------------------------
// Visual Builder types and iCal generation
// ---------------------------------------------------------------------------

/** Recurrence type for the visual builder. */
export type RecurrenceType = "none" | "daily" | "weekly";

/** Configuration for building iCal data from the visual date range builder. */
export interface DateRangeBuilderConfig {
  /** Start date in YYYY-MM-DD format. */
  startDate: string;
  /** End date in YYYY-MM-DD format (used as UNTIL for recurrence). */
  endDate: string;
  /** Start time hour (0-23). */
  startHour: number;
  /** Start time minute (0-59). */
  startMinute: number;
  /** End time hour (0-23). */
  endHour: number;
  /** End time minute (0-59). */
  endMinute: number;
  /** Recurrence type. */
  recurrence: RecurrenceType;
  /** For weekly recurrence: which days are active (Sun=0 through Sat=6). */
  weeklyDays: boolean[];
}

/**
 * Validate a date range builder configuration.
 * Returns null if valid, or an error message string.
 */
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

  // Validate individual field ranges before composite checks
  if (config.startHour < 0 || config.startHour > 23) {
    return "Start hour must be between 0 and 23";
  }
  if (config.endHour < 0 || config.endHour > 23) {
    return "End hour must be between 0 and 23";
  }
  if (config.startMinute < 0 || config.startMinute > 59) {
    return "Start minute must be between 0 and 59";
  }
  if (config.endMinute < 0 || config.endMinute > 59) {
    return "End minute must be between 0 and 59";
  }

  // Validate time range
  const startMinutes = config.startHour * 60 + config.startMinute;
  const endMinutes = config.endHour * 60 + config.endMinute;
  if (endMinutes <= startMinutes) {
    return "End time must be after start time";
  }

  // For weekly recurrence, at least one day must be selected
  if (config.recurrence === "weekly") {
    if (!config.weeklyDays.some(Boolean)) {
      return "Select at least one day for weekly recurrence";
    }
  }

  return null;
}

/**
 * Build iCalendar data from a date range builder configuration.
 * Returns a valid iCalendar string or empty string if configuration is incomplete.
 */
export function buildDateRangeIcal(config: DateRangeBuilderConfig): string {
  const validationError = validateDateRangeConfig(config);
  if (validationError) return "";

  const startParts = config.startDate.split("-");
  const endParts = config.endDate.split("-");

  const sh = String(config.startHour).padStart(2, "0");
  const sm = String(config.startMinute).padStart(2, "0");
  const eh = String(config.endHour).padStart(2, "0");
  const em = String(config.endMinute).padStart(2, "0");

  const dtstart = `${startParts.join("")}T${sh}${sm}00`;
  // DTEND uses the same date as DTSTART (event duration within a single day)
  const dtend = `${startParts.join("")}T${eh}${em}00`;

  // UNTIL date is the end date at 23:59:59
  const until = `${endParts.join("")}T235959`;

  const lines = [
    "BEGIN:VCALENDAR",
    "VERSION:2.0",
    "PRODID:-//Motus//Calendar//EN",
    "BEGIN:VEVENT",
    "SUMMARY:Custom Schedule",
    `DTSTART:${dtstart}`,
    `DTEND:${dtend}`,
  ];

  if (config.recurrence === "none") {
    // No recurrence - single event spanning from startDate to endDate
    // For "none" recurrence, DTEND uses the end date
    lines[lines.length - 1] = `DTEND:${endParts.join("")}T${eh}${em}00`;
  } else if (config.recurrence === "daily") {
    lines.push(`RRULE:FREQ=DAILY;UNTIL=${until}`);
  } else if (config.recurrence === "weekly") {
    const activeDays = config.weeklyDays
      .map((active, i) => (active ? ICAL_DAYS[i] : null))
      .filter(Boolean);
    lines.push(
      `RRULE:FREQ=WEEKLY;BYDAY=${activeDays.join(",")};UNTIL=${until}`,
    );
  }

  lines.push("END:VEVENT", "END:VCALENDAR");
  return lines.join("\r\n");
}

/**
 * Parse iCalendar data into a DateRangeBuilderConfig.
 * Used to populate the visual builder when editing an existing calendar.
 * Returns null if the data cannot be parsed into the builder format.
 */
export function parseIcalToDateRangeConfig(
  icalData: string,
): DateRangeBuilderConfig | null {
  if (!icalData) return null;

  try {
    const config: DateRangeBuilderConfig = {
      startDate: "",
      endDate: "",
      startHour: 8,
      startMinute: 0,
      endHour: 17,
      endMinute: 0,
      recurrence: "none",
      weeklyDays: [false, true, true, true, true, true, false],
    };

    // Extract DTSTART date and time
    const startMatch = icalData.match(
      /DTSTART[^:]*:(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})/,
    );
    if (startMatch) {
      config.startDate = `${startMatch[1]}-${startMatch[2]}-${startMatch[3]}`;
      config.startHour = parseInt(startMatch[4]);
      config.startMinute = parseInt(startMatch[5]);
    }

    // Extract DTEND date and time
    const endMatch = icalData.match(
      /DTEND[^:]*:(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})/,
    );
    if (endMatch) {
      config.endHour = parseInt(endMatch[4]);
      config.endMinute = parseInt(endMatch[5]);
    }

    // Extract RRULE
    const rruleMatch = icalData.match(/RRULE:([^\r\n]+)/);
    if (rruleMatch) {
      const rrule = rruleMatch[1];
      const params = parseRruleParams(rrule);

      if (params.FREQ === "DAILY") {
        config.recurrence = "daily";
      } else if (params.FREQ === "WEEKLY") {
        config.recurrence = "weekly";
        if (params.BYDAY) {
          const days = params.BYDAY.split(",").map((d) => d.trim());
          config.weeklyDays = ICAL_DAYS.map((code) => days.includes(code));
        }
      }

      // Extract UNTIL for end date
      if (params.UNTIL) {
        const untilMatch = params.UNTIL.match(/(\d{4})(\d{2})(\d{2})/);
        if (untilMatch) {
          config.endDate = `${untilMatch[1]}-${untilMatch[2]}-${untilMatch[3]}`;
        }
      }
    }

    // If no RRULE (single event), use DTEND date as end date
    if (!rruleMatch && endMatch) {
      config.endDate = `${endMatch[1]}-${endMatch[2]}-${endMatch[3]}`;
    }

    // If no end date parsed, default to start date + 30 days
    if (!config.endDate && config.startDate) {
      const start = new Date(config.startDate);
      start.setDate(start.getDate() + 30);
      config.endDate = start.toISOString().split("T")[0];
    }

    return config;
  } catch {
    return null;
  }
}

/**
 * Extract start and end dates from iCalendar data.
 * Returns the DTSTART and the end date (from RRULE:UNTIL if present, otherwise DTEND for single events).
 * For recurring events without UNTIL, returns null for end (indicating ongoing).
 */
export function getDateRange(icalData: string): {
  start: string | null;
  end: string | null;
} {
  if (!icalData) return { start: null, end: null };

  const startMatch = icalData.match(/DTSTART[^:]*:(\d{8}T?\d{0,6})/);

  const formatDate = (dateStr: string) => {
    // Parse YYYYMMDD or YYYYMMDDTHHMMSS
    const year = dateStr.substring(0, 4);
    const month = dateStr.substring(4, 6);
    const day = dateStr.substring(6, 8);
    return `${year}-${month}-${day}`;
  };

  let end: string | null = null;

  // Check if this is a recurring event
  const rruleMatch = icalData.match(/RRULE:([^\r\n]+)/);

  if (rruleMatch) {
    // For recurring events, only use UNTIL parameter for end date
    const params = parseRruleParams(rruleMatch[1]);
    if (params.UNTIL) {
      const untilMatch = params.UNTIL.match(/(\d{8})/);
      if (untilMatch) {
        end = formatDate(untilMatch[0]);
      }
    }
    // If no UNTIL, end is null (ongoing recurrence)
  } else {
    // For single events (no RRULE), use DTEND
    const endMatch = icalData.match(/DTEND[^:]*:(\d{8}T?\d{0,6})/);
    if (endMatch) {
      end = formatDate(endMatch[1]);
    }
  }

  return {
    start: startMatch ? formatDate(startMatch[1]) : null,
    end,
  };
}

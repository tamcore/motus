import { describe, it, expect } from "vitest";
import { getDateRange } from "./ical";

/**
 * Helper: build a minimal iCalendar string with a VEVENT.
 * Times are in UTC (no TZID).
 */
function makeIcal(opts: {
  dtstart: string;
  dtend: string;
  rrule?: string;
}): string {
  const lines = [
    "BEGIN:VCALENDAR",
    "VERSION:2.0",
    "PRODID:-//Motus//Test//EN",
    "BEGIN:VEVENT",
    "SUMMARY:Test Event",
    `DTSTART:${opts.dtstart}`,
    `DTEND:${opts.dtend}`,
  ];
  if (opts.rrule) {
    lines.push(`RRULE:${opts.rrule}`);
  }
  lines.push("END:VEVENT", "END:VCALENDAR");
  return lines.join("\r\n");
}

describe("getDateRange - extracting start and end dates", () => {
  it("should extract start date from DTSTART", () => {
    const ical = makeIcal({
      dtstart: "20250101T080000",
      dtend: "20250101T170000",
      rrule: "FREQ=DAILY",
    });

    const result = getDateRange(ical);
    expect(result.start).toBe("2025-01-01");
  });

  it("should extract end date from RRULE:UNTIL for recurring events", () => {
    const ical = makeIcal({
      dtstart: "20250101T080000",
      dtend: "20250101T170000",
      rrule: "FREQ=DAILY;UNTIL=20251231T235959",
    });

    const result = getDateRange(ical);
    expect(result.start).toBe("2025-01-01");
    expect(result.end).toBe("2025-12-31");
  });

  it("should extract end date from RRULE:UNTIL for normalized Traccar calendars", () => {
    // Simulates a normalized Traccar calendar where DTEND is DTSTART+24h
    // and the actual series end is in UNTIL
    const ical = makeIcal({
      dtstart: "20251105T200000",
      dtend: "20251106T200000", // +24h from start
      rrule: "FREQ=DAILY;UNTIL=20251110T200000",
    });

    const result = getDateRange(ical);
    expect(result.start).toBe("2025-11-05");
    expect(result.end).toBe("2025-11-10"); // From UNTIL, not DTEND
  });

  it("should fall back to DTEND when no RRULE exists (single event)", () => {
    const ical = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Motus//Test//EN
BEGIN:VEVENT
SUMMARY:Single Event
DTSTART:20250115T100000
DTEND:20250120T180000
END:VEVENT
END:VCALENDAR`;

    const result = getDateRange(ical);
    expect(result.start).toBe("2025-01-15");
    expect(result.end).toBe("2025-01-20");
  });

  it("should return null end date for ongoing recurrence (no UNTIL)", () => {
    const ical = makeIcal({
      dtstart: "20250101T080000",
      dtend: "20250101T170000",
      rrule: "FREQ=DAILY", // No UNTIL - runs forever
    });

    const result = getDateRange(ical);
    expect(result.start).toBe("2025-01-01");
    expect(result.end).toBeNull();
  });

  it("should return null for both dates when icalData is empty", () => {
    const result = getDateRange("");
    expect(result.start).toBeNull();
    expect(result.end).toBeNull();
  });
});

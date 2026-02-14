import { describe, it, expect, vi, afterEach } from "vitest";
import { isActiveNow, getActiveStatus, getDateRange } from "./ical";

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

describe("isActiveNow - recurrence boundary checks", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("should return false for expired daily recurrence (UNTIL in the past)", () => {
    // Daily recurrence from 00:00-23:59, UNTIL Nov 10, 2025
    const ical = makeIcal({
      dtstart: "20250101T000000",
      dtend: "20250101T235959",
      rrule: "FREQ=DAILY;UNTIL=20251110T235959",
    });

    // Fake "now" = Feb 17, 2026, 12:00 UTC (well past UNTIL)
    vi.useFakeTimers();
    vi.setSystemTime(new Date(Date.UTC(2026, 1, 17, 12, 0, 0)));

    expect(isActiveNow(ical)).toBe(false);
  });

  it("should return false for expired weekly recurrence (UNTIL in the past)", () => {
    // Weekly on Mon,Wed,Fri from 09:00-17:00, UNTIL Dec 31, 2025
    const ical = makeIcal({
      dtstart: "20250101T090000",
      dtend: "20250101T170000",
      rrule: "FREQ=WEEKLY;BYDAY=MO,WE,FR;UNTIL=20251231T235959",
    });

    // Fake "now" = Wednesday Feb 18, 2026 at 12:00 UTC (a Wednesday, but past UNTIL)
    vi.useFakeTimers();
    vi.setSystemTime(new Date(Date.UTC(2026, 1, 18, 12, 0, 0)));

    expect(isActiveNow(ical)).toBe(false);
  });

  it("should return true for active weekly recurrence (UNTIL in the future)", () => {
    // Weekly on Wednesday from 09:00-17:00, UNTIL Dec 31, 2026
    const ical = makeIcal({
      dtstart: "20250101T090000",
      dtend: "20250101T170000",
      rrule: "FREQ=WEEKLY;BYDAY=WE;UNTIL=20261231T235959",
    });

    // Fake "now" = Wednesday Feb 18, 2026 at 12:00 UTC (within range)
    vi.useFakeTimers();
    vi.setSystemTime(new Date(Date.UTC(2026, 1, 18, 12, 0, 0)));

    expect(isActiveNow(ical)).toBe(true);
  });

  it("should return true for 24/7 infinite daily recurrence (no UNTIL)", () => {
    // Daily from 00:00-23:59, no UNTIL -- runs forever
    const ical = makeIcal({
      dtstart: "20240101T000000",
      dtend: "20240101T235959",
      rrule: "FREQ=DAILY",
    });

    // Fake "now" = Feb 17, 2026, 14:30 UTC
    vi.useFakeTimers();
    vi.setSystemTime(new Date(Date.UTC(2026, 1, 17, 14, 30, 0)));

    expect(isActiveNow(ical)).toBe(true);
  });

  it("should return true for active daily recurrence with UNTIL in the future", () => {
    // Daily 08:00-18:00, UNTIL Mar 1, 2026
    const ical = makeIcal({
      dtstart: "20260101T080000",
      dtend: "20260101T180000",
      rrule: "FREQ=DAILY;UNTIL=20260301T235959",
    });

    // Fake "now" = Feb 17, 2026, 12:00 UTC (within time window and before UNTIL)
    vi.useFakeTimers();
    vi.setSystemTime(new Date(Date.UTC(2026, 1, 17, 12, 0, 0)));

    expect(isActiveNow(ical)).toBe(true);
  });
});

describe("getActiveStatus - recurrence boundary checks", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("should return inactive label for expired daily recurrence", () => {
    const ical = makeIcal({
      dtstart: "20250101T000000",
      dtend: "20250101T235959",
      rrule: "FREQ=DAILY;UNTIL=20251110T235959",
    });

    vi.useFakeTimers();
    vi.setSystemTime(new Date(Date.UTC(2026, 1, 17, 12, 0, 0)));

    const result = getActiveStatus(ical);
    expect(result.active).toBe(false);
  });

  it('should return "Active now" for 24/7 infinite recurrence', () => {
    const ical = makeIcal({
      dtstart: "20240101T000000",
      dtend: "20240101T235959",
      rrule: "FREQ=DAILY",
    });

    vi.useFakeTimers();
    vi.setSystemTime(new Date(Date.UTC(2026, 1, 17, 14, 30, 0)));

    const result = getActiveStatus(ical);
    expect(result.active).toBe(true);
    expect(result.label).toBe("Active now");
  });

  it("should return inactive for expired weekly recurrence", () => {
    const ical = makeIcal({
      dtstart: "20250101T090000",
      dtend: "20250101T170000",
      rrule: "FREQ=WEEKLY;BYDAY=MO,WE,FR;UNTIL=20251231T235959",
    });

    // Wednesday, within time window, but past UNTIL
    vi.useFakeTimers();
    vi.setSystemTime(new Date(Date.UTC(2026, 1, 18, 12, 0, 0)));

    const result = getActiveStatus(ical);
    expect(result.active).toBe(false);
  });
});

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

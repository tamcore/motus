import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Mock $app/environment before any imports that depend on it
vi.mock("$app/environment", () => ({
  browser: true,
}));

// Mock stores that depend on $app/environment
vi.mock("$lib/stores/settings", () => ({
  settings: {
    subscribe: vi.fn((fn: (value: unknown) => void) => {
      fn({
        dateFormat: "iso",
        speedUnit: "kmh",
        distanceUnit: "km",
        mapCenter: [0, 0],
        mapZoom: 13,
        mapOverlay: "osm_dark",
        mapLocationSet: false,
      });
      return () => {};
    }),
  },
}));

// Mock the API client
const mockGetCalendars = vi.fn();
const mockCreateCalendar = vi.fn();
const mockUpdateCalendar = vi.fn();
const mockDeleteCalendar = vi.fn();
const mockCheckCalendar = vi.fn();

vi.mock("$lib/api/client", () => ({
  api: {
    getCalendars: mockGetCalendars,
    createCalendar: mockCreateCalendar,
    updateCalendar: mockUpdateCalendar,
    deleteCalendar: mockDeleteCalendar,
    checkCalendar: mockCheckCalendar,
  },
  APIError: class APIError extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
    }
  },
}));

import type {
  Calendar,
  CalendarCheckResponse,
  CreateCalendarPayload,
  UpdateCalendarPayload,
} from "$lib/types/api";

import {
  CALENDAR_TEMPLATES,
  getScheduleSummary,
  getActiveStatus,
  isActiveNow,
  validateIcalData,
  validateDateRangeConfig,
  buildDateRangeIcal,
  parseIcalToDateRangeConfig,
  type TemplateId,
  type RecurrenceType,
  type DateRangeBuilderConfig,
} from "$lib/utils/ical";

// ---------------------------------------------------------------------------
// Test Helpers
// ---------------------------------------------------------------------------

function createMockCalendar(overrides: Partial<Calendar> = {}): Calendar {
  return {
    id: 1,
    userId: 1,
    name: "Test Calendar",
    data: CALENDAR_TEMPLATES[0].data, // Business Hours
    createdAt: "2026-02-15T10:00:00Z",
    updatedAt: "2026-02-15T10:00:00Z",
    ...overrides,
  };
}

function makeIcal(
  summary: string,
  dtstart: string,
  dtend: string,
  rrule: string,
): string {
  return [
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
  ].join("\r\n");
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("Calendar Management", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  // -------------------------------------------------------------------------
  // Calendar Templates
  // -------------------------------------------------------------------------

  describe("Calendar Templates", () => {
    it("should define all expected template IDs", () => {
      const ids = CALENDAR_TEMPLATES.map((t) => t.id);
      expect(ids).toContain("business_hours");
      expect(ids).toContain("weekends");
      expect(ids).toContain("weeknights");
      expect(ids).toContain("always");
      expect(ids).toContain("custom");
    });

    it("should have labels and descriptions for all templates", () => {
      for (const t of CALENDAR_TEMPLATES) {
        expect(t.label).toBeTruthy();
        expect(t.description).toBeTruthy();
      }
    });

    it("should have valid iCal data for non-custom templates", () => {
      for (const t of CALENDAR_TEMPLATES) {
        if (t.id === "custom") {
          expect(t.data).toBe("");
          continue;
        }
        expect(t.data).toContain("BEGIN:VCALENDAR");
        expect(t.data).toContain("END:VCALENDAR");
        expect(t.data).toContain("BEGIN:VEVENT");
        expect(t.data).toContain("END:VEVENT");
        expect(t.data).toContain("DTSTART:");
        expect(t.data).toContain("RRULE:");
      }
    });

    it("business hours template should have Mon-Fri BYDAY", () => {
      const biz = CALENDAR_TEMPLATES.find((t) => t.id === "business_hours");
      expect(biz).toBeDefined();
      expect(biz!.data).toContain("BYDAY=MO,TU,WE,TH,FR");
      expect(biz!.data).toContain("T080000");
      expect(biz!.data).toContain("T170000");
    });

    it("weekends template should have Sat-Sun BYDAY", () => {
      const weekends = CALENDAR_TEMPLATES.find((t) => t.id === "weekends");
      expect(weekends).toBeDefined();
      expect(weekends!.data).toContain("BYDAY=SA,SU");
    });

    it("weeknights template should have 6pm-8am window", () => {
      const nights = CALENDAR_TEMPLATES.find((t) => t.id === "weeknights");
      expect(nights).toBeDefined();
      expect(nights!.data).toContain("T180000");
      expect(nights!.data).toContain("T080000");
    });

    it("always template should have FREQ=DAILY", () => {
      const always = CALENDAR_TEMPLATES.find((t) => t.id === "always");
      expect(always).toBeDefined();
      expect(always!.data).toContain("FREQ=DAILY");
    });
  });

  // -------------------------------------------------------------------------
  // Schedule Summary
  // -------------------------------------------------------------------------

  describe("getScheduleSummary", () => {
    it("should return 'No schedule defined' for empty data", () => {
      expect(getScheduleSummary("")).toBe("No schedule defined");
      expect(getScheduleSummary("  ")).toBe("No schedule defined");
    });

    it("should return summary for business hours template", () => {
      const biz = CALENDAR_TEMPLATES.find((t) => t.id === "business_hours");
      const summary = getScheduleSummary(biz!.data);
      expect(summary).toContain("Mon-Fri");
      expect(summary).toContain("8:00 AM");
      expect(summary).toContain("5:00 PM");
    });

    it("should return summary for weekends template", () => {
      const weekends = CALENDAR_TEMPLATES.find((t) => t.id === "weekends");
      const summary = getScheduleSummary(weekends!.data);
      expect(summary).toContain("Sat-Sun");
    });

    it("should return summary for daily schedule", () => {
      const always = CALENDAR_TEMPLATES.find((t) => t.id === "always");
      const summary = getScheduleSummary(always!.data);
      expect(summary).toContain("Daily");
    });

    it("should handle all-day events", () => {
      const data = makeIcal(
        "All Day",
        "20240101T000000",
        "20240101T235959",
        "FREQ=DAILY",
      );
      const summary = getScheduleSummary(data);
      expect(summary).toContain("all day");
    });

    it("should handle custom schedule with specific days", () => {
      const data = makeIcal(
        "Custom",
        "20240101T090000",
        "20240101T120000",
        "FREQ=WEEKLY;BYDAY=MO,WE,FR",
      );
      const summary = getScheduleSummary(data);
      expect(summary).toContain("9:00 AM");
      expect(summary).toContain("12:00 PM");
    });

    it("should handle invalid iCal data gracefully", () => {
      const summary = getScheduleSummary("not valid ical data");
      expect(summary).toBe("No events defined");
    });
  });

  // -------------------------------------------------------------------------
  // Validation
  // -------------------------------------------------------------------------

  describe("validateIcalData", () => {
    it("should return error for empty data", () => {
      expect(validateIcalData("")).toBe("iCalendar data is required");
      expect(validateIcalData("  ")).toBe("iCalendar data is required");
    });

    it("should return error for missing VCALENDAR", () => {
      expect(validateIcalData("BEGIN:VEVENT\nEND:VEVENT")).toBe(
        "Missing BEGIN:VCALENDAR",
      );
    });

    it("should return error for missing END:VCALENDAR", () => {
      expect(
        validateIcalData(
          "BEGIN:VCALENDAR\nBEGIN:VEVENT\nDTSTART:20240101\nEND:VEVENT",
        ),
      ).toBe("Missing END:VCALENDAR");
    });

    it("should return error for missing VEVENT", () => {
      expect(
        validateIcalData("BEGIN:VCALENDAR\nVERSION:2.0\nEND:VCALENDAR"),
      ).toBe("Must contain at least one VEVENT");
    });

    it("should return error for unclosed VEVENT", () => {
      expect(
        validateIcalData(
          "BEGIN:VCALENDAR\nBEGIN:VEVENT\nDTSTART:20240101\nEND:VCALENDAR",
        ),
      ).toBe("VEVENT is not properly closed");
    });

    it("should return error for missing DTSTART", () => {
      expect(
        validateIcalData(
          "BEGIN:VCALENDAR\nBEGIN:VEVENT\nSUMMARY:Test\nEND:VEVENT\nEND:VCALENDAR",
        ),
      ).toBe("VEVENT must have a DTSTART");
    });

    it("should return null for valid iCal data", () => {
      const data = makeIcal(
        "Test",
        "20240101T080000",
        "20240101T170000",
        "FREQ=DAILY",
      );
      expect(validateIcalData(data)).toBeNull();
    });

    it("should return null for all predefined templates", () => {
      for (const t of CALENDAR_TEMPLATES) {
        if (t.id === "custom") continue;
        expect(validateIcalData(t.data)).toBeNull();
      }
    });
  });

  // -------------------------------------------------------------------------
  // Active Status
  // -------------------------------------------------------------------------

  describe("getActiveStatus", () => {
    it("should return 'No schedule' for empty data", () => {
      const result = getActiveStatus("");
      expect(result.active).toBe(false);
      expect(result.label).toBe("No schedule");
    });

    it("should return an object with active boolean and label string", () => {
      const data = makeIcal(
        "Test",
        "20240101T000000",
        "20240101T235959",
        "FREQ=DAILY",
      );
      const result = getActiveStatus(data);
      expect(typeof result.active).toBe("boolean");
      expect(typeof result.label).toBe("string");
    });

    it("should return active=true for 24/7 schedule", () => {
      const always = CALENDAR_TEMPLATES.find((t) => t.id === "always");
      const result = getActiveStatus(always!.data);
      // 24/7 schedule: 00:00 to 23:59:59 daily -- always active
      expect(result.active).toBe(true);
      expect(result.label).toBe("Active now");
    });
  });

  describe("isActiveNow", () => {
    it("should return false for empty data", () => {
      expect(isActiveNow("")).toBe(false);
    });

    it("should return true for 24/7 schedule", () => {
      const always = CALENDAR_TEMPLATES.find((t) => t.id === "always");
      expect(isActiveNow(always!.data)).toBe(true);
    });

    it("should handle invalid data gracefully", () => {
      expect(isActiveNow("not valid")).toBe(false);
    });
  });

  // -------------------------------------------------------------------------
  // API Client Methods
  // -------------------------------------------------------------------------

  describe("API Client - Calendar CRUD", () => {
    describe("getCalendars", () => {
      it("should return an array of calendars", async () => {
        const calendars = [
          createMockCalendar({ id: 1, name: "Business Hours" }),
          createMockCalendar({ id: 2, name: "Weekends" }),
        ];
        mockGetCalendars.mockResolvedValue(calendars);

        const { api } = await import("$lib/api/client");
        const result = await api.getCalendars();

        expect(mockGetCalendars).toHaveBeenCalledOnce();
        expect(result).toHaveLength(2);
        expect(result[0].name).toBe("Business Hours");
        expect(result[1].name).toBe("Weekends");
      });

      it("should return empty array when no calendars exist", async () => {
        mockGetCalendars.mockResolvedValue([]);

        const { api } = await import("$lib/api/client");
        const result = await api.getCalendars();

        expect(result).toEqual([]);
      });
    });

    describe("createCalendar", () => {
      it("should create a calendar with name and data", async () => {
        const payload: CreateCalendarPayload = {
          name: "My Schedule",
          data: CALENDAR_TEMPLATES[0].data,
        };

        const created = createMockCalendar({
          id: 5,
          name: "My Schedule",
          data: payload.data,
        });
        mockCreateCalendar.mockResolvedValue(created);

        const { api } = await import("$lib/api/client");
        const result = await api.createCalendar(payload);

        expect(mockCreateCalendar).toHaveBeenCalledWith(payload);
        expect(result.id).toBe(5);
        expect(result.name).toBe("My Schedule");
      });

      it("should propagate errors from the API", async () => {
        mockCreateCalendar.mockRejectedValue(new Error("name is required"));

        const { api } = await import("$lib/api/client");
        await expect(
          api.createCalendar({ name: "", data: "" }),
        ).rejects.toThrow("name is required");
      });
    });

    describe("updateCalendar", () => {
      it("should update calendar name and data", async () => {
        const payload: UpdateCalendarPayload = {
          name: "Updated Name",
          data: CALENDAR_TEMPLATES[1].data,
        };

        const updated = createMockCalendar({
          id: 1,
          name: "Updated Name",
          data: payload.data!,
          updatedAt: "2026-02-16T10:00:00Z",
        });
        mockUpdateCalendar.mockResolvedValue(updated);

        const { api } = await import("$lib/api/client");
        const result = await api.updateCalendar(1, payload);

        expect(mockUpdateCalendar).toHaveBeenCalledWith(1, payload);
        expect(result.name).toBe("Updated Name");
      });

      it("should allow partial updates", async () => {
        const payload: UpdateCalendarPayload = { name: "Just Name" };
        const updated = createMockCalendar({ name: "Just Name" });
        mockUpdateCalendar.mockResolvedValue(updated);

        const { api } = await import("$lib/api/client");
        const result = await api.updateCalendar(1, payload);

        expect(mockUpdateCalendar).toHaveBeenCalledWith(1, {
          name: "Just Name",
        });
        expect(result.name).toBe("Just Name");
      });
    });

    describe("deleteCalendar", () => {
      it("should delete a calendar by ID", async () => {
        mockDeleteCalendar.mockResolvedValue(undefined);

        const { api } = await import("$lib/api/client");
        await api.deleteCalendar(1);

        expect(mockDeleteCalendar).toHaveBeenCalledWith(1);
      });

      it("should propagate errors on delete failure", async () => {
        mockDeleteCalendar.mockRejectedValue(new Error("access denied"));

        const { api } = await import("$lib/api/client");
        await expect(api.deleteCalendar(999)).rejects.toThrow("access denied");
      });
    });

    describe("checkCalendar", () => {
      it("should return check response with active status", async () => {
        const response: CalendarCheckResponse = {
          calendarId: 1,
          name: "Business Hours",
          active: true,
          checkedAt: "2026-02-17T10:00:00Z",
        };
        mockCheckCalendar.mockResolvedValue(response);

        const { api } = await import("$lib/api/client");
        const result = await api.checkCalendar(1);

        expect(mockCheckCalendar).toHaveBeenCalledWith(1);
        expect(result.active).toBe(true);
        expect(result.calendarId).toBe(1);
      });

      it("should return inactive status", async () => {
        const response: CalendarCheckResponse = {
          calendarId: 1,
          name: "Business Hours",
          active: false,
          checkedAt: "2026-02-17T22:00:00Z",
        };
        mockCheckCalendar.mockResolvedValue(response);

        const { api } = await import("$lib/api/client");
        const result = await api.checkCalendar(1);

        expect(result.active).toBe(false);
      });
    });
  });

  // -------------------------------------------------------------------------
  // Type Definitions
  // -------------------------------------------------------------------------

  describe("Type Definitions", () => {
    it("Calendar should have all required fields", () => {
      const calendar: Calendar = {
        id: 1,
        name: "Test",
        data: "BEGIN:VCALENDAR...",
        createdAt: "2026-02-15T10:00:00Z",
        updatedAt: "2026-02-15T10:00:00Z",
      };

      expect(calendar.id).toBe(1);
      expect(calendar.name).toBe("Test");
      expect(calendar.data).toBeTruthy();
      expect(calendar.createdAt).toBeTruthy();
      expect(calendar.updatedAt).toBeTruthy();
    });

    it("Calendar should allow optional userId", () => {
      const calendar: Calendar = {
        id: 1,
        userId: 42,
        name: "Test",
        data: "BEGIN:VCALENDAR...",
        createdAt: "2026-02-15T10:00:00Z",
        updatedAt: "2026-02-15T10:00:00Z",
      };

      expect(calendar.userId).toBe(42);
    });

    it("CreateCalendarPayload should require name and data", () => {
      const payload: CreateCalendarPayload = {
        name: "My Calendar",
        data: "BEGIN:VCALENDAR...",
      };

      expect(payload.name).toBeTruthy();
      expect(payload.data).toBeTruthy();
    });

    it("UpdateCalendarPayload should allow partial fields", () => {
      const nameOnly: UpdateCalendarPayload = { name: "New Name" };
      const dataOnly: UpdateCalendarPayload = { data: "BEGIN:VCALENDAR..." };
      const both: UpdateCalendarPayload = {
        name: "New Name",
        data: "BEGIN:VCALENDAR...",
      };

      expect(nameOnly.name).toBe("New Name");
      expect(nameOnly.data).toBeUndefined();
      expect(dataOnly.data).toBeTruthy();
      expect(both.name).toBeTruthy();
      expect(both.data).toBeTruthy();
    });

    it("CalendarCheckResponse should have all fields", () => {
      const response: CalendarCheckResponse = {
        calendarId: 1,
        name: "Test",
        active: true,
        checkedAt: "2026-02-17T10:00:00Z",
      };

      expect(response.calendarId).toBe(1);
      expect(response.name).toBe("Test");
      expect(response.active).toBe(true);
      expect(response.checkedAt).toBeTruthy();
    });
  });

  // -------------------------------------------------------------------------
  // Edge Cases
  // -------------------------------------------------------------------------

  describe("Edge Cases", () => {
    it("should handle iCal data with Windows-style line endings", () => {
      const data =
        "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nSUMMARY:Test\r\nDTSTART:20240101T080000\r\nDTEND:20240101T170000\r\nRRULE:FREQ=DAILY\r\nEND:VEVENT\r\nEND:VCALENDAR";
      expect(validateIcalData(data)).toBeNull();
      const summary = getScheduleSummary(data);
      expect(summary).toContain("Daily");
    });

    it("should handle iCal data with Unix-style line endings", () => {
      const data =
        "BEGIN:VCALENDAR\nVERSION:2.0\nBEGIN:VEVENT\nSUMMARY:Test\nDTSTART:20240101T080000\nDTEND:20240101T170000\nRRULE:FREQ=DAILY\nEND:VEVENT\nEND:VCALENDAR";
      expect(validateIcalData(data)).toBeNull();
      const summary = getScheduleSummary(data);
      expect(summary).toContain("Daily");
    });

    it("should handle multiple VEVENT blocks", () => {
      const data = [
        "BEGIN:VCALENDAR",
        "VERSION:2.0",
        "BEGIN:VEVENT",
        "SUMMARY:Morning",
        "DTSTART:20240101T060000",
        "DTEND:20240101T120000",
        "RRULE:FREQ=DAILY",
        "END:VEVENT",
        "BEGIN:VEVENT",
        "SUMMARY:Evening",
        "DTSTART:20240101T180000",
        "DTEND:20240101T220000",
        "RRULE:FREQ=DAILY",
        "END:VEVENT",
        "END:VCALENDAR",
      ].join("\r\n");

      expect(validateIcalData(data)).toBeNull();
      const summary = getScheduleSummary(data);
      expect(summary).toContain("6:00 AM");
      expect(summary).toContain("6:00 PM");
    });

    it("should handle schedule summary for single day", () => {
      const data = makeIcal(
        "Monday Only",
        "20240101T100000",
        "20240101T140000",
        "FREQ=WEEKLY;BYDAY=MO",
      );
      const summary = getScheduleSummary(data);
      expect(summary).toContain("Mon");
      expect(summary).toContain("10:00 AM");
      expect(summary).toContain("2:00 PM");
    });

    it("should handle schedule summary for all 7 days", () => {
      const data = makeIcal(
        "Every Day",
        "20240101T090000",
        "20240101T170000",
        "FREQ=WEEKLY;BYDAY=SU,MO,TU,WE,TH,FR,SA",
      );
      const summary = getScheduleSummary(data);
      expect(summary).toContain("Every day");
    });
  });
});

// ---------------------------------------------------------------------------
// Visual Builder / Date Range Builder
// ---------------------------------------------------------------------------

describe("Date Range Builder", () => {
  function makeDefaultConfig(
    overrides: Partial<DateRangeBuilderConfig> = {},
  ): DateRangeBuilderConfig {
    return {
      startDate: "2026-03-01",
      endDate: "2026-03-31",
      startHour: 8,
      startMinute: 0,
      endHour: 17,
      endMinute: 0,
      recurrence: "weekly",
      weeklyDays: [false, true, true, true, true, true, false],
      ...overrides,
    };
  }

  // -------------------------------------------------------------------------
  // validateDateRangeConfig
  // -------------------------------------------------------------------------

  describe("validateDateRangeConfig", () => {
    it("should return null for a valid configuration", () => {
      const config = makeDefaultConfig();
      expect(validateDateRangeConfig(config)).toBeNull();
    });

    it("should return error when start date is empty", () => {
      const config = makeDefaultConfig({ startDate: "" });
      expect(validateDateRangeConfig(config)).toBe("Start date is required");
    });

    it("should return error when end date is empty", () => {
      const config = makeDefaultConfig({ endDate: "" });
      expect(validateDateRangeConfig(config)).toBe("End date is required");
    });

    it("should return error when end date is before start date", () => {
      const config = makeDefaultConfig({
        startDate: "2026-03-15",
        endDate: "2026-03-01",
      });
      expect(validateDateRangeConfig(config)).toBe(
        "End date must be on or after start date",
      );
    });

    it("should accept same start and end date", () => {
      const config = makeDefaultConfig({
        startDate: "2026-03-15",
        endDate: "2026-03-15",
      });
      expect(validateDateRangeConfig(config)).toBeNull();
    });

    it("should return error when end time is before start time", () => {
      const config = makeDefaultConfig({
        startHour: 17,
        startMinute: 0,
        endHour: 8,
        endMinute: 0,
      });
      expect(validateDateRangeConfig(config)).toBe(
        "End time must be after start time",
      );
    });

    it("should return error when end time equals start time", () => {
      const config = makeDefaultConfig({
        startHour: 9,
        startMinute: 0,
        endHour: 9,
        endMinute: 0,
      });
      expect(validateDateRangeConfig(config)).toBe(
        "End time must be after start time",
      );
    });

    it("should return error for invalid start hour", () => {
      const config = makeDefaultConfig({ startHour: 25 });
      expect(validateDateRangeConfig(config)).toBe(
        "Start hour must be between 0 and 23",
      );
    });

    it("should return error for negative start hour", () => {
      const config = makeDefaultConfig({ startHour: -1 });
      expect(validateDateRangeConfig(config)).not.toBeNull();
    });

    it("should return error for invalid end hour", () => {
      const config = makeDefaultConfig({ endHour: 24 });
      expect(validateDateRangeConfig(config)).toBe(
        "End hour must be between 0 and 23",
      );
    });

    it("should return error for invalid start minute", () => {
      const config = makeDefaultConfig({ startMinute: 60 });
      expect(validateDateRangeConfig(config)).toBe(
        "Start minute must be between 0 and 59",
      );
    });

    it("should return error for invalid end minute", () => {
      const config = makeDefaultConfig({ endMinute: -1 });
      expect(validateDateRangeConfig(config)).not.toBeNull();
    });

    it("should return error when weekly has no days selected", () => {
      const config = makeDefaultConfig({
        recurrence: "weekly",
        weeklyDays: [false, false, false, false, false, false, false],
      });
      expect(validateDateRangeConfig(config)).toBe(
        "Select at least one day for weekly recurrence",
      );
    });

    it("should accept daily recurrence with no days selected", () => {
      const config = makeDefaultConfig({
        recurrence: "daily",
        weeklyDays: [false, false, false, false, false, false, false],
      });
      expect(validateDateRangeConfig(config)).toBeNull();
    });

    it("should accept none recurrence with no days selected", () => {
      const config = makeDefaultConfig({
        recurrence: "none",
        weeklyDays: [false, false, false, false, false, false, false],
      });
      expect(validateDateRangeConfig(config)).toBeNull();
    });

    it("should return error for invalid start date string", () => {
      const config = makeDefaultConfig({ startDate: "not-a-date" });
      expect(validateDateRangeConfig(config)).toBe("Invalid start date");
    });

    it("should return error for invalid end date string", () => {
      const config = makeDefaultConfig({ endDate: "not-a-date" });
      expect(validateDateRangeConfig(config)).toBe("Invalid end date");
    });
  });

  // -------------------------------------------------------------------------
  // buildDateRangeIcal
  // -------------------------------------------------------------------------

  describe("buildDateRangeIcal", () => {
    it("should return empty string for invalid config", () => {
      const config = makeDefaultConfig({ startDate: "" });
      expect(buildDateRangeIcal(config)).toBe("");
    });

    it("should build valid iCal for weekly recurrence", () => {
      const config = makeDefaultConfig();
      const ical = buildDateRangeIcal(config);

      expect(ical).toContain("BEGIN:VCALENDAR");
      expect(ical).toContain("END:VCALENDAR");
      expect(ical).toContain("BEGIN:VEVENT");
      expect(ical).toContain("END:VEVENT");
      expect(ical).toContain("DTSTART:20260301T080000");
      expect(ical).toContain("DTEND:20260301T170000");
      expect(ical).toContain("FREQ=WEEKLY");
      expect(ical).toContain("BYDAY=MO,TU,WE,TH,FR");
      expect(ical).toContain("UNTIL=20260331T235959");
    });

    it("should build valid iCal for daily recurrence", () => {
      const config = makeDefaultConfig({ recurrence: "daily" });
      const ical = buildDateRangeIcal(config);

      expect(ical).toContain("FREQ=DAILY");
      expect(ical).toContain("UNTIL=20260331T235959");
      expect(ical).not.toContain("BYDAY");
    });

    it("should build valid iCal for no recurrence", () => {
      const config = makeDefaultConfig({
        recurrence: "none",
        startDate: "2026-03-01",
        endDate: "2026-03-05",
      });
      const ical = buildDateRangeIcal(config);

      expect(ical).toContain("DTSTART:20260301T080000");
      expect(ical).toContain("DTEND:20260305T170000");
      expect(ical).not.toContain("RRULE");
    });

    it("should pass validateIcalData for weekly config", () => {
      const config = makeDefaultConfig();
      const ical = buildDateRangeIcal(config);
      expect(validateIcalData(ical)).toBeNull();
    });

    it("should pass validateIcalData for daily config", () => {
      const config = makeDefaultConfig({ recurrence: "daily" });
      const ical = buildDateRangeIcal(config);
      expect(validateIcalData(ical)).toBeNull();
    });

    it("should pass validateIcalData for none config", () => {
      const config = makeDefaultConfig({ recurrence: "none" });
      const ical = buildDateRangeIcal(config);
      expect(validateIcalData(ical)).toBeNull();
    });

    it("should handle weekend-only weekly schedule", () => {
      const config = makeDefaultConfig({
        weeklyDays: [true, false, false, false, false, false, true],
      });
      const ical = buildDateRangeIcal(config);
      expect(ical).toContain("BYDAY=SU,SA");
    });

    it("should handle single day weekly schedule", () => {
      const config = makeDefaultConfig({
        weeklyDays: [false, true, false, false, false, false, false],
      });
      const ical = buildDateRangeIcal(config);
      expect(ical).toContain("BYDAY=MO");
    });

    it("should format time with leading zeros", () => {
      const config = makeDefaultConfig({
        startHour: 6,
        startMinute: 0,
        endHour: 9,
        endMinute: 15,
      });
      const ical = buildDateRangeIcal(config);
      expect(ical).toContain("T060000");
      expect(ical).toContain("T091500");
    });

    it("should produce parseable schedule summary", () => {
      const config = makeDefaultConfig();
      const ical = buildDateRangeIcal(config);
      const summary = getScheduleSummary(ical);
      expect(summary).toContain("Mon-Fri");
      expect(summary).toContain("8:00 AM");
      expect(summary).toContain("5:00 PM");
    });
  });

  // -------------------------------------------------------------------------
  // parseIcalToDateRangeConfig
  // -------------------------------------------------------------------------

  describe("parseIcalToDateRangeConfig", () => {
    it("should return null for empty input", () => {
      expect(parseIcalToDateRangeConfig("")).toBeNull();
    });

    it("should parse weekly recurrence with BYDAY", () => {
      const ical = buildDateRangeIcal(makeDefaultConfig());
      const parsed = parseIcalToDateRangeConfig(ical);

      expect(parsed).not.toBeNull();
      expect(parsed!.startDate).toBe("2026-03-01");
      expect(parsed!.startHour).toBe(8);
      expect(parsed!.startMinute).toBe(0);
      expect(parsed!.endHour).toBe(17);
      expect(parsed!.endMinute).toBe(0);
      expect(parsed!.recurrence).toBe("weekly");
      expect(parsed!.weeklyDays).toEqual([
        false,
        true,
        true,
        true,
        true,
        true,
        false,
      ]);
    });

    it("should parse daily recurrence", () => {
      const ical = buildDateRangeIcal(
        makeDefaultConfig({ recurrence: "daily" }),
      );
      const parsed = parseIcalToDateRangeConfig(ical);

      expect(parsed).not.toBeNull();
      expect(parsed!.recurrence).toBe("daily");
    });

    it("should parse single event (no recurrence)", () => {
      const ical = buildDateRangeIcal(
        makeDefaultConfig({ recurrence: "none" }),
      );
      const parsed = parseIcalToDateRangeConfig(ical);

      expect(parsed).not.toBeNull();
      expect(parsed!.recurrence).toBe("none");
    });

    it("should extract UNTIL date as endDate for weekly", () => {
      const ical = buildDateRangeIcal(makeDefaultConfig());
      const parsed = parseIcalToDateRangeConfig(ical);

      expect(parsed).not.toBeNull();
      expect(parsed!.endDate).toBe("2026-03-31");
    });

    it("should extract UNTIL date as endDate for daily", () => {
      const config = makeDefaultConfig({
        recurrence: "daily",
        endDate: "2026-04-15",
      });
      const ical = buildDateRangeIcal(config);
      const parsed = parseIcalToDateRangeConfig(ical);

      expect(parsed).not.toBeNull();
      expect(parsed!.endDate).toBe("2026-04-15");
    });

    it("should round-trip a weekly config", () => {
      const original = makeDefaultConfig();
      const ical = buildDateRangeIcal(original);
      const parsed = parseIcalToDateRangeConfig(ical);

      expect(parsed).not.toBeNull();
      expect(parsed!.startDate).toBe(original.startDate);
      expect(parsed!.endDate).toBe(original.endDate);
      expect(parsed!.startHour).toBe(original.startHour);
      expect(parsed!.startMinute).toBe(original.startMinute);
      expect(parsed!.endHour).toBe(original.endHour);
      expect(parsed!.endMinute).toBe(original.endMinute);
      expect(parsed!.recurrence).toBe(original.recurrence);
      expect(parsed!.weeklyDays).toEqual(original.weeklyDays);
    });

    it("should round-trip a daily config", () => {
      const original = makeDefaultConfig({
        recurrence: "daily",
        startHour: 6,
        startMinute: 30,
        endHour: 22,
        endMinute: 45,
      });
      const ical = buildDateRangeIcal(original);
      const parsed = parseIcalToDateRangeConfig(ical);

      expect(parsed).not.toBeNull();
      expect(parsed!.startDate).toBe(original.startDate);
      expect(parsed!.endDate).toBe(original.endDate);
      expect(parsed!.startHour).toBe(original.startHour);
      expect(parsed!.startMinute).toBe(original.startMinute);
      expect(parsed!.endHour).toBe(original.endHour);
      expect(parsed!.endMinute).toBe(original.endMinute);
      expect(parsed!.recurrence).toBe("daily");
    });

    it("should parse existing template data (business hours)", () => {
      const biz = CALENDAR_TEMPLATES.find((t) => t.id === "business_hours");
      const parsed = parseIcalToDateRangeConfig(biz!.data);

      expect(parsed).not.toBeNull();
      expect(parsed!.startHour).toBe(8);
      expect(parsed!.endHour).toBe(17);
      expect(parsed!.recurrence).toBe("weekly");
      // Business hours template: MO,TU,WE,TH,FR
      expect(parsed!.weeklyDays).toEqual([
        false,
        true,
        true,
        true,
        true,
        true,
        false,
      ]);
    });

    it("should parse existing template data (always/daily)", () => {
      const always = CALENDAR_TEMPLATES.find((t) => t.id === "always");
      const parsed = parseIcalToDateRangeConfig(always!.data);

      expect(parsed).not.toBeNull();
      expect(parsed!.recurrence).toBe("daily");
      expect(parsed!.startHour).toBe(0);
      expect(parsed!.endHour).toBe(23);
    });

    it("should fallback end date when no UNTIL and no DTEND date", () => {
      // Data with RRULE but no UNTIL
      const ical = [
        "BEGIN:VCALENDAR",
        "VERSION:2.0",
        "BEGIN:VEVENT",
        "DTSTART:20260301T080000",
        "DTEND:20260301T170000",
        "RRULE:FREQ=DAILY",
        "END:VEVENT",
        "END:VCALENDAR",
      ].join("\r\n");

      const parsed = parseIcalToDateRangeConfig(ical);
      expect(parsed).not.toBeNull();
      // Should have an endDate (defaulted to startDate + 30 days)
      expect(parsed!.endDate).toBeTruthy();
    });

    it("should handle single-event data with different DTEND date", () => {
      const ical = [
        "BEGIN:VCALENDAR",
        "VERSION:2.0",
        "BEGIN:VEVENT",
        "DTSTART:20260301T080000",
        "DTEND:20260305T170000",
        "END:VEVENT",
        "END:VCALENDAR",
      ].join("\r\n");

      const parsed = parseIcalToDateRangeConfig(ical);
      expect(parsed).not.toBeNull();
      expect(parsed!.startDate).toBe("2026-03-01");
      expect(parsed!.endDate).toBe("2026-03-05");
      expect(parsed!.recurrence).toBe("none");
    });
  });

  // -------------------------------------------------------------------------
  // Integration: build -> validate -> summarize
  // -------------------------------------------------------------------------

  describe("Integration: build -> validate -> summarize", () => {
    const configs: Array<{
      name: string;
      config: DateRangeBuilderConfig;
      expectedSummaryContains: string[];
    }> = [
      {
        name: "weekday mornings",
        config: {
          startDate: "2026-04-01",
          endDate: "2026-04-30",
          startHour: 6,
          startMinute: 0,
          endHour: 12,
          endMinute: 0,
          recurrence: "weekly",
          weeklyDays: [false, true, true, true, true, true, false],
        },
        expectedSummaryContains: ["Mon-Fri", "6:00 AM", "12:00 PM"],
      },
      {
        name: "daily afternoon",
        config: {
          startDate: "2026-05-01",
          endDate: "2026-05-31",
          startHour: 13,
          startMinute: 0,
          endHour: 18,
          endMinute: 0,
          recurrence: "daily",
          weeklyDays: [false, false, false, false, false, false, false],
        },
        expectedSummaryContains: ["Daily", "1:00 PM", "6:00 PM"],
      },
      {
        name: "weekend only",
        config: {
          startDate: "2026-06-01",
          endDate: "2026-06-30",
          startHour: 10,
          startMinute: 0,
          endHour: 16,
          endMinute: 0,
          recurrence: "weekly",
          weeklyDays: [true, false, false, false, false, false, true],
        },
        expectedSummaryContains: ["Sat-Sun", "10:00 AM", "4:00 PM"],
      },
    ];

    for (const { name, config, expectedSummaryContains } of configs) {
      it(`should produce valid iCal and correct summary for ${name}`, () => {
        const ical = buildDateRangeIcal(config);

        // Validate
        expect(validateIcalData(ical)).toBeNull();

        // Check summary
        const summary = getScheduleSummary(ical);
        for (const expected of expectedSummaryContains) {
          expect(summary).toContain(expected);
        }
      });
    }
  });
});

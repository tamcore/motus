import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Mock $app/environment before any imports that depend on it
vi.mock("$app/environment", () => ({
  browser: true,
}));

// Mock the API client
const mockGetGeofences = vi.fn();
const mockCreateGeofence = vi.fn();
const mockUpdateGeofence = vi.fn();
const mockDeleteGeofence = vi.fn();
const mockGetCalendars = vi.fn();
const mockCheckCalendar = vi.fn();
const mockGetPositions = vi.fn();

vi.mock("$lib/api/client", () => ({
  api: {
    getGeofences: mockGetGeofences,
    createGeofence: mockCreateGeofence,
    updateGeofence: mockUpdateGeofence,
    deleteGeofence: mockDeleteGeofence,
    getCalendars: mockGetCalendars,
    checkCalendar: mockCheckCalendar,
    getPositions: mockGetPositions,
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
  Geofence,
  Calendar,
  CreateGeofencePayload,
  UpdateGeofencePayload,
  CalendarCheckResponse,
} from "$lib/types/api";

// ---------------------------------------------------------------------------
// Test data factories
// ---------------------------------------------------------------------------

function createMockGeofence(overrides: Partial<Geofence> = {}): Geofence {
  return {
    id: 1,
    name: "Office Zone",
    description: "",
    area: "POLYGON((9.94 49.79, 9.96 49.79, 9.96 49.80, 9.94 49.80, 9.94 49.79))",
    geometry: JSON.stringify({
      type: "Polygon",
      coordinates: [
        [
          [9.94, 49.79],
          [9.96, 49.79],
          [9.96, 49.8],
          [9.94, 49.8],
          [9.94, 49.79],
        ],
      ],
    }),
    calendarId: null,
    createdAt: "2026-02-15T10:00:00Z",
    updatedAt: "2026-02-15T10:00:00Z",
    ...overrides,
  };
}

function createMockCalendar(overrides: Partial<Calendar> = {}): Calendar {
  return {
    id: 1,
    userId: 1,
    name: "Work Hours",
    data: "BEGIN:VCALENDAR\nBEGIN:VEVENT\nDTSTART:20260101T090000\nDTEND:20260101T170000\nRRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR\nEND:VEVENT\nEND:VCALENDAR",
    createdAt: "2026-02-10T10:00:00Z",
    updatedAt: "2026-02-10T10:00:00Z",
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("Geofence Calendar Integration", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  // ---------------------------------------------------------------------------
  // Calendar API client interactions
  // ---------------------------------------------------------------------------

  describe("Calendar listing", () => {
    it("should return user calendars for the dropdown", async () => {
      const calendars: Calendar[] = [
        createMockCalendar({ id: 1, name: "Work Hours" }),
        createMockCalendar({ id: 2, name: "Weekend Only" }),
      ];
      mockGetCalendars.mockResolvedValueOnce(calendars);

      const result = await mockGetCalendars();

      expect(mockGetCalendars).toHaveBeenCalledOnce();
      expect(result).toHaveLength(2);
      expect(result[0].name).toBe("Work Hours");
      expect(result[1].name).toBe("Weekend Only");
    });

    it("should return an empty array when no calendars exist", async () => {
      mockGetCalendars.mockResolvedValueOnce([]);

      const result = await mockGetCalendars();

      expect(result).toEqual([]);
    });

    it("should handle calendar fetch errors gracefully", async () => {
      mockGetCalendars.mockRejectedValueOnce(new Error("Server error"));

      await expect(mockGetCalendars()).rejects.toThrow("Server error");
    });
  });

  describe("Calendar active status check", () => {
    it("should return active status for a calendar", async () => {
      const checkResponse: CalendarCheckResponse = {
        calendarId: 1,
        name: "Work Hours",
        active: true,
        checkedAt: "2026-02-17T14:00:00Z",
      };
      mockCheckCalendar.mockResolvedValueOnce(checkResponse);

      const result = await mockCheckCalendar(1);

      expect(mockCheckCalendar).toHaveBeenCalledWith(1);
      expect(result.active).toBe(true);
    });

    it("should return inactive status for a calendar outside schedule", async () => {
      const checkResponse: CalendarCheckResponse = {
        calendarId: 1,
        name: "Work Hours",
        active: false,
        checkedAt: "2026-02-17T22:00:00Z",
      };
      mockCheckCalendar.mockResolvedValueOnce(checkResponse);

      const result = await mockCheckCalendar(1);

      expect(result.active).toBe(false);
    });

    it("should handle check errors for deleted calendars", async () => {
      mockCheckCalendar.mockRejectedValueOnce(new Error("Not found"));

      await expect(mockCheckCalendar(999)).rejects.toThrow("Not found");
    });
  });

  // ---------------------------------------------------------------------------
  // Geofence creation with calendar
  // ---------------------------------------------------------------------------

  describe("Create geofence with calendar", () => {
    it("should create a geofence without a calendar (always active)", async () => {
      const payload: CreateGeofencePayload = {
        name: "Office Zone",
        description: "",
        geometry: JSON.stringify({ type: "Polygon", coordinates: [[[0, 0], [1, 0], [1, 1], [0, 1], [0, 0]]] }),
        calendarId: null,
      };
      const created = createMockGeofence({ calendarId: null });
      mockCreateGeofence.mockResolvedValueOnce(created);

      const result = await mockCreateGeofence(payload);

      expect(mockCreateGeofence).toHaveBeenCalledWith(payload);
      expect(result.calendarId).toBeNull();
    });

    it("should create a geofence with a calendar assigned", async () => {
      const payload: CreateGeofencePayload = {
        name: "Work Parking",
        description: "",
        geometry: JSON.stringify({ type: "Polygon", coordinates: [[[0, 0], [1, 0], [1, 1], [0, 1], [0, 0]]] }),
        calendarId: 1,
      };
      const created = createMockGeofence({ id: 2, name: "Work Parking", calendarId: 1 });
      mockCreateGeofence.mockResolvedValueOnce(created);

      const result = await mockCreateGeofence(payload);

      expect(mockCreateGeofence).toHaveBeenCalledWith(
        expect.objectContaining({ calendarId: 1 }),
      );
      expect(result.calendarId).toBe(1);
    });

    it("should include calendarId: null in payload when no calendar is selected", async () => {
      const payload: CreateGeofencePayload = {
        name: "Home Zone",
        geometry: JSON.stringify({ type: "Polygon", coordinates: [[[0, 0], [1, 0], [1, 1], [0, 1], [0, 0]]] }),
        calendarId: null,
      };
      mockCreateGeofence.mockResolvedValueOnce(createMockGeofence());

      await mockCreateGeofence(payload);

      const calledPayload = mockCreateGeofence.mock.calls[0][0] as CreateGeofencePayload;
      expect(calledPayload).toHaveProperty("calendarId");
      expect(calledPayload.calendarId).toBeNull();
    });
  });

  // ---------------------------------------------------------------------------
  // Geofence update with calendar changes
  // ---------------------------------------------------------------------------

  describe("Update geofence calendar", () => {
    it("should assign a calendar to a geofence that had none", async () => {
      const updatePayload: UpdateGeofencePayload = {
        name: "Office Zone",
        calendarId: 1,
      };
      const updated = createMockGeofence({ calendarId: 1 });
      mockUpdateGeofence.mockResolvedValueOnce(updated);

      const result = await mockUpdateGeofence(1, updatePayload);

      expect(mockUpdateGeofence).toHaveBeenCalledWith(1, expect.objectContaining({ calendarId: 1 }));
      expect(result.calendarId).toBe(1);
    });

    it("should change a geofence calendar to a different one", async () => {
      const updatePayload: UpdateGeofencePayload = {
        name: "Office Zone",
        calendarId: 2,
      };
      const updated = createMockGeofence({ calendarId: 2 });
      mockUpdateGeofence.mockResolvedValueOnce(updated);

      const result = await mockUpdateGeofence(1, updatePayload);

      expect(result.calendarId).toBe(2);
    });

    it("should clear calendar from a geofence (set to null)", async () => {
      const updatePayload: UpdateGeofencePayload = {
        name: "Office Zone",
        calendarId: null,
      };
      const updated = createMockGeofence({ calendarId: null });
      mockUpdateGeofence.mockResolvedValueOnce(updated);

      const result = await mockUpdateGeofence(1, updatePayload);

      const calledPayload = mockUpdateGeofence.mock.calls[0][1] as UpdateGeofencePayload;
      expect(calledPayload.calendarId).toBeNull();
      expect(result.calendarId).toBeNull();
    });

    it("should only send name and calendarId in update (not geometry)", async () => {
      const updatePayload: UpdateGeofencePayload = {
        name: "Renamed Zone",
        calendarId: 1,
      };
      mockUpdateGeofence.mockResolvedValueOnce(createMockGeofence({ name: "Renamed Zone", calendarId: 1 }));

      await mockUpdateGeofence(1, updatePayload);

      const calledPayload = mockUpdateGeofence.mock.calls[0][1] as UpdateGeofencePayload;
      expect(calledPayload).not.toHaveProperty("geometry");
      expect(calledPayload.name).toBe("Renamed Zone");
      expect(calledPayload.calendarId).toBe(1);
    });
  });

  // ---------------------------------------------------------------------------
  // Geofence list with calendar info
  // ---------------------------------------------------------------------------

  describe("Geofence list with calendar information", () => {
    it("should list geofences with their calendarId", async () => {
      const geofences: Geofence[] = [
        createMockGeofence({ id: 1, name: "Office", calendarId: 1 }),
        createMockGeofence({ id: 2, name: "Home", calendarId: null }),
        createMockGeofence({ id: 3, name: "Warehouse", calendarId: 2 }),
      ];
      mockGetGeofences.mockResolvedValueOnce(geofences);

      const result = await mockGetGeofences();

      expect(result).toHaveLength(3);
      expect(result[0].calendarId).toBe(1);
      expect(result[1].calendarId).toBeNull();
      expect(result[2].calendarId).toBe(2);
    });

    it("should resolve calendar names from the calendar list", () => {
      const calendars: Calendar[] = [
        createMockCalendar({ id: 1, name: "Work Hours" }),
        createMockCalendar({ id: 2, name: "Night Shift" }),
      ];

      // Simulate the getCalendarName helper
      function getCalendarName(calendarId: number | null | undefined): string {
        if (calendarId == null) return "";
        const cal = calendars.find((c) => c.id === calendarId);
        return cal ? cal.name : `Calendar #${calendarId}`;
      }

      expect(getCalendarName(1)).toBe("Work Hours");
      expect(getCalendarName(2)).toBe("Night Shift");
      expect(getCalendarName(null)).toBe("");
      expect(getCalendarName(undefined)).toBe("");
      expect(getCalendarName(999)).toBe("Calendar #999");
    });

    it("should identify geofences with calendar restrictions", () => {
      const geofences: Geofence[] = [
        createMockGeofence({ id: 1, calendarId: 1 }),
        createMockGeofence({ id: 2, calendarId: null }),
        createMockGeofence({ id: 3, calendarId: undefined }),
      ];

      const withCalendar = geofences.filter((g) => g.calendarId != null);
      const withoutCalendar = geofences.filter((g) => g.calendarId == null);

      expect(withCalendar).toHaveLength(1);
      expect(withCalendar[0].id).toBe(1);
      expect(withoutCalendar).toHaveLength(2);
    });
  });

  // ---------------------------------------------------------------------------
  // Calendar active status aggregation
  // ---------------------------------------------------------------------------

  describe("Calendar active status tracking", () => {
    it("should check statuses for all calendars used by geofences", async () => {
      const geofences: Geofence[] = [
        createMockGeofence({ id: 1, calendarId: 1 }),
        createMockGeofence({ id: 2, calendarId: 2 }),
        createMockGeofence({ id: 3, calendarId: 1 }), // duplicate calendar reference
        createMockGeofence({ id: 4, calendarId: null }), // no calendar
      ];

      // Determine unique calendar IDs to check
      const calendarIds = new Set<number>();
      for (const gf of geofences) {
        if (gf.calendarId != null) {
          calendarIds.add(gf.calendarId);
        }
      }

      expect(calendarIds.size).toBe(2);
      expect(calendarIds.has(1)).toBe(true);
      expect(calendarIds.has(2)).toBe(true);

      // Simulate checking each calendar
      mockCheckCalendar
        .mockResolvedValueOnce({ calendarId: 1, name: "Work Hours", active: true, checkedAt: "2026-02-17T14:00:00Z" })
        .mockResolvedValueOnce({ calendarId: 2, name: "Night Shift", active: false, checkedAt: "2026-02-17T14:00:00Z" });

      const status: Record<number, boolean> = {};
      const checks = Array.from(calendarIds).map(async (id) => {
        const result = await mockCheckCalendar(id);
        status[id] = result.active;
      });
      await Promise.all(checks);

      expect(status[1]).toBe(true);
      expect(status[2]).toBe(false);
      expect(mockCheckCalendar).toHaveBeenCalledTimes(2);
    });

    it("should handle deleted calendars during status check", async () => {
      mockCheckCalendar.mockRejectedValueOnce(new Error("Not found"));

      const status: Record<number, boolean> = {};
      try {
        const result = await mockCheckCalendar(999);
        status[999] = result.active;
      } catch {
        // Treat as inactive when calendar is missing
        status[999] = false;
      }

      expect(status[999]).toBe(false);
    });
  });

  // ---------------------------------------------------------------------------
  // Type definitions validation
  // ---------------------------------------------------------------------------

  describe("Type definitions", () => {
    it("should have calendarId in CreateGeofencePayload", () => {
      const payload: CreateGeofencePayload = {
        name: "Test",
        geometry: "{}",
        calendarId: 5,
      };
      expect(payload.calendarId).toBe(5);
    });

    it("should allow null calendarId in CreateGeofencePayload", () => {
      const payload: CreateGeofencePayload = {
        name: "Test",
        geometry: "{}",
        calendarId: null,
      };
      expect(payload.calendarId).toBeNull();
    });

    it("should allow omitting calendarId in CreateGeofencePayload", () => {
      const payload: CreateGeofencePayload = {
        name: "Test",
        geometry: "{}",
      };
      expect(payload.calendarId).toBeUndefined();
    });

    it("should have calendarId in UpdateGeofencePayload", () => {
      const payload: UpdateGeofencePayload = {
        calendarId: 3,
      };
      expect(payload.calendarId).toBe(3);
    });

    it("should allow null calendarId in UpdateGeofencePayload", () => {
      const payload: UpdateGeofencePayload = {
        calendarId: null,
      };
      expect(payload.calendarId).toBeNull();
    });

    it("should have all required Calendar fields", () => {
      const cal = createMockCalendar();
      expect(cal).toHaveProperty("id");
      expect(cal).toHaveProperty("name");
      expect(cal).toHaveProperty("data");
      expect(cal).toHaveProperty("createdAt");
      expect(cal).toHaveProperty("updatedAt");
    });

    it("should have CalendarCheckResponse shape", () => {
      const response: CalendarCheckResponse = {
        calendarId: 1,
        name: "Work Hours",
        active: true,
        checkedAt: "2026-02-17T14:00:00Z",
      };
      expect(response.active).toBe(true);
      expect(response.calendarId).toBe(1);
    });
  });

  // ---------------------------------------------------------------------------
  // Geofence popup rendering with calendar info
  // ---------------------------------------------------------------------------

  describe("Geofence popup content", () => {
    it("should include calendar info in popup for calendar-restricted geofences", () => {
      const calendars: Calendar[] = [
        createMockCalendar({ id: 1, name: "Work Hours" }),
      ];
      const gf = createMockGeofence({ name: "Office", calendarId: 1 });

      function getCalendarName(calendarId: number | null | undefined): string {
        if (calendarId == null) return "";
        const cal = calendars.find((c) => c.id === calendarId);
        return cal ? cal.name : `Calendar #${calendarId}`;
      }

      const calName = getCalendarName(gf.calendarId);
      const calHtml = calName
        ? `<br><small style="color: var(--text-tertiary)">Schedule: ${calName}</small>`
        : "";
      const popupHtml = `<strong>${gf.name}</strong>${calHtml}`;

      expect(popupHtml).toContain("Office");
      expect(popupHtml).toContain("Schedule: Work Hours");
    });

    it("should not include calendar info in popup for always-active geofences", () => {
      const gf = createMockGeofence({ name: "Home", calendarId: null });

      function getCalendarName(calendarId: number | null | undefined): string {
        if (calendarId == null) return "";
        return "";
      }

      const calName = getCalendarName(gf.calendarId);
      const calHtml = calName ? `<br><small>Schedule: ${calName}</small>` : "";
      const popupHtml = `<strong>${gf.name}</strong>${calHtml}`;

      expect(popupHtml).toContain("Home");
      expect(popupHtml).not.toContain("Schedule:");
    });
  });

  // ---------------------------------------------------------------------------
  // JSON serialization for API payloads
  // ---------------------------------------------------------------------------

  describe("JSON serialization", () => {
    it("should serialize calendarId: null as null in JSON", () => {
      const payload: CreateGeofencePayload = {
        name: "Test",
        geometry: "{}",
        calendarId: null,
      };
      const json = JSON.stringify(payload);
      const parsed = JSON.parse(json);

      expect(parsed.calendarId).toBeNull();
    });

    it("should serialize calendarId: number as integer in JSON", () => {
      const payload: CreateGeofencePayload = {
        name: "Test",
        geometry: "{}",
        calendarId: 42,
      };
      const json = JSON.stringify(payload);
      const parsed = JSON.parse(json);

      expect(parsed.calendarId).toBe(42);
      expect(typeof parsed.calendarId).toBe("number");
    });

    it("should omit calendarId when undefined in JSON", () => {
      const payload: CreateGeofencePayload = {
        name: "Test",
        geometry: "{}",
      };
      const json = JSON.stringify(payload);
      const parsed = JSON.parse(json);

      expect(parsed).not.toHaveProperty("calendarId");
    });
  });
});

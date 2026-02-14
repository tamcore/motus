import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Mock $app/environment before importing modules that use it
vi.mock("$app/environment", () => ({ browser: false }));

// Use a shared reference for mock settings that the factory can capture
vi.mock("$lib/stores/settings", async () => {
  const { writable, get } = await import("svelte/store");
  const defaultSettings = {
    dateFormat: "iso" as "iso" | "locale" | "relative",
    timezone: "local",
    units: "metric" as const,
    defaultMapLat: 49.79,
    defaultMapLng: 9.95,
    defaultMapZoom: 13,
    mapLocationSet: false,
    mapOverlay: "none",
    mapOverlayOpacity: 80,
    showAllDevices: false,
  };
  const store = writable(defaultSettings);
  return {
    settings: store,
    getSettings: () => get(store),
  };
});

import { settings } from "$lib/stores/settings";
import { formatDate, formatMileage, mileageToDisplay, mileageFromDisplay } from "$lib/utils/formatting";

function updateSettings(overrides: Record<string, unknown>) {
  settings.update((s) => ({ ...s, ...overrides }));
}

describe("formatDate timezone handling", () => {
  beforeEach(() => {
    // Reset to defaults
    settings.set({
      dateFormat: "iso",
      timezone: "local",
      units: "metric",
      defaultMapLat: 49.79,
      defaultMapLng: 9.95,
      defaultMapZoom: 13,
      mapLocationSet: false,
      mapOverlay: "none",
      mapOverlayOpacity: 80,
      showAllDevices: false,
    });
  });

  describe('iso format with timezone="local"', () => {
    it("should NOT return raw toISOString when timezone is local", () => {
      updateSettings({ dateFormat: "iso", timezone: "local" });
      const utcDate = "2024-06-15T14:30:00Z";
      const result = formatDate(utcDate);

      // Should be a formatted ISO-like string (YYYY-MM-DD HH:mm:ss)
      expect(result).toMatch(/^\d{4}-\d{2}-\d{2}\s+\d{2}[.:]\d{2}[.:]\d{2}$/);
    });

    it("should use Intl.DateTimeFormat for local timezone (produces local time, not UTC)", () => {
      updateSettings({ dateFormat: "iso", timezone: "local" });
      const localResult = formatDate("2024-06-15T14:30:00Z");

      // Verify the format is correct ISO-like (YYYY-MM-DD HH:mm:ss)
      expect(localResult).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/);

      // The result should match the system's local time, not necessarily UTC.
      // We verify by comparing with Intl.DateTimeFormat directly (no timeZone = local).
      const expected = new Date("2024-06-15T14:30:00Z");
      const localHour = expected.getHours(); // local hour
      expect(localResult).toContain(String(localHour).padStart(2, "0"));
    });
  });

  describe("iso format with explicit timezone", () => {
    it("should format correctly with America/New_York", () => {
      updateSettings({ dateFormat: "iso", timezone: "America/New_York" });
      // 2024-06-15T14:30:00Z = 2024-06-15 10:30:00 EDT (UTC-4 in summer)
      const result = formatDate("2024-06-15T14:30:00Z");
      expect(result).toContain("2024-06-15");
      expect(result).toContain("10");
      expect(result).toContain("30");
      expect(result).toContain("00");
    });

    it("should format correctly with UTC", () => {
      updateSettings({ dateFormat: "iso", timezone: "UTC" });
      const result = formatDate("2024-06-15T14:30:00Z");
      expect(result).toContain("2024-06-15");
      expect(result).toContain("14");
      expect(result).toContain("30");
    });
  });

  describe('locale format with timezone="local"', () => {
    it("should produce a locale string for local timezone", () => {
      updateSettings({ dateFormat: "locale", timezone: "local" });
      const result = formatDate("2024-06-15T14:30:00Z");
      expect(result).toBeTruthy();
      expect(result.length).toBeGreaterThan(5);
    });
  });

  describe("default/fallback format", () => {
    it("should use Intl.DateTimeFormat, not toISOString, in the default case", () => {
      updateSettings({ dateFormat: "unknown" as "iso", timezone: "local" });
      const result = formatDate("2024-06-15T14:30:00Z");
      // Should produce a formatted string
      expect(result).toMatch(/^\d{4}-\d{2}-\d{2}\s+\d{2}[.:]\d{2}[.:]\d{2}$/);
    });
  });

  describe("invalid date handling", () => {
    it("should return the original string for invalid dates", () => {
      updateSettings({ dateFormat: "iso", timezone: "local" });
      const result = formatDate("not-a-date");
      expect(result).toBe("not-a-date");
    });
  });
});

describe("formatMileage", () => {
  beforeEach(() => {
    settings.set({
      dateFormat: "iso",
      timezone: "local",
      units: "metric",
      defaultMapLat: 49.79,
      defaultMapLng: 9.95,
      defaultMapZoom: 13,
      mapLocationSet: false,
      mapOverlay: "none",
      mapOverlayOpacity: 80,
      showAllDevices: false,
    });
  });

  it("should return dash for null", () => {
    expect(formatMileage(null)).toBe("—");
  });

  it("should return dash for undefined", () => {
    expect(formatMileage(undefined)).toBe("—");
  });

  it("should format metric mileage with km suffix", () => {
    updateSettings({ units: "metric" });
    const result = formatMileage(117000);
    expect(result).toContain("km");
    expect(result).toContain("117");
  });

  it("should format imperial mileage with mi suffix", () => {
    updateSettings({ units: "imperial" });
    const result = formatMileage(117000);
    expect(result).toContain("mi");
    // 117000 km ≈ 72,700 mi
    expect(result).toContain("72");
  });

  it("should round to whole numbers", () => {
    updateSettings({ units: "metric" });
    const result = formatMileage(117000.7);
    expect(result).toContain("117,001");
  });
});

describe("mileageToDisplay / mileageFromDisplay", () => {
  beforeEach(() => {
    settings.set({
      dateFormat: "iso",
      timezone: "local",
      units: "metric",
      defaultMapLat: 49.79,
      defaultMapLng: 9.95,
      defaultMapZoom: 13,
      mapLocationSet: false,
      mapOverlay: "none",
      mapOverlayOpacity: 80,
      showAllDevices: false,
    });
  });

  it("metric: returns same value", () => {
    updateSettings({ units: "metric" });
    expect(mileageToDisplay(100)).toBe(100);
    expect(mileageFromDisplay(100)).toBe(100);
  });

  it("imperial: converts km to miles and back", () => {
    updateSettings({ units: "imperial" });
    const miles = mileageToDisplay(100);
    expect(miles).toBeCloseTo(62.14, 1);
    const km = mileageFromDisplay(miles);
    expect(km).toBeCloseTo(100, 1);
  });
});

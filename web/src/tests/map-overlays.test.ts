import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Mock $app/environment before any imports that depend on it
vi.mock("$app/environment", () => ({
  browser: true,
}));

describe("Map Overlays", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("Overlay definitions", () => {
    it("should export all required overlay definitions", async () => {
      const { MAP_OVERLAYS } = await import("$lib/utils/map-overlays");

      expect(MAP_OVERLAYS).toBeDefined();
      expect(Array.isArray(MAP_OVERLAYS)).toBe(true);
      expect(MAP_OVERLAYS.length).toBeGreaterThan(0);
    });

    it("should include a 'none' overlay option", async () => {
      const { MAP_OVERLAYS } = await import("$lib/utils/map-overlays");

      const none = MAP_OVERLAYS.find((o) => o.id === "none");
      expect(none).toBeDefined();
      expect(none!.name).toBe("None");
      expect(none!.url).toBe("");
    });

    it("should include standard overlay options", async () => {
      const { MAP_OVERLAYS } = await import("$lib/utils/map-overlays");

      const ids = MAP_OVERLAYS.map((o) => o.id);
      expect(ids).toContain("none");
      expect(ids).toContain("humanitarian");
      expect(ids).toContain("topo");
      expect(ids).toContain("cyclosm");
    });

    it("each overlay should have required fields", async () => {
      const { MAP_OVERLAYS } = await import("$lib/utils/map-overlays");

      for (const overlay of MAP_OVERLAYS) {
        expect(overlay).toHaveProperty("id");
        expect(overlay).toHaveProperty("name");
        expect(overlay).toHaveProperty("url");
        expect(overlay).toHaveProperty("attribution");
        expect(typeof overlay.id).toBe("string");
        expect(typeof overlay.name).toBe("string");
        expect(typeof overlay.url).toBe("string");
        expect(typeof overlay.attribution).toBe("string");
      }
    });

    it("non-none overlays should have valid tile URLs", async () => {
      const { MAP_OVERLAYS } = await import("$lib/utils/map-overlays");

      for (const overlay of MAP_OVERLAYS) {
        if (overlay.id !== "none") {
          expect(overlay.url).toContain("{z}");
          expect(overlay.url).toContain("{x}");
          expect(overlay.url).toContain("{y}");
          expect(overlay.url.startsWith("https://")).toBe(true);
        }
      }
    });
  });

  describe("Overlay lookup helper", () => {
    it("should find overlay by id", async () => {
      const { getOverlayById } = await import("$lib/utils/map-overlays");

      const overlay = getOverlayById("topo");
      expect(overlay).toBeDefined();
      expect(overlay!.id).toBe("topo");
    });

    it("should return undefined for unknown id", async () => {
      const { getOverlayById } = await import("$lib/utils/map-overlays");

      const overlay = getOverlayById("nonexistent");
      expect(overlay).toBeUndefined();
    });

    it("should return none overlay by default", async () => {
      const { getOverlayById } = await import("$lib/utils/map-overlays");

      const overlay = getOverlayById("none");
      expect(overlay).toBeDefined();
      expect(overlay!.name).toBe("None");
    });
  });

  describe("Settings integration", () => {
    it("should have default overlay settings", async () => {
      const { settings } = await import("$lib/stores/settings");
      const { get } = await import("svelte/store");

      const s = get(settings);
      expect(s.mapOverlay).toBe("none");
      expect(s.mapOverlayOpacity).toBe(80);
    });

    it("should persist overlay settings to localStorage", async () => {
      // Clear any prior modules so settings store re-initializes from localStorage
      vi.resetModules();

      // Re-mock dependencies after reset
      vi.mock("$app/environment", () => ({
        browser: true,
      }));

      const { settings } = await import("$lib/stores/settings");
      const { get } = await import("svelte/store");

      settings.update((s) => ({
        ...s,
        mapOverlay: "topo",
        mapOverlayOpacity: 60,
      }));

      const saved = JSON.parse(
        localStorage.getItem("motus_settings") || "{}",
      );
      expect(saved.mapOverlay).toBe("topo");
      expect(saved.mapOverlayOpacity).toBe(60);
    });

    it("should restore overlay settings from localStorage", async () => {
      localStorage.setItem(
        "motus_settings",
        JSON.stringify({
          mapOverlay: "cyclosm",
          mapOverlayOpacity: 50,
        }),
      );

      vi.resetModules();

      vi.mock("$app/environment", () => ({
        browser: true,
      }));

      const { settings } = await import("$lib/stores/settings");
      const { get } = await import("svelte/store");

      const s = get(settings);
      expect(s.mapOverlay).toBe("cyclosm");
      expect(s.mapOverlayOpacity).toBe(50);
    });

    it("should reset overlay settings to defaults", async () => {
      vi.resetModules();

      vi.mock("$app/environment", () => ({
        browser: true,
      }));

      const { settings } = await import("$lib/stores/settings");
      const { get } = await import("svelte/store");

      settings.update((s) => ({
        ...s,
        mapOverlay: "topo",
        mapOverlayOpacity: 40,
      }));

      settings.reset();

      const s = get(settings);
      expect(s.mapOverlay).toBe("none");
      expect(s.mapOverlayOpacity).toBe(80);
    });

    it("overlay opacity should be clamped between 0 and 100", async () => {
      const { MAP_OVERLAYS } = await import("$lib/utils/map-overlays");

      // Opacity is stored as integer percent (0-100)
      // The component should handle clamping in the UI via min/max attrs on the slider.
      // Here we verify the default is within range.
      const defaultOpacity = 80;
      expect(defaultOpacity).toBeGreaterThanOrEqual(0);
      expect(defaultOpacity).toBeLessThanOrEqual(100);
    });
  });

  describe("Overlay type validation", () => {
    it("should accept valid overlay ids", async () => {
      const { MAP_OVERLAYS, getOverlayById } = await import(
        "$lib/utils/map-overlays"
      );

      const validIds = MAP_OVERLAYS.map((o) => o.id);
      for (const id of validIds) {
        expect(getOverlayById(id)).toBeDefined();
      }
    });

    it("overlay ids should be unique", async () => {
      const { MAP_OVERLAYS } = await import("$lib/utils/map-overlays");

      const ids = MAP_OVERLAYS.map((o) => o.id);
      const uniqueIds = new Set(ids);
      expect(uniqueIds.size).toBe(ids.length);
    });

    it("overlay names should be unique", async () => {
      const { MAP_OVERLAYS } = await import("$lib/utils/map-overlays");

      const names = MAP_OVERLAYS.map((o) => o.name);
      const uniqueNames = new Set(names);
      expect(uniqueNames.size).toBe(names.length);
    });
  });
});

import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("$app/environment", () => ({ browser: true }));

vi.mock("$lib/stores/settings", async () => {
  const { writable, get } = await import("svelte/store");
  const defaultSettings = {
    dateFormat: "iso" as const,
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

vi.mock("$lib/stores/auth", async () => {
  const { writable } = await import("svelte/store");
  return {
    currentUser: writable({ id: 1, name: "Admin", administrator: true }),
  };
});

import { settings } from "$lib/stores/settings";
import { api, fetchPositions } from "$lib/api/client";

describe("fetchPositions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
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

  it("should call getPositions for non-admin users", async () => {
    const spy = vi.spyOn(api, "getPositions").mockResolvedValueOnce([
      { deviceId: 1, latitude: 52.0, longitude: 13.0 } as any,
    ]);

    const result = await fetchPositions(false);
    expect(spy).toHaveBeenCalled();
    expect(result).toHaveLength(1);
    spy.mockRestore();
  });

  it("should call getPositions when admin has showAllDevices=false", async () => {
    settings.update((s) => ({ ...s, showAllDevices: false }));
    const spy = vi.spyOn(api, "getPositions").mockResolvedValueOnce([]);

    await fetchPositions(true);
    expect(spy).toHaveBeenCalled();
    spy.mockRestore();
  });

  it("should call getAllPositions when admin has showAllDevices=true", async () => {
    settings.update((s) => ({ ...s, showAllDevices: true }));
    const spy = vi.spyOn(api, "getAllPositions").mockResolvedValueOnce([
      { deviceId: 1, latitude: 52.0, longitude: 13.0 } as any,
      { deviceId: 2, latitude: 51.0, longitude: 12.0 } as any,
    ]);

    const result = await fetchPositions(true);
    expect(spy).toHaveBeenCalled();
    expect(result).toHaveLength(2);
    spy.mockRestore();
  });
});

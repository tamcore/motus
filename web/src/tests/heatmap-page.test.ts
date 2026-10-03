import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import { writable } from "svelte/store";

const mocks = vi.hoisted(() => ({
  fetchDevices: vi.fn(),
  getPositionPoints: vi.fn(),
}));

vi.mock("$app/environment", () => ({ browser: true }));
vi.mock("$lib/api/client", () => ({
  api: { getPositionPoints: mocks.getPositionPoints },
  fetchDevices: mocks.fetchDevices,
}));
vi.mock("$lib/stores/auth", () => ({
  currentUser: writable({ id: 1, email: "admin@motus.local", administrator: true }),
}));
vi.mock("$lib/stores/refresh", () => ({ refreshHandler: writable(null) }));
vi.mock("$lib/stores/theme", () => ({ theme: writable("light") }));
vi.mock("$lib/composables/useLeaflet", () => {
  const map = { fitBounds: vi.fn(), removeLayer: vi.fn() };
  const L = { latLngBounds: () => ({ pad: () => ({}) }) };
  return {
    useLeaflet: () => ({
      initialize: vi.fn().mockResolvedValue(undefined),
      cleanup: vi.fn(),
      getMap: () => map,
      getLeaflet: () => L,
      getTileLayer: () => null,
    }),
  };
});
vi.mock("$lib/utils/leaflet-heat", () => ({
  loadHeatLayer: vi.fn().mockResolvedValue(() => ({ addTo: () => ({}) })),
}));

import { settings } from "$lib/stores/settings";
import HeatmapPage from "../routes/heatmap/+page.svelte";

describe("heatmap page", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    settings.update((s) => ({ ...s, showAllDevices: false }));
    // An admin owns no devices; with "All users" the instance's devices appear.
    mocks.fetchDevices.mockImplementation(async () => {
      let all = false;
      settings.subscribe((s) => (all = s.showAllDevices))();
      return all ? [{ id: 7, name: "Other user's car", uniqueId: "7", status: "online" }] : [];
    });
    mocks.getPositionPoints.mockResolvedValue([
      { lat: 49.79, lon: 9.95, speed: 10, fixTime: "2026-10-01T10:00:00Z" },
    ]);
  });

  it("reloads the heatmap when an admin toggles All users", async () => {
    render(HeatmapPage);
    // Let onMount finish its initial (empty) load before toggling.
    await waitFor(() => expect(mocks.fetchDevices).toHaveBeenCalledTimes(1));
    await new Promise((r) => setTimeout(r, 0));
    expect(screen.getByText("0 points")).toBeInTheDocument();
    expect(mocks.getPositionPoints).not.toHaveBeenCalled();

    await fireEvent.change(screen.getByRole("checkbox"), { target: { checked: true } });

    await waitFor(() => expect(screen.getByText("1 points")).toBeInTheDocument());
    expect(mocks.getPositionPoints).toHaveBeenCalledWith(expect.objectContaining({ deviceId: 7 }));
  });
});

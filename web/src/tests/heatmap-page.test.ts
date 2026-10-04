import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import { writable } from "svelte/store";

const mocks = vi.hoisted(() => ({
  fetchDevices: vi.fn(),
  getPositionPoints: vi.fn(),
}));

vi.mock("$lib/api/client", () => ({
  api: { getPositionPoints: mocks.getPositionPoints },
  fetchDevices: mocks.fetchDevices,
}));
vi.mock("$lib/stores/auth", () => ({
  currentUser: writable({ id: 1, email: "admin@motus.local", administrator: true }),
  isAdmin: writable(true),
}));
vi.mock("$lib/stores/refresh", () => ({ refreshHandler: writable(null) }));
vi.mock("$lib/stores/theme", () => ({ isDark: writable(false) }));
vi.mock("$lib/composables/useLeaflet", () => {
  const map = { fitBounds: vi.fn(), removeLayer: vi.fn() };
  const L = { latLngBounds: () => ({ pad: () => ({}) }) };
  return {
    useLeaflet: () => ({
      initialize: vi.fn().mockResolvedValue(undefined),
      cleanup: vi.fn(),
      getMap: () => map,
      getLeaflet: () => L,
    }),
  };
});
vi.mock("leaflet.heat", () => {
  (window as unknown as { L: Record<string, unknown> }).L.heatLayer = () => ({ addTo: () => ({}) });
  return {};
});

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

  describe("custom range", () => {
    beforeEach(() => {
      // A zone east of UTC, so UTC midnight and local midnight differ.
      vi.stubEnv("TZ", "Europe/Berlin");
    });
    afterEach(() => {
      vi.unstubAllEnvs();
    });

    it("queries from local midnight of the start day to the end of the end day", async () => {
      mocks.fetchDevices.mockResolvedValue([{ id: 7, name: "Car", uniqueId: "7", status: "online" }]);
      render(HeatmapPage);
      await waitFor(() => expect(mocks.getPositionPoints).toHaveBeenCalled());
      mocks.getPositionPoints.mockClear();

      await fireEvent.change(screen.getByLabelText("Date Range"), { target: { value: "custom" } });
      await fireEvent.input(screen.getByLabelText("From"), { target: { value: "2026-01-13" } });
      await fireEvent.input(screen.getByLabelText("To"), { target: { value: "2026-01-15" } });
      await fireEvent.click(screen.getByRole("button", { name: "Apply" }));

      await waitFor(() => expect(mocks.getPositionPoints).toHaveBeenCalled());
      expect(mocks.getPositionPoints).toHaveBeenLastCalledWith(
        expect.objectContaining({
          from: "2026-01-12T23:00:00.000Z",
          to: "2026-01-15T22:59:59.999Z",
        }),
      );
    });

    it("falls back to the last 7 days when the custom start is empty", async () => {
      mocks.fetchDevices.mockResolvedValue([
        { id: 7, name: "Car", uniqueId: "7", status: "online" },
        { id: 8, name: "Bike", uniqueId: "8", status: "online" },
      ]);
      const { container } = render(HeatmapPage);
      await waitFor(() => expect(mocks.getPositionPoints).toHaveBeenCalled());
      mocks.getPositionPoints.mockClear();

      await fireEvent.change(screen.getByLabelText("Date Range"), { target: { value: "custom" } });
      await fireEvent.change(container.querySelector("#device-filter")!, { target: { value: "8" } });

      await waitFor(() => expect(mocks.getPositionPoints).toHaveBeenCalled());
      const { from, to } = mocks.getPositionPoints.mock.calls.at(-1)![0];
      const spanDays = (Date.parse(to) - Date.parse(from)) / 86_400_000;
      expect(spanDays).toBeGreaterThan(6.9);
      expect(spanDays).toBeLessThan(7.1);
    });
  });
});

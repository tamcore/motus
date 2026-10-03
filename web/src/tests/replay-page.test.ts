import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, waitFor } from "@testing-library/svelte";
import { readable } from "svelte/store";

Object.defineProperty(window, "matchMedia", {
  writable: true,
  value: vi.fn().mockImplementation((query: string) => ({
    matches: false,
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
  })),
});

const pageUrl ={ current: new URL("http://localhost/reports/replay") };

vi.mock("$app/environment", () => ({ browser: true }));
vi.mock("$app/stores", () => ({
  page: { subscribe: (fn: (v: unknown) => void) => readable({ url: pageUrl.current }).subscribe(fn) },
}));

const getPositionPoints = vi.fn();
vi.mock("$lib/api/client", () => ({
  api: { getPositionPoints: (...a: unknown[]) => getPositionPoints(...a) },
  fetchDevices: vi.fn().mockResolvedValue([{ id: 1, name: "Car" }]),
}));

function layer() {
  const l: Record<string, unknown> = {};
  l.addTo = () => l;
  l.bindPopup = () => l;
  l.setLatLng = () => l;
  l.setLatLngs = () => l;
  l.getElement = () => null;
  l.getBounds = () => ({ pad: () => [[0, 0], [1, 1]] });
  return l;
}
const fakeMap = {
  invalidateSize: vi.fn(),
  removeLayer: vi.fn(),
  fitBounds: vi.fn(),
  panTo: vi.fn(),
};
const fakeL = {
  polyline: vi.fn(() => layer()),
  circleMarker: vi.fn(() => layer()),
  marker: vi.fn(() => layer()),
  divIcon: vi.fn(() => ({})),
};
vi.mock("$lib/composables/useLeaflet", () => ({
  useLeaflet: () => ({
    initialize: vi.fn().mockResolvedValue(undefined),
    cleanup: vi.fn(),
    getMap: () => fakeMap,
    getLeaflet: () => fakeL,
    getTileLayer: () => null,
  }),
}));

vi.mock("chart.js", () => {
  class Chart {
    static register() {}
    data: { datasets: { data: unknown[] }[] };
    constructor(_ctx: unknown, cfg: { data: { datasets: { data: unknown[] }[] } }) {
      this.data = cfg.data;
    }
    update() {}
    destroy() {}
    resize() {}
  }
  return { Chart, registerables: [] };
});

import ReplayPage from "../routes/reports/replay/+page.svelte";
import RoutePage from "../routes/reports/route/+page.svelte";

describe("reports/replay page", () => {
  beforeEach(() => {
    getPositionPoints.mockReset();
    HTMLCanvasElement.prototype.getContext = vi.fn(() => ({})) as never;
  });

  it("loads points from the query params and shows the playback bar", async () => {
    pageUrl.current = new URL(
      "http://localhost/reports/replay?deviceId=1&from=2025-01-01T00:00:00Z&to=2025-01-02T00:00:00Z",
    );
    getPositionPoints.mockResolvedValue([
      { lat: 51.5, lon: -0.09, speed: 30, fixTime: "2025-01-01T00:00:00Z", course: 90, altitude: 12 },
      { lat: 51.51, lon: -0.08, speed: 40, fixTime: "2025-01-01T00:01:00Z" },
    ]);

    const { container } = render(ReplayPage);

    await waitFor(() => expect(container.querySelector(".playback-bar")).not.toBeNull());
    expect(getPositionPoints).toHaveBeenCalledWith({
      deviceId: 1,
      from: "2025-01-01T00:00:00.000Z",
      to: "2025-01-02T00:00:00.000Z",
      limit: 10000,
    });
    expect(container.querySelector(".error-message")).toBeNull();
  });

  it("repairs an offset timestamp whose '+' was decoded to a space", async () => {
    // Trip links built from a non-UTC server carry "+02:00"; unencoded, the
    // browser decodes "+" to a space and the API rejects the value (400).
    pageUrl.current = new URL(
      "http://localhost/reports/replay?deviceId=1&from=2026-10-03T07:48:24+02:00&to=2026-10-03T08:17:34+02:00",
    );
    getPositionPoints.mockResolvedValue([
      { lat: 50, lon: 10, speed: 30, fixTime: "2026-10-03T05:48:24Z" },
      { lat: 50.1, lon: 10.1, speed: 40, fixTime: "2026-10-03T05:49:24Z" },
    ]);

    const { container } = render(ReplayPage);

    await waitFor(() => expect(container.querySelector(".playback-bar")).not.toBeNull());
    expect(getPositionPoints).toHaveBeenCalledWith({
      deviceId: 1,
      from: "2026-10-03T05:48:24.000Z",
      to: "2026-10-03T06:17:34.000Z",
      limit: 10000,
    });
  });
});

describe("reports/route page", () => {
  beforeEach(() => {
    getPositionPoints.mockReset();
  });

  it("repairs an offset timestamp whose '+' was decoded to a space", async () => {
    pageUrl.current = new URL(
      "http://localhost/reports/route?deviceId=1&from=2026-10-03T07:48:24+02:00&to=2026-10-03T08:17:34+02:00",
    );
    getPositionPoints.mockResolvedValue([
      { lat: 50, lon: 10, speed: 30, fixTime: "2026-10-03T05:48:24Z" },
      { lat: 50.1, lon: 10.1, speed: 40, fixTime: "2026-10-03T05:49:24Z" },
    ]);

    const { container } = render(RoutePage);

    await waitFor(() => expect(container.querySelector(".controls-container")).not.toBeNull());
    expect(getPositionPoints).toHaveBeenCalledWith({
      deviceId: 1,
      from: "2026-10-03T05:48:24.000Z",
      to: "2026-10-03T06:17:34.000Z",
      limit: 10000,
    });
  });
});

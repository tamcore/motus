import { describe, it, expect, vi, beforeEach } from "vitest";

// leaflet.heat is a legacy script: evaluating it once extends whatever the
// global `L` is at that moment. Module caching means it never runs again.
const { heatLayer, evaluations } = vi.hoisted(() => ({
  heatLayer: vi.fn(),
  evaluations: { count: 0 },
}));
vi.mock("leaflet.heat", () => {
  evaluations.count++;
  (globalThis as unknown as { L: Record<string, unknown> }).L.heatLayer = heatLayer;
  return {};
});

import { loadHeatLayer } from "./leaflet-heat";

// Each `await import('leaflet')` yields a fresh interop wrapper around the
// same Leaflet exports object (the bundler's __toESM helper).
const leafletExports = { Layer: class {}, version: "1.9.4" };
const wrapper = () => ({ ...leafletExports, default: leafletExports });

describe("loadHeatLayer", () => {
  beforeEach(() => {
    delete (window as unknown as { L?: unknown }).L;
  });

  it("returns the heatLayer factory registered by the plugin", async () => {
    const factory = await loadHeatLayer(wrapper() as never);
    expect(factory).toBe(heatLayer);
  });

  it("still returns the factory on a later visit with a new Leaflet wrapper", async () => {
    // Regression: the heatmap page used to set window.L to the per-import
    // wrapper and read heatLayer from it, so revisiting the page (plugin
    // already evaluated) found no heatLayer and rendered nothing.
    await loadHeatLayer(wrapper() as never);
    const factory = await loadHeatLayer(wrapper() as never);
    expect(factory).toBe(heatLayer);
    expect(evaluations.count).toBe(1);
  });
});

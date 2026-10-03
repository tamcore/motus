import { describe, it, expect, beforeAll } from "vitest";
import * as leaflet from "leaflet";
import {
  GEOFENCE_STYLE,
  geofenceDrawOptions,
  layerToGeoJSON,
} from "./geofence-draw";

// leaflet-draw is a UMD script that extends the global L.
const L: any = (leaflet as any).default ?? leaflet;
(globalThis as any).L = L;

beforeAll(async () => {
  await import("leaflet-draw");
});

describe("geofenceDrawOptions", () => {
  // leaflet-draw 1.0.4's L.GeometryUtil.readableArea assigns to an undeclared
  // variable (`type = typeof isMetric`). In the production bundle (strict-mode
  // ES modules) that throws a ReferenceError on every mouse move while a
  // rectangle is drawn, so the rectangle never resizes on screen. The
  // rectangle tool shows the area by default, which is what triggers it.
  it("disables the area tooltip for rectangles (leaflet-draw strict-mode bug)", () => {
    const opts = geofenceDrawOptions();
    expect(opts.draw.rectangle).toMatchObject({ showArea: false, shapeOptions: GEOFENCE_STYLE });
    // Polygons must not opt into the broken area readout either.
    expect(opts.draw.polygon.showArea ?? false).toBe(false);
  });
});

describe("layerToGeoJSON", () => {
  it("converts a rectangle into a closed polygon ring in [lng, lat] order", () => {
    const rect = L.rectangle([
      [52.5, 13.3],
      [52.55, 13.45],
    ]);
    const geom = layerToGeoJSON(L, rect)!;
    expect(geom.type).toBe("Polygon");
    const ring = geom.coordinates[0];
    expect(ring).toHaveLength(5);
    expect(ring[0]).toEqual(ring[4]);
    expect(ring).toContainEqual([13.3, 52.5]);
    expect(ring).toContainEqual([13.45, 52.55]);
  });

  it("converts a polygon into a closed ring", () => {
    const poly = L.polygon([
      [52.5, 13.3],
      [52.55, 13.35],
      [52.5, 13.4],
    ]);
    const geom = layerToGeoJSON(L, poly)!;
    expect(geom.coordinates[0]).toEqual([
      [13.3, 52.5],
      [13.35, 52.55],
      [13.4, 52.5],
      [13.3, 52.5],
    ]);
  });

  it("approximates a circle with a closed 33-point ring", () => {
    const circle = L.circle([52.52, 13.4], { radius: 500 });
    const geom = layerToGeoJSON(L, circle)!;
    expect(geom.type).toBe("Polygon");
    expect(geom.coordinates[0]).toHaveLength(33);
    expect(geom.coordinates[0][0]).toEqual(geom.coordinates[0][32]);
  });

  it("returns null for unsupported layers", () => {
    expect(layerToGeoJSON(L, L.marker([52.52, 13.4]))).toBeNull();
  });
});

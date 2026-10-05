/**
 * Helpers for the geofences page: leaflet-draw configuration and conversion
 * of drawn Leaflet layers into the GeoJSON payload POSTed to /api/geofences.
 *
 * `L` is passed in because Leaflet is imported dynamically on the page (it
 * touches `window` at import time and cannot be loaded during SSR).
 */

import type { Geofence } from "$lib/types/api";
import { buildPopupElement, type PopupRow } from "./popup";

export const GEOFENCE_STYLE = {
  color: "#00d4ff",
  weight: 2,
  fillOpacity: 0.15,
};

/** GeoJSON layer for a geofence with its name/description/schedule popup bound. Throws on invalid geometry JSON. */
export function geofenceGeoJSON(L: any, gf: Geofence, scheduleName?: string): any {
  const layer = L.geoJSON(JSON.parse(gf.geometry || "{}"), { style: () => GEOFENCE_STYLE });
  const rows: PopupRow[] = [{ type: "heading", text: gf.name }];
  if (gf.description) rows.push({ type: "text", text: gf.description });
  if (scheduleName) rows.push({ type: "note", text: `Schedule: ${scheduleName}`, className: "text-tertiary" });
  return layer.bindPopup(buildPopupElement(rows));
}

/** GeoJSON Polygon geometry as produced by {@link layerToGeoJSON}. */
interface PolygonGeometry {
  type: "Polygon";
  coordinates: [number, number][][];
}

/**
 * Options for `new L.Control.Draw(...)`.
 *
 * `showArea` is disabled for rectangles: leaflet-draw 1.0.4's
 * `L.GeometryUtil.readableArea` assigns to an undeclared variable
 * (`type = typeof isMetric`), which throws a ReferenceError in the strict-mode
 * production bundle on every mouse move, so the rectangle never renders while
 * dragging. Polygons keep the leaflet-draw default (`showArea: false`).
 */
export function geofenceDrawOptions(featureGroup?: unknown) {
  return {
    position: "topleft" as const,
    draw: {
      polyline: false as const,
      marker: false as const,
      circlemarker: false as const,
      rectangle: {
        showArea: false,
        shapeOptions: GEOFENCE_STYLE,
      },
      polygon: {
        allowIntersection: false,
        showArea: false,
        shapeOptions: GEOFENCE_STYLE,
      },
      circle: {
        shapeOptions: GEOFENCE_STYLE,
      },
    },
    edit: {
      featureGroup,
      remove: false, // deletion is handled via the sidebar
    },
  };
}

/**
 * Convert a Leaflet drawn layer into a GeoJSON Polygon geometry.
 * Circles are approximated by a 32-vertex polygon; polygons and rectangles
 * are converted to a closed ring in [lng, lat] order. Returns null for any
 * other layer type.
 */
export function layerToGeoJSON(L: any, layer: any): PolygonGeometry | null {
  if (layer instanceof L.Circle) {
    const center = layer.getLatLng();
    const radius = layer.getRadius();
    const points: [number, number][] = [];
    for (let i = 0; i < 32; i++) {
      const angle = (i / 32) * 2 * Math.PI;
      // Approximate meter offset to degrees
      const dLat = (radius * Math.cos(angle)) / 111320;
      const dLng = (radius * Math.sin(angle)) / (111320 * Math.cos((center.lat * Math.PI) / 180));
      points.push([center.lng + dLng, center.lat + dLat]);
    }
    points.push(points[0]);
    return { type: "Polygon", coordinates: [points] };
  }
  if (layer instanceof L.Polygon) {
    // L.Rectangle extends L.Polygon.
    const coords: [number, number][] = layer.getLatLngs()[0].map((ll: any) => [ll.lng, ll.lat]);
    coords.push(coords[0]);
    return { type: "Polygon", coordinates: [coords] };
  }
  return null;
}

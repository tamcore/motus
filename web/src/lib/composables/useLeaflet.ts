/**
 * Composable for initializing and managing a Leaflet map instance.
 *
 * Handles the dynamic import of Leaflet (SSR-safe), map creation,
 * default tile layer, zoom control positioning, and marker icon fix.
 *
 * Usage:
 *   const { initialize, cleanup, getMap, getLeaflet } = useLeaflet();
 *   onMount(async () => { await initialize(container, { center, zoom }); });
 *   onDestroy(() => { cleanup(); });
 */

import type * as L from "leaflet";

export interface UseLeafletOptions {
  /** Map center as [lat, lng]. Defaults to [49.79, 9.95]. */
  center?: [number, number];
  /** Initial zoom level. Defaults to 6. */
  zoom?: number;
  /** Whether to show the zoom control. Defaults to true (positioned topright). */
  zoomControl?: boolean;
  /** Tile layer URL template. Defaults to OSM. */
  tileUrl?: string;
  /** Tile layer attribution. Defaults to OSM attribution. */
  tileAttribution?: string;
  /** Max zoom for the tile layer. Defaults to 19. */
  maxZoom?: number;
}

const DEFAULT_CENTER: [number, number] = [51.1657, 10.4515];
const DEFAULT_ZOOM = 6;
const DEFAULT_TILE_URL = "https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png";
const DEFAULT_TILE_ATTRIBUTION = "&copy; OpenStreetMap contributors";

// Leaflet marker icon CDN URLs (fixes broken paths in bundled environments)
const MARKER_ICON_URL =
  "https://unpkg.com/leaflet@1.9.4/dist/images/marker-icon.png";
const MARKER_ICON_RETINA_URL =
  "https://unpkg.com/leaflet@1.9.4/dist/images/marker-icon-2x.png";
const MARKER_SHADOW_URL =
  "https://unpkg.com/leaflet@1.9.4/dist/images/marker-shadow.png";

export interface UseLeafletReturn {
  /** Initialize the map on the given container element. */
  initialize: (
    container: HTMLElement,
    options?: UseLeafletOptions,
  ) => Promise<void>;
  /** Remove the map and clean up resources. */
  cleanup: () => void;
  /** Get the current Leaflet map instance (null before initialize). */
  getMap: () => L.Map | null;
  /** Get the Leaflet library module (null before initialize). */
  getLeaflet: () => typeof import("leaflet") | null;
  /** Get the tile layer instance (null before initialize). */
  getTileLayer: () => L.TileLayer | null;
}

export function useLeaflet(): UseLeafletReturn {
  let map: L.Map | null = null;
  let leaflet: typeof import("leaflet") | null = null;
  let tileLayer: L.TileLayer | null = null;

  async function initialize(
    container: HTMLElement,
    options: UseLeafletOptions = {},
  ): Promise<void> {
    const {
      center = DEFAULT_CENTER,
      zoom = DEFAULT_ZOOM,
      zoomControl = true,
      tileUrl = DEFAULT_TILE_URL,
      tileAttribution = DEFAULT_TILE_ATTRIBUTION,
      maxZoom = DEFAULT_TILE_MAX_ZOOM,
    } = options;

    // Dynamic import (SSR-safe)
    const L = await import("leaflet");
    await import("leaflet/dist/leaflet.css");
    leaflet = L;

    // Fix default marker icon paths for bundled environments
    fixMarkerIcons(L);

    // Create map with zoom control disabled initially so we can position it
    map = L.map(container, {
      center,
      zoom,
      zoomControl: false,
    });

    if (zoomControl) {
      L.control.zoom({ position: "topright" }).addTo(map);
    }

    tileLayer = L.tileLayer(tileUrl, {
      attribution: tileAttribution,
      maxZoom,
    }).addTo(map);
  }

  function cleanup(): void {
    if (map) {
      map.remove();
      map = null;
    }
    leaflet = null;
    tileLayer = null;
  }

  function getMap(): L.Map | null {
    return map;
  }

  function getLeaflet(): typeof import("leaflet") | null {
    return leaflet;
  }

  function getTileLayer(): L.TileLayer | null {
    return tileLayer;
  }

  return { initialize, cleanup, getMap, getLeaflet, getTileLayer };
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

const DEFAULT_TILE_MAX_ZOOM = 19;

/**
 * Fix Leaflet's default marker icon paths which break in bundled
 * environments (Vite, webpack, etc.) due to missing asset references.
 */
function fixMarkerIcons(L: typeof import("leaflet")): void {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const proto = L.Icon.Default.prototype as any;
  if (proto._getIconUrl) {
    delete proto._getIconUrl;
  }
  L.Icon.Default.mergeOptions({
    iconRetinaUrl: MARKER_ICON_RETINA_URL,
    iconUrl: MARKER_ICON_URL,
    shadowUrl: MARKER_SHADOW_URL,
  });
}

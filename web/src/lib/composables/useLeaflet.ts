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
import markerIconUrl from "leaflet/dist/images/marker-icon.png";
import markerIconRetinaUrl from "leaflet/dist/images/marker-icon-2x.png";
import markerShadowUrl from "leaflet/dist/images/marker-shadow.png";

export interface UseLeafletOptions {
  /** Map center as [lat, lng]. Defaults to [49.79, 9.95]. */
  center?: [number, number];
  /** Initial zoom level. Defaults to 6. */
  zoom?: number;
  /** Whether to show the zoom control. Defaults to true (positioned topright). */
  zoomControl?: boolean;
  /** Tile layer URL template. Defaults to BKG TopPlusOpen (web). */
  tileUrl?: string;
  /** Tile layer attribution. Defaults to BKG TopPlusOpen attribution. */
  tileAttribution?: string;
  /** Max zoom for the tile layer. Defaults to 18 (BKG TopPlusOpen max). */
  maxZoom?: number;
}

const DEFAULT_CENTER: [number, number] = [51.1657, 10.4515];
const DEFAULT_ZOOM = 6;
// BKG TopPlusOpen (EU-hosted, German federal mapping service, no API key,
// open license dl-de/by-2.0). WMTS RESTful endpoint: no {s} subdomain and
// note the {z}/{y}/{x} order. "web" is the full-color base layer.
const DEFAULT_TILE_URL =
  "https://sgx.geodatenzentrum.de/wmts_topplus_open/tile/1.0.0/web/default/WEBMERCATOR/{z}/{y}/{x}.png";
const DEFAULT_TILE_ATTRIBUTION =
  '&copy; <a href="https://www.bkg.bund.de">BKG</a> TopPlusOpen';

// Leaflet marker icon assets bundled locally via Vite (fixes broken paths in
// bundled environments without any runtime CDN dependency).
export const LEAFLET_MARKER_ICON_URL = markerIconUrl;
export const LEAFLET_MARKER_ICON_RETINA_URL = markerIconRetinaUrl;
export const LEAFLET_MARKER_SHADOW_URL = markerShadowUrl;

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

const DEFAULT_TILE_MAX_ZOOM = 18;

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
    iconRetinaUrl: LEAFLET_MARKER_ICON_RETINA_URL,
    iconUrl: LEAFLET_MARKER_ICON_URL,
    shadowUrl: LEAFLET_MARKER_SHADOW_URL,
  });
}

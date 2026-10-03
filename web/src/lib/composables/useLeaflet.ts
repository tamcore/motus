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
}

const DEFAULT_CENTER: [number, number] = [51.1657, 10.4515];
const DEFAULT_ZOOM = 6;
const TILE_URL = "https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png";
const TILE_ATTRIBUTION = '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors';
const TILE_MAX_ZOOM = 19;

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
}

export function useLeaflet(): UseLeafletReturn {
  let map: L.Map | null = null;
  let leaflet: typeof import("leaflet") | null = null;

  async function initialize(
    container: HTMLElement,
    options: UseLeafletOptions = {},
  ): Promise<void> {
    const {
      center = DEFAULT_CENTER,
      zoom = DEFAULT_ZOOM,
      zoomControl = true,
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

    L.tileLayer(TILE_URL, {
      attribution: TILE_ATTRIBUTION,
      maxZoom: TILE_MAX_ZOOM,
    }).addTo(map);
  }

  function cleanup(): void {
    if (map) {
      map.remove();
      map = null;
    }
    leaflet = null;
  }

  function getMap(): L.Map | null {
    return map;
  }

  function getLeaflet(): typeof import("leaflet") | null {
    return leaflet;
  }

  return { initialize, cleanup, getMap, getLeaflet };
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

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
    iconRetinaUrl: markerIconRetinaUrl,
    iconUrl: markerIconUrl,
    shadowUrl: markerShadowUrl,
  });
}

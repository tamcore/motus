import type * as L from "leaflet";
import markerIconUrl from "leaflet/dist/images/marker-icon.png";
import markerIconRetinaUrl from "leaflet/dist/images/marker-icon-2x.png";
import markerShadowUrl from "leaflet/dist/images/marker-shadow.png";

const DEFAULT_CENTER: [number, number] = [51.1657, 10.4515];
const DEFAULT_ZOOM = 6;
const TILE_URL = "https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png";
const TILE_ATTRIBUTION = '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors';
const TILE_MAX_ZOOM = 19;

export function useLeaflet() {
  let map: L.Map | null = null;
  let leaflet: typeof import("leaflet") | null = null;

  async function initialize(
    container: HTMLElement,
    { center = DEFAULT_CENTER, zoom = DEFAULT_ZOOM }: { center?: [number, number]; zoom?: number } = {},
  ): Promise<void> {
    const L = await import("leaflet");
    await import("leaflet/dist/leaflet.css");
    leaflet = L;

    fixMarkerIcons(L);

    map = L.map(container, { center, zoom, zoomControl: false });
    L.control.zoom({ position: "topright" }).addTo(map);
    L.tileLayer(TILE_URL, {
      attribution: TILE_ATTRIBUTION,
      maxZoom: TILE_MAX_ZOOM,
    }).addTo(map);
  }

  function cleanup(): void {
    map?.remove();
    map = null;
    leaflet = null;
  }

  return {
    initialize,
    cleanup,
    getMap: (): L.Map | null => map,
    getLeaflet: (): typeof import("leaflet") | null => leaflet,
  };
}

/** Bundlers break Leaflet's default marker icon paths. */
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

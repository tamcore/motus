/**
 * Map overlay definitions for OSM-based tile layers.
 *
 * Each overlay can be applied on top of (or replace) the base tile layer
 * on the map page. The `id` is persisted in user settings.
 */

export interface MapOverlay {
  /** Unique identifier stored in settings. */
  id: string;
  /** Human-readable display name. */
  name: string;
  /** Leaflet tile URL template. Empty string for "none". */
  url: string;
  /** Attribution string for the tile provider. */
  attribution: string;
  /** Maximum zoom level supported by this tile server. */
  maxZoom: number;
}

export const MAP_OVERLAYS: readonly MapOverlay[] = [
  {
    id: "none",
    name: "None",
    url: "",
    attribution: "",
    maxZoom: 19,
  },
  {
    id: "humanitarian",
    name: "Humanitarian (HOT)",
    url: "https://a.tile.openstreetmap.fr/hot/{z}/{x}/{y}.png",
    attribution:
      '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors, Tiles style by <a href="https://www.hotosm.org/">HOT</a>',
    maxZoom: 19,
  },
  {
    id: "topo",
    name: "OpenTopoMap",
    url: "https://{s}.tile.opentopomap.org/{z}/{x}/{y}.png",
    attribution:
      '&copy; <a href="https://opentopomap.org">OpenTopoMap</a> (<a href="https://creativecommons.org/licenses/by-sa/3.0/">CC-BY-SA</a>)',
    maxZoom: 17,
  },
  {
    id: "cyclosm",
    name: "CyclOSM (Cycling)",
    url: "https://{s}.tile-cyclosm.openstreetmap.fr/cyclosm/{z}/{x}/{y}.png",
    attribution:
      '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors, <a href="https://www.cyclosm.org/">CyclOSM</a>',
    maxZoom: 20,
  },
  {
    id: "railway",
    name: "OpenRailwayMap",
    url: "https://{s}.tiles.openrailwaymap.org/standard/{z}/{x}/{y}.png",
    attribution:
      '&copy; <a href="https://www.openrailwaymap.org/">OpenRailwayMap</a>, <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
    maxZoom: 19,
  },
] as const;

/**
 * Look up an overlay definition by its id.
 * Returns undefined if no overlay matches.
 */
export function getOverlayById(id: string): MapOverlay | undefined {
  return MAP_OVERLAYS.find((o) => o.id === id);
}

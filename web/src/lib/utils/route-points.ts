import type { PositionPoint } from "$lib/types/api";

/** Position fields the route and replay views use. Speed is in km/h. */
export interface RoutePosition {
  fixTime: string;
  latitude: number;
  longitude: number;
  speed: number;
  course: number | null;
  altitude: number | null;
}

export function toRoutePositions(points: readonly PositionPoint[]): RoutePosition[] {
  return points.map((p) => ({
    latitude: p.lat,
    longitude: p.lon,
    speed: p.speed,
    fixTime: p.fixTime,
    course: p.course ?? null,
    altitude: p.altitude ?? null,
  }));
}

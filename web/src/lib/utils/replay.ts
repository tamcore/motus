import type { RoutePosition } from "./route-points";

/** Linear lat/lng interpolation; course turns the short way round. */
export function interpolatePosition(
  p1: RoutePosition,
  p2: RoutePosition,
  fraction: number,
): { lat: number; lng: number; course: number } {
  const lat = p1.latitude + (p2.latitude - p1.latitude) * fraction;
  const lng = p1.longitude + (p2.longitude - p1.longitude) * fraction;
  const c1 = p1.course ?? 0;
  const c2 = p2.course ?? 0;
  let diff = c2 - c1;
  if (diff > 180) diff -= 360;
  if (diff < -180) diff += 360;
  return { lat, lng, course: c1 + diff * fraction };
}

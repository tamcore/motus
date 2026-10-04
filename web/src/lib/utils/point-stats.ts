import type { PositionPoint } from "$lib/types/api";

interface PointStats {
  /** Epoch ms of the earliest valid fixTime, NaN when none is valid. */
  earliest: number;
  latest: number;
  avgSpeed: number;
  maxSpeed: number;
}

// Single loop: Math.min/max(...arr) throws RangeError for large arrays.
export function pointStats(points: readonly PositionPoint[]): PointStats | null {
  if (points.length === 0) return null;
  let earliest = Infinity;
  let latest = -Infinity;
  let sum = 0;
  let maxSpeed = -Infinity;
  for (const p of points) {
    const t = Date.parse(p.fixTime);
    if (!isNaN(t)) {
      if (t < earliest) earliest = t;
      if (t > latest) latest = t;
    }
    sum += p.speed;
    if (p.speed > maxSpeed) maxSpeed = p.speed;
  }
  if (earliest === Infinity) earliest = latest = NaN;
  return { earliest, latest, avgSpeed: sum / points.length, maxSpeed };
}

/** Trip fields needed to link to the route and replay views. */
export interface TripRange {
  deviceId: number;
  startTime: string;
  endTime: string;
}

/**
 * Link to a per-trip report view. The times are percent-encoded: the server
 * emits RFC 3339 offsets such as "+02:00", and an unencoded "+" is decoded to
 * a space, which the API rejects.
 */
export function tripLink(path: "/reports/route" | "/reports/replay", trip: TripRange): string {
  const query = new URLSearchParams({
    deviceId: String(trip.deviceId),
    from: trip.startTime,
    to: trip.endTime,
  });
  return `${path}?${query}`;
}

// "2026-10-03T07:48:24 02:00": a "+02:00" offset decoded from an unencoded link.
const SPACE_OFFSET = /^(.*T[\d:.]+) (\d{2}:?\d{2})$/;

/**
 * Normalize a from/to query parameter to a UTC ISO timestamp for the API.
 * Repairs a "+" offset that was decoded to a space (links created before
 * tripLink encoded them). Returns null when the value is missing or invalid.
 */
export function normalizeTimeParam(value: string | null): string | null {
  if (!value) return null;
  const repaired = value.trim().replace(SPACE_OFFSET, "$1+$2");
  const time = new Date(repaired);
  return Number.isNaN(time.getTime()) ? null : time.toISOString();
}

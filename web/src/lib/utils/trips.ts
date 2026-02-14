export interface Position {
  timestamp?: string;
  fixTime?: string;
  latitude: number;
  longitude: number;
  speed: number;
  address?: string | null;
  deviceId?: number;
}

/**
 * Get the time string from a position, handling both
 * `fixTime` (from API) and `timestamp` (legacy) fields.
 */
export function getPositionTime(pos: Position): string {
  return pos.fixTime || pos.timestamp || "";
}

export interface Trip {
  id: string;
  deviceId: number;
  deviceName: string;
  startTime: string;
  endTime: string;
  duration: number;
  distance: number;
  avgSpeed: number;
  maxSpeed: number;
  positions: Position[];
}

const STOP_THRESHOLD = 5; // km/h
const MIN_STOP_DURATION = 300; // 5 minutes in seconds - ignore brief stops (traffic lights)
const MIN_TRIP_DURATION = 60; // seconds
const MAX_POSITION_GAP = 3600; // 1 hour in seconds - split trip if gap exceeds this

export function haversineDistance(
  lat1: number,
  lon1: number,
  lat2: number,
  lon2: number,
): number {
  const R = 6371;
  const dLat = ((lat2 - lat1) * Math.PI) / 180;
  const dLon = ((lon2 - lon1) * Math.PI) / 180;
  const a =
    Math.sin(dLat / 2) * Math.sin(dLat / 2) +
    Math.cos((lat1 * Math.PI) / 180) *
      Math.cos((lat2 * Math.PI) / 180) *
      Math.sin(dLon / 2) *
      Math.sin(dLon / 2);
  const c = 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
  return R * c;
}

export function calculateTotalDistance(positions: Position[]): number {
  let total = 0;
  for (let i = 1; i < positions.length; i++) {
    total += haversineDistance(
      positions[i - 1].latitude,
      positions[i - 1].longitude,
      positions[i].latitude,
      positions[i].longitude,
    );
  }
  return total;
}

/**
 * Create a Trip object from a list of positions.
 */
function createTrip(
  positions: Position[],
  deviceName: string,
  deviceId: number,
  tripIndex: number,
): Trip {
  const duration =
    (new Date(getPositionTime(positions[positions.length - 1])).getTime() -
      new Date(getPositionTime(positions[0])).getTime()) /
    1000;

  const movingSpeeds = positions
    .map((p) => p.speed ?? 0)
    .filter((s) => s > STOP_THRESHOLD);

  return {
    id: `trip-${deviceId}-${tripIndex}`,
    deviceId,
    deviceName,
    startTime: getPositionTime(positions[0]),
    endTime: getPositionTime(positions[positions.length - 1]),
    duration,
    distance: calculateTotalDistance(positions),
    avgSpeed:
      movingSpeeds.length > 0
        ? movingSpeeds.reduce((sum, s) => sum + s, 0) / movingSpeeds.length
        : 0,
    maxSpeed: Math.max(...positions.map((p) => p.speed ?? 0)),
    positions: [...positions],
  };
}

/**
 * Finalize and push a trip if it meets the minimum duration requirement.
 */
function finalizeTrip(
  currentTrip: Position[],
  trips: Trip[],
  deviceName: string,
  deviceId: number,
): void {
  if (currentTrip.length < 2) return;

  const duration =
    (new Date(getPositionTime(currentTrip[currentTrip.length - 1])).getTime() -
      new Date(getPositionTime(currentTrip[0])).getTime()) /
    1000;

  if (duration >= MIN_TRIP_DURATION) {
    trips.push(createTrip(currentTrip, deviceName, deviceId, trips.length));
  }
}

/**
 * Detect trips from a list of positions. A trip starts when speed exceeds
 * STOP_THRESHOLD and ends when:
 * 1. There's an extended stop (MIN_STOP_DURATION), or
 * 2. There's a large time gap between positions (MAX_POSITION_GAP)
 * Brief stops like traffic lights are included in the trip.
 * Time gaps (e.g., device powered off) split trips automatically.
 */
export function detectTrips(
  positions: Position[],
  deviceName: string,
  deviceId: number,
): Trip[] {
  if (positions.length === 0) return [];

  // Sort positions by time to ensure correct ordering.
  const sorted = [...positions].sort(
    (a, b) =>
      new Date(getPositionTime(a)).getTime() -
      new Date(getPositionTime(b)).getTime(),
  );

  const trips: Trip[] = [];
  let currentTrip: Position[] = [];
  let stopStartTime: Date | null = null;
  let lastPositionTime: Date | null = null;

  sorted.forEach((pos) => {
    const speed = pos.speed ?? 0;
    const time = new Date(getPositionTime(pos));

    // Check for large time gap (device off, GPS loss, etc.)
    if (lastPositionTime && currentTrip.length > 0) {
      const gap = (time.getTime() - lastPositionTime.getTime()) / 1000;
      if (gap >= MAX_POSITION_GAP) {
        // Large gap detected - finalize current trip
        finalizeTrip(currentTrip, trips, deviceName, deviceId);
        currentTrip = [];
        stopStartTime = null;
      }
    }

    lastPositionTime = time;

    if (speed > STOP_THRESHOLD) {
      // Moving - reset stop timer and add to current trip.
      stopStartTime = null;
      currentTrip.push(pos);
    } else {
      // Stopped or slow.
      if (currentTrip.length > 0) {
        if (stopStartTime === null) {
          // Just stopped - start the stop timer.
          stopStartTime = time;
          currentTrip.push(pos);
        } else {
          // Already stopped - check how long.
          const stopDuration =
            (time.getTime() - stopStartTime.getTime()) / 1000;

          if (stopDuration >= MIN_STOP_DURATION) {
            // Extended stop (5+ minutes) - finalize current trip.
            finalizeTrip(currentTrip, trips, deviceName, deviceId);
            currentTrip = [];
            stopStartTime = null;
          } else {
            // Brief stop (traffic light, etc.) - keep in trip.
            currentTrip.push(pos);
          }
        }
      }
      // If no current trip, ignore stationary positions.
    }
  });

  // Handle trip in progress at end of data.
  finalizeTrip(currentTrip, trips, deviceName, deviceId);

  return trips;
}

export function formatDate(date: string): string {
  return new Date(date).toLocaleString();
}

export function formatDuration(seconds: number): string {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}

export function formatDistance(km: number): string {
  return `${km.toFixed(2)} km`;
}

export function exportTripsToCSV(trips: Trip[]): void {
  const headers = [
    "Device",
    "Start Time",
    "End Time",
    "Duration (s)",
    "Distance (km)",
    "Max Speed (km/h)",
  ];
  const rows = trips.map((trip) => [
    trip.deviceName,
    trip.startTime,
    trip.endTime,
    String(trip.duration),
    trip.distance.toFixed(2),
    trip.maxSpeed.toFixed(1),
  ]);

  const csv = [headers.join(","), ...rows.map((row) => row.join(","))].join(
    "\n",
  );

  const blob = new Blob([csv], { type: "text/csv" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `motus-trips-${new Date().toISOString().slice(0, 10)}.csv`;
  a.click();
  URL.revokeObjectURL(url);
}

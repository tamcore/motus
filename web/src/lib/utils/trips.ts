import { downloadCSV } from "./download";
import { dateValue } from "./date-range";

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
}

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

/** Sum of haversine distances (km) between consecutive points. */
export function pathDistance(
  points: readonly { latitude: number; longitude: number }[],
): number {
  let total = 0;
  for (let i = 1; i < points.length; i++) {
    total += haversineDistance(
      points[i - 1].latitude,
      points[i - 1].longitude,
      points[i].latitude,
      points[i].longitude,
    );
  }
  return total;
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

  downloadCSV(headers, rows, `motus-trips-${dateValue(new Date())}.csv`);
}

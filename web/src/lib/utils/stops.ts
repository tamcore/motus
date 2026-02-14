import type { Position } from './trips';
import { getPositionTime } from './trips';

export interface Stop {
	id: string;
	deviceName: string;
	latitude: number;
	longitude: number;
	address: string;
	arrivalTime: string;
	departureTime: string;
	duration: number; // seconds
}

const STOP_THRESHOLD = 1; // km/h - device is considered stopped below this
const MIN_STOP_DURATION = 300; // 5 minutes in seconds

/**
 * Format coordinates as a fallback address string.
 */
function coordinateFallback(lat: number, lon: number): string {
	return `${lat.toFixed(5)}, ${lon.toFixed(5)}`;
}

/**
 * Get the best address for a set of stop positions. Prefers the server-provided
 * address field (from server-side geocoding) on any position in the stop.
 * Falls back to a coordinate string if no address is available.
 */
function getStopAddress(positions: Position[], avgLat: number, avgLon: number): string {
	// Find the first position that has a server-provided address.
	for (const pos of positions) {
		if (pos.address) {
			return pos.address;
		}
	}
	return coordinateFallback(avgLat, avgLon);
}

/**
 * Detect stops from a list of positions. A stop is when a device remains
 * below STOP_THRESHOLD speed for at least MIN_STOP_DURATION seconds.
 *
 * Addresses come from the server-side geocoding (position.address field).
 * No client-side Nominatim calls are made.
 */
export function detectStops(
	positions: Position[],
	deviceName: string
): Stop[] {
	if (positions.length === 0) return [];

	// Sort positions by time.
	const sorted = [...positions].sort(
		(a, b) =>
			new Date(getPositionTime(a)).getTime() - new Date(getPositionTime(b)).getTime()
	);

	const stops: Stop[] = [];
	let stopStart: Position | null = null;
	let stopPositions: Position[] = [];

	for (const pos of sorted) {
		const speed = pos.speed ?? 0;

		if (speed < STOP_THRESHOLD) {
			if (!stopStart) {
				stopStart = pos;
			}
			stopPositions.push(pos);
		} else if (stopStart && stopPositions.length > 0) {
			// Moving again - check if the stop was long enough.
			const duration =
				(new Date(getPositionTime(pos)).getTime() -
					new Date(getPositionTime(stopStart)).getTime()) /
				1000;

			if (duration >= MIN_STOP_DURATION) {
				const avgLat =
					stopPositions.reduce((sum, p) => sum + p.latitude, 0) / stopPositions.length;
				const avgLon =
					stopPositions.reduce((sum, p) => sum + p.longitude, 0) / stopPositions.length;
				const address = getStopAddress(stopPositions, avgLat, avgLon);

				stops.push({
					id: `stop-${stops.length}`,
					deviceName,
					latitude: avgLat,
					longitude: avgLon,
					address,
					arrivalTime: getPositionTime(stopStart),
					departureTime: getPositionTime(stopPositions[stopPositions.length - 1]),
					duration
				});
			}

			stopStart = null;
			stopPositions = [];
		}
	}

	// Handle a stop that is still in progress at end of data.
	if (stopStart && stopPositions.length > 0) {
		const last = stopPositions[stopPositions.length - 1];
		const duration =
			(new Date(getPositionTime(last)).getTime() -
				new Date(getPositionTime(stopStart)).getTime()) /
			1000;

		if (duration >= MIN_STOP_DURATION) {
			const avgLat =
				stopPositions.reduce((sum, p) => sum + p.latitude, 0) / stopPositions.length;
			const avgLon =
				stopPositions.reduce((sum, p) => sum + p.longitude, 0) / stopPositions.length;
			const address = getStopAddress(stopPositions, avgLat, avgLon);

			stops.push({
				id: `stop-${stops.length}`,
				deviceName,
				latitude: avgLat,
				longitude: avgLon,
				address,
				arrivalTime: getPositionTime(stopStart),
				departureTime: getPositionTime(last),
				duration
			});
		}
	}

	return stops;
}

/**
 * Export stops to CSV and trigger a browser download.
 */
export function exportStopsToCSV(stops: Stop[]): void {
	const headers = [
		'Device',
		'Address',
		'Arrival Time',
		'Departure Time',
		'Duration (s)',
		'Latitude',
		'Longitude'
	];
	const rows = stops.map((stop) => [
		`"${stop.deviceName}"`,
		`"${stop.address.replace(/"/g, '""')}"`,
		stop.arrivalTime,
		stop.departureTime,
		String(Math.round(stop.duration)),
		stop.latitude.toFixed(6),
		stop.longitude.toFixed(6)
	]);

	const csv = [headers.join(','), ...rows.map((row) => row.join(','))].join('\n');

	const blob = new Blob([csv], { type: 'text/csv' });
	const url = URL.createObjectURL(blob);
	const a = document.createElement('a');
	a.href = url;
	a.download = `motus-stops-${new Date().toISOString().slice(0, 10)}.csv`;
	a.click();
	URL.revokeObjectURL(url);
}

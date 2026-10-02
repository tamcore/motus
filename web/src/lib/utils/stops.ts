import { downloadCSV } from './download';

export interface Stop {
	id: string;
	deviceId: number;
	deviceName: string;
	latitude: number;
	longitude: number;
	address: string;
	arrivalTime: string;
	departureTime: string;
	duration: number; // seconds
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

	downloadCSV(headers, rows, `motus-stops-${new Date().toISOString().slice(0, 10)}.csv`);
}

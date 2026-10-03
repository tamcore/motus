import type { RoutePosition } from './route-points';
import { downloadFile } from './download';

export function generateGPX(positions: RoutePosition[], name: string): string {
	const trackPoints = positions
		.map(
			(p) =>
				`      <trkpt lat="${p.latitude}" lon="${p.longitude}">
        <time>${p.fixTime}</time>
        <speed>${p.speed ?? 0}</speed>
      </trkpt>`
		)
		.join('\n');

	return `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="Motus GPS Tracker">
  <trk>
    <name>${name}</name>
    <trkseg>
${trackPoints}
    </trkseg>
  </trk>
</gpx>`;
}

export function downloadGPX(positions: RoutePosition[], filename: string): void {
	const gpx = generateGPX(positions, filename);
	downloadFile(gpx, 'application/gpx+xml', `${filename}.gpx`);
}

import type { Position } from './trips';
import { getPositionTime } from './trips';

export function generateGPX(positions: Position[], name: string): string {
	const trackPoints = positions
		.map(
			(p) =>
				`      <trkpt lat="${p.latitude}" lon="${p.longitude}">
        <time>${getPositionTime(p)}</time>
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

export function downloadGPX(positions: Position[], filename: string): void {
	const gpx = generateGPX(positions, filename);
	const blob = new Blob([gpx], { type: 'application/gpx+xml' });
	const url = URL.createObjectURL(blob);
	const a = document.createElement('a');
	a.href = url;
	a.download = `${filename}.gpx`;
	a.click();
	URL.revokeObjectURL(url);
}

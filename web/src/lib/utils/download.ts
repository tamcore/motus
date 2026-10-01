export function downloadFile(content: string, type: string, filename: string): void {
	const url = URL.createObjectURL(new Blob([content], { type }));
	const a = document.createElement('a');
	a.href = url;
	a.download = filename;
	a.click();
	URL.revokeObjectURL(url);
}

export function downloadCSV(headers: string[], rows: string[][], filename: string): void {
	downloadFile([headers, ...rows].map((row) => row.join(',')).join('\n'), 'text/csv', filename);
}

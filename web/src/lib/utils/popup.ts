export type PopupRow =
	| { type: 'heading'; text: string }
	| { type: 'text'; text: string }
	| { type: 'note'; text: string; className?: string };

/**
 * Builds a Leaflet popup element from rows of typed content using DOM
 * construction (textContent only), so user-controlled values cannot inject
 * HTML or execute scripts regardless of what the server stored.
 */
export function buildPopupElement(rows: PopupRow[]): HTMLElement {
	const container = document.createElement('div');

	for (const row of rows) {
		if (row.type === 'heading') {
			const strong = document.createElement('strong');
			strong.textContent = row.text;
			container.appendChild(strong);
		} else {
			const br = document.createElement('br');
			container.appendChild(br);

			if (row.type === 'note') {
				const small = document.createElement('small');
				if (row.className) small.className = row.className;
				small.textContent = row.text;
				container.appendChild(small);
			} else {
				container.appendChild(document.createTextNode(row.text));
			}
		}
	}

	return container;
}

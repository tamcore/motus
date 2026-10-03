import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import BatteryIndicator from '$lib/components/BatteryIndicator.svelte';

function renderLevel(level: number | null | undefined) {
	const { container } = render(BatteryIndicator, { props: { level } });
	return container.querySelector('.battery');
}

describe('BatteryIndicator', () => {
	it('shows the percentage', () => {
		const el = renderLevel(64);
		expect(el).not.toBeNull();
		expect(el!.textContent).toContain('64%');
		expect(el!.classList.contains('battery-low')).toBe(false);
		expect(el!.getAttribute('aria-label')).toBe('Battery 64%');
	});

	it('rounds the displayed percentage', () => {
		const el = renderLevel(87.4);
		expect(el!.textContent).toContain('87%');
		expect(el!.getAttribute('aria-label')).toBe('Battery 87%');
	});

	it('marks low battery visually and for screen readers', () => {
		const el = renderLevel(12);
		expect(el!.classList.contains('battery-low')).toBe(true);
		expect(el!.textContent).toContain('12%');
		expect(el!.getAttribute('aria-label')).toBe('Battery 12%, low');
	});

	it('treats 0% as low and 20% and above as not low', () => {
		expect(renderLevel(0)!.classList.contains('battery-low')).toBe(true);
		expect(renderLevel(20)!.classList.contains('battery-low')).toBe(false);
		expect(renderLevel(100)!.classList.contains('battery-low')).toBe(false);
	});

	it('compares the unrounded level against the 20% threshold', () => {
		// 19.6 displays as 20% but is still below the threshold.
		const el = renderLevel(19.6);
		expect(el!.textContent).toContain('20%');
		expect(el!.classList.contains('battery-low')).toBe(true);
		expect(el!.getAttribute('aria-label')).toBe('Battery 20%, low');
	});

	it('renders nothing when the level is unknown', () => {
		for (const level of [null, undefined, Number.NaN]) {
			const { container } = render(BatteryIndicator, { props: { level } });
			expect(container.querySelector('.battery')).toBeNull();
			expect(container.textContent?.trim()).toBe('');
		}
	});

	it('renders a dash placeholder when requested and the level is unknown', () => {
		const { container } = render(BatteryIndicator, { props: { level: undefined, placeholder: true } });
		expect(container.querySelector('.battery')).toBeNull();
		const ph = container.querySelector('.battery-unknown');
		expect(ph).not.toBeNull();
		expect(ph!.textContent).toBe('—');
	});
});

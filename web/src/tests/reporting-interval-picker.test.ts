import { describe, it, expect } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import ReportingIntervalHost from './helpers/ReportingIntervalHost.svelte';

function boundValue(container: HTMLElement): string {
	return container.querySelector('#bound-value')!.textContent ?? '';
}

function presetButtons(container: HTMLElement): HTMLButtonElement[] {
	return Array.from(container.querySelectorAll<HTMLButtonElement>('button.interval-preset'));
}

describe('ReportingIntervalPicker', () => {
	it('offers 5 sec, 20 sec, 1 min, 5 min, 10 min and Custom', () => {
		const { container } = render(ReportingIntervalHost);
		expect(presetButtons(container).map((b) => b.textContent?.trim())).toEqual([
			'5 sec',
			'20 sec',
			'1 min',
			'5 min',
			'10 min',
			'Custom'
		]);
	});

	it('defaults to 1 min when no value is set', async () => {
		const { container } = render(ReportingIntervalHost);
		await tick();
		expect(boundValue(container)).toBe('60');
		const oneMin = presetButtons(container).find((b) => b.textContent?.trim() === '1 min')!;
		expect(oneMin.getAttribute('aria-pressed')).toBe('true');
		expect(container.querySelector('input[name="frequency"]')).toBeNull();
	});

	it('sets the interval in seconds when a preset is clicked', async () => {
		const { container } = render(ReportingIntervalHost);
		for (const [label, seconds] of [
			['5 sec', '5'],
			['20 sec', '20'],
			['5 min', '300'],
			['10 min', '600']
		]) {
			const btn = presetButtons(container).find((b) => b.textContent?.trim() === label)!;
			await fireEvent.click(btn);
			await tick();
			expect(boundValue(container)).toBe(seconds);
			expect(btn.getAttribute('aria-pressed')).toBe('true');
		}
	});

	it('shows a seconds input for a custom interval', async () => {
		const { container } = render(ReportingIntervalHost);
		const custom = presetButtons(container).find((b) => b.textContent?.trim() === 'Custom')!;
		await fireEvent.click(custom);
		await tick();
		expect(custom.getAttribute('aria-pressed')).toBe('true');

		const input = container.querySelector<HTMLInputElement>('input[name="frequency"]')!;
		expect(input).not.toBeNull();
		await fireEvent.input(input, { target: { value: '45' } });
		await tick();
		expect(boundValue(container)).toBe('45');
	});

	it('starts in custom mode for a non-preset value', async () => {
		const { container } = render(ReportingIntervalHost, { props: { value: '45' } });
		await tick();
		const custom = presetButtons(container).find((b) => b.textContent?.trim() === 'Custom')!;
		expect(custom.getAttribute('aria-pressed')).toBe('true');
		expect(container.querySelector<HTMLInputElement>('input[name="frequency"]')!.value).toBe('45');
	});

	it('keeps custom mode while typing a value that matches a preset', async () => {
		const { container } = render(ReportingIntervalHost);
		await fireEvent.click(presetButtons(container).find((b) => b.textContent?.trim() === 'Custom')!);
		await tick();
		const input = container.querySelector<HTMLInputElement>('input[name="frequency"]')!;
		await fireEvent.input(input, { target: { value: '60' } });
		await tick();
		expect(boundValue(container)).toBe('60');
		expect(container.querySelector('input[name="frequency"]')).not.toBeNull();
		const custom = presetButtons(container).find((b) => b.textContent?.trim() === 'Custom')!;
		expect(custom.getAttribute('aria-pressed')).toBe('true');
	});

	// Regression: default and custom mode were derived only at mount and went
	// stale when the parent changed the bound value (e.g. a form reset).
	it('returns to the 1 min default when the parent resets the value', async () => {
		const { container } = render(ReportingIntervalHost);
		await fireEvent.click(presetButtons(container).find((b) => b.textContent?.trim() === 'Custom')!);
		await tick();
		await fireEvent.input(container.querySelector<HTMLInputElement>('input[name="frequency"]')!, {
			target: { value: '45' }
		});
		await tick();
		expect(boundValue(container)).toBe('45');

		await fireEvent.click(container.querySelector<HTMLButtonElement>('#parent-reset')!);
		await tick();

		expect(boundValue(container)).toBe('60');
		const oneMin = presetButtons(container).find((b) => b.textContent?.trim() === '1 min')!;
		expect(oneMin.getAttribute('aria-pressed')).toBe('true');
		expect(container.querySelector('input[name="frequency"]')).toBeNull();
	});

	it('follows external changes between preset and custom values', async () => {
		const { container, rerender } = render(ReportingIntervalHost, { props: { externalValue: '45' } });
		await tick();

		await fireEvent.click(container.querySelector<HTMLButtonElement>('#parent-set')!);
		await tick();
		const custom = presetButtons(container).find((b) => b.textContent?.trim() === 'Custom')!;
		expect(custom.getAttribute('aria-pressed')).toBe('true');
		expect(container.querySelector<HTMLInputElement>('input[name="frequency"]')!.value).toBe('45');

		await rerender({ externalValue: '300' });
		await fireEvent.click(container.querySelector<HTMLButtonElement>('#parent-set')!);
		await tick();
		expect(boundValue(container)).toBe('300');
		const fiveMin = presetButtons(container).find((b) => b.textContent?.trim() === '5 min')!;
		expect(fiveMin.getAttribute('aria-pressed')).toBe('true');
		expect(container.querySelector('input[name="frequency"]')).toBeNull();
	});
});

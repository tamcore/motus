import { describe, it, expect } from 'vitest';
import { buildPopupElement } from '$lib/utils/popup';

describe('buildPopupElement', () => {
	it('renders heading text safely — does not create script or img elements', () => {
		const el = buildPopupElement([{ type: 'heading', text: '<img src=x onerror=alert(1)>' }]);
		expect(el.querySelectorAll('img, script').length).toBe(0);
		expect(el.textContent).toContain('<img src=x onerror=alert(1)>');
	});

	it('renders description text safely', () => {
		const el = buildPopupElement([
			{ type: 'heading', text: 'Safe Name' },
			{ type: 'text', text: '<script>alert(1)</script>' }
		]);
		expect(el.querySelectorAll('script').length).toBe(0);
		expect(el.textContent).toContain('<script>alert(1)</script>');
	});

	it('omits br when only a heading row is provided', () => {
		const el = buildPopupElement([{ type: 'heading', text: 'My Fence' }]);
		expect(el.querySelectorAll('br').length).toBe(0);
		expect(el.querySelector('strong')?.textContent).toBe('My Fence');
	});

	it('adds a br before each text and note row', () => {
		const el = buildPopupElement([
			{ type: 'heading', text: 'Name' },
			{ type: 'text', text: 'Line 1' },
			{ type: 'text', text: 'Line 2' }
		]);
		expect(el.querySelectorAll('br').length).toBe(2);
	});

	it('renders note rows as <small> elements with optional className', () => {
		const el = buildPopupElement([
			{ type: 'heading', text: 'Fence' },
			{ type: 'note', text: 'Schedule: Work Hours', className: 'muted' }
		]);
		const small = el.querySelector('small');
		expect(small).not.toBeNull();
		expect(small?.textContent).toBe('Schedule: Work Hours');
		expect(small?.className).toBe('muted');
	});

	it('note row text is safe — no inline HTML execution', () => {
		const el = buildPopupElement([{ type: 'note', text: '<b>bold</b>' }]);
		expect(el.querySelectorAll('b').length).toBe(0);
		expect(el.textContent).toContain('<b>bold</b>');
	});

	it('returns an HTMLElement', () => {
		const el = buildPopupElement([{ type: 'heading', text: 'Test' }]);
		expect(el instanceof HTMLElement).toBe(true);
	});
});

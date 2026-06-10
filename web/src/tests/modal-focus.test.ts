import { describe, it, expect } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import ModalFocusHost from './helpers/ModalFocusHost.svelte';

async function openModal(container: HTMLElement): Promise<void> {
	const trigger = container.querySelector<HTMLButtonElement>('#outside-trigger')!;
	trigger.focus();
	await fireEvent.click(trigger);
	await tick();
}

function dialog(): HTMLElement {
	const el = document.querySelector<HTMLElement>('[role="dialog"]');
	expect(el).not.toBeNull();
	return el!;
}

describe('Modal focus management', () => {
	it('moves focus into the dialog when opened', async () => {
		const { container } = render(ModalFocusHost);
		await openModal(container);

		expect(dialog().contains(document.activeElement)).toBe(true);
	});

	it('wraps Tab from the last focusable element to the first', async () => {
		const { container } = render(ModalFocusHost);
		await openModal(container);

		const footerBtn = document.querySelector<HTMLButtonElement>('#footer-btn')!;
		footerBtn.focus();
		await fireEvent.keyDown(dialog(), { key: 'Tab' });

		const closeBtn = document.querySelector<HTMLButtonElement>('.close-button')!;
		expect(document.activeElement).toBe(closeBtn);
	});

	it('wraps Shift+Tab from the first focusable element to the last', async () => {
		const { container } = render(ModalFocusHost);
		await openModal(container);

		const closeBtn = document.querySelector<HTMLButtonElement>('.close-button')!;
		closeBtn.focus();
		await fireEvent.keyDown(dialog(), { key: 'Tab', shiftKey: true });

		const footerBtn = document.querySelector<HTMLButtonElement>('#footer-btn')!;
		expect(document.activeElement).toBe(footerBtn);
	});

	it('restores focus to the previously focused element on close', async () => {
		const { container } = render(ModalFocusHost);
		await openModal(container);

		await fireEvent.keyDown(window, { key: 'Escape' });
		await tick();

		const trigger = container.querySelector<HTMLButtonElement>('#outside-trigger')!;
		expect(document.activeElement).toBe(trigger);
	});
});

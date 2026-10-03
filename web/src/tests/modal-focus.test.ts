import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import ModalFocusHost from './helpers/ModalFocusHost.svelte';

async function openModal(container: HTMLElement): Promise<HTMLDialogElement> {
	await fireEvent.click(container.querySelector<HTMLButtonElement>('#outside-trigger')!);
	await tick();
	return document.querySelector<HTMLDialogElement>('dialog')!;
}

const closes = (container: HTMLElement) => container.querySelector('#closes')!.textContent;

// Focus trap, Escape and focus restore come from the native modal dialog;
// these tests check that Modal drives it.
describe('Modal', () => {
	afterEach(() => vi.restoreAllMocks());

	it('opens as a native modal dialog', async () => {
		const showModal = vi.spyOn(HTMLDialogElement.prototype, 'showModal');
		const { container } = render(ModalFocusHost);
		const dialog = await openModal(container);
		expect(showModal).toHaveBeenCalledOnce();
		expect(dialog.open).toBe(true);
		expect(dialog.querySelector('#modal-title')?.textContent).toBe('Test Modal');
	});

	it('closes when the dialog closes natively (Escape)', async () => {
		const { container } = render(ModalFocusHost);
		const dialog = await openModal(container);
		dialog.close();
		await tick();
		expect(document.querySelector('dialog')).toBeNull();
		expect(closes(container)).toBe('1');
	});

	it('closes on a backdrop click but not on a click inside', async () => {
		const { container } = render(ModalFocusHost);
		const dialog = await openModal(container);
		await fireEvent.click(dialog.querySelector('#first-btn')!);
		expect(document.querySelector('dialog')).not.toBeNull();
		await fireEvent.click(dialog);
		await tick();
		expect(document.querySelector('dialog')).toBeNull();
		expect(closes(container)).toBe('1');
	});

	it('closes the native dialog when the parent closes it, without a close event', async () => {
		const connectedOnClose: boolean[] = [];
		const nativeClose = HTMLDialogElement.prototype.close;
		vi.spyOn(HTMLDialogElement.prototype, 'close').mockImplementation(function (this: HTMLDialogElement) {
			connectedOnClose.push(this.isConnected);
			nativeClose.call(this);
		});
		const { container } = render(ModalFocusHost);
		const dialog = await openModal(container);
		await fireEvent.click(dialog.querySelector('#parent-close')!);
		await tick();
		// Still in the DOM, so the browser can restore focus.
		expect(connectedOnClose).toEqual([true]);
		expect(document.querySelector('dialog')).toBeNull();
		expect(closes(container)).toBe('0');
	});
});

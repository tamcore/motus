<script lang="ts">
	import { createEventDispatcher } from 'svelte';

	export let open = false;
	export let title = '';

	const dispatch = createEventDispatcher();

	let dialogEl: HTMLDialogElement | null = null;

	// The native modal dialog traps focus, closes on Escape and restores focus
	// on close(); close it while it is still in the DOM.
	$: if (!open && dialogEl?.open) dialogEl.close();

	function showModal(node: HTMLDialogElement) {
		node.showModal();
	}

	function closeModal() {
		if (!open) return;
		open = false;
		dispatch('close');
	}

	function handleClick(e: MouseEvent) {
		if (e.target === dialogEl) closeModal();
	}
</script>

{#if open}
	<!-- svelte-ignore a11y_no_redundant_roles -->
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
	<dialog
		class="modal"
		role="dialog"
		aria-labelledby="modal-title"
		bind:this={dialogEl}
		use:showModal
		on:close={(e) => e.currentTarget === dialogEl && closeModal()}
		on:click={handleClick}
	>
		<div class="modal-header">
			<h2 id="modal-title" class="modal-title">{title}</h2>
			<button class="close-button" on:click={closeModal} aria-label="Close dialog">
				&#x2715;
			</button>
		</div>
		<div class="modal-body">
			<slot />
		</div>
		{#if $$slots.footer}
			<div class="modal-footer">
				<slot name="footer" />
			</div>
		{/if}
	</dialog>
{/if}

<style>
	.modal {
		/* restore the UA centering that the global `* { margin: 0 }` reset removes */
		margin: auto;
		padding: 0;
		border: none;
		color: var(--text-primary);
		background-color: var(--bg-primary);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-xl);
		max-width: 600px;
		width: calc(100% - 2 * var(--space-4));
		max-height: 90vh;
		overflow: auto;
	}

	.modal::backdrop {
		background-color: rgba(0, 0, 0, 0.7);
	}

	.modal-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: var(--space-6);
		border-bottom: 1px solid var(--border-color);
	}

	.modal-title {
		font-size: var(--text-xl);
		font-weight: var(--font-semibold);
		color: var(--text-primary);
	}

	.close-button {
		background: none;
		border: none;
		font-size: var(--text-2xl);
		color: var(--text-secondary);
		cursor: pointer;
		padding: 0;
		width: 32px;
		height: 32px;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: var(--radius-md);
		transition: all var(--transition-fast);
	}

	.close-button:hover {
		background-color: var(--bg-hover);
		color: var(--text-primary);
	}

	.modal-body {
		padding: var(--space-6);
	}

	.modal-footer {
		padding: var(--space-6);
		border-top: 1px solid var(--border-color);
	}
</style>

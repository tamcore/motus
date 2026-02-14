<script lang="ts">
	import { createEventDispatcher } from 'svelte';

	export let open = false;
	export let title = '';

	const dispatch = createEventDispatcher();

	function closeModal() {
		open = false;
		dispatch('close');
	}

	function handleBackdropClick(e: MouseEvent) {
		if (e.target === e.currentTarget) {
			closeModal();
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			closeModal();
		}
	}
</script>

<svelte:window on:keydown={handleKeydown} />

{#if open}
	<!-- svelte-ignore a11y-click-events-have-key-events -->
	<div class="modal-backdrop" on:click={handleBackdropClick} role="presentation">
		<div class="modal" role="dialog" aria-modal="true" aria-labelledby="modal-title">
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
		</div>
	</div>
{/if}

<style>
	.modal-backdrop {
		position: fixed;
		inset: 0;
		background-color: rgba(0, 0, 0, 0.7);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 1000;
		padding: var(--space-4);
	}

	.modal {
		background-color: var(--bg-primary);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-xl);
		max-width: 600px;
		width: 100%;
		max-height: 90vh;
		overflow: auto;
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

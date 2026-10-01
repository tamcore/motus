<script lang="ts">
	import Button from '$lib/components/Button.svelte';

	export let confirmLabel: string;
	export let busyLabel: string;
	export let fallbackError: string;
	export let onConfirm: () => Promise<void>;
	export let onCancel: () => void;

	let busy = false;
	let error = '';

	async function confirm() {
		busy = true;
		error = '';
		try {
			await onConfirm();
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : fallbackError;
		} finally {
			busy = false;
		}
	}
</script>

<div class="confirm-overlay">
	<p class="confirm-text"><slot /></p>
	{#if error}
		<div class="message error">{error}</div>
	{/if}
	<div class="confirm-actions">
		<Button variant="secondary" size="sm" on:click={onCancel}>Cancel</Button>
		<Button variant="danger" size="sm" loading={busy} on:click={confirm}>
			{busy ? busyLabel : confirmLabel}
		</Button>
	</div>
</div>

<style>
	.confirm-overlay {
		margin-top: var(--space-4);
		padding: var(--space-4);
		background-color: rgba(255, 68, 68, 0.05);
		border: 1px solid var(--error);
		border-radius: var(--radius-md);
	}

	.confirm-text {
		font-size: var(--text-sm);
		color: var(--text-primary);
		margin-bottom: var(--space-3);
		line-height: 1.5;
	}

	.confirm-actions {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-3);
	}

	.message {
		padding: var(--space-3) var(--space-4);
		border-radius: var(--radius-md);
		font-size: var(--text-sm);
	}

	.message.error {
		background-color: rgba(255, 68, 68, 0.1);
		color: var(--error);
		border: 1px solid var(--error);
	}
</style>

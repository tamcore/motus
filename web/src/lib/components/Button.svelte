<script lang="ts">
	export let variant: 'primary' | 'secondary' | 'danger' = 'primary';
	export let size: 'sm' | 'md' | 'lg' = 'md';
	export let disabled = false;
	export let loading = false;
	export let type: 'button' | 'submit' = 'button';
</script>

<button
	{type}
	class="btn btn-{variant} btn-{size}"
	disabled={disabled || loading}
	class:loading
	on:click
>
	{#if loading}
		<span class="spinner" aria-hidden="true"></span>
	{/if}
	<slot />
</button>

<style>
	.btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: var(--space-2);
		border: none;
		border-radius: var(--radius-md);
		font-weight: var(--font-medium);
		cursor: pointer;
		transition: all var(--transition-fast);
		white-space: nowrap;
	}

	.btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.btn-sm {
		padding: var(--space-2) var(--space-3);
		font-size: var(--text-sm);
	}

	.btn-md {
		padding: var(--space-3) var(--space-4);
		font-size: var(--text-base);
	}

	.btn-lg {
		padding: var(--space-4) var(--space-6);
		font-size: var(--text-lg);
	}

	.btn-primary {
		background-color: var(--accent-primary);
		color: var(--text-inverse);
	}

	.btn-primary:hover:not(:disabled) {
		background-color: var(--accent-hover);
	}

	.btn-secondary {
		background-color: var(--bg-secondary);
		color: var(--text-primary);
		border: 1px solid var(--border-color);
	}

	.btn-secondary:hover:not(:disabled) {
		background-color: var(--bg-hover);
	}

	.btn-danger {
		background-color: var(--error);
		color: var(--text-inverse);
	}

	.btn-danger:hover:not(:disabled) {
		background-color: #ff3333;
	}

	.spinner {
		width: 1em;
		height: 1em;
		border: 2px solid currentColor;
		border-right-color: transparent;
		border-radius: 50%;
		animation: spin 0.6s linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
</style>

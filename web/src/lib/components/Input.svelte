<script lang="ts">
	export let type: string = 'text';
	/** A number (or null when empty) for type="number", else a string. */
	export let value: string | number | null = '';
	export let placeholder: string = '';
	export let label: string = '';
	export let error: string = '';
	export let required: boolean = false;
	export let name: string = '';
</script>

<div class="input-group">
	{#if label}
		<label for={name} class="label">
			{label}
			{#if required}<span class="required" aria-hidden="true">*</span>{/if}
		</label>
	{/if}

	<input
		{type}
		id={name}
		{name}
		{placeholder}
		{required}
		class="input"
		class:has-error={!!error}
		bind:value
		aria-invalid={error ? 'true' : undefined}
		aria-describedby={error ? `${name}-error` : undefined}
	/>

	{#if error}
		<span id="{name}-error" class="error-text" role="alert">{error}</span>
	{/if}
</div>

<style>
	.input-group {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	.label {
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		color: var(--text-primary);
	}

	.required {
		color: var(--error);
	}

	.input.has-error {
		border-color: var(--error);
	}

	.error-text {
		font-size: var(--text-sm);
		color: var(--error);
	}
</style>

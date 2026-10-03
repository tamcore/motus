<script lang="ts">
	import Input from '$lib/components/Input.svelte';
	import { DEFAULT_REPORTING_INTERVAL_SECONDS, REPORTING_INTERVAL_PRESETS } from '$lib/utils/commands';

	/** Reporting interval in seconds, as entered text (parsed by the caller). */
	export let value = '';

	const isPreset = (v: string) => REPORTING_INTERVAL_PRESETS.some((p) => String(p.seconds) === v);

	// Initial value only: forms mount the picker after setting it.
	if (!value) value = String(DEFAULT_REPORTING_INTERVAL_SECONDS);
	let custom = !isPreset(value);

	function selectPreset(seconds: number) {
		custom = false;
		value = String(seconds);
	}
</script>

<div class="interval-picker">
	<span class="form-label" id="interval-presets-label">Interval</span>
	<div class="interval-presets" role="group" aria-labelledby="interval-presets-label">
		{#each REPORTING_INTERVAL_PRESETS as preset}
			<button
				type="button"
				class="interval-preset"
				class:active={!custom && value === String(preset.seconds)}
				aria-pressed={!custom && value === String(preset.seconds)}
				on:click={() => selectPreset(preset.seconds)}
			>
				{preset.label}
			</button>
		{/each}
		<button
			type="button"
			class="interval-preset"
			class:active={custom}
			aria-pressed={custom}
			on:click={() => (custom = true)}
		>
			Custom
		</button>
	</div>
	{#if custom}
		<Input name="frequency" label="Interval (seconds)" placeholder="30" bind:value />
	{/if}
</div>

<style>
	.interval-picker {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	.form-label {
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		color: var(--text-secondary);
	}

	.interval-presets {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}

	.interval-preset {
		padding: var(--space-1) var(--space-3);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-sm);
		cursor: pointer;
	}

	.interval-preset:hover {
		border-color: var(--accent-primary);
	}

	.interval-preset.active {
		background-color: var(--accent-primary);
		border-color: var(--accent-primary);
		color: var(--text-inverse);
	}
</style>

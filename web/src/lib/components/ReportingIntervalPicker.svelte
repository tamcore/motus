<script lang="ts">
	import Input from '$lib/components/Input.svelte';
	import { DEFAULT_REPORTING_INTERVAL_SECONDS, REPORTING_INTERVAL_PRESETS } from '$lib/utils/commands';

	/** Reporting interval in seconds, as entered text (parsed by the caller). */
	export let value = '';

	const isPreset = (v: string) => REPORTING_INTERVAL_PRESETS.some((p) => String(p.seconds) === v);

	let custom = false;
	// Last value written by the picker itself. Any other change of `value`
	// comes from the parent (initial value, form reset, ...) and re-derives
	// the default and custom mode instead of keeping stale mount-time state.
	let ownValue: string | undefined;

	$: if (value !== ownValue) syncFromParent(value);

	function syncFromParent(v: string) {
		if (!v) {
			custom = false;
			ownValue = value = String(DEFAULT_REPORTING_INTERVAL_SECONDS);
		} else {
			custom = !isPreset(v);
			ownValue = v;
		}
	}

	function selectPreset(seconds: number) {
		custom = false;
		ownValue = value = String(seconds);
	}

	function selectCustom() {
		custom = true;
	}

	function onCustomInput(e: Event) {
		ownValue = (e.target as HTMLInputElement).value;
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
			on:click={selectCustom}
		>
			Custom
		</button>
	</div>
	{#if custom}
		<Input name="frequency" label="Interval (seconds)" placeholder="30" bind:value on:input={onCustomInput} />
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

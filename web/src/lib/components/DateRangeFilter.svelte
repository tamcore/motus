<script lang="ts">
	import { RELATIVE_DATE_PRESETS, type DatePreset } from '$lib/utils/date-range';

	export let preset: DatePreset;
	export let from: string;
	export let to: string;
	/** The custom date inputs get the ids `${idPrefix}-from` and `${idPrefix}-to`. */
	export let idPrefix = 'date';
</script>

<div class="filter-group">
	<span class="filter-label">Date Range</span>
	<div class="preset-buttons">
		{#each RELATIVE_DATE_PRESETS as p (p.value)}
			<button class="preset-btn" class:active={preset === p.value}
				on:click={() => (preset = p.value)}>{p.label}</button>
		{/each}
	</div>
</div>
{#if preset === 'custom'}
	<div class="filter-group date-inputs">
		<label for="{idPrefix}-from" class="filter-label">From</label>
		<input id="{idPrefix}-from" type="date" bind:value={from} class="field-sm" />
		<label for="{idPrefix}-to" class="filter-label">To</label>
		<input id="{idPrefix}-to" type="date" bind:value={to} class="field-sm" />
	</div>
{/if}

<style>
	.preset-buttons {
		display: flex;
		gap: var(--space-1);
	}

	.preset-btn {
		padding: var(--space-2) var(--space-3);
		background-color: var(--bg-primary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-sm);
		cursor: pointer;
		transition: all var(--transition-fast);
	}

	.preset-btn:hover {
		background-color: var(--bg-hover);
	}

	.preset-btn.active {
		background-color: var(--accent-primary);
		color: var(--text-inverse);
		border-color: var(--accent-primary);
	}

	.date-inputs {
		flex-direction: row;
		align-items: center;
	}
</style>

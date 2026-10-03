<script lang="ts">
	import {
		TRAIL_PRESETS,
		customTrailRange,
		isRelativePreset,
		resolveTrailRange,
		type TrailPreset,
		type TrailRange
	} from '$lib/utils/trail-range';

	export let range: TrailRange;
	/** Called immediately for presets, on Apply for a custom range. */
	export let onChange: (range: TrailRange) => void;

	let mode: TrailPreset = range.preset;
	let fromDate = '';
	let fromTime = '';
	let toDate = '';
	let toTime = '';
	let error = '';

	const pad2 = (n: number) => String(n).padStart(2, '0');
	const dateValue = (d: Date) =>
		`${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
	const timeValue = (d: Date) => `${pad2(d.getHours())}:${pad2(d.getMinutes())}`;

	function fillInputs(r: TrailRange) {
		const { from, to } = resolveTrailRange(r);
		fromDate = dateValue(from);
		toDate = dateValue(to);
		// Times stay empty (whole days) unless a custom range has explicit ones.
		const custom = r.preset === 'custom';
		const fromIsDayStart = from.getHours() === 0 && from.getMinutes() === 0;
		const toIsDayEnd = to.getHours() === 23 && to.getMinutes() === 59;
		fromTime = custom && !fromIsDayStart ? timeValue(from) : '';
		toTime = custom && !toIsDayEnd ? timeValue(to) : '';
	}

	// Follow external changes (URL params, bookmarks) to the applied range.
	let lastRange: TrailRange | null = null;
	$: if (range !== lastRange) {
		lastRange = range;
		mode = range.preset;
		error = '';
		fillInputs(range);
	}

	function handleSelect(e: Event) {
		const value = (e.currentTarget as HTMLSelectElement).value;
		error = '';
		if (isRelativePreset(value)) {
			mode = value;
			onChange({ preset: value });
		} else {
			mode = 'custom';
		}
	}

	function apply() {
		const result = customTrailRange(fromDate, fromTime, toDate, toTime);
		if (!result) {
			error = 'Choose a start date before the end date';
			return;
		}
		error = '';
		onChange(result);
	}
</script>

<div class="trail-range">
	<label for="trail-range-select" class="trail-range-label">Trail range</label>
	<select
		id="trail-range-select"
		class="trail-range-select"
		value={mode}
		on:change={handleSelect}
	>
		{#each TRAIL_PRESETS as preset (preset.value)}
			<option value={preset.value}>{preset.label}</option>
		{/each}
	</select>

	{#if mode === 'custom'}
		<form class="trail-custom-range" on:submit|preventDefault={apply}>
			<span class="trail-sub-label">From</span>
			<div class="trail-datetime">
				<input
					id="trail-from-date"
					class="trail-input"
					type="date"
					aria-label="From date"
					bind:value={fromDate}
				/>
				<input
					id="trail-from-time"
					class="trail-input trail-input--time"
					type="time"
					aria-label="From time (optional)"
					bind:value={fromTime}
				/>
			</div>
			<span class="trail-sub-label">To</span>
			<div class="trail-datetime">
				<input
					id="trail-to-date"
					class="trail-input"
					type="date"
					aria-label="To date"
					bind:value={toDate}
				/>
				<input
					id="trail-to-time"
					class="trail-input trail-input--time"
					type="time"
					aria-label="To time (optional)"
					bind:value={toTime}
				/>
			</div>
			<span class="trail-hint">Time is optional; empty means the whole day.</span>
			{#if error}
				<span class="trail-range-error" role="alert">{error}</span>
			{/if}
			<div class="trail-apply-row">
				<button type="submit" class="trail-apply">Apply</button>
			</div>
		</form>
	{/if}
</div>

<style>
	.trail-range {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		margin-top: var(--space-3);
	}

	.trail-range-label,
	.trail-sub-label {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
	}

	.trail-custom-range {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}

	.trail-datetime {
		display: flex;
		gap: var(--space-1);
	}

	.trail-range-select,
	.trail-input {
		padding: var(--space-1) var(--space-2);
		background-color: var(--bg-primary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-sm);
		width: 100%;
		min-width: 0;
	}

	.trail-input--time {
		flex: 0 0 6.5rem;
		width: 6.5rem;
	}

	.trail-range-select:focus,
	.trail-input:focus {
		outline: none;
		border-color: var(--accent-primary);
	}

	.trail-hint {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
	}

	.trail-range-error {
		font-size: var(--text-xs);
		color: var(--error, #ff4444);
	}

	.trail-apply {
		padding: var(--space-1) var(--space-3);
		background-color: var(--bg-tertiary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-sm);
		cursor: pointer;
	}

	.trail-apply:hover {
		border-color: var(--accent-primary);
	}

	.trail-apply-row {
		display: flex;
		justify-content: flex-end;
	}
</style>

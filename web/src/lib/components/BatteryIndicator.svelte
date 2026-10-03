<script lang="ts">
	const LOW_BATTERY_THRESHOLD = 20;

	export let level: number | null | undefined = undefined;
	/** Render a dash when the level is unknown (e.g. in table cells). */
	export let placeholder = false;

	// The backend only stores levels within 0..100, so no clamping is needed.
	// Compare the unrounded level so e.g. 19.6% (shown as 20%) is still low.
	$: status =
		level != null && Number.isFinite(level)
			? { percent: Math.round(level), low: level < LOW_BATTERY_THRESHOLD }
			: null;
	// Fill width of the battery body (inner area is 14 units wide).
	$: fill = status ? Math.max(1, (status.percent / 100) * 14) : 0;
</script>

{#if status}
	<span
		class="battery"
		class:battery-low={status.low}
		role="img"
		aria-label="Battery {status.percent}%{status.low ? ', low' : ''}"
		title={status.low ? `Battery ${status.percent}% – needs charging` : `Battery ${status.percent}%`}
	>
		<svg viewBox="0 0 20 10" width="20" height="10" aria-hidden="true">
			<rect x="0.5" y="0.5" width="16" height="9" rx="1.5" fill="none" stroke="currentColor" />
			<rect x="17" y="3" width="2" height="4" rx="0.5" fill="currentColor" />
			<rect x="1.5" y="1.5" width={fill} height="7" rx="0.5" fill="currentColor" />
		</svg>
		<span class="battery-percent">{status.percent}%</span>
	</span>
{:else if placeholder}
	<span class="battery-unknown" title="No battery level reported">—</span>
{/if}

<style>
	.battery {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1, 4px);
		font-size: var(--text-sm);
		color: var(--text-secondary);
		white-space: nowrap;
		font-variant-numeric: tabular-nums;
	}

	.battery-low {
		color: var(--error);
		font-weight: 600;
	}

	.battery-unknown {
		color: var(--text-tertiary, var(--text-secondary));
	}
</style>

<script lang="ts">
	// Checkbox list selecting geofences or devices, e.g. the filters of the
	// notification rule editor or the geofences attached to a device. CSS hooks
	// are prefixed with `name` (e.g. .geofence-checkbox, .device-unavailable).
	export let name: 'geofence' | 'device';
	export let label: string;
	export let options: Array<{ id: number; label: string; unavailable: boolean }>;
	/** Selected IDs; empty means "all". */
	export let selected: number[];
	export let noOptionsHint: string;
	export let allHint: string;
	export let someHint: string;
</script>

<div class="form-group {name}-filter">
	<span class="form-label">{label}</span>
	{#if options.length === 0}
		<span class="form-hint">{noOptionsHint}</span>
	{:else}
		<div class="filter-grid">
			{#each options as o (o.id)}
				<label
					class="filter-checkbox {name}-checkbox"
					class:unavailable={o.unavailable}
					class:geofence-unavailable={o.unavailable && name === 'geofence'}
					class:device-unavailable={o.unavailable && name === 'device'}
					title={o.unavailable
						? `This ${name} was deleted or is no longer accessible. Untick it to remove it.`
						: undefined}
				>
					<input type="checkbox" value={o.id} bind:group={selected} />
					<span>{o.label}</span>
				</label>
			{/each}
		</div>
		{#if options.some((o) => o.unavailable && selected.includes(o.id))}
			<span class="form-hint form-hint--warning {name}-unavailable-hint">
				Unavailable {name}s have no effect. Untick them to remove them.
			</span>
		{/if}
		<span class="form-hint {name}-filter-hint">{selected.length === 0 ? allHint : someHint}</span>
	{/if}
</div>

<style>
	.filter-grid {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: var(--space-1) var(--space-3);
	}

	.filter-checkbox {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-sm);
		color: var(--text-primary);
		cursor: pointer;
		user-select: none;
	}

	.filter-checkbox input[type='checkbox'] {
		width: 0.9rem;
		height: 0.9rem;
		accent-color: var(--accent-primary);
		cursor: pointer;
		margin: 0;
	}

	.form-hint {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}

	.form-hint--warning {
		color: var(--warning);
	}

	.unavailable span {
		font-style: italic;
		color: var(--warning);
	}
</style>

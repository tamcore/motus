<script lang="ts">
	// Geofence or device filter of the notification rule editor. CSS hooks are
	// prefixed with `name` (e.g. .geofence-checkbox, .device-unavailable).
	export let name: 'geofence' | 'device';
	export let label: string;
	export let options: Array<{ id: number; label: string; unavailable: boolean }>;
	/** Selected IDs; empty means the rule applies to all. */
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
		<div class="event-type-grid">
			{#each options as o (o.id)}
				<label
					class="event-type-checkbox {name}-checkbox"
					class:unavailable={o.unavailable}
					class:geofence-unavailable={o.unavailable && name === 'geofence'}
					class:device-unavailable={o.unavailable && name === 'device'}
					title={o.unavailable
						? `This ${name} was deleted or is no longer accessible. Untick it to remove it from the rule.`
						: undefined}
				>
					<input type="checkbox" value={o.id} bind:group={selected} />
					<span>{o.label}</span>
				</label>
			{/each}
		</div>
		{#if options.some((o) => o.unavailable && selected.includes(o.id))}
			<span class="form-hint form-hint--warning {name}-unavailable-hint">
				Unavailable {name}s never trigger this rule. Untick them to remove them.
			</span>
		{/if}
		<span class="form-hint {name}-filter-hint">{selected.length === 0 ? allHint : someHint}</span>
	{/if}
</div>

<style>
	.unavailable span {
		font-style: italic;
		color: var(--warning);
	}
</style>

<script lang="ts">
	import { settings } from '$lib/stores/settings';
	import { currentUser } from '$lib/stores/auth';
	import { createEventDispatcher } from 'svelte';

	export let label = 'All users';

	const dispatch = createEventDispatcher<{ change: boolean }>();

	$: isAdmin = ($currentUser as Record<string, unknown> | null)?.administrator === true;

	function toggle() {
		settings.update((s) => ({ ...s, showAllDevices: !s.showAllDevices }));
		dispatch('change', !$settings.showAllDevices);
	}
</script>

{#if isAdmin}
	<label class="admin-toggle" title="Include resources from all users">
		<input type="checkbox" checked={$settings.showAllDevices} on:change={toggle} />
		<span class="toggle-label">{label}</span>
	</label>
{/if}

<style>
	.admin-toggle {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1, 0.25rem);
		cursor: pointer;
		font-size: var(--text-xs, 0.75rem);
		color: var(--text-secondary, #666);
		user-select: none;
		white-space: nowrap;
	}

	.admin-toggle:hover {
		color: var(--text-primary, #333);
	}

	input[type="checkbox"] {
		width: 0.85rem;
		height: 0.85rem;
		accent-color: var(--color-primary, #3b82f6);
		cursor: pointer;
		margin: 0;
	}

	.toggle-label {
		line-height: 1;
	}
</style>

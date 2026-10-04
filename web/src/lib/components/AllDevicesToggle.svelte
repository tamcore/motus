<script lang="ts">
	import { settings } from '$lib/stores/settings';
	import { isAdmin } from '$lib/stores/auth';
	import { createEventDispatcher } from 'svelte';

	const dispatch = createEventDispatcher<{ change: boolean }>();

	function toggle() {
		settings.update((s) => ({ ...s, showAllDevices: !s.showAllDevices }));
		dispatch('change', !$settings.showAllDevices);
	}
</script>

{#if $isAdmin}
	<label class="admin-toggle" title="Include resources from all users">
		<input type="checkbox" checked={$settings.showAllDevices} on:change={toggle} />
		<span class="toggle-label">All users</span>
	</label>
{/if}

<style>
	.admin-toggle {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		cursor: pointer;
		font-size: var(--text-xs);
		color: var(--text-secondary);
		user-select: none;
		white-space: nowrap;
	}

	.admin-toggle:hover {
		color: var(--text-primary);
	}

	input[type="checkbox"] {
		width: 0.85rem;
		height: 0.85rem;
		accent-color: var(--accent-primary);
		cursor: pointer;
		margin: 0;
	}

	.toggle-label {
		line-height: 1;
	}
</style>

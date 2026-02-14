<script lang="ts">
	import StatusIndicator from './StatusIndicator.svelte';

	interface Device {
		id: number;
		name: string;
		uniqueId: string;
		status: string;
	}

	export let devices: Device[] = [];
	export let selectedId: number | null = null;
	export let searchQuery = '';

	$: filtered = devices.filter(
		(d) =>
			d.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
			d.uniqueId.toLowerCase().includes(searchQuery.toLowerCase())
	);

	function getStatusType(status: string): 'online' | 'offline' | 'idle' | 'moving' {
		if (status === 'online') return 'online';
		if (status === 'idle') return 'idle';
		if (status === 'moving') return 'moving';
		return 'offline';
	}
</script>

<aside class="sidebar">
	<div class="sidebar-header">
		<h2 class="sidebar-title">Devices</h2>
		<span class="device-count">{filtered.length}</span>
	</div>

	<div class="search-box">
		<input
			type="search"
			placeholder="Search devices..."
			class="search-input"
			bind:value={searchQuery}
		/>
	</div>

	<div class="device-list">
		{#each filtered as device (device.id)}
			<button
				class="device-item"
				class:selected={selectedId === device.id}
				on:click={() => (selectedId = device.id)}
			>
				<div class="device-info">
					<span class="device-name">{device.name}</span>
					<span class="device-uid">{device.uniqueId}</span>
				</div>
				<StatusIndicator status={getStatusType(device.status)} />
			</button>
		{:else}
			<div class="no-results">No devices found</div>
		{/each}
	</div>
</aside>

<style>
	.sidebar {
		width: 300px;
		background-color: var(--bg-secondary);
		border-right: 1px solid var(--border-color);
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	.sidebar-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: var(--space-4);
		border-bottom: 1px solid var(--border-color);
	}

	.sidebar-title {
		font-size: var(--text-lg);
		font-weight: var(--font-semibold);
		color: var(--text-primary);
	}

	.device-count {
		font-size: var(--text-sm);
		color: var(--text-secondary);
		background-color: var(--bg-tertiary);
		padding: var(--space-1) var(--space-2);
		border-radius: var(--radius-full);
	}

	.search-box {
		padding: var(--space-3);
		border-bottom: 1px solid var(--border-color);
	}

	.search-input {
		width: 100%;
		padding: var(--space-2) var(--space-3);
		background-color: var(--bg-primary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-sm);
	}

	.search-input:focus {
		outline: none;
		border-color: var(--accent-primary);
	}

	.device-list {
		flex: 1;
		overflow-y: auto;
	}

	.device-item {
		display: flex;
		align-items: center;
		justify-content: space-between;
		width: 100%;
		padding: var(--space-3) var(--space-4);
		background: none;
		border: none;
		border-bottom: 1px solid var(--border-color);
		cursor: pointer;
		text-align: left;
		transition: background-color var(--transition-fast);
	}

	.device-item:hover {
		background-color: var(--bg-hover);
	}

	.device-item.selected {
		background-color: var(--bg-active);
		border-left: 3px solid var(--accent-primary);
	}

	.device-info {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.device-name {
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		color: var(--text-primary);
	}

	.device-uid {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
	}

	.no-results {
		padding: var(--space-6);
		text-align: center;
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}

	@media (max-width: 768px) {
		.sidebar {
			width: 100%;
			max-height: 250px;
			border-right: none;
			border-bottom: 1px solid var(--border-color);
		}
	}
</style>

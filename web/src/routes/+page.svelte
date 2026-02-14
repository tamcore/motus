<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api, fetchDevices } from '$lib/api/client';
	import { currentUser } from '$lib/stores/auth';
	import { refreshHandler } from '$lib/stores/refresh';
	import type { Position } from '$lib/types/api';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import StatusIndicator from '$lib/components/StatusIndicator.svelte';
	import AllDevicesToggle from '$lib/components/AllDevicesToggle.svelte';
	import { formatRelative, formatSpeed, getCardinalDirection, formatCoordinates } from '$lib/utils/formatting';

	interface Device {
		id: number;
		name: string;
		uniqueId: string;
		status: string;
		protocol?: string;
		lastUpdate?: string;
		ownerName?: string;
	}

	let loading = true;
	let devices: Device[] = [];
	let positionMap: Map<number, Position> = new Map();
	let positionsToday = 0;
	let statusFilter: 'all' | 'online' | 'offline' = 'all';

	$: onlineCount = devices.filter((d) => d.status === 'online').length;
	$: offlineCount = devices.filter((d) => d.status === 'offline').length;
	$: filteredDevices =
		statusFilter === 'all'
			? devices
			: devices.filter((d) => d.status === statusFilter);

	/** Check whether a device status counts as "active" (online, moving, or idle). */
	function isDeviceActive(status: string): boolean {
		return status === 'online' || status === 'moving' || status === 'idle';
	}

	/** Get display location for a position (address if available, otherwise coordinates). */
	function getLocationText(pos: Position): string {
		if (pos.address) return pos.address;
		return formatCoordinates(pos.latitude, pos.longitude);
	}

	async function loadDashboard() {
		const isAdmin = ($currentUser as Record<string, unknown> | null)?.administrator === true;
		const [deviceList, latestPositions, todayPositions] = await Promise.all([
			fetchDevices(isAdmin) as Promise<Device[]>,
			api.getPositions().catch(() => [] as Position[]),
			(async () => {
				const todayStart = new Date();
				todayStart.setHours(0, 0, 0, 0);
				return api.getPositions({
					from: todayStart.toISOString(),
					to: new Date().toISOString(),
					limit: 10000
				}).catch(() => [] as Position[]);
			})()
		]);

		devices = deviceList;

		const newMap = new Map<number, Position>();
		for (const pos of latestPositions) {
			newMap.set(pos.deviceId, pos);
		}
		positionMap = newMap;
		positionsToday = todayPositions.length;
	}

	onMount(async () => {
		try {
			await loadDashboard();
		} catch (error) {
			console.error('Failed to load dashboard:', error);
		} finally {
			loading = false;
		}
		$refreshHandler = loadDashboard;
	});

	onDestroy(() => { $refreshHandler = null; });

	function getStatusType(status: string): 'online' | 'offline' | 'idle' | 'moving' {
		if (status === 'online') return 'online';
		if (status === 'idle') return 'idle';
		if (status === 'moving') return 'moving';
		return 'offline';
	}

	function setFilter(filter: 'all' | 'online' | 'offline') {
		// Toggle off if clicking the same filter
		statusFilter = statusFilter === filter ? 'all' : filter;
	}

	async function reloadDevices() {
		const isAdmin = ($currentUser as Record<string, unknown> | null)?.administrator === true;
		try {
			devices = (await fetchDevices(isAdmin)) as Device[];
		} catch {
			console.error('Failed to reload devices');
		}
	}
</script>

<svelte:head>
	<title>Dashboard - Motus</title>
</svelte:head>

<div class="dashboard">
	<div class="container">
		<h1 class="page-title">Dashboard</h1>

		<div class="stats-grid">
			<button class="stat-card" class:active={statusFilter === 'all'} on:click={() => setFilter('all')}>
				<div class="stat-icon">
					<svg viewBox="0 0 24 24" width="32" height="32" fill="none" stroke="var(--text-secondary)" stroke-width="1.5">
						<rect x="5" y="2" width="14" height="20" rx="2"/>
						<line x1="12" y1="18" x2="12" y2="18.01" stroke-width="2"/>
					</svg>
				</div>
				<div class="stat-content">
					<div class="stat-label">Total Devices</div>
					{#if loading}
						<Skeleton width="80px" height="32px" />
					{:else}
						<div class="stat-value">{devices.length}</div>
					{/if}
				</div>
			</button>

			<button class="stat-card" class:active={statusFilter === 'online'} on:click={() => setFilter('online')}>
				<div class="stat-icon">
					<svg viewBox="0 0 24 24" width="32" height="32" fill="none" stroke="var(--status-online)" stroke-width="1.5">
						<circle cx="12" cy="12" r="10"/>
						<path d="M9 12l2 2 4-4"/>
					</svg>
				</div>
				<div class="stat-content">
					<div class="stat-label">Online</div>
					{#if loading}
						<Skeleton width="80px" height="32px" />
					{:else}
						<div class="stat-value status-online">{onlineCount}</div>
					{/if}
				</div>
			</button>

			<button class="stat-card" class:active={statusFilter === 'offline'} on:click={() => setFilter('offline')}>
				<div class="stat-icon">
					<svg viewBox="0 0 24 24" width="32" height="32" fill="none" stroke="var(--status-offline)" stroke-width="1.5">
						<circle cx="12" cy="12" r="10"/>
						<line x1="15" y1="9" x2="9" y2="15"/>
						<line x1="9" y1="9" x2="15" y2="15"/>
					</svg>
				</div>
				<div class="stat-content">
					<div class="stat-label">Offline</div>
					{#if loading}
						<Skeleton width="80px" height="32px" />
					{:else}
						<div class="stat-value status-offline">{offlineCount}</div>
					{/if}
				</div>
			</button>

			<div class="stat-card">
				<div class="stat-icon">
					<svg viewBox="0 0 24 24" width="32" height="32" fill="none" stroke="var(--accent-primary)" stroke-width="1.5">
						<path d="M12 2C8.13 2 5 5.13 5 9c0 5.25 7 13 7 13s7-7.75 7-13c0-3.87-3.13-7-7-7z"/>
						<circle cx="12" cy="9" r="2.5"/>
					</svg>
				</div>
				<div class="stat-content">
					<div class="stat-label">Positions Today</div>
					{#if loading}
						<Skeleton width="80px" height="32px" />
					{:else}
						<div class="stat-value">{positionsToday}</div>
					{/if}
				</div>
			</div>
		</div>

		<div class="devices-header">
			<h2 class="section-title">
				{#if statusFilter === 'all'}
					All Devices
				{:else if statusFilter === 'online'}
					Online Devices
				{:else}
					Offline Devices
				{/if}
				{#if !loading}
					<span class="device-count">({filteredDevices.length})</span>
				{/if}
			</h2>
			<AllDevicesToggle on:change={reloadDevices} />
			{#if filteredDevices.length > 6}
				<a href="/devices" class="view-all-link">View All →</a>
			{/if}
		</div>

		{#if loading}
			<div class="device-list">
				{#each Array(4) as _}
					<div class="device-card">
						<Skeleton width="100%" height="80px" variant="rect" />
					</div>
				{/each}
			</div>
		{:else if filteredDevices.length === 0}
			<div class="empty-state">
				<svg viewBox="0 0 24 24" width="64" height="64" fill="none" stroke="var(--text-tertiary)" stroke-width="1">
					<rect x="5" y="2" width="14" height="20" rx="2"/>
					<line x1="12" y1="18" x2="12" y2="18.01" stroke-width="2"/>
				</svg>
				{#if statusFilter === 'all'}
					<p>No devices yet</p>
					<a href="/devices" class="empty-link">Add your first device</a>
				{:else if statusFilter === 'online'}
					<p>No online devices</p>
				{:else}
					<p>No offline devices</p>
				{/if}
			</div>
		{:else}
			<div class="device-list">
				{#each filteredDevices.slice(0, 6) as device (device.id)}
					{@const pos = positionMap.get(device.id)}
					<a href="/map?device={device.id}" class="device-card" class:other-user={device.ownerName}>
						<div class="device-header">
							<h3 class="device-name">{device.name}</h3>
							{#if device.ownerName}
								<span class="owner-badge" title="Owned by {device.ownerName}">{device.ownerName}</span>
							{/if}
							<StatusIndicator status={getStatusType(device.status)} />
						</div>
						<div class="device-meta">
							<span class="device-id">{device.uniqueId}</span>
							{#if device.protocol}
								<span class="device-protocol">{device.protocol}</span>
							{/if}
						</div>
						{#if pos}
							<div class="position-info">
								{#if isDeviceActive(device.status)}
									<span class="info-speed">{formatSpeed(pos.speed)}</span>
									{#if pos.course != null}
										<span class="info-heading">
											<svg class="compass-icon" viewBox="0 0 24 24" width="12" height="12"
												style="transform: rotate({pos.course}deg)">
												<path d="M12 2 L16 20 L12 16 L8 20 Z" fill="currentColor" stroke="none"/>
											</svg>
											{getCardinalDirection(pos.course)}
										</span>
									{/if}
								{/if}
								<span class="info-location" title={getLocationText(pos)}>
									{getLocationText(pos)}
								</span>
							</div>
						{/if}
						{#if device.lastUpdate}
							<div class="last-seen">
								Last seen: {formatRelative(new Date(device.lastUpdate))}
							</div>
						{/if}
					</a>
				{/each}
			</div>
		{/if}
	</div>
</div>

<style>
	.dashboard {
		padding: var(--space-6) 0;
	}

	.page-title {
		font-size: var(--text-3xl);
		font-weight: var(--font-bold);
		color: var(--text-primary);
		margin-bottom: var(--space-6);
	}

	.stats-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
		gap: var(--space-4);
		margin-bottom: var(--space-8);
	}

	.stat-card {
		display: flex;
		align-items: center;
		gap: var(--space-4);
		padding: var(--space-6);
		background-color: var(--bg-secondary);
		border: 2px solid var(--border-color);
		border-radius: var(--radius-lg);
		transition: all var(--transition-fast);
		cursor: pointer;
		text-align: left;
		width: 100%;
	}

	.stat-card:hover {
		border-color: var(--border-hover);
		box-shadow: var(--shadow-md);
		transform: translateY(-2px);
	}

	.stat-card.active {
		border-color: var(--accent-primary);
		background-color: rgba(0, 212, 255, 0.05);
		box-shadow: var(--shadow-md);
	}

	.stat-icon {
		flex-shrink: 0;
	}

	.stat-content {
		flex: 1;
	}

	.stat-label {
		font-size: var(--text-sm);
		color: var(--text-secondary);
		margin-bottom: var(--space-1);
	}

	.stat-value {
		font-size: var(--text-2xl);
		font-weight: var(--font-bold);
		color: var(--text-primary);
	}

	.devices-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: var(--space-4);
	}

	.section-title {
		font-size: var(--text-xl);
		font-weight: var(--font-semibold);
		color: var(--text-primary);
		margin: 0;
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}

	.device-count {
		font-size: var(--text-lg);
		color: var(--text-secondary);
		font-weight: var(--font-normal);
	}

	.view-all-link {
		color: var(--accent-primary);
		text-decoration: none;
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		transition: opacity var(--transition-fast);
	}

	.view-all-link:hover {
		opacity: 0.8;
	}

	.device-list {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
		gap: var(--space-4);
	}

	.device-card {
		padding: var(--space-4);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
		text-decoration: none;
		transition: all var(--transition-fast);
	}

	.device-card:hover {
		border-color: var(--accent-primary);
		box-shadow: var(--shadow-md);
	}

	.device-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: var(--space-3);
	}

	.device-name {
		font-size: var(--text-lg);
		font-weight: var(--font-semibold);
		color: var(--text-primary);
		margin: 0;
	}

	.device-meta {
		display: flex;
		gap: var(--space-3);
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}

	.device-protocol {
		padding: var(--space-1) var(--space-2);
		background-color: var(--bg-tertiary);
		border-radius: var(--radius-sm);
		font-size: var(--text-xs);
		text-transform: uppercase;
	}

	.last-seen {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
		margin-top: var(--space-2);
	}

	.empty-state {
		text-align: center;
		padding: var(--space-16) var(--space-4);
	}

	.empty-state p {
		color: var(--text-secondary);
		margin: var(--space-4) 0;
	}

	.empty-link {
		color: var(--accent-primary);
		text-decoration: none;
		font-weight: var(--font-medium);
	}

	.empty-link:hover {
		text-decoration: underline;
	}

	/* ==== POSITION INFO ==== */
	.position-info {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
		margin-top: var(--space-2);
		font-size: var(--text-xs);
	}

	.info-speed {
		color: var(--accent-primary);
		font-weight: var(--font-semibold);
		white-space: nowrap;
	}

	.info-heading {
		display: inline-flex;
		align-items: center;
		gap: 3px;
		color: var(--text-secondary);
		white-space: nowrap;
	}

	.compass-icon {
		color: var(--accent-primary);
		flex-shrink: 0;
	}

	.info-location {
		color: var(--text-tertiary);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		flex: 1 1 100%;
	}

	.device-card.other-user {
		border-left: 3px solid var(--color-warning, #f59e0b);
		background: color-mix(in srgb, var(--color-warning, #f59e0b) 4%, transparent);
	}

	.owner-badge {
		display: inline-block;
		font-size: 0.6rem;
		padding: 0.05rem 0.3rem;
		border-radius: 0.2rem;
		background: color-mix(in srgb, var(--color-warning, #f59e0b) 15%, transparent);
		color: var(--text-secondary, #666);
		line-height: 1.2;
		white-space: nowrap;
	}
</style>

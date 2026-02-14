<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { browser } from '$app/environment';
	import { page } from '$app/stores';
	import { api, fetchDevices } from '$lib/api/client';
	import { currentUser } from '$lib/stores/auth';
	import { refreshHandler } from '$lib/stores/refresh';
	import { detectTrips, exportTripsToCSV } from '$lib/utils/trips';
	import { detectStops, exportStopsToCSV } from '$lib/utils/stops';
	import { formatDate, formatDuration, formatDistance, formatSpeed } from '$lib/utils/formatting';
	import type { Trip, Position } from '$lib/utils/trips';
	import type { Stop } from '$lib/utils/stops';
	import { Chart, registerables } from 'chart.js';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import Button from '$lib/components/Button.svelte';
	import AllDevicesToggle from '$lib/components/AllDevicesToggle.svelte';

	Chart.register(...registerables);

	let chartCanvas: HTMLCanvasElement;
	let chartInstance: Chart | null = null;

	interface Device { id: number; name: string; uniqueId: string; status: string; }

	let loading = true;
	let fetchingTrips = false;
	let fetchingStops = false;
	let devices: Device[] = [];
	let trips: Trip[] = [];
	let stops: Stop[] = [];
	let selectedDeviceId = '';
	let activeTab: 'trips' | 'stops' | 'summary' = 'trips';
	let datePreset: 'day' | 'week' | 'month' | 'custom' = 'week';
	let customFrom = '';
	let customTo = '';

	// Column visibility configuration
	let columnConfig: Record<string, boolean> = {
		device: true,
		startTime: true,
		endTime: true,
		duration: true,
		distance: true,
		avgSpeed: true,
		maxSpeed: true,
	};
	let showColumnConfig = false;
	let columnConfigLoaded = false;

	const columnLabels: Record<string, string> = {
		device: 'Device',
		startTime: 'Start Time',
		endTime: 'End Time',
		duration: 'Duration',
		distance: 'Distance',
		avgSpeed: 'Avg Speed',
		maxSpeed: 'Max Speed',
	};

	function getTripAvgSpeed(trip: Trip): number {
		return trip.avgSpeed;
	}

	// A trip is considered ongoing if its last position is within the last 5 minutes.
	const ONGOING_THRESHOLD_MS = 5 * 60 * 1000;
	function isTripOngoing(trip: Trip): boolean {
		return Date.now() - new Date(trip.endTime).getTime() < ONGOING_THRESHOLD_MS;
	}

	// Save column config on change (only after initial load)
	$: if (browser && columnConfigLoaded) {
		localStorage.setItem('motus_report_columns', JSON.stringify(columnConfig));
	}

	$: totalDistance = trips.reduce((sum, t) => sum + t.distance, 0);
	$: totalDuration = trips.reduce((sum, t) => sum + t.duration, 0);
	$: avgSpeed = trips.length > 0
		? trips.reduce((sum, t) => sum + t.avgSpeed, 0) / trips.length : 0;

	$: if (activeTab === 'summary' && trips.length > 0 && chartCanvas) {
		buildChart();
	}

	function buildChart() {
		if (chartInstance) chartInstance.destroy();
		const ctx = chartCanvas.getContext('2d');
		if (!ctx) return;
		const sorted = [...trips].sort(
			(a, b) => new Date(a.startTime).getTime() - new Date(b.startTime).getTime()
		);
		chartInstance = new Chart(ctx, {
			type: 'line',
			data: {
				labels: sorted.map((t) => formatDate(t.startTime)),
				datasets: [{
					label: 'Distance (km)',
					data: sorted.map((t) => Number(t.distance.toFixed(2))),
					borderColor: '#00d4ff',
					backgroundColor: 'rgba(0, 212, 255, 0.1)',
					fill: true, tension: 0.3
				}]
			},
			options: {
				responsive: true, maintainAspectRatio: false,
				plugins: { legend: { labels: { color: '#a0a0a0' } } },
				scales: {
					x: { ticks: { color: '#a0a0a0', maxRotation: 45 }, grid: { color: '#3a3a3a' } },
					y: { ticks: { color: '#a0a0a0' }, grid: { color: '#3a3a3a' } }
				}
			}
		});
	}

	function getDateRange(): { from: string; to: string } {
		const now = new Date();
		const to = now.toISOString();
		if (datePreset === 'custom') {
			return {
				from: customFrom ? new Date(customFrom).toISOString() : to,
				to: customTo ? new Date(customTo + 'T23:59:59').toISOString() : to
			};
		}
		const from = new Date(now);
		if (datePreset === 'day') from.setDate(from.getDate() - 1);
		else if (datePreset === 'week') from.setDate(from.getDate() - 7);
		else if (datePreset === 'month') from.setDate(from.getDate() - 30);
		return { from: from.toISOString(), to };
	}

	async function fetchReports() {
		fetchingTrips = true;
		fetchingStops = true;
		try {
			const { from, to } = getDateRange();
			const deviceIds = selectedDeviceId
				? [Number(selectedDeviceId)] : devices.map((d) => d.id);
			const allTrips: Trip[] = [];
			const allStops: Stop[] = [];
			// Fetch positions for each device independently so that a
			// failure for one device does not prevent the others from
			// being displayed. This fixes the bug where only one demo
			// device appeared in the reports page.
			await Promise.all(deviceIds.map(async (devId) => {
				const device = devices.find((d) => d.id === devId);
				if (!device) return;
				try {
					const positions = (await api.getPositions({
						deviceId: devId, from, to, limit: 10000
					})) as Position[];
					const deviceTrips = detectTrips(positions, device.name, device.id);
					const deviceStops = detectStops(positions, device.name);
					allTrips.push(...deviceTrips);
					allStops.push(...deviceStops);
				} catch (err) {
					console.error(`Failed to fetch reports for device ${device.name}:`, err);
				}
			}));
			allTrips.sort((a, b) => new Date(b.startTime).getTime() - new Date(a.startTime).getTime());
			allStops.sort((a, b) => new Date(b.arrivalTime).getTime() - new Date(a.arrivalTime).getTime());
			trips = allTrips;
			stops = allStops;
		} catch (error) {
			console.error('Failed to fetch reports:', error);
		} finally {
			fetchingTrips = false;
			fetchingStops = false;
		}
	}

	onMount(async () => {
		// Load column config from localStorage
		if (browser) {
			const saved = localStorage.getItem('motus_report_columns');
			if (saved) {
				try {
					columnConfig = { ...columnConfig, ...JSON.parse(saved) };
				} catch {
					// ignore malformed data
				}
			}
			columnConfigLoaded = true;
		}

		try {
			const isAdmin = ($currentUser as Record<string, unknown> | null)?.administrator === true;
			devices = (await fetchDevices(isAdmin)) as Device[];

			// Pre-select device from query param (?device=ID)
			const deviceParam = $page.url.searchParams.get('device');
			if (deviceParam && devices.some((d) => String(d.id) === deviceParam)) {
				selectedDeviceId = deviceParam;
			}

			// Auto-load reports after devices are loaded
			if (devices.length > 0) {
				await fetchReports();
			}
		} catch (error) {
			console.error('Failed to load devices:', error);
		} finally {
			loading = false;
		}
		$refreshHandler = reloadDevices;
	});

	onDestroy(() => { $refreshHandler = null; });

	async function reloadDevices() {
		const isAdmin = ($currentUser as Record<string, unknown> | null)?.administrator === true;
		try {
			devices = (await fetchDevices(isAdmin)) as Device[];
		} catch {
			console.error('Failed to reload devices');
		}
	}
</script>

<svelte:head><title>Reports - Motus</title></svelte:head>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<div class="reports-page" on:click={() => { if (showColumnConfig) showColumnConfig = false; }} role="presentation">
	<div class="container">
		<div class="page-header">
			<h1 class="page-title">Reports</h1>
			<div class="header-actions">
				<a href="/reports/charts" class="charts-link">
					<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
						<polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/>
					</svg>
					Device Charts
				</a>
				{#if activeTab === 'trips' && trips.length > 0}
					<Button variant="secondary" on:click={() => exportTripsToCSV(trips)}>Export Trips CSV</Button>
				{/if}
				{#if activeTab === 'stops' && stops.length > 0}
					<Button variant="secondary" on:click={() => exportStopsToCSV(stops)}>Export Stops CSV</Button>
				{/if}
			</div>
		</div>

		<div class="filters-bar">
			<div class="filter-group">
				<label for="device-select" class="filter-label">Device <AllDevicesToggle on:change={reloadDevices} /></label>
				{#if loading}
					<Skeleton width="200px" height="40px" />
				{:else}
					<select id="device-select" bind:value={selectedDeviceId} class="select">
						<option value="">All Devices</option>
						{#each devices as device}
							<option value={String(device.id)}>{device.name}</option>
						{/each}
					</select>
				{/if}
			</div>
			<div class="filter-group">
				<span class="filter-label">Date Range</span>
				<div class="preset-buttons">
					<button class="preset-btn" class:active={datePreset === 'day'}
						on:click={() => datePreset = 'day'}>Last 24h</button>
					<button class="preset-btn" class:active={datePreset === 'week'}
						on:click={() => datePreset = 'week'}>Last 7d</button>
					<button class="preset-btn" class:active={datePreset === 'month'}
						on:click={() => datePreset = 'month'}>Last 30d</button>
					<button class="preset-btn" class:active={datePreset === 'custom'}
						on:click={() => datePreset = 'custom'}>Custom</button>
				</div>
			</div>
			{#if datePreset === 'custom'}
				<div class="filter-group date-inputs">
					<label for="date-from" class="filter-label">From</label>
					<input id="date-from" type="date" bind:value={customFrom} class="date-input" />
					<label for="date-to" class="filter-label">To</label>
					<input id="date-to" type="date" bind:value={customTo} class="date-input" />
				</div>
			{/if}
			<Button on:click={fetchReports} loading={fetchingTrips}>Apply</Button>
		</div>

		<div class="tabs" role="tablist">
			<button class="tab-btn" class:active={activeTab === 'trips'}
				on:click={() => activeTab = 'trips'} role="tab"
				aria-selected={activeTab === 'trips'}>Trips</button>
			<button class="tab-btn" class:active={activeTab === 'stops'}
				on:click={() => activeTab = 'stops'} role="tab"
				aria-selected={activeTab === 'stops'}>Stops</button>
			<button class="tab-btn" class:active={activeTab === 'summary'}
				on:click={() => activeTab = 'summary'} role="tab"
				aria-selected={activeTab === 'summary'}>Summary</button>
		</div>

		{#if fetchingTrips || fetchingStops}
			<div class="loading-state"><Skeleton width="100%" height="200px" variant="rect" /></div>
		{:else if activeTab === 'trips' && trips.length === 0}
			<div class="empty-state">
				<svg viewBox="0 0 24 24" width="64" height="64" fill="none" stroke="var(--text-tertiary)" stroke-width="1">
					<path d="M9 17H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2h-4"/>
					<polyline points="12 15 17 21 7 21 12 15"/>
				</svg>
				<p>No trips detected</p>
				<p class="empty-hint">Trips are detected when a device moves faster than 5 km/h for at least 60 seconds. Select a device and date range, then click Apply.</p>
			</div>
		{:else if activeTab === 'trips'}
			<div class="table-toolbar">
				<div class="column-settings">
					<!-- svelte-ignore a11y-click-events-have-key-events -->
					<button
						class="settings-btn"
						on:click|stopPropagation={() => showColumnConfig = !showColumnConfig}
						aria-label="Configure columns"
						title="Configure columns"
					>
						<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
							<circle cx="12" cy="12" r="3"/>
							<path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>
						</svg>
						Columns
					</button>
					{#if showColumnConfig}
						<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
						<div
							class="column-dropdown"
							on:click|stopPropagation
							on:keydown={(e) => e.key === 'Escape' && (showColumnConfig = false)}
							role="group"
							aria-label="Column visibility options"
							tabindex="-1"
						>
							{#each Object.keys(columnLabels) as key}
								<label class="column-option">
									<input type="checkbox" bind:checked={columnConfig[key]} />
									{columnLabels[key]}
								</label>
							{/each}
						</div>
					{/if}
				</div>
			</div>
			<div class="table-wrapper">
				<table class="trips-table">
					<thead><tr>
						{#if columnConfig.device}<th>Device</th>{/if}
						{#if columnConfig.startTime}<th>Start Time</th>{/if}
						{#if columnConfig.endTime}<th>End Time</th>{/if}
						{#if columnConfig.duration}<th>Duration</th>{/if}
						{#if columnConfig.distance}<th>Distance</th>{/if}
						{#if columnConfig.avgSpeed}<th>Avg Speed</th>{/if}
						{#if columnConfig.maxSpeed}<th>Max Speed</th>{/if}
						<th>Actions</th>
					</tr></thead>
					<tbody>
						{#each trips as trip (trip.id)}
							<tr class:ongoing={isTripOngoing(trip)}>
								{#if columnConfig.device}<td>{trip.deviceName}</td>{/if}
								{#if columnConfig.startTime}<td>{formatDate(trip.startTime)}</td>{/if}
								{#if columnConfig.endTime}
									<td>
										{#if isTripOngoing(trip)}
											<span class="live-badge">
												<span class="live-dot"></span>
												In progress
											</span>
										{:else}
											{formatDate(trip.endTime)}
										{/if}
									</td>
								{/if}
								{#if columnConfig.duration}<td>{formatDuration(trip.duration)}</td>{/if}
								{#if columnConfig.distance}<td>{formatDistance(trip.distance)}</td>{/if}
								{#if columnConfig.avgSpeed}<td>{formatSpeed(getTripAvgSpeed(trip))}</td>{/if}
								{#if columnConfig.maxSpeed}<td>{formatSpeed(trip.maxSpeed)}</td>{/if}
								<td>
									<a href="/reports/route?deviceId={trip.deviceId}&from={trip.startTime}&to={trip.endTime}" class="view-link">Route</a>
									<a href="/reports/replay?deviceId={trip.deviceId}&from={trip.startTime}&to={trip.endTime}" class="view-link replay-link">Replay</a>
									{#if isTripOngoing(trip)}
										<a href="/map?device={trip.deviceId}" class="view-link live-link">Live</a>
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{:else if activeTab === 'stops' && stops.length === 0}
			<div class="empty-state">
				<svg viewBox="0 0 24 24" width="64" height="64" fill="none" stroke="var(--text-tertiary)" stroke-width="1">
					<circle cx="12" cy="12" r="10"/>
					<rect x="9" y="9" width="6" height="6" rx="1"/>
				</svg>
				<p>No stops detected</p>
				<p class="empty-hint">Stops are detected when a device remains below 1 km/h for at least 5 minutes. Select a device and date range, then click Apply.</p>
			</div>
		{:else if activeTab === 'stops'}
			<div class="table-wrapper">
				<table class="trips-table">
					<thead><tr>
						<th>Device</th><th>Address</th><th>Arrival</th>
						<th>Departure</th><th>Duration</th>
					</tr></thead>
					<tbody>
						{#each stops as stop (stop.id)}
							<tr>
								<td>{stop.deviceName}</td>
								<td class="address-cell" title={stop.address}>{stop.address}</td>
								<td>{formatDate(stop.arrivalTime)}</td>
								<td>{formatDate(stop.departureTime)}</td>
								<td>{formatDuration(stop.duration)}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{:else if activeTab === 'summary'}
			<div class="chart-container">
				<canvas bind:this={chartCanvas}></canvas>
			</div>
			<div class="stats-grid">
				<div class="stat-card">
					<div class="stat-label">Total Distance</div>
					<div class="stat-value">{formatDistance(totalDistance)}</div>
				</div>
				<div class="stat-card">
					<div class="stat-label">Total Trips</div>
					<div class="stat-value">{trips.length}</div>
				</div>
				<div class="stat-card">
					<div class="stat-label">Average Speed</div>
					<div class="stat-value">{formatSpeed(avgSpeed)}</div>
				</div>
				<div class="stat-card">
					<div class="stat-label">Total Duration</div>
					<div class="stat-value">{formatDuration(totalDuration)}</div>
				</div>
			</div>
		{/if}
	</div>
</div>

<style>
	.reports-page { padding: var(--space-6) 0; }
	.page-header {
		display: flex; justify-content: space-between; align-items: center;
		margin-bottom: var(--space-6);
	}
	.page-title {
		font-size: var(--text-3xl); font-weight: var(--font-bold);
		color: var(--text-primary); margin: 0;
	}
	.chart-container {
		height: 300px; margin-bottom: var(--space-6); padding: var(--space-4);
		background-color: var(--bg-secondary); border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
	}
	.filters-bar {
		display: flex; flex-wrap: wrap; align-items: flex-end; gap: var(--space-4);
		padding: var(--space-4); background-color: var(--bg-secondary);
		border: 1px solid var(--border-color); border-radius: var(--radius-lg);
		margin-bottom: var(--space-6);
	}
	.filter-group { display: flex; flex-direction: column; gap: var(--space-2); }
	.filter-label { font-size: var(--text-sm); font-weight: var(--font-medium); color: var(--text-secondary); }
	.select {
		padding: var(--space-3) var(--space-4); background-color: var(--bg-primary);
		border: 1px solid var(--border-color); border-radius: var(--radius-md);
		color: var(--text-primary); font-size: var(--text-base); min-width: 180px;
	}
	.preset-buttons { display: flex; gap: var(--space-1); }
	.preset-btn {
		padding: var(--space-2) var(--space-3); background-color: var(--bg-primary);
		border: 1px solid var(--border-color); border-radius: var(--radius-md);
		color: var(--text-primary); font-size: var(--text-sm); cursor: pointer;
		transition: all var(--transition-fast);
	}
	.preset-btn:hover { background-color: var(--bg-hover); }
	.preset-btn.active {
		background-color: var(--accent-primary); color: var(--text-inverse);
		border-color: var(--accent-primary);
	}
	.date-inputs { flex-direction: row; align-items: center; }
	.date-input {
		padding: var(--space-2) var(--space-3); background-color: var(--bg-primary);
		border: 1px solid var(--border-color); border-radius: var(--radius-md);
		color: var(--text-primary); font-size: var(--text-sm);
	}
	.tabs {
		display: flex; gap: var(--space-1); margin-bottom: var(--space-6);
		border-bottom: 1px solid var(--border-color);
	}
	.tab-btn {
		padding: var(--space-3) var(--space-4); background: none; border: none;
		border-bottom: 2px solid transparent; color: var(--text-secondary);
		font-size: var(--text-base); font-weight: var(--font-medium);
		cursor: pointer; transition: all var(--transition-fast);
	}
	.tab-btn:hover { color: var(--text-primary); }
	.tab-btn.active { color: var(--accent-primary); border-bottom-color: var(--accent-primary); }

	/* Column settings */
	.table-toolbar {
		display: flex; justify-content: flex-end; margin-bottom: var(--space-3);
	}
	.column-settings {
		position: relative;
	}
	.settings-btn {
		display: flex; align-items: center; gap: var(--space-2);
		padding: var(--space-2) var(--space-3);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color); border-radius: var(--radius-md);
		color: var(--text-secondary); font-size: var(--text-sm);
		cursor: pointer; transition: all var(--transition-fast);
	}
	.settings-btn:hover {
		background-color: var(--bg-hover); color: var(--text-primary);
	}
	.column-dropdown {
		position: absolute; top: calc(100% + var(--space-2)); right: 0;
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color); border-radius: var(--radius-md);
		box-shadow: var(--shadow-lg); min-width: 180px;
		z-index: 100; padding: var(--space-2);
	}
	.column-option {
		display: flex; align-items: center; gap: var(--space-2);
		padding: var(--space-2) var(--space-3);
		font-size: var(--text-sm); color: var(--text-primary);
		cursor: pointer; border-radius: var(--radius-sm);
		transition: background-color var(--transition-fast);
	}
	.column-option:hover {
		background-color: var(--bg-hover);
	}
	.column-option input[type="checkbox"] {
		accent-color: var(--accent-primary);
	}

	.table-wrapper { overflow-x: auto; }
	.trips-table { width: 100%; border-collapse: collapse; }
	.trips-table th, .trips-table td {
		padding: var(--space-3) var(--space-4); text-align: left;
		border-bottom: 1px solid var(--border-color);
	}
	.trips-table th {
		font-size: var(--text-sm); font-weight: var(--font-semibold);
		color: var(--text-secondary); background-color: var(--bg-secondary); white-space: nowrap;
	}
	.trips-table td { font-size: var(--text-sm); color: var(--text-primary); }
	.trips-table tr:hover td { background-color: var(--bg-secondary); }
	.view-link { color: var(--accent-primary); text-decoration: none; font-weight: var(--font-medium); }
	.view-link:hover { text-decoration: underline; }
	.replay-link { margin-left: var(--space-2); }
	.stats-grid {
		display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
		gap: var(--space-4);
	}
	.stat-card {
		padding: var(--space-6); background-color: var(--bg-secondary);
		border: 1px solid var(--border-color); border-radius: var(--radius-lg); text-align: center;
	}
	.stat-label { font-size: var(--text-sm); color: var(--text-secondary); margin-bottom: var(--space-2); }
	.stat-value { font-size: var(--text-2xl); font-weight: var(--font-bold); color: var(--text-primary); }
	.header-actions { display: flex; gap: var(--space-2); align-items: center; }
	.charts-link {
		display: inline-flex; align-items: center; gap: var(--space-2);
		padding: var(--space-3) var(--space-4); background-color: var(--bg-secondary);
		border: 1px solid var(--border-color); border-radius: var(--radius-md);
		color: var(--accent-primary); text-decoration: none; font-size: var(--text-base);
		font-weight: var(--font-medium); transition: all var(--transition-fast);
	}
	.charts-link:hover { background-color: var(--bg-hover); }
	.address-cell {
		max-width: 300px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
	}
	.empty-state { text-align: center; padding: var(--space-16) var(--space-4); }
	.empty-state p { color: var(--text-secondary); margin-top: var(--space-4); }
	.empty-hint { font-size: var(--text-sm); color: var(--text-tertiary); }
	.loading-state { padding: var(--space-4); }

	/* Ongoing trip highlight */
	tr.ongoing > td:first-child { border-left: 3px solid var(--success); }
	tr.ongoing td { background-color: rgba(0, 255, 136, 0.04); }
	tr.ongoing:hover td { background-color: rgba(0, 255, 136, 0.08); }

	/* Live badge */
	.live-badge {
		display: inline-flex; align-items: center; gap: var(--space-2);
		color: var(--success); font-weight: var(--font-medium);
	}
	.live-dot {
		width: 8px; height: 8px; border-radius: 50%;
		background-color: var(--success); flex-shrink: 0;
		animation: live-pulse 1.5s ease-in-out infinite;
	}
	@keyframes live-pulse {
		0%, 100% { opacity: 1; transform: scale(1); }
		50% { opacity: 0.4; transform: scale(1.4); }
	}
	.live-link { color: var(--success); margin-left: var(--space-2); }
	.live-link:hover { color: var(--success); }

	@media (max-width: 768px) {
		.filters-bar { flex-direction: column; align-items: stretch; }
		.filter-group { width: 100%; }
		.select { width: 100%; }
	}
</style>

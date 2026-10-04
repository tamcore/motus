<script lang="ts">
	import { dateValue, resolveDatePreset, type DatePreset } from '$lib/utils/date-range';
	import { onMount, onDestroy } from 'svelte';
	import { api, fetchDevices } from '$lib/api/client';
	import { refreshHandler } from '$lib/stores/refresh';
	import { isDark } from '$lib/stores/theme';
	import { useLeaflet } from '$lib/composables/useLeaflet';
	import type { Device, PositionPoint } from '$lib/types/api';
	import { pointStats } from '$lib/utils/point-stats';
	import Button from '$lib/components/Button.svelte';
	import AllDevicesToggle from '$lib/components/AllDevicesToggle.svelte';
	import type { HeatLayer, HeatMapOptions } from 'leaflet';

	// Points the heat layer renders; the server samples each device's range to this.
	const HEATMAP_MAX_POINTS = 10000;

	const leafletMap = useLeaflet();

	let mapContainer: HTMLDivElement;
	let heatLayer: typeof import('leaflet').heatLayer | null = null;
	let heatLayerInstance: HeatLayer | null = null;

	// Data
	let devices: Device[] = [];
	let positions: PositionPoint[] = [];
	let loading = false;
	let loadingCount = 0;
	let error = '';
	let layerError = '';

	// Filters
	let selectedDeviceId = '';
	let dateRange: DatePreset = 'week';
	let customFrom = '';
	let customTo = '';

	// Heatmap options
	let radius = 25;
	let opacityPercent = 60;
	let blur = 15;
	let maxIntensity = 1.0;
	let showHeatmap = true;
	let intensityMode: 'density' | 'speed' = 'density';

	$: minOpacity = opacityPercent / 100;

	// Dark mode gradient (blue -> cyan -> green -> yellow -> red)
	const darkGradient: Record<number, string> = {
		0.0: '#0000ff',
		0.25: '#00d4ff',
		0.5: '#00ff88',
		0.75: '#ffaa00',
		1.0: '#ff4444'
	};

	// Light mode gradient (more saturated for visibility on light tiles)
	const lightGradient: Record<number, string> = {
		0.0: '#0066ff',
		0.25: '#00ccff',
		0.5: '#00ff66',
		0.75: '#ffcc00',
		1.0: '#ff3333'
	};

	let mapReady = false;

	$: gradient = $isDark ? darkGradient : lightGradient;

	onMount(async () => {
		// Initialize map via composable
		await leafletMap.initialize(mapContainer, {
			center: [49.79, 9.95],
			zoom: 12,
		});

		try {
			// leaflet.heat extends the global L once; keep that object across revisits.
			const w = window as unknown as { L?: typeof import('leaflet') };
			const L = leafletMap.getLeaflet();
			w.L ??= (L as any)?.default ?? L;
			await import('leaflet.heat');
			heatLayer = w.L?.heatLayer ?? null;
			if (!heatLayer) throw new Error('leaflet.heat did not register L.heatLayer');
		} catch (err) {
			console.error('Failed to load heatmap layer:', err);
			layerError = 'Failed to load the heatmap layer. Please reload the page.';
		}

		mapReady = true;

		await loadDevices();
		await loadHeatmap();
		$refreshHandler = loadDevices;
	});

	onDestroy(() => {
		$refreshHandler = null;
		leafletMap.cleanup();
	});

	async function loadDevices() {
		try {
			devices = await fetchDevices();
		} catch (err) {
			console.error('Failed to load devices:', err);
		}
	}

	async function loadHeatmap() {
		loading = true;
		loadingCount = 0;
		error = '';

		try {
			const now = new Date();
			const weekAgo = new Date(resolveDatePreset('week', '', '', now).from);
			const { from: fromISO, to: toISO } = resolveDatePreset(dateRange, customFrom, customTo, now, weekAgo);

			// The heatmap renders at most HEATMAP_MAX_POINTS, so let the server
			// sample the range instead of downloading every position.
			const load = (deviceId: number) =>
				api
					.getPositionPoints({ deviceId, from: fromISO, to: toISO, limit: HEATMAP_MAX_POINTS })
					.then((points) => {
						loadingCount += points.length;
						return points;
					});

			if (selectedDeviceId) {
				positions = await load(parseInt(selectedDeviceId));
			} else {
				// All devices — fan out per device so each gets its own sampling budget.
				const results = await Promise.all(
					devices.map((d) => load(d.id).catch(() => [] as PositionPoint[])),
				);
				positions = results.flat();
			}

			if (positions.length > 0) {
				fitMapToPositions();
			}

			renderHeatmap();
		} catch (err) {
			console.error('Failed to load heatmap data:', err);
			error = 'Failed to load position data. Please try again.';
		} finally {
			loading = false;
		}
	}

	function samplePositions(data: PositionPoint[], maxPoints: number = HEATMAP_MAX_POINTS): PositionPoint[] {
		if (data.length <= maxPoints) {
			return data;
		}
		const step = Math.ceil(data.length / maxPoints);
		return data.filter((_, i) => i % step === 0);
	}

	function fitMapToPositions() {
		const L = leafletMap.getLeaflet();
		const map = leafletMap.getMap();
		if (!L || !map || positions.length === 0) return;

		// Manual min/max instead of positions.map(L.latLng) to avoid allocating
		// N temporary objects for large datasets (e.g. 635k positions).
		let minLat = Infinity, maxLat = -Infinity;
		let minLon = Infinity, maxLon = -Infinity;
		for (const p of positions) {
			if (p.lat < minLat) minLat = p.lat;
			if (p.lat > maxLat) maxLat = p.lat;
			if (p.lon < minLon) minLon = p.lon;
			if (p.lon > maxLon) maxLon = p.lon;
		}
		const bounds = L.latLngBounds([minLat, minLon], [maxLat, maxLon]);
		map.fitBounds(bounds.pad(0.1));
	}

	function renderHeatmap() {
		const L = leafletMap.getLeaflet();
		const map = leafletMap.getMap();

		// Remove existing layer
		if (heatLayerInstance && map) {
			map.removeLayer(heatLayerInstance);
			heatLayerInstance = null;
		}

		if (!showHeatmap || positions.length === 0 || !L || !map || !heatLayer) {
			return;
		}

		const sampled = samplePositions(positions);

		const heatData: [number, number, number][] = sampled.map((p) => {
			let intensity: number;
			if (intensityMode === 'speed') {
				// Use speed as intensity: higher speed = hotter
				intensity = Math.min(p.speed / 120, 1);
			} else {
				// Density mode: all points equal weight, density comes from overlap
				intensity = 0.6;
			}
			return [p.lat, p.lon, intensity];
		});

		const options: HeatMapOptions = {
			radius,
			blur,
			minOpacity,
			max: maxIntensity,
			gradient
		};

		heatLayerInstance = heatLayer(heatData, options).addTo(map);
	}

	function toggleHeatmap() {
		showHeatmap = !showHeatmap;
		renderHeatmap();
	}

	function resetControls() {
		radius = 25;
		opacityPercent = 60;
		blur = 15;
		maxIntensity = 1.0;
		intensityMode = 'density';
		renderHeatmap();
	}

	async function exportImage() {
		try {
			const html2canvas = (await import('html2canvas')).default;
			const canvas = await html2canvas(mapContainer, {
				useCORS: true,
				allowTaint: true,
				backgroundColor: null,
				scale: 2
			});

			const link = document.createElement('a');
			link.download = `motus-heatmap-${dateValue(new Date())}.png`;
			link.href = canvas.toDataURL('image/png');
			link.click();
		} catch (err) {
			console.error('Export failed:', err);
			// Fallback: prompt user to use browser screenshot
			const msg =
				'Image export encountered an issue. You can use your browser\'s screenshot ' +
				'tool (Ctrl+Shift+S on Firefox, or Ctrl+Shift+I > screenshot on Chrome) ' +
				'to capture the map.';
			alert(msg);
		}
	}

	// Reactively re-render when slider controls change
	$: if (mapReady) {
		// Track reactive dependencies
		void radius;
		void blur;
		void minOpacity;
		void maxIntensity;
		void gradient;
		void intensityMode;
		// Re-render with new settings
		renderHeatmap();
	}

	function handleDateRangeChange() {
		if (dateRange !== 'custom') {
			loadHeatmap();
		}
	}

	function handleDeviceChange() {
		loadHeatmap();
	}

	// The admin "All users" toggle changes which devices are in scope, so the
	// heatmap must be reloaded too, not just the device list.
	async function handleScopeChange() {
		await loadDevices();
		if (selectedDeviceId && !devices.some((d) => String(d.id) === selectedDeviceId)) {
			selectedDeviceId = '';
		}
		await loadHeatmap();
	}

	function applyCustomRange() {
		if (customFrom) {
			loadHeatmap();
		}
	}


	// Compute time range of loaded data for display
	$: stats = pointStats(positions);
	$: dataTimeRange = (() => {
		if (!stats || isNaN(stats.earliest)) return '';
		const fmt = (d: Date) =>
			d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
		return `${fmt(new Date(stats.earliest))} - ${fmt(new Date(stats.latest))}`;
	})();

	// Compute speed stats for display
	$: speedStats = stats
		? { avg: stats.avgSpeed.toFixed(1), max: stats.maxSpeed.toFixed(1) }
		: null;
</script>

<svelte:head>
	<title>Heatmap - Motus</title>
</svelte:head>

<div class="heatmap-page">
	<aside class="controls-sidebar">
		<div class="sidebar-header">
			<h2>Heatmap</h2>
			<span class="data-count" class:loading-badge={loading}>
				{#if loading}
					{loadingCount.toLocaleString()} loading…
				{:else}
					{positions.length.toLocaleString()} points
				{/if}
			</span>
		</div>

		<!-- Date Range -->
		<div class="control-group">
			<label for="date-range">Date Range</label>
			<select
				id="date-range"
				bind:value={dateRange}
				on:change={handleDateRangeChange}
				class="select"
			>
				<option value="day">Last 24 Hours</option>
				<option value="week">Last 7 Days</option>
				<option value="month">Last 30 Days</option>
				<option value="all">All Time</option>
				<option value="custom">Custom Range</option>
			</select>

			{#if dateRange === 'custom'}
				<div class="custom-range">
					<label for="date-from" class="sub-label">From</label>
					<input
						id="date-from"
						type="date"
						bind:value={customFrom}
						class="input"
					/>
					<label for="date-to" class="sub-label">To</label>
					<input
						id="date-to"
						type="date"
						bind:value={customTo}
						class="input"
					/>
					<Button size="sm" on:click={applyCustomRange}>Apply</Button>
				</div>
			{/if}
		</div>

		<!-- Device Filter -->
		<div class="control-group">
			<label for="device-filter">Device <AllDevicesToggle on:change={handleScopeChange} /></label>
			<select
				id="device-filter"
				bind:value={selectedDeviceId}
				on:change={handleDeviceChange}
				class="select"
			>
				<option value="">All Devices</option>
				{#each devices as device}
					<option value={String(device.id)}>{device.name}</option>
				{/each}
			</select>
		</div>

		<div class="controls-divider"></div>

		<!-- Intensity Mode -->
		<div class="control-group">
			<label for="intensity-mode">Color By</label>
			<select
				id="intensity-mode"
				bind:value={intensityMode}
				class="select"
			>
				<option value="density">Position Density</option>
				<option value="speed">Speed</option>
			</select>
		</div>

		<!-- Radius -->
		<div class="control-group">
			<label for="radius-slider">
				Radius
				<span class="control-value">{radius}px</span>
			</label>
			<input
				id="radius-slider"
				type="range"
				min="5"
				max="50"
				step="1"
				bind:value={radius}
				class="slider"
			/>
		</div>

		<!-- Blur -->
		<div class="control-group">
			<label for="blur-slider">
				Blur
				<span class="control-value">{blur}px</span>
			</label>
			<input
				id="blur-slider"
				type="range"
				min="1"
				max="40"
				step="1"
				bind:value={blur}
				class="slider"
			/>
		</div>

		<!-- Opacity -->
		<div class="control-group">
			<label for="opacity-slider">
				Opacity
				<span class="control-value">{opacityPercent}%</span>
			</label>
			<input
				id="opacity-slider"
				type="range"
				min="20"
				max="100"
				step="5"
				bind:value={opacityPercent}
				class="slider"
			/>
		</div>

		<!-- Actions -->
		<div class="control-actions">
			<Button variant="secondary" on:click={toggleHeatmap}>
				{showHeatmap ? 'Hide' : 'Show'} Layer
			</Button>
			<Button variant="secondary" on:click={fitMapToPositions} disabled={positions.length === 0}>
				Fit to Data
			</Button>
			<Button variant="secondary" on:click={exportImage} disabled={positions.length === 0}>
				Export Image
			</Button>
			<Button variant="secondary" on:click={resetControls}>
				Reset
			</Button>
		</div>

		<!-- Legend -->
		{#if showHeatmap && positions.length > 0}
			<div class="legend">
				<div class="legend-title">
					{intensityMode === 'speed' ? 'Speed' : 'Activity'}
				</div>
				<div
					class="legend-gradient"
					style="background: linear-gradient(to right, {Object.values(gradient).join(', ')})"
				></div>
				<div class="legend-labels">
					<span>{intensityMode === 'speed' ? '0 km/h' : 'Low'}</span>
					<span>{intensityMode === 'speed' ? '120+ km/h' : 'High'}</span>
				</div>
			</div>
		{/if}

		<!-- Data Stats -->
		{#if positions.length > 0 && !loading}
			<div class="stats">
				{#if dataTimeRange}
					<div class="stat-row">
						<span class="stat-label">Period</span>
						<span class="stat-value">{dataTimeRange}</span>
					</div>
				{/if}
				{#if speedStats}
					<div class="stat-row">
						<span class="stat-label">Avg Speed</span>
						<span class="stat-value">{speedStats.avg} km/h</span>
					</div>
					<div class="stat-row">
						<span class="stat-label">Max Speed</span>
						<span class="stat-value">{speedStats.max} km/h</span>
					</div>
				{/if}
				<div class="stat-row">
					<span class="stat-label">Displayed</span>
					<span class="stat-value">
						{Math.min(positions.length, HEATMAP_MAX_POINTS).toLocaleString()}
						{#if positions.length > HEATMAP_MAX_POINTS}
							/ {positions.length.toLocaleString()}
						{/if}
					</span>
				</div>
			</div>
		{/if}

		<!-- Error Message -->
		{#if error}
			<div class="error-message">{error}</div>
		{/if}
		{#if layerError}
			<div class="error-message">{layerError}</div>
		{/if}

		<!-- Empty State -->
		{#if positions.length === 0 && !loading && !error}
			<div class="empty-message">
				<p>No position data found</p>
				<p class="hint">Try a different date range or device</p>
			</div>
		{/if}
	</aside>

	<!-- Map -->
	<div class="map-wrapper">
		<div class="map-container" class:dark-tiles={$isDark} bind:this={mapContainer}></div>

		<!-- On-map floating legend -->
		{#if showHeatmap && positions.length > 0 && !loading}
			<div class="map-legend" role="img" aria-label="Heatmap legend">
				<div class="map-legend-title">
					{intensityMode === 'speed' ? 'Speed' : 'Activity'}
				</div>
				<div
					class="map-legend-gradient"
					style="background: linear-gradient(to right, {Object.values(gradient).join(', ')})"
				></div>
				<div class="map-legend-labels">
					<span>{intensityMode === 'speed' ? '0' : 'Low'}</span>
					<span>{intensityMode === 'speed' ? '120+' : 'High'}</span>
				</div>
			</div>
		{/if}

		{#if loading}
			<div class="map-loading">
				<div class="spinner spinner-lg"></div>
				{#if loadingCount > 0}
					<span class="map-loading-count">{loadingCount.toLocaleString()} pts</span>
				{/if}
			</div>
		{/if}
	</div>
</div>

<style>
	.heatmap-page {
		display: flex;
		height: calc(100vh - 65px);
		overflow: hidden;
	}

	.controls-sidebar {
		width: 300px;
		min-width: 300px;
		background-color: var(--bg-secondary);
		border-right: 1px solid var(--border-color);
		padding: var(--space-4);
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}

	.sidebar-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding-bottom: var(--space-3);
		border-bottom: 1px solid var(--border-color);
	}

	.sidebar-header h2 {
		font-size: var(--text-lg);
		font-weight: var(--font-semibold);
		color: var(--text-primary);
		margin: 0;
	}

	.data-count {
		font-size: var(--text-xs);
		color: var(--text-secondary);
		background-color: var(--bg-tertiary);
		padding: var(--space-1) var(--space-2);
		border-radius: var(--radius-full);
	}

	.loading-badge {
		color: var(--accent-primary);
	}

	.control-group {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}

	.control-group > label {
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		color: var(--text-primary);
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.control-value {
		font-weight: var(--font-normal);
		color: var(--text-secondary);
		font-size: var(--text-xs);
	}

	.sub-label {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
		margin-top: var(--space-1);
	}

	.custom-range {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		margin-top: var(--space-1);
	}

	.select,
	.input {
		padding: var(--space-2) var(--space-3);
		background-color: var(--bg-primary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-sm);
		width: 100%;
	}

	.select:focus,
	.input:focus {
		outline: none;
		border-color: var(--accent-primary);
	}

	.controls-divider {
		height: 1px;
		background-color: var(--border-color);
		margin: var(--space-1) 0;
	}

	.slider {
		width: 100%;
		height: 6px;
		border-radius: var(--radius-full);
		background: var(--bg-tertiary);
		outline: none;
		cursor: pointer;
		-webkit-appearance: none;
		appearance: none;
	}

	.slider::-webkit-slider-thumb {
		-webkit-appearance: none;
		width: 16px;
		height: 16px;
		border-radius: 50%;
		background: var(--accent-primary);
		cursor: pointer;
		border: 2px solid var(--bg-primary);
		box-shadow: var(--shadow-sm);
	}

	.slider::-moz-range-thumb {
		width: 16px;
		height: 16px;
		border-radius: 50%;
		background: var(--accent-primary);
		cursor: pointer;
		border: 2px solid var(--bg-primary);
		box-shadow: var(--shadow-sm);
	}

	.slider:focus-visible::-webkit-slider-thumb {
		outline: 2px solid var(--accent-primary);
		outline-offset: 2px;
	}

	.control-actions {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		padding-top: var(--space-2);
		border-top: 1px solid var(--border-color);
	}

	/* Legend */
	.legend {
		padding: var(--space-3);
		background-color: var(--bg-primary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
	}

	.legend-title {
		font-size: var(--text-xs);
		font-weight: var(--font-semibold);
		color: var(--text-primary);
		margin-bottom: var(--space-2);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.legend-gradient {
		height: 12px;
		border-radius: var(--radius-sm);
		margin-bottom: var(--space-1);
	}

	.legend-labels {
		display: flex;
		justify-content: space-between;
		font-size: var(--text-xs);
		color: var(--text-tertiary);
	}

	/* Stats */
	.stats {
		padding: var(--space-3);
		background-color: var(--bg-primary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	.stat-row {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.stat-label {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
	}

	.stat-value {
		font-size: var(--text-xs);
		color: var(--text-primary);
		font-weight: var(--font-medium);
	}

	/* Messages */
	.error-message {
		padding: var(--space-3);
		background-color: rgba(255, 68, 68, 0.1);
		border: 1px solid var(--error);
		border-radius: var(--radius-md);
		color: var(--error);
		font-size: var(--text-sm);
		text-align: center;
	}

	.empty-message {
		padding: var(--space-6) var(--space-4);
		text-align: center;
	}

	.empty-message p {
		color: var(--text-secondary);
		margin-bottom: var(--space-1);
	}

	.hint {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
	}

	/* Map */
	.map-wrapper {
		flex: 1;
		position: relative;
	}

	.map-container {
		width: 100%;
		height: 100%;
	}

	.map-container.dark-tiles :global(.leaflet-tile-pane) {
		filter: invert(1) hue-rotate(180deg) brightness(0.9) contrast(0.9);
	}

	/* On-map floating legend */
	.map-legend {
		position: absolute;
		bottom: 30px;
		left: 12px;
		z-index: 400;
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		padding: var(--space-2) var(--space-3);
		box-shadow: var(--shadow-md);
		min-width: 140px;
		pointer-events: none;
	}

	.map-legend-title {
		font-size: var(--text-xs);
		font-weight: var(--font-semibold);
		color: var(--text-primary);
		margin-bottom: var(--space-1);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.map-legend-gradient {
		height: 8px;
		border-radius: var(--radius-sm);
		margin-bottom: 2px;
	}

	.map-legend-labels {
		display: flex;
		justify-content: space-between;
		font-size: 10px;
		color: var(--text-tertiary);
	}

	.map-loading {
		flex-direction: column;
		gap: var(--space-3);
		background-color: rgba(0, 0, 0, 0.3);
		z-index: 500;
	}

	.map-loading-count {
		color: #fff;
		font-size: 0.875rem;
		font-variant-numeric: tabular-nums;
		text-shadow: 0 1px 3px rgba(0, 0, 0, 0.8);
	}

	/* Mobile responsive */
	@media (max-width: 768px) {
		.heatmap-page {
			flex-direction: column;
		}

		.controls-sidebar {
			width: 100%;
			min-width: unset;
			max-height: 280px;
			border-right: none;
			border-bottom: 1px solid var(--border-color);
		}
	}
</style>

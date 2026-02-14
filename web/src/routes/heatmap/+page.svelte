<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api, fetchDevices } from '$lib/api/client';
	import { currentUser } from '$lib/stores/auth';
	import { refreshHandler } from '$lib/stores/refresh';
	import { theme } from '$lib/stores/theme';
	import { useLeaflet } from '$lib/composables/useLeaflet';
	import type { Device, Position } from '$lib/types/api';
	import Button from '$lib/components/Button.svelte';
	import AllDevicesToggle from '$lib/components/AllDevicesToggle.svelte';
	import type { HeatMapOptions } from 'leaflet.heat';

	const leafletMap = useLeaflet();

	let mapContainer: HTMLDivElement;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let heatLayerInstance: any = null;

	// Data
	let devices: Device[] = [];
	let positions: Position[] = [];
	let loading = false;
	let error = '';

	// Filters
	let selectedDeviceId = '';
	let dateRange = 'last7d';
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

	let currentTheme: string = 'dark';
	let mapReady = false;

	const unsubscribeTheme = theme.subscribe((t) => {
		if (t === 'auto' && typeof window !== 'undefined') {
			currentTheme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
		} else {
			currentTheme = t === 'light' ? 'light' : 'dark';
		}
	});

	$: gradient = currentTheme === 'dark' ? darkGradient : lightGradient;

	$: if (mapReady) {
		updateTileLayer(currentTheme);
	}

	function updateTileLayer(themeMode: string) {
		const tileLayer = leafletMap.getTileLayer();
		if (!tileLayer) return;
		const url =
			themeMode === 'dark'
				? 'https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png'
				: 'https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png';
		tileLayer.setUrl(url);
	}

	onMount(async () => {
		const isDark = currentTheme === 'dark';
		const tileUrl = isDark
			? 'https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png'
			: 'https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png';

		// Initialize map via composable
		await leafletMap.initialize(mapContainer, {
			center: [49.79, 9.95],
			zoom: 12,
			tileUrl,
			tileAttribution: '&copy; OpenStreetMap contributors &copy; CARTO',
		});

		// leaflet.heat is a legacy browser plugin that extends the global `L` object.
		// Since Leaflet is imported as an ES module (no global `L`), we must set
		// window.L before importing the plugin so it can find Leaflet's namespace.
		const L = leafletMap.getLeaflet();
		if (L) {
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			(window as any).L = L;
		}
		await import('leaflet.heat');

		mapReady = true;

		await loadDevices();
		await loadHeatmap();
		$refreshHandler = loadDevices;
	});

	onDestroy(() => {
		$refreshHandler = null;
		unsubscribeTheme();
		leafletMap.cleanup();
	});

	async function loadDevices() {
		try {
			const isAdmin = ($currentUser as Record<string, unknown> | null)?.administrator === true;
			devices = await fetchDevices(isAdmin);
		} catch (err) {
			console.error('Failed to load devices:', err);
		}
	}

	function getDateRange(): { from: Date; to: Date } {
		const to = new Date();
		let from: Date;

		switch (dateRange) {
			case 'last24h':
				from = new Date(Date.now() - 24 * 60 * 60 * 1000);
				break;
			case 'last7d':
				from = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000);
				break;
			case 'last30d':
				from = new Date(Date.now() - 30 * 24 * 60 * 60 * 1000);
				break;
			case 'all':
				from = new Date('2020-01-01');
				break;
			case 'custom':
				from = customFrom
					? new Date(customFrom)
					: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000);
				return {
					from,
					to: customTo ? new Date(customTo + 'T23:59:59') : to
				};
			default:
				from = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000);
		}

		return { from, to };
	}

	async function loadHeatmap() {
		loading = true;
		error = '';

		try {
			const { from, to } = getDateRange();

			const params: {
				from: string;
				to: string;
				limit: number;
				deviceId?: number;
			} = {
				from: from.toISOString(),
				to: to.toISOString(),
				limit: 50000
			};

			if (selectedDeviceId) {
				params.deviceId = parseInt(selectedDeviceId);
			}

			positions = await api.getPositions(params);

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

	function samplePositions(data: Position[], maxPoints: number = 10000): Position[] {
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

		const bounds = L.latLngBounds(
			positions.map((p) => L.latLng(p.latitude, p.longitude))
		);
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

		if (!showHeatmap || positions.length === 0 || !L || !map) {
			return;
		}

		const sampled = samplePositions(positions);

		const heatData: [number, number, number][] = sampled.map((p) => {
			let intensity: number;
			if (intensityMode === 'speed') {
				// Use speed as intensity: higher speed = hotter
				const speed = p.speed ?? 0;
				intensity = Math.min(speed / 120, 1);
			} else {
				// Density mode: all points equal weight, density comes from overlap
				intensity = 0.6;
			}
			return [p.latitude, p.longitude, intensity];
		});

		const options: HeatMapOptions = {
			radius,
			blur,
			minOpacity,
			max: maxIntensity,
			gradient
		};

		// leaflet.heat adds heatLayer to the global window.L, not to the
		// ES module namespace returned by getLeaflet(), so read it from there.
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const globalL = (window as any).L;
		if (!globalL?.heatLayer) return;
		heatLayerInstance = globalL.heatLayer(heatData, options).addTo(map);
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
			link.download = `motus-heatmap-${new Date().toISOString().slice(0, 10)}.png`;
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

	function applyCustomRange() {
		if (customFrom) {
			loadHeatmap();
		}
	}

	function fitToData() {
		fitMapToPositions();
	}

	// Compute time range of loaded data for display
	$: dataTimeRange = (() => {
		if (positions.length === 0) return '';
		const times = positions.map((p) => new Date(p.fixTime).getTime()).filter((t) => !isNaN(t));
		if (times.length === 0) return '';
		const earliest = new Date(Math.min(...times));
		const latest = new Date(Math.max(...times));
		const fmt = (d: Date) =>
			d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
		return `${fmt(earliest)} - ${fmt(latest)}`;
	})();

	// Compute speed stats for display
	$: speedStats = (() => {
		if (positions.length === 0) return null;
		const speeds = positions.map((p) => p.speed ?? 0);
		const avg = speeds.reduce((a, b) => a + b, 0) / speeds.length;
		const max = Math.max(...speeds);
		return { avg: avg.toFixed(1), max: max.toFixed(1) };
	})();
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
					Loading...
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
				<option value="last24h">Last 24 Hours</option>
				<option value="last7d">Last 7 Days</option>
				<option value="last30d">Last 30 Days</option>
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
			<label for="device-filter">Device <AllDevicesToggle on:change={loadDevices} /></label>
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
			<Button variant="secondary" on:click={fitToData} disabled={positions.length === 0}>
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
						{Math.min(positions.length, 10000).toLocaleString()}
						{#if positions.length > 10000}
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
		<div class="map-container" bind:this={mapContainer}></div>

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
				<div class="spinner"></div>
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

	.map-container :global(.leaflet-container) {
		height: 100%;
		width: 100%;
		background-color: var(--bg-tertiary);
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
		position: absolute;
		inset: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		background-color: rgba(0, 0, 0, 0.3);
		z-index: 500;
	}

	.spinner {
		width: 48px;
		height: 48px;
		border: 4px solid var(--border-color);
		border-top-color: var(--accent-primary);
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	/* Leaflet popup override */
	.map-container :global(.leaflet-popup-content-wrapper) {
		background-color: var(--bg-secondary);
		color: var(--text-primary);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-lg);
	}

	.map-container :global(.leaflet-popup-tip) {
		background-color: var(--bg-secondary);
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

<script lang="ts">
	import { dateValue, resolveDatePreset, type DatePreset } from '$lib/utils/date-range';
	import { onMount, onDestroy } from 'svelte';
	import { browser } from '$app/environment';
	import { page } from '$app/stores';
	import { api, fetchDevices } from '$lib/api/client';
	import { refreshHandler } from '$lib/stores/refresh';
	import { isDark } from '$lib/stores/theme';
	import { formatDate } from '$lib/utils/formatting';
	import {
		METRICS,
		getAvailableMetrics,
		buildDatasets,
		buildScales,
		chartColors,
		exportChartDataToCSV,
	} from '$lib/utils/chart-metrics';
	import type { Device, Position } from '$lib/types/api';
	import type { MetricDefinition } from '$lib/utils/chart-metrics';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import Button from '$lib/components/Button.svelte';
	import AllDevicesToggle from '$lib/components/AllDevicesToggle.svelte';
	import { Chart, registerables } from 'chart.js';

	Chart.register(...registerables);

	let loading = true;
	let fetching = false;
	let devices: Device[] = [];
	let positions: Position[] = [];
	let errorMsg = '';

	// Configuration
	let selectedDeviceId = '';
	let periodPreset: DatePreset = 'today';
	let customFrom = '';
	let customTo = '';
	let selectedMetrics: string[] = ['speed'];
	let positionLimit = 0;
	let availableMetrics: MetricDefinition[] = METRICS;

	// Chart
	let chartCanvas: HTMLCanvasElement;
	let chartInstance: Chart | null = null;

	$: if (chartCanvas && positions.length > 0 && selectedMetrics.length > 0) {
		rebuildChart($isDark);
	}

	function toggleMetric(metricId: string) {
		if (selectedMetrics.includes(metricId)) {
			selectedMetrics = selectedMetrics.filter((m) => m !== metricId);
		} else {
			selectedMetrics = [...selectedMetrics, metricId];
		}
	}

	function isMetricSelected(metricId: string, _metrics: string[]): boolean {
		return _metrics.includes(metricId);
	}

	async function fetchData() {
		if (!selectedDeviceId) {
			errorMsg = 'Please select a device.';
			return;
		}
		errorMsg = '';
		fetching = true;
		positions = [];

		try {
			const { from, to } = resolveDatePreset(periodPreset, customFrom, customTo);
			const result = await api.getPositions({
				deviceId: Number(selectedDeviceId),
				from,
				to,
				limit: positionLimit,
			});

			if (!result || result.length === 0) {
				errorMsg = 'No positions found for the selected period.';
				positions = [];
				if (chartInstance) {
					chartInstance.destroy();
					chartInstance = null;
				}
				return;
			}

			// Sort by fixTime ascending
			positions = [...result].sort(
				(a, b) => new Date(a.fixTime).getTime() - new Date(b.fixTime).getTime()
			);

			// Filter metrics to those with actual data
			availableMetrics = getAvailableMetrics(positions);
			const availableIds = new Set(availableMetrics.map((m) => m.id));
			selectedMetrics = selectedMetrics.filter((id) => availableIds.has(id));
		} catch (err) {
			console.error('Failed to fetch positions:', err);
			errorMsg = 'Failed to fetch data. Please try again.';
		} finally {
			fetching = false;
		}
	}

	function rebuildChart(dark: boolean) {
		if (!chartCanvas || positions.length === 0 || selectedMetrics.length === 0) return;

		if (chartInstance) {
			chartInstance.destroy();
			chartInstance = null;
		}

		const ctx = chartCanvas.getContext('2d');
		if (!ctx) return;

		const { labels, datasets } = buildDatasets(positions, selectedMetrics);
		const scales = buildScales(selectedMetrics, dark);
		const colors = chartColors(dark);

		chartInstance = new Chart(ctx, {
			type: 'line',
			data: {
				labels,
				datasets: datasets as any,
			},
			options: {
				responsive: true,
				maintainAspectRatio: false,
				interaction: {
					mode: 'index',
					intersect: false,
				},
				plugins: {
					legend: {
						labels: {
							color: colors.tick,
							usePointStyle: true,
							padding: 16,
						},
					},
					tooltip: {
						backgroundColor: colors.tooltipBg,
						titleColor: colors.tooltipText,
						bodyColor: colors.tooltipText,
						borderColor: colors.tooltipBorder,
						borderWidth: 1,
						padding: 12,
						callbacks: {
							title: (items) => (items.length > 0 ? formatDate(new Date(items[0].parsed.x ?? NaN)) : ''),
							label: (item) => {
								const value = item.parsed.y;
								if (value === null || value === undefined) return '';
								return `${item.dataset.label}: ${typeof value === 'number' ? value.toFixed(2) : value}`;
							},
						},
					},
				},
				scales: scales as any,
			},
		});
	}

	function handleExportCSV() {
		if (positions.length === 0) return;
		const device = devices.find((d) => d.id === Number(selectedDeviceId));
		const deviceName = device?.name || 'unknown';
		exportChartDataToCSV(positions, selectedMetrics, deviceName);
	}

	function handleExportPNG() {
		if (!chartCanvas) return;
		const link = document.createElement('a');
		link.download = `motus-chart-${dateValue(new Date())}.png`;
		link.href = chartCanvas.toDataURL('image/png');
		link.click();
	}

	onMount(async () => {
		await reloadDevices();
		// Pre-select device from query param (?device=ID)
		const deviceParam = $page.url.searchParams.get('device');
		if (deviceParam && devices.some((d) => String(d.id) === deviceParam)) {
			selectedDeviceId = deviceParam;
		}
		loading = false;
		$refreshHandler = reloadDevices;
	});

	onDestroy(() => {
		$refreshHandler = null;
		if (chartInstance) {
			chartInstance.destroy();
			chartInstance = null;
		}
	});

	async function reloadDevices() {
		try {
			devices = await fetchDevices();
		} catch (err) {
			console.error('Failed to load devices:', err);
			errorMsg = 'Failed to load devices.';
		}
	}

	// Readable period labels
	const periodOptions: { value: DatePreset; label: string }[] = [
		{ value: 'today', label: 'Today' },
		{ value: 'yesterday', label: 'Yesterday' },
		{ value: 'thisWeek', label: 'This Week' },
		{ value: 'prevWeek', label: 'Previous Week' },
		{ value: 'thisMonth', label: 'This Month' },
		{ value: 'prevMonth', label: 'Previous Month' },
		{ value: 'all', label: 'All Time' },
		{ value: 'custom', label: 'Custom' },
	];
</script>

<svelte:head><title>Device Charts - Motus</title></svelte:head>

<div class="charts-page">
	<div class="container">
		<!-- Header -->
		<div class="page-header">
			<div class="page-title-group">
				<h1 class="page-title">Device Charts</h1>
				<a href="/reports" class="back-link">Back to Reports</a>
			</div>
			<div class="header-actions">
				{#if positions.length > 0}
					<Button variant="secondary" size="sm" on:click={handleExportCSV}>
						Export CSV
					</Button>
					<Button variant="secondary" size="sm" on:click={handleExportPNG}>
						Export PNG
					</Button>
				{/if}
			</div>
		</div>

		<!-- Configuration Panel -->
		<div class="config-panel">
			<!-- Row 1: Device + Period + Apply -->
			<div class="config-row">
				<div class="filter-group">
					<label for="chart-device" class="filter-label">Device <AllDevicesToggle on:change={reloadDevices} /></label>
					{#if loading}
						<Skeleton width="200px" height="40px" />
					{:else}
						<select id="chart-device" bind:value={selectedDeviceId} class="select">
							<option value="">Select a device...</option>
							{#each devices as device}
								<option value={String(device.id)}>{device.name}</option>
							{/each}
						</select>
					{/if}
				</div>

				<div class="filter-group">
					<label for="chart-period" class="filter-label">Period</label>
					<select id="chart-period" bind:value={periodPreset} class="select">
						{#each periodOptions as opt}
							<option value={opt.value}>{opt.label}</option>
						{/each}
					</select>
				</div>

				{#if periodPreset === 'custom'}
					<div class="filter-group">
						<label for="chart-from" class="filter-label">From</label>
						<input id="chart-from" type="date" bind:value={customFrom} class="field-sm" />
					</div>
					<div class="filter-group">
						<label for="chart-to" class="filter-label">To</label>
						<input id="chart-to" type="date" bind:value={customTo} class="field-sm" />
					</div>
				{/if}

				<div class="filter-group filter-group-action">
					<Button on:click={fetchData} loading={fetching}>
						{fetching ? 'Loading...' : 'Apply'}
					</Button>
				</div>
			</div>

			<!-- Row 2: Metric selection chips -->
			<div class="config-row">
				<div class="filter-group metrics-group">
					<span class="filter-label">Metrics</span>
					<div class="metric-chips" role="group" aria-label="Select metrics to chart">
						{#each availableMetrics as metric}
							<button
								class="metric-chip"
								class:active={isMetricSelected(metric.id, selectedMetrics)}
								on:click={() => toggleMetric(metric.id)}
								aria-pressed={isMetricSelected(metric.id, selectedMetrics)}
								style="--chip-color: {metric.color}"
							>
								<span class="chip-dot" style="background-color: {metric.color}"></span>
								{metric.label}
							</button>
						{/each}
					</div>
				</div>
			</div>

			<!-- Row 3: Position limit -->
			<div class="config-row config-row-secondary">
				<div class="filter-group limit-group">
					<label for="chart-limit" class="filter-label">Max positions</label>
					<select id="chart-limit" bind:value={positionLimit} class="select select-sm">
						<option value={0}>All</option>
						<option value={1000}>1,000</option>
						<option value={2500}>2,500</option>
						<option value={5000}>5,000</option>
						<option value={10000}>10,000</option>
					</select>
				</div>
				{#if positions.length > 0}
					<span class="position-count">{positions.length} positions loaded</span>
				{/if}
			</div>
		</div>

		<!-- Error Message -->
		{#if errorMsg}
			<div class="charts-error" role="alert">
				<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2">
					<circle cx="12" cy="12" r="10"/>
					<line x1="12" y1="8" x2="12" y2="12"/>
					<line x1="12" y1="16" x2="12.01" y2="16"/>
				</svg>
				{errorMsg}
			</div>
		{/if}

		<!-- Chart Area -->
		{#if fetching}
			<div class="chart-loading">
				<Skeleton width="100%" height="400px" variant="rect" />
			</div>
		{:else if positions.length > 0 && selectedMetrics.length > 0}
			<div class="chart-container">
				<canvas bind:this={chartCanvas}></canvas>
			</div>

			<!-- Stats summary row -->
			<div class="stats-row">
				<div class="stat-item">
					<span class="stat-label">Period</span>
					<span class="stat-value">
						{formatDate(positions[0].fixTime)} &ndash; {formatDate(positions[positions.length - 1].fixTime)}
					</span>
				</div>
				<div class="stat-item">
					<span class="stat-label">Data Points</span>
					<span class="stat-value">{positions.length}</span>
				</div>
				{#if selectedMetrics.includes('speed')}
					{@const speeds = positions.map(p => p.speed ?? 0)}
					{@const maxSpeed = Math.max(...speeds)}
					{@const avgSpeed = speeds.reduce((a, b) => a + b, 0) / speeds.length}
					<div class="stat-item">
						<span class="stat-label">Avg Speed</span>
						<span class="stat-value">{avgSpeed.toFixed(1)} km/h</span>
					</div>
					<div class="stat-item">
						<span class="stat-label">Max Speed</span>
						<span class="stat-value">{maxSpeed.toFixed(1)} km/h</span>
					</div>
				{/if}
				{#if selectedMetrics.includes('altitude')}
					{@const alts = positions.map(p => p.altitude).filter((a): a is number => a !== null && a !== undefined)}
					{#if alts.length > 0}
						{@const minAlt = Math.min(...alts)}
						{@const maxAlt = Math.max(...alts)}
						<div class="stat-item">
							<span class="stat-label">Altitude Range</span>
							<span class="stat-value">{minAlt.toFixed(0)}m &ndash; {maxAlt.toFixed(0)}m</span>
						</div>
					{/if}
				{/if}
			</div>
		{:else if !fetching && !errorMsg && positions.length === 0}
			<div class="empty-state">
				<svg viewBox="0 0 24 24" width="64" height="64" fill="none" stroke="var(--text-tertiary)" stroke-width="1">
					<polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/>
				</svg>
				<p>Select a device and time period to view charts</p>
				<p class="empty-hint">Choose one or more metrics and click Apply to generate a chart from position data.</p>
			</div>
		{/if}
	</div>
</div>

<style>
	.charts-page {
		padding: var(--space-6) 0;
	}

	/* Header */
	.page-header {
		align-items: flex-start;
	}

	.page-title-group {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}

	.back-link {
		font-size: var(--text-sm);
		color: var(--accent-primary);
		text-decoration: none;
	}

	.back-link:hover {
		text-decoration: underline;
	}

	.header-actions {
		gap: var(--space-2);
		flex-shrink: 0;
	}

	/* Config panel */
	.config-panel {
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
		padding: var(--space-4);
		margin-bottom: var(--space-6);
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
	}

	.config-row {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		gap: var(--space-4);
	}

	.config-row-secondary {
		align-items: center;
		padding-top: var(--space-2);
		border-top: 1px solid var(--border-color);
	}

	.filter-group-action {
		align-self: flex-end;
	}

	.select-sm {
		padding: var(--space-2) var(--space-3);
		font-size: var(--text-sm);
		min-width: 120px;
	}

	/* Metric chips */
	.metrics-group {
		flex: 1;
		min-width: 0;
	}

	.metric-chips {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}

	.metric-chip {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		padding: var(--space-2) var(--space-3);
		background-color: var(--bg-primary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-full);
		color: var(--text-secondary);
		font-size: var(--text-sm);
		cursor: pointer;
		transition: all var(--transition-fast);
		white-space: nowrap;
	}

	.metric-chip:hover {
		background-color: var(--bg-hover);
		color: var(--text-primary);
	}

	.metric-chip.active {
		background-color: color-mix(in srgb, var(--chip-color) 15%, var(--bg-primary));
		border-color: var(--chip-color);
		color: var(--text-primary);
	}

	.chip-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.limit-group {
		flex-direction: row;
		align-items: center;
		gap: var(--space-3);
	}

	.position-count {
		font-size: var(--text-sm);
		color: var(--text-tertiary);
	}

	/* Error */
	.charts-error {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		padding: var(--space-3) var(--space-4);
		background-color: color-mix(in srgb, var(--error) 10%, var(--bg-secondary));
		border: 1px solid var(--error);
		border-radius: var(--radius-md);
		color: var(--error);
		font-size: var(--text-sm);
		margin-bottom: var(--space-4);
	}

	/* Chart */
	.chart-container {
		height: 450px;
		padding: var(--space-4);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
		margin-bottom: var(--space-4);
	}

	.chart-loading {
		margin-bottom: var(--space-4);
	}

	/* Stats row */
	.stats-row {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-4);
		padding: var(--space-4);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
	}

	.stat-item {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		min-width: 120px;
	}

	.stat-label {
		font-size: var(--text-xs);
		font-weight: var(--font-medium);
		color: var(--text-tertiary);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.stat-value {
		font-size: var(--text-sm);
		font-weight: var(--font-semibold);
		color: var(--text-primary);
	}

	/* Empty state */

	/* Responsive */
	@media (max-width: 768px) {
		.page-header {
			flex-direction: column;
			gap: var(--space-3);
		}

		.header-actions {
			width: 100%;
		}

		.config-row {
			flex-direction: column;
			align-items: stretch;
		}

		.filter-group-action {
			align-self: stretch;
		}

		.select {
			width: 100%;
			min-width: unset;
		}

		.chart-container {
			height: 300px;
			padding: var(--space-2);
		}

		.stats-row {
			flex-direction: column;
		}

		.metric-chips {
			gap: var(--space-1);
		}

		.metric-chip {
			font-size: var(--text-xs);
			padding: var(--space-1) var(--space-2);
		}
	}
</style>

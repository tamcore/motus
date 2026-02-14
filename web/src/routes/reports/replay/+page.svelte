<script lang="ts">
	import { onMount, onDestroy, tick } from 'svelte';
	import { page } from '$app/stores';
	import { api, fetchDevices } from '$lib/api/client';
	import { currentUser } from '$lib/stores/auth';
	import { refreshHandler } from '$lib/stores/refresh';
	import { useLeaflet } from '$lib/composables/useLeaflet';
	import { theme } from '$lib/stores/theme';
	import { haversineDistance } from '$lib/utils/trips';
	import { hasMetricData } from '$lib/utils/chart-metrics';
	import { formatDate, formatDuration, formatSpeed, formatDistance } from '$lib/utils/formatting';
	import type { Device, Position } from '$lib/types/api';
	import { Chart, registerables } from 'chart.js';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import Button from '$lib/components/Button.svelte';
	import AllDevicesToggle from '$lib/components/AllDevicesToggle.svelte';

	Chart.register(...registerables);

	// ---------------------------------------------------------------------------
	// Theme tracking
	// ---------------------------------------------------------------------------
	let isDark = true;
	const unsubscribeTheme = theme.subscribe((t) => {
		if (t === 'auto' && typeof window !== 'undefined') {
			isDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
		} else {
			isDark = t !== 'light';
		}
	});

	// ---------------------------------------------------------------------------
	// Helpers (API Position uses fixTime directly)
	// ---------------------------------------------------------------------------
	function getTime(pos: Position): string {
		return pos.fixTime;
	}

	function calcTotalDistance(positions: Position[]): number {
		let total = 0;
		for (let i = 1; i < positions.length; i++) {
			total += haversineDistance(
				positions[i - 1].latitude, positions[i - 1].longitude,
				positions[i].latitude, positions[i].longitude
			);
		}
		return total;
	}

	// ---------------------------------------------------------------------------
	// Leaflet composable
	// ---------------------------------------------------------------------------
	const leafletMap = useLeaflet();

	// ---------------------------------------------------------------------------
	// DOM refs
	// ---------------------------------------------------------------------------
	let mapEl: HTMLDivElement;
	let chartCanvas: HTMLCanvasElement;
	let containerEl: HTMLDivElement;

	// ---------------------------------------------------------------------------
	// State: data
	// ---------------------------------------------------------------------------
	let devices: Device[] = [];
	let positions: Position[] = [];
	let loading = true;
	let fetching = false;
	let errorMessage = '';

	// ---------------------------------------------------------------------------
	// State: filters (from query params or user input)
	// ---------------------------------------------------------------------------
	let selectedDeviceId = '';
	let datePreset: 'day' | 'week' | 'month' | 'custom' = 'week';
	let customFrom = '';
	let customTo = '';

	// ---------------------------------------------------------------------------
	// State: playback
	// ---------------------------------------------------------------------------
	let currentIndex = 0;
	let playing = false;
	let playbackSpeed = 1;
	let cameraFollow = true;
	let animFrameId: number | null = null;
	let lastFrameTime = 0;
	let accumulatedTime = 0;

	const PLAYBACK_SPEEDS = [1, 2, 5, 10, 25, 50];

	// ---------------------------------------------------------------------------
	// State: chart
	// ---------------------------------------------------------------------------
	let chartInstance: Chart | null = null;
	let chartPanelOpen = true;
	let chartMetric: 'speed' | 'altitude' | 'course' = 'speed';
	let hasAltitudeData = false;
	let chartPanelHeight = 250;
	let isDraggingSeparator = false;
	let chartCursorColor = '#ffffff';

	// ---------------------------------------------------------------------------
	// State: map layers (Leaflet objects)
	// ---------------------------------------------------------------------------
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let marker: any = null;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let routePolyline: any = null;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let traveledPolyline: any = null;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let startMarker: any = null;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let endMarker: any = null;

	let isFullscreen = false;

	// ---------------------------------------------------------------------------
	// Derived values
	// ---------------------------------------------------------------------------
	$: currentPosition = positions.length > 0 ? positions[currentIndex] : null;
	$: totalDistance = positions.length > 1 ? calcTotalDistance(positions) : 0;
	$: traveledDistance = currentIndex > 0
		? calcTotalDistance(positions.slice(0, currentIndex + 1))
		: 0;
	$: totalDurationSec = positions.length > 1
		? (new Date(getTime(positions[positions.length - 1])).getTime()
			- new Date(getTime(positions[0])).getTime()) / 1000
		: 0;
	$: elapsedDurationSec = currentIndex > 0 && positions.length > 1
		? (new Date(getTime(positions[currentIndex])).getTime()
			- new Date(getTime(positions[0])).getTime()) / 1000
		: 0;
	$: avgSpeed = totalDurationSec > 0 ? totalDistance / (totalDurationSec / 3600) : 0;
	$: maxSpeed = positions.length > 0
		? Math.max(...positions.map((p) => p.speed ?? 0))
		: 0;
	$: progress = positions.length > 1
		? (currentIndex / (positions.length - 1)) * 100
		: 0;

	// Parse initial query params
	$: qDeviceId = $page.url.searchParams.get('deviceId');
	$: qFrom = $page.url.searchParams.get('from');
	$: qTo = $page.url.searchParams.get('to');

	// ---------------------------------------------------------------------------
	// Lifecycle
	// ---------------------------------------------------------------------------
	onMount(async () => {
		await leafletMap.initialize(mapEl, {
			center: [49.79, 9.95],
			zoom: 6,
			zoomControl: true,
		});

		try {
			const isAdmin = ($currentUser as Record<string, unknown> | null)?.administrator === true;
			devices = (await fetchDevices(isAdmin)) as unknown as Device[];
		} catch (e) {
			console.error('Failed to load devices:', e);
		}

		// If query params provided, auto-load
		if (qDeviceId && qFrom && qTo) {
			selectedDeviceId = qDeviceId;
			datePreset = 'custom';
			// Extract date-only for the custom inputs
			customFrom = qFrom.slice(0, 10);
			customTo = qTo.slice(0, 10);
			await loadPositions(Number(qDeviceId), qFrom, qTo);
		}

		loading = false;
		$refreshHandler = reloadDevices;
	});

	onDestroy(() => {
		$refreshHandler = null;
		stopPlayback();
		destroyChart();
		leafletMap.cleanup();
		unsubscribeTheme();
	});

	async function reloadDevices() {
		const isAdmin = ($currentUser as Record<string, unknown> | null)?.administrator === true;
		try {
			devices = (await fetchDevices(isAdmin)) as unknown as Device[];
		} catch {
			console.error('Failed to reload devices');
		}
	}

	// ---------------------------------------------------------------------------
	// Data loading
	// ---------------------------------------------------------------------------
	function getDateRange(): { from: string; to: string } {
		const now = new Date();
		const to = now.toISOString();
		if (datePreset === 'custom') {
			return {
				from: customFrom ? new Date(customFrom).toISOString() : to,
				to: customTo ? new Date(customTo + 'T23:59:59').toISOString() : to,
			};
		}
		const from = new Date(now);
		if (datePreset === 'day') from.setDate(from.getDate() - 1);
		else if (datePreset === 'week') from.setDate(from.getDate() - 7);
		else if (datePreset === 'month') from.setDate(from.getDate() - 30);
		return { from: from.toISOString(), to };
	}

	async function handleLoad() {
		if (!selectedDeviceId) {
			errorMessage = 'Please select a device.';
			return;
		}
		errorMessage = '';
		const { from, to } = getDateRange();
		await loadPositions(Number(selectedDeviceId), from, to);
	}

	async function loadPositions(deviceId: number, from: string, to: string) {
		fetching = true;
		errorMessage = '';
		stopPlayback();
		positions = [];
		currentIndex = 0;
		clearMapLayers();

		try {
			const raw = await api.getPositions({ deviceId, from, to, limit: 10000 });
			const sorted = ([...raw] as unknown as Position[]).sort(
				(a, b) => new Date(getTime(a)).getTime() - new Date(getTime(b)).getTime()
			);
			positions = sorted;
			hasAltitudeData = hasMetricData(positions, 'altitude');
			if (chartMetric === 'altitude' && !hasAltitudeData) {
				chartMetric = 'speed';
			}

			if (positions.length === 0) {
				errorMessage = 'No positions found for the selected device and time range.';
			} else {
				// Wait for Svelte to update the DOM (filters hide, map area expands)
				// then invalidate the map size and draw the route with fitBounds.
				// Two invalidateSize calls are needed: one after tick() for the Svelte
				// DOM update, and one after requestAnimationFrame for the browser to
				// complete the layout reflow. This ensures Leaflet detects the correct
				// container dimensions and loads tiles for the visible viewport.
				await tick();
				const map = leafletMap.getMap();
				if (map) {
					map.invalidateSize();
				}
				drawRoute();
				buildChart();

				// Second invalidateSize after the browser has fully reflowed the layout
				requestAnimationFrame(() => {
					const m = leafletMap.getMap();
					if (m) {
						m.invalidateSize();
					}
				});
			}
		} catch (e) {
			console.error('Failed to load positions:', e);
			errorMessage = 'Failed to load positions. Please try again.';
		} finally {
			fetching = false;
		}
	}

	// ---------------------------------------------------------------------------
	// Map drawing
	// ---------------------------------------------------------------------------
	function clearMapLayers() {
		const map = leafletMap.getMap();
		if (marker && map) { map.removeLayer(marker); marker = null; }
		if (routePolyline && map) { map.removeLayer(routePolyline); routePolyline = null; }
		if (traveledPolyline && map) { map.removeLayer(traveledPolyline); traveledPolyline = null; }
		if (startMarker && map) { map.removeLayer(startMarker); startMarker = null; }
		if (endMarker && map) { map.removeLayer(endMarker); endMarker = null; }
	}

	function drawRoute() {
		const L = leafletMap.getLeaflet();
		const map = leafletMap.getMap();
		if (!L || !map || positions.length === 0) return;

		const coords = positions.map((p) => [p.latitude, p.longitude] as [number, number]);

		// Full route (dimmed)
		routePolyline = L.polyline(coords, {
			color: '#555555',
			weight: 3,
			opacity: 0.5,
		}).addTo(map);

		// Traveled portion (bright)
		traveledPolyline = L.polyline([], {
			color: '#00d4ff',
			weight: 4,
			opacity: 0.9,
		}).addTo(map);

		// Start marker (green circle)
		const first = positions[0];
		startMarker = L.circleMarker([first.latitude, first.longitude], {
			radius: 8,
			fillColor: '#00ff88',
			fillOpacity: 1,
			color: 'white',
			weight: 2,
		}).addTo(map).bindPopup('Start');

		// End marker (red circle)
		const last = positions[positions.length - 1];
		endMarker = L.circleMarker([last.latitude, last.longitude], {
			radius: 8,
			fillColor: '#ff4444',
			fillOpacity: 1,
			color: 'white',
			weight: 2,
		}).addTo(map).bindPopup('End');

		// Animated marker (directional triangle)
		marker = createDirectionalMarker(L, first);
		marker.addTo(map);

		// Fit bounds
		map.fitBounds(routePolyline.getBounds().pad(0.1));

		updateMapPosition();
	}

	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	function createDirectionalMarker(L: any, pos: Position): any {
		const course = pos.course ?? 0;
		return L.marker([pos.latitude, pos.longitude], {
			icon: L.divIcon({
				className: 'replay-marker',
				html: `<div style="
					width: 0; height: 0;
					border-left: 10px solid transparent;
					border-right: 10px solid transparent;
					border-bottom: 20px solid #00d4ff;
					transform: rotate(${course}deg);
					filter: drop-shadow(0 2px 4px rgba(0,0,0,0.5));
				"></div>`,
				iconSize: [20, 20],
				iconAnchor: [10, 10],
			}),
			zIndexOffset: 1000,
		});
	}

	function updateMapPosition() {
		const L = leafletMap.getLeaflet();
		const map = leafletMap.getMap();
		if (!L || !map || !marker || positions.length === 0) return;

		const pos = positions[currentIndex];
		const course = pos.course ?? 0;

		// Update marker position and rotation
		marker.setLatLng([pos.latitude, pos.longitude]);
		const iconEl = marker.getElement();
		if (iconEl) {
			const triangle = iconEl.querySelector('div');
			if (triangle) {
				triangle.style.transform = `rotate(${course}deg)`;
			}
		}

		// Update traveled polyline
		const traveledCoords = positions
			.slice(0, currentIndex + 1)
			.map((p) => [p.latitude, p.longitude] as [number, number]);
		if (traveledPolyline) {
			traveledPolyline.setLatLngs(traveledCoords);
		}

		// Camera follow
		if (cameraFollow) {
			map.panTo([pos.latitude, pos.longitude], { animate: true, duration: 0.3 });
		}
	}

	// ---------------------------------------------------------------------------
	// Interpolation helper for smooth animation
	// ---------------------------------------------------------------------------
	function getInterpolatedPosition(
		p1: Position,
		p2: Position,
		fraction: number
	): { lat: number; lng: number; course: number } {
		const lat = p1.latitude + (p2.latitude - p1.latitude) * fraction;
		const lng = p1.longitude + (p2.longitude - p1.longitude) * fraction;
		// Interpolate course angle (handle wrapping)
		const c1 = p1.course ?? 0;
		const c2 = p2.course ?? 0;
		let diff = c2 - c1;
		if (diff > 180) diff -= 360;
		if (diff < -180) diff += 360;
		const course = c1 + diff * fraction;
		return { lat, lng, course };
	}

	// ---------------------------------------------------------------------------
	// Playback engine using requestAnimationFrame
	// ---------------------------------------------------------------------------
	function startPlayback() {
		if (positions.length < 2) return;
		if (currentIndex >= positions.length - 1) {
			currentIndex = 0;
			updateMapPosition();
			updateChartCursor();
		}
		playing = true;
		lastFrameTime = performance.now();
		accumulatedTime = 0;
		animFrameId = requestAnimationFrame(playbackTick);
	}

	function playbackTick(timestamp: number) {
		if (!playing) return;

		const deltaMs = timestamp - lastFrameTime;
		lastFrameTime = timestamp;

		// Accumulate real time scaled by playback speed
		accumulatedTime += deltaMs * playbackSpeed;

		// How much GPS time should advance
		if (currentIndex < positions.length - 1) {
			const currentTime = new Date(getTime(positions[currentIndex])).getTime();
			const nextTime = new Date(getTime(positions[currentIndex + 1])).getTime();
			const gpsDelta = nextTime - currentTime;

			if (gpsDelta <= 0 || accumulatedTime >= gpsDelta) {
				// Move to next position
				accumulatedTime -= Math.max(gpsDelta, 0);
				currentIndex++;
				updateMapPosition();
				updateChartCursor();

				if (currentIndex >= positions.length - 1) {
					stopPlayback();
					return;
				}
			} else {
				// Interpolate between current and next position for smooth animation
				const fraction = accumulatedTime / gpsDelta;
				const interp = getInterpolatedPosition(
					positions[currentIndex],
					positions[currentIndex + 1],
					fraction
				);
				const L = leafletMap.getLeaflet();
				const map = leafletMap.getMap();
				if (L && map && marker) {
					marker.setLatLng([interp.lat, interp.lng]);
					const iconEl = marker.getElement();
					if (iconEl) {
						const triangle = iconEl.querySelector('div');
						if (triangle) {
							triangle.style.transform = `rotate(${interp.course}deg)`;
						}
					}
					if (cameraFollow) {
						map.panTo([interp.lat, interp.lng], { animate: false });
					}
				}
			}
		}

		animFrameId = requestAnimationFrame(playbackTick);
	}

	function pausePlayback() {
		playing = false;
		if (animFrameId !== null) {
			cancelAnimationFrame(animFrameId);
			animFrameId = null;
		}
		// Snap to nearest discrete position
		updateMapPosition();
	}

	function stopPlayback() {
		playing = false;
		if (animFrameId !== null) {
			cancelAnimationFrame(animFrameId);
			animFrameId = null;
		}
	}

	function handlePlayPause() {
		if (playing) {
			pausePlayback();
		} else {
			startPlayback();
		}
	}

	function handleReset() {
		stopPlayback();
		currentIndex = 0;
		accumulatedTime = 0;
		updateMapPosition();
		updateChartCursor();
	}

	function handleProgressChange(e: Event) {
		const target = e.target as HTMLInputElement;
		const newIndex = parseInt(target.value);
		const wasPlaying = playing;
		if (wasPlaying) pausePlayback();
		currentIndex = newIndex;
		accumulatedTime = 0;
		updateMapPosition();
		updateChartCursor();
		if (wasPlaying) startPlayback();
	}

	function handleSpeedChange(newSpeed: number) {
		playbackSpeed = newSpeed;
	}

	// ---------------------------------------------------------------------------
	// Chart
	// ---------------------------------------------------------------------------

	/** Convert hex color to rgba string. */
	function hexToRgba(hex: string, alpha: number): string {
		const r = parseInt(hex.slice(1, 3), 16);
		const g = parseInt(hex.slice(3, 5), 16);
		const b = parseInt(hex.slice(5, 7), 16);
		return `rgba(${r}, ${g}, ${b}, ${alpha})`;
	}

	function getChartData(): { labels: string[]; data: number[]; label: string; color: string } {
		const labels = positions.map((p) => {
			const d = new Date(getTime(p));
			return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
		});

		switch (chartMetric) {
			case 'altitude':
				return {
					labels,
					data: positions.map((p) => p.altitude ?? 0),
					label: 'Altitude (m)',
					color: '#00ff88',
				};
			case 'course':
				return {
					labels,
					data: positions.map((p) => p.course ?? 0),
					label: 'Course (deg)',
					color: '#ffaa00',
				};
			case 'speed':
			default:
				return {
					labels,
					data: positions.map((p) => p.speed ?? 0),
					label: 'Speed (km/h)',
					color: '#00d4ff',
				};
		}
	}

	function buildChart() {
		if (!chartCanvas || positions.length === 0) return;
		destroyChart();

		const ctx = chartCanvas.getContext('2d');
		if (!ctx) return;

		const chartData = getChartData();

		// Theme-aware colors (matching reports/charts page)
		const gridColor = isDark ? '#3a3a3a' : '#e0e0e0';
		const tickColor = isDark ? '#a0a0a0' : '#666666';
		const tooltipBg = isDark ? '#2d2d2d' : '#ffffff';
		const tooltipText = isDark ? '#ffffff' : '#1a1a1a';
		const tooltipBorder = isDark ? '#404040' : '#e0e0e0';
		const cursorPointBg = isDark ? '#ffffff' : '#1a1a1a';

		chartInstance = new Chart(ctx, {
			type: 'line',
			data: {
				labels: chartData.labels,
				datasets: [{
					label: chartData.label,
					data: chartData.data,
					borderColor: chartData.color,
					backgroundColor: hexToRgba(chartData.color, 0.15),
					fill: true,
					tension: 0.2,
					pointRadius: 0,
					pointHitRadius: 8,
					borderWidth: 2,
				}],
			},
			options: {
				responsive: true,
				maintainAspectRatio: false,
				animation: false,
				interaction: {
					mode: 'index',
					intersect: false,
				},
				plugins: {
					legend: {
						display: false,
					},
					tooltip: {
						backgroundColor: tooltipBg,
						titleColor: tooltipText,
						bodyColor: tooltipText,
						borderColor: tooltipBorder,
						borderWidth: 1,
						padding: 10,
						callbacks: {
							title: (items) => {
								if (items.length > 0) {
									return chartData.labels[items[0].dataIndex];
								}
								return '';
							},
						},
					},
				},
				scales: {
					x: {
						display: true,
						ticks: {
							color: tickColor,
							maxRotation: 0,
							autoSkip: true,
							maxTicksLimit: 10,
							font: { size: 10 },
						},
						grid: { color: gridColor },
					},
					y: {
						display: true,
						ticks: { color: tickColor, font: { size: 10 } },
						grid: { color: gridColor },
					},
				},
				onClick: (_event, elements) => {
					if (elements.length > 0) {
						const idx = elements[0].index;
						jumpToIndex(idx);
					}
				},
			},
		});

		// Store cursorPointBg for updateChartCursor to use
		chartCursorColor = cursorPointBg;
		updateChartCursor();
	}

	function destroyChart() {
		if (chartInstance) {
			chartInstance.destroy();
			chartInstance = null;
		}
	}

	function updateChartCursor() {
		if (!chartInstance || positions.length === 0) return;

		// Use a vertical line annotation via a custom plugin approach:
		// We create a second dataset that's transparent except at currentIndex
		const cursorData = positions.map((_, i) => i === currentIndex ? chartInstance!.data.datasets[0].data[i] as number : null);

		// Use the current metric's color for the cursor border ring
		const chartData = getChartData();

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const cursorDataset: any = {
			label: 'Cursor',
			data: cursorData,
			borderColor: 'transparent',
			backgroundColor: 'transparent',
			pointRadius: positions.map((_, i) => i === currentIndex ? 6 : 0),
			pointBackgroundColor: chartCursorColor,
			pointBorderColor: chartData.color,
			pointBorderWidth: 2,
			showLine: false,
			fill: false,
		};

		if (chartInstance.data.datasets.length < 2) {
			chartInstance.data.datasets.push(cursorDataset);
		} else {
			chartInstance.data.datasets[1].data = cursorData;
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			const ds = chartInstance.data.datasets[1] as any;
			ds.pointRadius = positions.map((_, i) => i === currentIndex ? 6 : 0);
			ds.pointBackgroundColor = chartCursorColor;
			ds.pointBorderColor = chartData.color;
		}

		chartInstance.update('none');
	}

	function jumpToIndex(idx: number) {
		const wasPlaying = playing;
		if (wasPlaying) pausePlayback();
		currentIndex = Math.max(0, Math.min(idx, positions.length - 1));
		accumulatedTime = 0;
		updateMapPosition();
		updateChartCursor();
		if (wasPlaying) startPlayback();
	}

	// Rebuild chart when metric or theme changes
	$: if (chartMetric && positions.length > 0 && chartCanvas) {
		rebuildChartWithTheme(isDark);
	}

	// Wrapper to make isDark a dependency so chart rebuilds on theme change
	function rebuildChartWithTheme(_isDark: boolean) {
		buildChart();
	}

	// ---------------------------------------------------------------------------
	// Separator drag for chart panel resize
	// ---------------------------------------------------------------------------
	function handleSeparatorMouseDown(e: MouseEvent) {
		e.preventDefault();
		isDraggingSeparator = true;
		document.addEventListener('mousemove', handleSeparatorMouseMove);
		document.addEventListener('mouseup', handleSeparatorMouseUp);
	}

	function handleSeparatorMouseMove(e: MouseEvent) {
		if (!isDraggingSeparator || !containerEl) return;
		const containerRect = containerEl.getBoundingClientRect();
		const newHeight = containerRect.bottom - e.clientY;
		chartPanelHeight = Math.max(120, Math.min(newHeight, containerRect.height - 200));
	}

	function handleSeparatorMouseUp() {
		isDraggingSeparator = false;
		document.removeEventListener('mousemove', handleSeparatorMouseMove);
		document.removeEventListener('mouseup', handleSeparatorMouseUp);
		// Resize chart after drag
		if (chartInstance) {
			chartInstance.resize();
		}
	}

	// ---------------------------------------------------------------------------
	// Fullscreen toggle
	// ---------------------------------------------------------------------------
	function toggleFullscreen() {
		if (!containerEl) return;
		if (!document.fullscreenElement) {
			containerEl.requestFullscreen().then(() => { isFullscreen = true; });
		} else {
			document.exitFullscreen().then(() => { isFullscreen = false; });
		}
	}
</script>

<svelte:head><title>Drive Replay - Motus</title></svelte:head>

<div class="replay-page" bind:this={containerEl}>
	<!-- Filters bar -->
	{#if positions.length === 0 && !fetching}
		<div class="filters-section">
			<div class="container">
				<h1 class="page-title">Drive Replay</h1>
				<p class="page-subtitle">Replay a GPS drive with synchronized map and chart visualization.</p>

				<div class="filters-bar">
					<div class="filter-group">
						<label for="replay-device" class="filter-label">Device <AllDevicesToggle on:change={reloadDevices} /></label>
						{#if loading}
							<Skeleton width="200px" height="40px" />
						{:else}
							<select id="replay-device" bind:value={selectedDeviceId} class="select">
								<option value="">Select device...</option>
								{#each devices as device}
									<option value={device.id}>{device.name}</option>
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
							<label for="replay-from" class="filter-label">From</label>
							<input id="replay-from" type="date" bind:value={customFrom} class="date-input" />
							<label for="replay-to" class="filter-label">To</label>
							<input id="replay-to" type="date" bind:value={customTo} class="date-input" />
						</div>
					{/if}

					<Button on:click={handleLoad} loading={fetching}>Load Drive</Button>
				</div>

				{#if errorMessage}
					<div class="error-message">{errorMessage}</div>
				{/if}
			</div>
		</div>
	{/if}

	<!-- Main replay area -->
	<div class="replay-layout" class:has-data={positions.length > 0} class:has-chart={chartPanelOpen && positions.length > 0}>
		<!-- Map area -->
		<div class="map-area" style={chartPanelOpen && positions.length > 0 ? `flex: 1 1 0; min-height: 300px;` : ''}>
			<div bind:this={mapEl} class="replay-map"></div>

			{#if fetching}
				<div class="map-loading">
					<div class="spinner"></div>
					<span>Loading positions...</span>
				</div>
			{/if}

			<!-- Info overlay (top-right) -->
			{#if currentPosition}
				<div class="info-overlay">
					<div class="info-row">
						<span class="info-label">Time</span>
						<span class="info-value">{formatDate(getTime(currentPosition))}</span>
					</div>
					<div class="info-row">
						<span class="info-label">Speed</span>
						<span class="info-value">{formatSpeed(currentPosition.speed)}</span>
					</div>
					<div class="info-row">
						<span class="info-label">Position</span>
						<span class="info-value">
							{currentPosition.latitude.toFixed(5)}, {currentPosition.longitude.toFixed(5)}
						</span>
					</div>
					{#if currentPosition.altitude != null}
						<div class="info-row">
							<span class="info-label">Altitude</span>
							<span class="info-value">{currentPosition.altitude.toFixed(0)} m</span>
						</div>
					{/if}
				</div>
			{/if}

			<!-- Map controls (top-left) -->
			{#if positions.length > 0}
				<div class="map-controls">
					<button
						class="map-control-btn"
						class:active={cameraFollow}
						on:click={() => cameraFollow = !cameraFollow}
						title={cameraFollow ? 'Disable camera follow' : 'Enable camera follow'}
						aria-label="Toggle camera follow"
					>
						<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
							<circle cx="12" cy="12" r="10"/>
							<circle cx="12" cy="12" r="3"/>
							<line x1="12" y1="2" x2="12" y2="6"/>
							<line x1="12" y1="18" x2="12" y2="22"/>
							<line x1="2" y1="12" x2="6" y2="12"/>
							<line x1="18" y1="12" x2="22" y2="12"/>
						</svg>
					</button>
					<button
						class="map-control-btn"
						on:click={toggleFullscreen}
						title={isFullscreen ? 'Exit fullscreen' : 'Fullscreen'}
						aria-label="Toggle fullscreen"
					>
						<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
							{#if isFullscreen}
								<polyline points="4 14 8 14 8 18"/>
								<polyline points="20 10 16 10 16 6"/>
								<line x1="14" y1="10" x2="21" y2="3"/>
								<line x1="3" y1="21" x2="10" y2="14"/>
							{:else}
								<polyline points="15 3 21 3 21 9"/>
								<polyline points="9 21 3 21 3 15"/>
								<line x1="21" y1="3" x2="14" y2="10"/>
								<line x1="3" y1="21" x2="10" y2="14"/>
							{/if}
						</svg>
					</button>
					<button
						class="map-control-btn"
						on:click={() => { chartPanelOpen = !chartPanelOpen; }}
						title={chartPanelOpen ? 'Hide chart' : 'Show chart'}
						aria-label="Toggle chart panel"
					>
						<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
							<line x1="18" y1="20" x2="18" y2="10"/>
							<line x1="12" y1="20" x2="12" y2="4"/>
							<line x1="6" y1="20" x2="6" y2="14"/>
						</svg>
					</button>
					<button
						class="map-control-btn"
						on:click={() => { stopPlayback(); positions = []; currentIndex = 0; clearMapLayers(); destroyChart(); errorMessage = ''; }}
						title="Change device/dates"
						aria-label="Back to selection"
					>
						<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
							<line x1="19" y1="12" x2="5" y2="12"/>
							<polyline points="12 19 5 12 12 5"/>
						</svg>
					</button>
				</div>
			{/if}

			<!-- Stats bar (bottom-left, above controls) -->
			{#if positions.length > 0}
				<div class="stats-bar">
					<div class="stat-item">
						<span class="stat-label">Distance</span>
						<span class="stat-value">{formatDistance(totalDistance)}</span>
					</div>
					<div class="stat-item">
						<span class="stat-label">Duration</span>
						<span class="stat-value">{formatDuration(totalDurationSec)}</span>
					</div>
					<div class="stat-item">
						<span class="stat-label">Avg Speed</span>
						<span class="stat-value">{formatSpeed(avgSpeed)}</span>
					</div>
					<div class="stat-item">
						<span class="stat-label">Max Speed</span>
						<span class="stat-value">{formatSpeed(maxSpeed)}</span>
					</div>
				</div>
			{/if}
		</div>

		<!-- Draggable separator -->
		{#if chartPanelOpen && positions.length > 0}
			<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
			<div
				class="panel-separator"
				on:mousedown={handleSeparatorMouseDown}
				role="separator"
				aria-orientation="horizontal"
				aria-label="Resize chart panel"
			>
				<div class="separator-handle"></div>
			</div>
		{/if}

		<!-- Chart panel -->
		{#if chartPanelOpen && positions.length > 0}
			<div class="chart-panel" style="height: {chartPanelHeight}px; flex-shrink: 0;">
				<div class="chart-header">
					<div class="chart-metric-selector">
						<button class="metric-btn" class:active={chartMetric === 'speed'}
							on:click={() => chartMetric = 'speed'}>Speed</button>
						{#if hasAltitudeData}
							<button class="metric-btn" class:active={chartMetric === 'altitude'}
								on:click={() => chartMetric = 'altitude'}>Altitude</button>
						{/if}
						<button class="metric-btn" class:active={chartMetric === 'course'}
							on:click={() => chartMetric = 'course'}>Course</button>
					</div>
				</div>
				<div class="chart-body">
					<canvas bind:this={chartCanvas}></canvas>
				</div>
			</div>
		{/if}

		<!-- Playback controls (fixed at bottom) -->
		{#if positions.length > 0}
			<div class="playback-bar">
				<div class="progress-row">
					<span class="time-label">{currentPosition ? formatDate(getTime(currentPosition)) : '--'}</span>
					<input
						type="range"
						class="progress-slider"
						min="0"
						max={positions.length - 1}
						value={currentIndex}
						on:input={handleProgressChange}
						aria-label="Playback progress"
					/>
					<span class="time-label">{formatDistance(traveledDistance)} / {formatDistance(totalDistance)}</span>
				</div>

				<div class="controls-row">
					<button class="ctrl-btn" on:click={handleReset} title="Reset" aria-label="Reset playback">
						<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
							<polygon points="19 20 9 12 19 4 19 20"/>
							<line x1="5" y1="19" x2="5" y2="5"/>
						</svg>
					</button>

					<button class="ctrl-btn primary" on:click={handlePlayPause} title={playing ? 'Pause' : 'Play'}
						aria-label={playing ? 'Pause playback' : 'Start playback'}>
						{#if playing}
							<svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor">
								<rect x="6" y="4" width="4" height="16"/>
								<rect x="14" y="4" width="4" height="16"/>
							</svg>
						{:else}
							<svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor">
								<polygon points="5 3 19 12 5 21 5 3"/>
							</svg>
						{/if}
					</button>

					<div class="speed-selector">
						{#each PLAYBACK_SPEEDS as s}
							<button
								class="speed-btn"
								class:active={playbackSpeed === s}
								on:click={() => handleSpeedChange(s)}
								aria-label="Set speed to {s}x"
							>{s}x</button>
						{/each}
					</div>

					<span class="position-counter">{currentIndex + 1} / {positions.length}</span>
				</div>
			</div>
		{/if}
	</div>
</div>

<style>
	.replay-page {
		height: calc(100vh - 65px);
		display: flex;
		flex-direction: column;
		overflow: hidden;
		background-color: var(--bg-primary);
	}

	/* Filters section (shown when no data loaded) */
	.filters-section {
		padding: var(--space-6) 0;
	}

	.container {
		max-width: 1200px;
		margin: 0 auto;
		padding: 0 var(--space-4);
	}

	.page-title {
		font-size: var(--text-3xl);
		font-weight: var(--font-bold);
		color: var(--text-primary);
		margin: 0 0 var(--space-2);
	}

	.page-subtitle {
		color: var(--text-secondary);
		font-size: var(--text-base);
		margin: 0 0 var(--space-6);
	}

	.filters-bar {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		gap: var(--space-4);
		padding: var(--space-4);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
	}

	.filter-group {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	.filter-label {
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		color: var(--text-secondary);
	}

	.select {
		padding: var(--space-3) var(--space-4);
		background-color: var(--bg-primary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-base);
		min-width: 200px;
	}

	.preset-buttons {
		display: flex;
		gap: var(--space-1);
	}

	.preset-btn {
		padding: var(--space-2) var(--space-3);
		background-color: var(--bg-primary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-sm);
		cursor: pointer;
		transition: all var(--transition-fast);
	}

	.preset-btn:hover {
		background-color: var(--bg-hover);
	}

	.preset-btn.active {
		background-color: var(--accent-primary);
		color: var(--text-inverse);
		border-color: var(--accent-primary);
	}

	.date-inputs {
		flex-direction: row;
		align-items: center;
	}

	.date-input {
		padding: var(--space-2) var(--space-3);
		background-color: var(--bg-primary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-sm);
	}

	.error-message {
		margin-top: var(--space-4);
		padding: var(--space-3) var(--space-4);
		background-color: rgba(255, 68, 68, 0.1);
		border: 1px solid var(--error);
		border-radius: var(--radius-md);
		color: var(--error);
		font-size: var(--text-sm);
	}

	/* Main replay layout */
	.replay-layout {
		flex: 1;
		display: flex;
		flex-direction: column;
		overflow: hidden;
		position: relative;
	}

	.replay-layout:not(.has-data) {
		flex: none;
	}

	/* Map area */
	.map-area {
		flex: 1;
		position: relative;
		min-height: 0;
	}

	.replay-map {
		width: 100%;
		height: 100%;
	}

	.replay-page :global(.leaflet-container) {
		height: 100%;
		width: 100%;
		background-color: var(--bg-tertiary);
	}

	.map-loading {
		position: absolute;
		top: 50%;
		left: 50%;
		transform: translate(-50%, -50%);
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: var(--space-3);
		color: var(--text-secondary);
		z-index: 500;
	}

	.spinner {
		width: 40px;
		height: 40px;
		border: 4px solid var(--border-color);
		border-top-color: var(--accent-primary);
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}

	@keyframes spin {
		to { transform: rotate(360deg); }
	}

	/* Info overlay */
	.info-overlay {
		position: absolute;
		top: var(--space-3);
		right: var(--space-3);
		background-color: var(--bg-primary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
		padding: var(--space-3);
		box-shadow: var(--shadow-lg);
		z-index: 500;
		min-width: 220px;
		font-size: var(--text-sm);
	}

	.info-row {
		display: flex;
		justify-content: space-between;
		gap: var(--space-4);
		padding: var(--space-1) 0;
	}

	.info-row:not(:last-child) {
		border-bottom: 1px solid var(--border-color);
	}

	.info-label {
		color: var(--text-tertiary);
		font-weight: var(--font-medium);
	}

	.info-value {
		color: var(--text-primary);
		font-family: monospace;
	}

	/* Map controls (top-left) */
	.map-controls {
		position: absolute;
		top: var(--space-3);
		left: var(--space-3);
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		z-index: 500;
	}

	.map-control-btn {
		width: 36px;
		height: 36px;
		display: flex;
		align-items: center;
		justify-content: center;
		background-color: var(--bg-primary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-secondary);
		cursor: pointer;
		transition: all var(--transition-fast);
		box-shadow: var(--shadow-sm);
	}

	.map-control-btn:hover {
		background-color: var(--bg-hover);
		color: var(--text-primary);
	}

	.map-control-btn.active {
		background-color: var(--accent-primary);
		color: var(--text-inverse);
		border-color: var(--accent-primary);
	}

	/* Stats bar */
	.stats-bar {
		position: absolute;
		bottom: var(--space-3);
		left: var(--space-3);
		display: flex;
		gap: var(--space-4);
		background-color: var(--bg-primary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
		padding: var(--space-2) var(--space-4);
		box-shadow: var(--shadow-md);
		z-index: 500;
		font-size: var(--text-xs);
	}

	.stat-item {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 1px;
	}

	.stat-label {
		color: var(--text-tertiary);
		font-size: var(--text-xs);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.stat-value {
		color: var(--text-primary);
		font-weight: var(--font-semibold);
		font-size: var(--text-sm);
		white-space: nowrap;
	}

	/* Panel separator (draggable) */
	.panel-separator {
		height: 8px;
		background-color: var(--bg-secondary);
		border-top: 1px solid var(--border-color);
		border-bottom: 1px solid var(--border-color);
		cursor: row-resize;
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
		z-index: 10;
	}

	.separator-handle {
		width: 40px;
		height: 3px;
		background-color: var(--text-tertiary);
		border-radius: var(--radius-full);
	}

	.panel-separator:hover .separator-handle {
		background-color: var(--accent-primary);
	}

	/* Chart panel */
	.chart-panel {
		display: flex;
		flex-direction: column;
		background-color: var(--bg-secondary);
		overflow: hidden;
	}

	.chart-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: var(--space-2) var(--space-4);
		border-bottom: 1px solid var(--border-color);
		flex-shrink: 0;
	}

	.chart-metric-selector {
		display: flex;
		gap: var(--space-1);
	}

	.metric-btn {
		padding: var(--space-1) var(--space-3);
		background-color: var(--bg-tertiary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-sm);
		color: var(--text-primary);
		cursor: pointer;
		font-size: var(--text-xs);
		transition: all var(--transition-fast);
	}

	.metric-btn:hover {
		background-color: var(--bg-hover);
	}

	.metric-btn.active {
		background-color: var(--accent-primary);
		color: var(--text-inverse);
		border-color: var(--accent-primary);
	}

	.chart-body {
		flex: 1;
		padding: var(--space-2) var(--space-3);
		min-height: 0;
	}

	.chart-body canvas {
		width: 100% !important;
		height: 100% !important;
	}

	/* Playback bar (fixed at bottom of layout) */
	.playback-bar {
		background-color: var(--bg-secondary);
		border-top: 1px solid var(--border-color);
		padding: var(--space-3) var(--space-4);
		flex-shrink: 0;
		z-index: 10;
	}

	.progress-row {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		margin-bottom: var(--space-2);
	}

	.time-label {
		font-size: var(--text-xs);
		color: var(--text-secondary);
		white-space: nowrap;
		min-width: 60px;
	}

	.time-label:first-child {
		text-align: left;
	}

	.time-label:last-child {
		text-align: right;
	}

	.progress-slider {
		flex: 1;
		height: 6px;
		border-radius: var(--radius-full);
		background: var(--bg-tertiary);
		outline: none;
		cursor: pointer;
		-webkit-appearance: none;
		appearance: none;
	}

	.progress-slider::-webkit-slider-thumb {
		-webkit-appearance: none;
		width: 14px;
		height: 14px;
		border-radius: 50%;
		background: var(--accent-primary);
		cursor: pointer;
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
	}

	.progress-slider::-moz-range-thumb {
		width: 14px;
		height: 14px;
		border: none;
		border-radius: 50%;
		background: var(--accent-primary);
		cursor: pointer;
	}

	.controls-row {
		display: flex;
		align-items: center;
		gap: var(--space-3);
	}

	.ctrl-btn {
		width: 36px;
		height: 36px;
		display: flex;
		align-items: center;
		justify-content: center;
		background-color: var(--bg-tertiary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		cursor: pointer;
		transition: all var(--transition-fast);
	}

	.ctrl-btn:hover {
		background-color: var(--bg-hover);
	}

	.ctrl-btn.primary {
		width: 44px;
		height: 44px;
		background-color: var(--accent-primary);
		color: var(--text-inverse);
		border-color: var(--accent-primary);
		border-radius: 50%;
	}

	.ctrl-btn.primary:hover {
		background-color: var(--accent-hover);
	}

	.speed-selector {
		display: flex;
		align-items: center;
		gap: var(--space-1);
		margin-left: auto;
	}

	.speed-btn {
		padding: var(--space-1) var(--space-2);
		background-color: var(--bg-tertiary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-sm);
		color: var(--text-primary);
		cursor: pointer;
		font-size: var(--text-xs);
		transition: all var(--transition-fast);
	}

	.speed-btn:hover {
		background-color: var(--bg-hover);
	}

	.speed-btn.active {
		background-color: var(--accent-primary);
		color: var(--text-inverse);
		border-color: var(--accent-primary);
	}

	.position-counter {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
		font-family: monospace;
		min-width: 80px;
		text-align: right;
	}

	/* Fullscreen adjustments */
	.replay-page:fullscreen {
		height: 100vh;
	}

	.replay-page:fullscreen .filters-section {
		display: none;
	}

	/* Responsive */
	@media (max-width: 768px) {
		.filters-bar {
			flex-direction: column;
			align-items: stretch;
		}

		.filter-group {
			width: 100%;
		}

		.select {
			width: 100%;
		}

		.info-overlay {
			top: auto;
			bottom: var(--space-3);
			right: var(--space-3);
			min-width: 180px;
			font-size: var(--text-xs);
		}

		.stats-bar {
			display: none;
		}

		.progress-row .time-label {
			display: none;
		}

		.speed-selector {
			gap: 0;
		}
	}
</style>

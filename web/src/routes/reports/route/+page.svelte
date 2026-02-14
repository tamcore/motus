<script lang="ts">
	import { onMount, onDestroy, tick } from 'svelte';
	import { page } from '$app/stores';
	import { api } from '$lib/api/client';
	import { useLeaflet } from '$lib/composables/useLeaflet';
	import { getPositionTime } from '$lib/utils/trips';
	import { formatDate, formatSpeed } from '$lib/utils/formatting';
	import { downloadGPX } from '$lib/utils/gpx';
	import type { Position } from '$lib/utils/trips';
	import RoutePlayback from '$lib/components/RoutePlayback.svelte';

	const leafletMap = useLeaflet();

	let mapEl: HTMLDivElement;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let marker: any = null;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let polyline: any = null;
	let positions: Position[] = [];
	let currentIndex = 0;
	let playing = false;
	let speed = 1;
	let loading = true;
	let animationTimer: ReturnType<typeof setTimeout> | null = null;

	$: currentPosition = positions.length > 0 ? positions[currentIndex] : null;

	// Parse query params: ?deviceId=X&from=Y&to=Z
	$: deviceId = $page.url.searchParams.get('deviceId');
	$: from = $page.url.searchParams.get('from');
	$: to = $page.url.searchParams.get('to');

	onMount(async () => {
		// Initialize map via composable
		await leafletMap.initialize(mapEl, {
			center: [0, 0],
			zoom: 13,
			zoomControl: true,
		});

		const L = leafletMap.getLeaflet()!;
		const map = leafletMap.getMap()!;

		if (deviceId && from && to) {
			try {
				positions = await api.getPositions({
					deviceId: Number(deviceId), from, to, limit: 10000
				}) as Position[];
			} catch (e) {
				console.error('Failed to load positions:', e);
			}
		}

		if (positions.length > 0) {
			// Ensure the DOM is fully laid out before drawing the route
			await tick();
			map.invalidateSize();

			const coords = positions.map((p) => [p.latitude, p.longitude] as [number, number]);
			polyline = L.polyline(coords, { color: '#00d4ff', weight: 3 }).addTo(map);
			map.fitBounds(polyline.getBounds());
			marker = L.marker(coords[0]).addTo(map);
		}
		loading = false;
	});

	onDestroy(() => {
		if (animationTimer) clearTimeout(animationTimer);
		leafletMap.cleanup();
	});

	function updateMarkerPosition() {
		const map = leafletMap.getMap();
		if (!marker || !map || positions.length === 0) return;
		const pos = positions[currentIndex];
		marker.setLatLng([pos.latitude, pos.longitude]);
		map.panTo([pos.latitude, pos.longitude]);
	}

	function animateStep() {
		if (!playing || currentIndex >= positions.length - 1) {
			playing = false;
			return;
		}
		const current = positions[currentIndex];
		const next = positions[currentIndex + 1];
		currentIndex++;
		updateMarkerPosition();

		const timeDiff = new Date(getPositionTime(next)).getTime() - new Date(getPositionTime(current)).getTime();
		const delay = Math.max(Math.min(timeDiff / speed, 2000), 50);
		animationTimer = setTimeout(animateStep, delay);
	}

	function handlePlay() { playing = true; animateStep(); }
	function handlePause() { playing = false; if (animationTimer) clearTimeout(animationTimer); }
	function handleStop() {
		playing = false;
		if (animationTimer) clearTimeout(animationTimer);
		currentIndex = 0;
		updateMarkerPosition();
	}
	function handleSpeedChange(e: CustomEvent<number>) { speed = e.detail; }
	function handleTimelineChange(e: CustomEvent<Event>) {
		const target = (e.detail as Event).target as HTMLInputElement;
		currentIndex = parseInt(target.value);
		updateMarkerPosition();
	}
	function handleDownload() {
		downloadGPX(positions, `route-${deviceId || 'unknown'}`);
	}
</script>

<svelte:head><title>Route Playback - Motus</title></svelte:head>

<div class="route-page">
	<div class="map-container">
		<div bind:this={mapEl} class="route-map"></div>
		{#if loading}
			<div class="map-loading">Loading route...</div>
		{/if}
	</div>

	{#if positions.length > 0}
		<div class="controls-container">
			<RoutePlayback {playing} {speed} totalPositions={positions.length} {currentIndex}
				on:play={handlePlay} on:pause={handlePause} on:stop={handleStop}
				on:speedChange={handleSpeedChange} on:timelineChange={handleTimelineChange}
				on:download={handleDownload} />
		</div>
	{/if}

	{#if currentPosition}
		<div class="info-panel">
			<div class="info-item">
				<span class="info-label">Time</span>
				<span class="info-value">{formatDate(getPositionTime(currentPosition))}</span>
			</div>
			<div class="info-item">
				<span class="info-label">Speed</span>
				<span class="info-value">{formatSpeed(currentPosition.speed)}</span>
			</div>
			<div class="info-item">
				<span class="info-label">Location</span>
				<span class="info-value">
					{currentPosition.latitude.toFixed(6)}, {currentPosition.longitude.toFixed(6)}
				</span>
			</div>
		</div>
	{/if}
</div>

<style>
	.route-page { height: calc(100vh - 60px); display: flex; flex-direction: column; position: relative; }
	.map-container { flex: 1; position: relative; }
	.route-map { width: 100%; height: 100%; }
	.map-container :global(.leaflet-container) {
		height: 100%;
		width: 100%;
		background-color: var(--bg-tertiary);
	}
	.map-loading {
		position: absolute; top: 50%; left: 50%; transform: translate(-50%, -50%);
		color: var(--text-secondary); font-size: var(--text-lg);
	}
	.controls-container {
		padding: var(--space-4); background-color: var(--bg-secondary);
		border-top: 1px solid var(--border-color);
	}
	.info-panel {
		position: absolute; top: var(--space-4); right: var(--space-4);
		background-color: var(--bg-primary); border: 1px solid var(--border-color);
		border-radius: var(--radius-lg); padding: var(--space-4);
		box-shadow: var(--shadow-lg); min-width: 250px; z-index: 500;
	}
	.info-item {
		display: flex; justify-content: space-between;
		padding: var(--space-2) 0; border-bottom: 1px solid var(--border-color);
	}
	.info-item:last-child { border-bottom: none; }
	.info-label { color: var(--text-secondary); font-weight: var(--font-medium); font-size: var(--text-sm); }
	.info-value { color: var(--text-primary); font-size: var(--text-sm); }
</style>

<script lang="ts">
	import { page } from '$app/stores';
	import { onMount, onDestroy } from 'svelte';
	import { useLeaflet } from '$lib/composables/useLeaflet';
	import { useUserLocation, userLocationLayers } from '$lib/composables/useUserLocation';
	import { buildPopupElement } from '$lib/utils/popup';
	import type { Device, WebSocketMessage } from '$lib/types/api';
	import { WebSocketManager } from '$lib/stores/websocket';
	import { persisted } from '$lib/stores/persisted';
	import { formatSpeed, getCardinalDirection } from '$lib/utils/formatting';
	import { speedToKmh } from '$lib/api/client';

	const API_BASE = '/api';
	const SHARE_UNIT_KEY = 'motus_share_units';
	const TRAIL_POINT_LIMIT = 200;

	/**
	 * Position type for the share page. Uses optional id/deviceId since
	 * the initial load may not always include them.
	 */
	interface SharedPosition {
		id?: number;
		deviceId?: number;
		latitude: number;
		longitude: number;
		speed: number | null;
		course: number | null;
		fixTime: string;
		attributes?: Record<string, unknown>;
	}

	const leafletMap = useLeaflet();
	const userLocation = useUserLocation();
	const userLayers = userLocationLayers(() => leafletMap.getLeaflet(), () => leafletMap.getMap(), false);

	// State
	let token = '';
	let device: Device | null = null;
	let positions: SharedPosition[] = [];
	let error = '';
	let loading = true;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let marker: any = null;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let trailLayer: any = null;
	let trailPoints: Array<[number, number]> = [];
	let mapContainer: HTMLDivElement;

	let socket: WebSocketManager | null = null;
	let unsubscribers: Array<() => void> = [];
	let wsConnected = false;

	// UI controls
	const units = persisted<'metric' | 'imperial'>(SHARE_UNIT_KEY, 'metric', (v) => (v === 'imperial' ? v : null));
	let autoCenter = true;
	let showTrail = true;
	let showInfo = true;

	$: token = $page.params.token || '';
	$: position = positions.length > 0 ? positions[0] : null;
	$: formattedSpeed = position?.speed == null ? '--' : formatSpeed(position.speed, $units);
	$: formattedCourse = position?.course != null ? `${Math.round(position.course)}` : '--';
	$: courseDirection = position?.course != null ? getCardinalDirection(position.course) : '';
	$: lastUpdateText = position ? formatTimeAgo(position.fixTime) : 'No data';

	function toggleUnits() {
		units.update((u) => (u === 'metric' ? 'imperial' : 'metric'));
		// Update marker popup if present
		if (marker && position) {
			marker.getPopup()?.setContent(getPopupContent());
		}
	}

	function formatSpeedText(speed: number | null | undefined): string {
		return speed == null ? '--' : formatSpeed(speed, $units);
	}

	function formatTimeAgo(dateStr: string): string {
		const d = new Date(dateStr);
		if (isNaN(d.getTime())) return dateStr;
		const now = new Date();
		const diff = now.getTime() - d.getTime();
		const seconds = Math.floor(diff / 1000);
		const minutes = Math.floor(seconds / 60);
		const hours = Math.floor(minutes / 60);
		if (seconds < 0) return 'just now';
		if (seconds < 60) return 'just now';
		if (minutes < 60) return `${minutes}m ago`;
		if (hours < 24) return `${hours}h ${minutes % 60}m ago`;
		return d.toLocaleString();
	}

	// --- API ---
	async function fetchSharedDevice(): Promise<boolean> {
		try {
			const response = await fetch(`${API_BASE}/share/${token}`);
			if (!response.ok) {
				if (response.status === 404) {
					error = 'This share link has expired or is invalid.';
				} else {
					error = 'Failed to load shared device.';
				}
				return false;
			}
			const data = await response.json();
			device = data.device;
			positions = (data.positions || []).map((pos: SharedPosition) => speedToKmh(pos));

			// Initialize trail from initial position
			const latestPos = positions.length > 0 ? positions[0] : null;
			if (latestPos) {
				trailPoints = [[latestPos.latitude, latestPos.longitude]];
			}
			return true;
		} catch {
			error = 'Failed to load shared device. Please try again later.';
			return false;
		} finally {
			loading = false;
		}
	}

	// --- WebSocket ---
	function connectWebSocket() {
		socket = new WebSocketManager(`?shareToken=${encodeURIComponent(token)}`);
		unsubscribers = [
			socket.connected.subscribe((v) => (wsConnected = v)),
			socket.lastMessage.subscribe((msg) => msg && handleWebSocketMessage(msg)),
		];
		socket.connect();
	}

	function disconnectWebSocket() {
		unsubscribers.forEach((unsubscribe) => unsubscribe());
		socket?.disconnect();
	}

	function handleWebSocketMessage(data: WebSocketMessage) {
		if (data.positions && Array.isArray(data.positions)) {
			for (const pos of data.positions) {
				// The socket manager already converted speed to km/h.
				const newPos: SharedPosition = {
					id: pos.id,
					deviceId: pos.deviceId,
					latitude: pos.latitude,
					longitude: pos.longitude,
					speed: pos.speed ?? null,
					course: pos.course ?? null,
					fixTime: pos.fixTime || new Date().toISOString(),
					attributes: pos.attributes
				};

				// Update positions array (most recent first)
				positions = [newPos];

				// Add to trail
				const point: [number, number] = [newPos.latitude, newPos.longitude];
				trailPoints = [...trailPoints, point];
				if (trailPoints.length > TRAIL_POINT_LIMIT) {
					trailPoints = trailPoints.slice(-TRAIL_POINT_LIMIT);
				}

				// Update map
				updateMarker(newPos);
				if (showTrail) {
					drawTrail();
				}
			}
		}

		if (data.devices && Array.isArray(data.devices)) {
			for (const dev of data.devices) {
				if (device && dev.id === device.id) {
					device = { ...device, status: dev.status };
				}
			}
		}
	}

	// --- Map ---
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	function createDirectionalIcon(course: number | null, isMoving: boolean): any {
		const L = leafletMap.getLeaflet();
		if (!L) return null;

		const rotation = course != null ? course : 0;
		const color = isMoving ? '#00d4ff' : '#00ff88';
		const size = 32;

		if (isMoving && course != null) {
			// Arrow marker showing direction of travel
			return L.divIcon({
				className: 'share-marker',
				html: `<div style="
					width: ${size}px; height: ${size}px;
					display: flex; align-items: center; justify-content: center;
					transform: rotate(${rotation}deg);
				">
					<svg width="${size}" height="${size}" viewBox="0 0 32 32">
						<path d="M16 4 L24 26 L16 20 L8 26 Z"
							fill="${color}" stroke="white" stroke-width="2"
							stroke-linejoin="round"/>
					</svg>
				</div>`,
				iconSize: [size, size],
				iconAnchor: [size / 2, size / 2]
			});
		}

		// Static circle marker when not moving
		return L.divIcon({
			className: 'share-marker',
			html: `<div style="
				width: 20px; height: 20px;
				background: ${color};
				border: 3px solid white;
				border-radius: 50%;
				box-shadow: 0 2px 8px rgba(0,0,0,0.4);
			"></div>`,
			iconSize: [20, 20],
			iconAnchor: [10, 10]
		});
	}

	function getPopupContent(): HTMLElement | string {
		if (!position || !device) return '';
		const speed = formatSpeedText(position.speed);
		const course = position.course != null ? `${Math.round(position.course)}deg ${getCardinalDirection(position.course)}` : 'N/A';
		const time = position.fixTime ? new Date(position.fixTime).toLocaleString() : 'Unknown';
		return buildPopupElement([
			{ type: 'heading', text: device.name },
			{ type: 'text', text: `Speed: ${speed}` },
			{ type: 'text', text: `Course: ${course}` },
			{ type: 'text', text: `Time: ${time}` }
		]);
	}

	function updateMarker(pos: SharedPosition) {
		const L = leafletMap.getLeaflet();
		const map = leafletMap.getMap();
		if (!map || !L) return;

		const latlng = L.latLng(pos.latitude, pos.longitude);
		const isMoving = pos.speed != null && pos.speed > 2;
		const icon = createDirectionalIcon(pos.course, isMoving);

		if (marker) {
			// Smooth transition
			marker.setLatLng(latlng);
			marker.setIcon(icon);
			marker.getPopup()?.setContent(getPopupContent());
		} else {
			marker = L.marker(latlng, { icon })
				.addTo(map)
				.bindPopup(getPopupContent());
		}

		if (autoCenter) {
			map.panTo(latlng, { animate: true, duration: 0.5 });
		}
	}

	function drawTrail() {
		const L = leafletMap.getLeaflet();
		if (!trailLayer || !L || trailPoints.length < 2) return;
		trailLayer.clearLayers();

		// Main trail line
		L.polyline(trailPoints, {
			color: '#00d4ff',
			weight: 3,
			opacity: 0.7,
			dashArray: undefined
		}).addTo(trailLayer);

		// Start point indicator
		const start = trailPoints[0];
		L.circleMarker(start, {
			radius: 5,
			fillColor: '#00ff88',
			fillOpacity: 1,
			color: 'white',
			weight: 2
		}).addTo(trailLayer);
	}

	function clearTrail() {
		if (trailLayer) trailLayer.clearLayers();
	}

	function toggleTrail() {
		showTrail = !showTrail;
		if (showTrail) {
			drawTrail();
		} else {
			clearTrail();
		}
	}

	function centerOnDevice() {
		const map = leafletMap.getMap();
		if (!map || !position) return;
		map.setView([position.latitude, position.longitude], map.getZoom() < 13 ? 15 : map.getZoom());
	}

	async function initMap() {
		// Initialize map via composable (handles dynamic import, tiles, zoom, icon fix)
		await leafletMap.initialize(mapContainer, {
			center: [51.505, -0.09],
			zoom: 13,
		});

		const L = leafletMap.getLeaflet()!;
		const map = leafletMap.getMap()!;

		trailLayer = L.layerGroup().addTo(map);

		if (position) {
			updateMarker(position);
			if (showTrail && trailPoints.length > 1) {
				drawTrail();
			}
			map.setView([position.latitude, position.longitude], 15);
		}
	}

	$: userLayers.sync($userLocation);

	// --- Update timer for "last update" display ---
	let updateTimer: ReturnType<typeof setInterval> | null = null;

	onMount(async () => {
		const ok = await fetchSharedDevice();
		if (!ok) return;

		await initMap();
		connectWebSocket();

		// Refresh "time ago" display every 15 seconds
		updateTimer = setInterval(() => {
			// Trigger reactive update by reassigning positions
			positions = [...positions];
		}, 15000);
	});

	onDestroy(() => {
		disconnectWebSocket();
		if (updateTimer) clearInterval(updateTimer);
		userLocation.stop();
		leafletMap.cleanup();
	});
</script>

<svelte:head>
	<title>{device ? `Tracking: ${device.name}` : 'Shared Device'} - Motus</title>
</svelte:head>

<div class="share-page">
	{#if loading}
		<div class="loading-screen">
			<div class="spinner spinner-lg"></div>
			<p>Loading shared device...</p>
		</div>
	{:else if error}
		<div class="error-screen">
			<svg viewBox="0 0 24 24" width="64" height="64" fill="none" stroke="#ff4444" stroke-width="1.5">
				<circle cx="12" cy="12" r="10"/>
				<line x1="15" y1="9" x2="9" y2="15"/>
				<line x1="9" y1="9" x2="15" y2="15"/>
			</svg>
			<h1>Share Link Unavailable</h1>
			<p>{error}</p>
			<a href="/" class="home-link">Go to Motus</a>
		</div>
	{:else}
		<!-- Map fills the viewport -->
		<div class="map-container" bind:this={mapContainer}></div>

		<!-- Connection status indicator -->
		<div class="ws-indicator" class:connected={wsConnected}>
			<span class="ws-dot"></span>
			<span class="ws-label">{wsConnected ? 'Live' : 'Reconnecting...'}</span>
		</div>

		<!-- Header overlay -->
		<div class="overlay-header">
			<div class="header-left">
				<svg class="logo-icon" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2">
					<path d="M12 2C8.13 2 5 5.13 5 9c0 5.25 7 13 7 13s7-7.75 7-13c0-3.87-3.13-7-7-7z"/>
					<circle cx="12" cy="9" r="2.5"/>
				</svg>
				<span class="header-brand">Motus</span>
			</div>
			<div class="header-center">
				<span class="header-device-name">{device?.name || 'Device'}</span>
				<span class="header-update">{lastUpdateText}</span>
			</div>
			<div class="header-right">
				<button
					class="control-btn"
					class:active={showInfo}
					on:click={() => showInfo = !showInfo}
					title="Toggle info panel"
					aria-label="Toggle info panel"
				>
					<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
						<circle cx="12" cy="12" r="10"/>
						<line x1="12" y1="16" x2="12" y2="12"/>
						<line x1="12" y1="8" x2="12.01" y2="8"/>
					</svg>
				</button>
			</div>
		</div>

		<!-- Info panel overlay -->
		{#if showInfo && position}
			<div class="info-panel">
				<!-- Speed -->
				<div class="info-block">
					<div class="info-main-value">{formattedSpeed}</div>
					<button class="unit-toggle" on:click={toggleUnits} title="Toggle units">
						{$units === 'metric' ? 'km/h' : 'mph'}
					</button>
				</div>

				<!-- Course/Heading -->
				<div class="info-block">
					<div class="info-row">
						<svg class="compass-arrow" viewBox="0 0 24 24" width="20" height="20"
							style="transform: rotate({position.course ?? 0}deg)">
							<path d="M12 2 L16 20 L12 16 L8 20 Z" fill="currentColor" stroke="none"/>
						</svg>
						<span class="info-value">{formattedCourse}&deg;</span>
						<span class="info-label">{courseDirection}</span>
					</div>
				</div>

				<!-- Coordinates -->
				<div class="info-block coords">
					<div class="coord-row">
						<span class="coord-label">Lat</span>
						<span class="coord-value">{position.latitude.toFixed(5)}</span>
					</div>
					<div class="coord-row">
						<span class="coord-label">Lon</span>
						<span class="coord-value">{position.longitude.toFixed(5)}</span>
					</div>
				</div>
			</div>
		{/if}

		<!-- Map controls -->
		<div class="map-controls">
			<button
				class="control-btn"
				class:active={autoCenter}
				on:click={() => { autoCenter = !autoCenter; if (autoCenter) centerOnDevice(); }}
				title={autoCenter ? 'Auto-center on' : 'Auto-center off'}
				aria-label="Toggle auto-center"
			>
				<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
					<circle cx="12" cy="12" r="3"/>
					<path d="M12 2v4m0 12v4M2 12h4m12 0h4"/>
				</svg>
			</button>
			<button
				class="control-btn"
				class:active={showTrail}
				on:click={toggleTrail}
				title={showTrail ? 'Hide trail' : 'Show trail'}
				aria-label="Toggle trail"
			>
				<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
					<path d="M3 17l4-4 4 4 4-8 4 4"/>
				</svg>
			</button>
			<button
				class="control-btn"
				on:click={centerOnDevice}
				title="Center on device"
				aria-label="Center on device"
			>
				<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
					<path d="M12 2C8.13 2 5 5.13 5 9c0 5.25 7 13 7 13s7-7.75 7-13c0-3.87-3.13-7-7-7z"/>
					<circle cx="12" cy="9" r="2.5"/>
				</svg>
			</button>
			<button
				class="control-btn"
				class:active={$userLocation.active}
				on:click={() => userLayers.toggle(userLocation)}
				title={$userLocation.active ? 'Hide my location' : 'Show my location'}
				aria-label={$userLocation.active ? 'Hide my location' : 'Show my location'}
			>
				<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
					<circle cx="12" cy="12" r="3"/>
					<path d="M12 2v4m0 12v4M2 12h4m12 0h4"/>
					<circle cx="12" cy="12" r="8" stroke-opacity="0.3"/>
				</svg>
			</button>
		</div>

		{#if $userLocation.error}
			<div class="locate-error" role="alert">
				{$userLocation.error}
			</div>
		{/if}

		<!-- No position data overlay -->
		{#if !position}
			<div class="no-data-overlay">
				<p>Waiting for position data...</p>
			</div>
		{/if}
	{/if}
</div>

<style>
	.share-page {
		height: 100vh;
		width: 100vw;
		position: relative;
		overflow: hidden;
		background-color: var(--bg-primary, #1a1a1a);
		color: var(--text-primary, #e0e0e0);
	}

	/* Loading / Error screens */
	.loading-screen,
	.error-screen {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		height: 100vh;
		gap: 1rem;
		padding: 2rem;
		text-align: center;
	}

	.loading-screen p,
	.error-screen p {
		color: var(--text-secondary, #a0a0a0);
		margin: 0;
	}

	.error-screen h1 {
		font-size: 1.5rem;
		font-weight: 600;
		color: var(--text-primary, #e0e0e0);
		margin: 0.5rem 0;
	}

	.home-link {
		margin-top: 1rem;
		padding: 0.75rem 1.5rem;
		background-color: var(--accent-primary, #00d4ff);
		color: #000;
		text-decoration: none;
		border-radius: 8px;
		font-weight: 500;
		transition: opacity 0.2s;
	}

	.home-link:hover {
		opacity: 0.85;
	}

	/* Map container */
	.map-container {
		position: absolute;
		inset: 0;
		z-index: 0;
	}

	/* Hide default marker styling */
	.map-container :global(.share-marker) {
		background: none !important;
		border: none !important;
	}

	/* Leaflet popup theming */
	.map-container :global(.leaflet-popup-content-wrapper) {
		background-color: var(--bg-secondary, #2d2d2d);
		color: var(--text-primary, #e0e0e0);
		border-radius: 8px;
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
	}

	.map-container :global(.leaflet-popup-tip) {
		background-color: var(--bg-secondary, #2d2d2d);
	}

	/* Connection indicator */
	.ws-indicator {
		position: absolute;
		top: 68px;
		left: 12px;
		z-index: 1000;
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 4px 12px;
		background-color: rgba(26, 26, 26, 0.85);
		backdrop-filter: blur(8px);
		border: 1px solid rgba(255, 255, 255, 0.1);
		border-radius: 20px;
		font-size: 0.75rem;
		color: #a0a0a0;
	}

	/* Header overlay */
	.overlay-header {
		position: absolute;
		top: 0;
		left: 0;
		right: 0;
		z-index: 1000;
		display: flex;
		align-items: center;
		padding: 10px 16px;
		background: linear-gradient(to bottom, rgba(26, 26, 26, 0.9), rgba(26, 26, 26, 0));
		pointer-events: none;
	}

	.overlay-header > * {
		pointer-events: auto;
	}

	.header-left {
		display: flex;
		align-items: center;
		gap: 6px;
		color: var(--accent-primary, #00d4ff);
		flex-shrink: 0;
	}

	.header-brand {
		font-size: 1rem;
		font-weight: 700;
	}

	.header-center {
		flex: 1;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 1px;
		min-width: 0;
	}

	.header-device-name {
		font-size: 0.9375rem;
		font-weight: 600;
		color: var(--text-primary, #fff);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		max-width: 100%;
	}

	.header-update {
		font-size: 0.6875rem;
		color: var(--text-secondary, #a0a0a0);
	}

	.header-right {
		flex-shrink: 0;
	}

	/* Info panel */
	.info-panel {
		position: absolute;
		bottom: 80px;
		left: 12px;
		z-index: 1000;
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding: 12px;
		background-color: rgba(26, 26, 26, 0.9);
		backdrop-filter: blur(12px);
		border: 1px solid rgba(255, 255, 255, 0.1);
		border-radius: 12px;
		min-width: 160px;
	}

	.info-block {
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.info-main-value {
		font-size: 1.5rem;
		font-weight: 700;
		color: var(--text-primary, #fff);
		line-height: 1;
	}

	.unit-toggle {
		padding: 2px 8px;
		background-color: rgba(255, 255, 255, 0.1);
		border: 1px solid rgba(255, 255, 255, 0.15);
		border-radius: 4px;
		color: var(--text-secondary, #a0a0a0);
		font-size: 0.6875rem;
		cursor: pointer;
		transition: all 0.15s ease;
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}

	.unit-toggle:hover {
		background-color: rgba(255, 255, 255, 0.15);
		color: var(--text-primary, #fff);
	}

	.info-row {
		display: flex;
		align-items: center;
		gap: 6px;
	}

	.compass-arrow {
		color: var(--accent-primary, #00d4ff);
		transition: transform 0.3s ease;
		flex-shrink: 0;
	}

	.info-value {
		font-size: 0.9375rem;
		font-weight: 600;
		color: var(--text-primary, #fff);
	}

	.info-label {
		font-size: 0.75rem;
		color: var(--text-secondary, #a0a0a0);
	}

	.info-block.coords {
		flex-direction: column;
		align-items: stretch;
		gap: 2px;
		padding-top: 6px;
		border-top: 1px solid rgba(255, 255, 255, 0.08);
	}

	.coord-row {
		display: flex;
		justify-content: space-between;
		gap: 12px;
	}

	.coord-label {
		font-size: 0.6875rem;
		color: var(--text-secondary, #a0a0a0);
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}

	.coord-value {
		font-size: 0.8125rem;
		font-weight: 500;
		color: var(--text-primary, #fff);
		font-variant-numeric: tabular-nums;
	}

	/* Map controls */
	.map-controls {
		position: absolute;
		bottom: 80px;
		right: 12px;
		z-index: 1000;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.control-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 40px;
		height: 40px;
		background-color: rgba(26, 26, 26, 0.85);
		backdrop-filter: blur(8px);
		border: 1px solid rgba(255, 255, 255, 0.1);
		border-radius: 10px;
		color: var(--text-secondary, #a0a0a0);
		cursor: pointer;
		transition: all 0.15s ease;
	}

	.control-btn:hover {
		background-color: rgba(45, 45, 45, 0.9);
		color: var(--text-primary, #fff);
	}

	.control-btn.active {
		color: var(--accent-primary, #00d4ff);
		border-color: var(--accent-primary, #00d4ff);
		background-color: rgba(0, 212, 255, 0.1);
	}

	/* Locate error */
	.locate-error {
		position: absolute;
		bottom: calc(80px + 8px * 5 + 40px * 4 + 12px);
		right: 12px;
		z-index: 1000;
		max-width: 200px;
		padding: 6px 10px;
		background-color: rgba(255, 68, 68, 0.15);
		border: 1px solid rgba(255, 68, 68, 0.4);
		border-radius: 8px;
		font-size: 0.6875rem;
		color: #ff6666;
	}

	/* No data overlay */
	.no-data-overlay {
		position: absolute;
		bottom: 0;
		left: 0;
		right: 0;
		z-index: 999;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 16px;
		background: linear-gradient(to top, rgba(26, 26, 26, 0.9), rgba(26, 26, 26, 0));
	}

	.no-data-overlay p {
		padding: 8px 20px;
		background-color: rgba(26, 26, 26, 0.9);
		border: 1px solid rgba(255, 255, 255, 0.1);
		border-radius: 20px;
		font-size: 0.875rem;
		color: var(--text-secondary, #a0a0a0);
	}

	/* Mobile adjustments */
	@media (max-width: 480px) {
		.overlay-header {
			padding: 8px 12px;
		}

		.header-brand {
			display: none;
		}

		.header-device-name {
			font-size: 0.875rem;
		}

		.info-panel {
			bottom: 70px;
			left: 8px;
			right: 8px;
			min-width: unset;
			flex-direction: row;
			flex-wrap: wrap;
			justify-content: space-between;
		}

		.info-block {
			flex: 0 0 auto;
		}

		.info-block.coords {
			flex: 1 0 100%;
			flex-direction: row;
			justify-content: space-around;
			border-top: 1px solid rgba(255, 255, 255, 0.08);
			padding-top: 8px;
		}

		.map-controls {
			bottom: 70px;
			right: 8px;
		}

		.control-btn {
			width: 36px;
			height: 36px;
		}

		.ws-indicator {
			top: 56px;
			left: 8px;
		}
	}

	/* Tablet */
	@media (min-width: 481px) and (max-width: 768px) {
		.info-panel {
			bottom: 76px;
		}

		.map-controls {
			bottom: 76px;
		}
	}
</style>

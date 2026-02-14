<script lang="ts">
	import { page } from '$app/stores';
	import { onMount, onDestroy } from 'svelte';
	import { useLeaflet } from '$lib/composables/useLeaflet';
	import { useUserLocation } from '$lib/composables/useUserLocation';
	import type { Device } from '$lib/types/api';

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

	// User location layers
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let userAccuracyCircle: any = null;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let userDotMarker: any = null;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let userHeadingMarker: any = null;
	let firstUserFix = false;

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

	// WebSocket state
	let ws: WebSocket | null = null;
	let wsConnected = false;
	let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	let pingInterval: ReturnType<typeof setInterval> | null = null;

	// UI controls
	let units: 'metric' | 'imperial' = 'metric';
	let autoCenter = true;
	let showTrail = true;
	let showInfo = true;

	$: token = $page.params.token || '';
	$: position = positions.length > 0 ? positions[0] : null;
	$: formattedSpeed = formatSpeedWithUnits(position?.speed, units);
	$: formattedCourse = position?.course != null ? `${Math.round(position.course)}` : '--';
	$: courseDirection = position?.course != null ? getCardinalDirection(position.course) : '';
	$: lastUpdateText = position ? formatTimeAgo(position.fixTime) : 'No data';

	// Initialize unit preference from localStorage
	function loadUnitPreference(): 'metric' | 'imperial' {
		if (typeof window === 'undefined') return 'metric';
		try {
			const saved = localStorage.getItem(SHARE_UNIT_KEY);
			if (saved === 'imperial') return 'imperial';
		} catch {
			// Ignore
		}
		return 'metric';
	}

	function saveUnitPreference(u: 'metric' | 'imperial') {
		try {
			localStorage.setItem(SHARE_UNIT_KEY, u);
		} catch {
			// Ignore
		}
	}

	function toggleUnits() {
		units = units === 'metric' ? 'imperial' : 'metric';
		saveUnitPreference(units);
		// Update marker popup if present
		if (marker && position) {
			marker.getPopup()?.setContent(getPopupContent());
		}
	}

	function formatSpeedWithUnits(speed: number | null | undefined, u: 'metric' | 'imperial'): string {
		if (speed === null || speed === undefined) return '--';
		if (u === 'imperial') {
			const mph = speed * 0.621371;
			return `${mph.toFixed(1)} mph`;
		}
		return `${speed.toFixed(1)} km/h`;
	}

	function formatSpeed(speed: number | null | undefined): string {
		return formatSpeedWithUnits(speed, units);
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

	function getCardinalDirection(degrees: number): string {
		const dirs = ['N', 'NE', 'E', 'SE', 'S', 'SW', 'W', 'NW'];
		const idx = Math.round(degrees / 45) % 8;
		return dirs[idx];
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
			// Normalize speed from knots (Traccar API) to km/h (internal UI unit).
			positions = (data.positions || []).map((pos: SharedPosition) =>
				pos.speed != null ? { ...pos, speed: pos.speed * 1.852 } : pos
			);

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
		if (typeof window === 'undefined') return;
		if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
			return;
		}

		// Clean up existing socket
		if (ws) {
			ws.onopen = null;
			ws.onmessage = null;
			ws.onclose = null;
			ws.onerror = null;
			ws = null;
		}

		const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		const url = `${protocol}//${window.location.host}/api/socket?shareToken=${token}`;

		const socket = new WebSocket(url);
		ws = socket;

		socket.onopen = () => {
			if (ws !== socket) return;
			wsConnected = true;

			// Ping every 30s to keep connection alive
			if (pingInterval) clearInterval(pingInterval);
			pingInterval = setInterval(() => {
				if (ws && ws.readyState === WebSocket.OPEN) {
					ws.send(JSON.stringify({ type: 'ping' }));
				}
			}, 30000);
		};

		socket.onmessage = (event) => {
			if (ws !== socket) return;
			try {
				const data = JSON.parse(event.data);
				handleWebSocketMessage(data);
			} catch {
				// Ignore malformed messages
			}
		};

		socket.onclose = () => {
			if (ws !== socket) return;
			wsConnected = false;
			if (pingInterval) {
				clearInterval(pingInterval);
				pingInterval = null;
			}
			ws = null;
			// Reconnect after 5 seconds
			reconnectTimer = setTimeout(connectWebSocket, 5000);
		};

		socket.onerror = () => {
			socket.close();
		};
	}

	function disconnectWebSocket() {
		if (pingInterval) {
			clearInterval(pingInterval);
			pingInterval = null;
		}
		if (reconnectTimer) {
			clearTimeout(reconnectTimer);
			reconnectTimer = null;
		}
		if (ws) {
			ws.onopen = null;
			ws.onmessage = null;
			ws.onclose = null;
			ws.onerror = null;
			ws.close();
			ws = null;
		}
		wsConnected = false;
	}

	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	function handleWebSocketMessage(data: any) {
		if (data.positions && Array.isArray(data.positions)) {
			for (const pos of data.positions) {
				const newPos: SharedPosition = {
					id: pos.id,
					deviceId: pos.deviceId,
					latitude: pos.latitude,
					longitude: pos.longitude,
					// Normalize speed from knots (Traccar API) to km/h (internal UI unit).
				speed: pos.speed != null ? pos.speed * 1.852 : null,
					course: pos.course ?? null,
					fixTime: pos.fixTime || pos.timestamp || new Date().toISOString(),
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

	function getPopupContent(): string {
		if (!position || !device) return '';
		const speed = formatSpeed(position.speed);
		const course = position.course != null ? `${Math.round(position.course)}deg ${getCardinalDirection(position.course)}` : 'N/A';
		const time = position.fixTime ? new Date(position.fixTime).toLocaleString() : 'Unknown';
		return `<strong>${device.name}</strong><br>Speed: ${speed}<br>Course: ${course}<br>Time: ${time}`;
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
			tileAttribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
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

	// React to user position changes
	$: if (userLocation.position) {
		updateUserLocationLayers();
	}

	// React to heading changes
	$: if (userLocation.heading !== null || userLocation.position) {
		updateUserHeadingLayer();
	}

	function updateUserLocationLayers() {
		const L = leafletMap.getLeaflet();
		const map = leafletMap.getMap();
		const pos = userLocation.position;
		if (!L || !map || !pos) return;

		const latlng: [number, number] = [pos.lat, pos.lng];

		if (userAccuracyCircle) {
			userAccuracyCircle.setLatLng(latlng);
			userAccuracyCircle.setRadius(pos.accuracy);
		} else {
			userAccuracyCircle = L.circle(latlng, {
				radius: pos.accuracy,
				color: '#4285F4',
				fillColor: '#4285F4',
				fillOpacity: 0.1,
				weight: 1,
				interactive: false,
			}).addTo(map);
		}

		const dotHtml = `
			<div class="user-location-dot">
				<div class="user-location-dot-inner"></div>
			</div>`;

		if (userDotMarker) {
			userDotMarker.setLatLng(latlng);
		} else {
			userDotMarker = L.marker(latlng, {
				icon: L.divIcon({
					className: 'user-location-marker',
					html: dotHtml,
					iconSize: [16, 16],
					iconAnchor: [8, 8],
				}),
				interactive: false,
				zIndexOffset: 1000,
			}).addTo(map);
		}

		if (!firstUserFix) {
			firstUserFix = true;
			// Don't pan — let the user compare their position to the tracked device
		}
	}

	function updateUserHeadingLayer() {
		const L = leafletMap.getLeaflet();
		const map = leafletMap.getMap();
		const pos = userLocation.position;
		const heading = userLocation.heading;
		if (!L || !map || !pos) return;

		const latlng: [number, number] = [pos.lat, pos.lng];

		if (userHeadingMarker) {
			map.removeLayer(userHeadingMarker);
			userHeadingMarker = null;
		}

		if (heading === null || !userLocation.active) return;

		const coneHtml = `
			<svg width="40" height="40" viewBox="0 0 40 40" xmlns="http://www.w3.org/2000/svg"
				style="transform: rotate(${heading}deg); transform-origin: 20px 20px;">
				<path d="M20 0 L26 20 L20 16 L14 20 Z" fill="#4285F4" fill-opacity="0.5"/>
			</svg>`;

		userHeadingMarker = L.marker(latlng, {
			icon: L.divIcon({
				className: 'user-heading-marker',
				html: coneHtml,
				iconSize: [40, 40],
				iconAnchor: [20, 20],
			}),
			interactive: false,
			zIndexOffset: 999,
		}).addTo(map);
	}

	function removeUserLocationLayers() {
		const map = leafletMap.getMap();
		if (!map) return;
		if (userAccuracyCircle) { map.removeLayer(userAccuracyCircle); userAccuracyCircle = null; }
		if (userDotMarker) { map.removeLayer(userDotMarker); userDotMarker = null; }
		if (userHeadingMarker) { map.removeLayer(userHeadingMarker); userHeadingMarker = null; }
	}

	async function toggleLocateMe() {
		if (userLocation.active) {
			userLocation.stop();
			firstUserFix = false;
			removeUserLocationLayers();
		} else {
			await userLocation.start();
		}
	}

	// --- Update timer for "last update" display ---
	let updateTimer: ReturnType<typeof setInterval> | null = null;

	onMount(async () => {
		units = loadUnitPreference();

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
			<div class="spinner"></div>
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
						{units === 'metric' ? 'km/h' : 'mph'}
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
				class:active={userLocation.active}
				on:click={toggleLocateMe}
				title={userLocation.active ? 'Hide my location' : 'Show my location'}
				aria-label={userLocation.active ? 'Hide my location' : 'Show my location'}
			>
				<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
					<circle cx="12" cy="12" r="3"/>
					<path d="M12 2v4m0 12v4M2 12h4m12 0h4"/>
					<circle cx="12" cy="12" r="8" stroke-opacity="0.3"/>
				</svg>
			</button>
		</div>

		{#if userLocation.error}
			<div class="locate-error" role="alert">
				{userLocation.error}
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

	.spinner {
		width: 48px;
		height: 48px;
		border: 4px solid var(--border-color, #333);
		border-top-color: var(--accent-primary, #00d4ff);
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}

	@keyframes spin {
		to { transform: rotate(360deg); }
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

	.map-container :global(.leaflet-container) {
		height: 100%;
		width: 100%;
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

	.ws-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background-color: #ff4444;
		flex-shrink: 0;
	}

	.ws-indicator.connected .ws-dot {
		background-color: #00ff88;
		animation: pulse-dot 2s ease-in-out infinite;
	}

	.ws-indicator.connected .ws-label {
		color: #00ff88;
	}

	@keyframes pulse-dot {
		0%, 100% { opacity: 1; }
		50% { opacity: 0.5; }
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

	/* User location dot */
	.map-container :global(.user-location-marker) {
		background: none !important;
		border: none !important;
	}

	.map-container :global(.user-heading-marker) {
		background: none !important;
		border: none !important;
	}

	.map-container :global(.user-location-dot) {
		position: relative;
		width: 16px;
		height: 16px;
	}

	.map-container :global(.user-location-dot::before) {
		content: '';
		position: absolute;
		inset: 0;
		border-radius: 50%;
		background: #4285F4;
		opacity: 0.4;
		animation: user-location-pulse 1.8s ease-out infinite;
	}

	.map-container :global(.user-location-dot-inner) {
		width: 16px;
		height: 16px;
		background: #4285F4;
		border: 2.5px solid white;
		border-radius: 50%;
		box-shadow: 0 2px 6px rgba(0, 0, 0, 0.4);
		position: relative;
		z-index: 1;
	}

	@keyframes user-location-pulse {
		0% { transform: scale(1); opacity: 0.4; }
		100% { transform: scale(2.5); opacity: 0; }
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

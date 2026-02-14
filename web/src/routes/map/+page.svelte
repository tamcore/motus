<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/stores';
	import { api, fetchDevices, fetchPositions } from '$lib/api/client';
	import { wsManager } from '$lib/stores/websocket';
	import { settings } from '$lib/stores/settings';
	import { currentUser } from '$lib/stores/auth';
	import { refreshHandler } from '$lib/stores/refresh';
	import { useLeaflet } from '$lib/composables/useLeaflet';
	import { getOverlayById } from '$lib/utils/map-overlays';
	import type { Device, Position } from '$lib/types/api';
	import StatusIndicator from '$lib/components/StatusIndicator.svelte';
	import MapLayerControl from '$lib/components/MapLayerControl.svelte';
	import AllDevicesToggle from '$lib/components/AllDevicesToggle.svelte';
	import Button from '$lib/components/Button.svelte';
	import { formatSpeed, formatRelative, getCardinalDirection } from '$lib/utils/formatting';
	import { useUserLocation } from '$lib/composables/useUserLocation';

	const leafletMap = useLeaflet();
	const userLocation = useUserLocation();

	const GEOFENCE_STYLE = { color: '#00d4ff', weight: 2, fillOpacity: 0.15 };

	let mapContainer: HTMLDivElement;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let markers: Map<number, any> = new Map();
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let trailLayer: any;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let geofenceLayer: any;

	let devices: Device[] = [];
	let positions: Map<number, Position> = new Map();
	let selectedDeviceId: number | null = null;
	let sidebarOpen = true;
	let searchQuery = '';
	let showTrail = false;
	let trailPositions: Position[] = [];
	let loading = true;
	let wsConnected = false;

	// Live tracking state
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let liveTrails: Map<number, any> = new Map();
	let autoFollow = false;
	let selectedDeviceForFollow: number | null = null;
	let wsUnsubscribe: (() => void) | null = null;
	let wsConnUnsubscribe: (() => void) | null = null;

	// User location layers
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let userAccuracyCircle: any = null;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let userDotMarker: any = null;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let userHeadingMarker: any = null;
	let firstUserFix = false;

	// Map overlay state
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let overlayTileLayer: any = null;
	let currentOverlayId: string = $settings.mapOverlay;
	let currentOverlayOpacity: number = $settings.mapOverlayOpacity;

	$: selectedDevice = devices.find((d) => d.id === selectedDeviceId) || null;
	$: selectedPosition = selectedDeviceId ? positions.get(selectedDeviceId) : null;
	$: filtered = devices.filter(
		(d) =>
			d.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
			d.uniqueId.toLowerCase().includes(searchQuery.toLowerCase())
	);

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

		// Accuracy circle
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

		// Pulsing blue dot
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

		// Pan to first fix
		if (!firstUserFix) {
			firstUserFix = true;
			map.setView(latlng, Math.max(map.getZoom(), 15));
		}
	}

	function updateUserHeadingLayer() {
		const L = leafletMap.getLeaflet();
		const map = leafletMap.getMap();
		const pos = userLocation.position;
		const heading = userLocation.heading;
		if (!L || !map || !pos) return;

		const latlng: [number, number] = [pos.lat, pos.lng];

		// Remove existing heading marker
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

	onMount(async () => {
		// Subscribe to WebSocket connection state IMMEDIATELY (before any async work)
		// so the indicator reflects the correct status from the start.
		// Svelte stores deliver their current value synchronously on subscribe.
		wsConnUnsubscribe = wsManager.connected.subscribe((connected) => {
			wsConnected = connected;
		});

		// Initialize map via composable (handles dynamic import, tiles, zoom control)
		await leafletMap.initialize(mapContainer, {
			center: [49.79, 9.95],
			zoom: 6,
		});

		const L = leafletMap.getLeaflet()!;
		const map = leafletMap.getMap()!;

		trailLayer = L.layerGroup().addTo(map);

		// Restore saved overlay from settings
		if (currentOverlayId !== 'none') {
			applyOverlay(currentOverlayId, currentOverlayOpacity);
		}

		// Check for device query param
		const deviceParam = $page.url.searchParams.get('device');
		if (deviceParam) {
			const parsed = parseInt(deviceParam, 10);
			if (!isNaN(parsed)) {
				selectedDeviceId = parsed;
			}
		}

		try {
			const isAdmin = ($currentUser as Record<string, unknown> | null)?.administrator === true;
			const [devs, pos] = await Promise.all([
				fetchDevices(isAdmin),
				fetchPositions(isAdmin)
			]);
			void loadGeofences();
			devices = devs;

			for (const p of pos) {
				positions.set(p.deviceId, p);
				addMarker(p);
			}
			positions = positions;

			// If a specific device was requested via query param, zoom to it
			if (selectedDeviceId && positions.has(selectedDeviceId)) {
				const targetPos = positions.get(selectedDeviceId)!;
				map.setView([targetPos.latitude, targetPos.longitude], 16);

				// Open the marker popup for the selected device
				const marker = markers.get(selectedDeviceId);
				if (marker) {
					marker.openPopup();
				}
			} else if (markers.size > 0) {
				// Otherwise fit bounds to all markers
				const group = L.featureGroup(Array.from(markers.values()));
				map.fitBounds(group.getBounds().pad(0.1));
			}
		} catch (error) {
			console.error('Failed to load map data:', error);
		} finally {
			loading = false;
		}

		// Subscribe to WebSocket messages AFTER Leaflet and map are initialized
		// This ensures L and map are available when processing position updates
		wsUnsubscribe = wsManager.lastMessage.subscribe((msg) => {
			const wsL = leafletMap.getLeaflet();
			const wsMap = leafletMap.getMap();
			if (!msg || !wsL || !wsMap) return;

			if (msg.positions) {
				for (const pos of msg.positions as unknown as Position[]) {
					positions.set(pos.deviceId, pos);
					updateMarker(pos);
					updateLiveTrail(pos);

					// Auto-pan to selected device's new position
					if (selectedDeviceId === pos.deviceId) {
						wsMap.panTo([pos.latitude, pos.longitude], { animate: true, duration: 0.5 });
					}

					// Or if auto-follow is explicitly enabled for a different device
					if (autoFollow && selectedDeviceForFollow === pos.deviceId && selectedDeviceForFollow !== selectedDeviceId) {
						wsMap.panTo([pos.latitude, pos.longitude]);
					}
				}
				// Trigger Svelte reactivity for the positions map
				positions = positions;
			}

			if (msg.devices) {
				for (const dev of msg.devices as unknown as Device[]) {
					const idx = devices.findIndex((d) => d.id === dev.id);
					if (idx >= 0) {
						devices[idx] = dev;
					}
				}
				// Trigger Svelte reactivity for devices array
				devices = devices;
			}
		});
		$refreshHandler = reloadDevices;
	});

	onDestroy(() => {
		$refreshHandler = null;
		if (wsUnsubscribe) wsUnsubscribe();
		if (wsConnUnsubscribe) wsConnUnsubscribe();
		clearLiveTrails();
		userLocation.stop();
		leafletMap.cleanup();
	});

	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	function createMarkerIcon(status: string): any {
		const L = leafletMap.getLeaflet();
		if (!L) return null;

		const color =
			status === 'online'
				? 'var(--map-marker-online)'
				: status === 'moving'
					? '#00d4ff'
					: '#ff4444';

		return L.divIcon({
			className: 'custom-marker',
			html: `<div style="
				width: 24px; height: 24px;
				background: ${color};
				border: 3px solid white;
				border-radius: 50%;
				box-shadow: 0 2px 6px rgba(0,0,0,0.4);
			"></div>`,
			iconSize: [24, 24],
			iconAnchor: [12, 12]
		});
	}

	function getPopupContent(device: Device, pos: Position): string {
		const speed = pos.speed != null ? formatSpeed(pos.speed) : 'N/A';
		const time = pos.fixTime ? new Date(pos.fixTime).toLocaleString() : 'Unknown';
		return `<strong>${device.name}</strong><br>Speed: ${speed}<br>Time: ${time}`;
	}

	function addMarker(pos: Position) {
		const L = leafletMap.getLeaflet();
		const map = leafletMap.getMap();
		const device = devices.find((d) => d.id === pos.deviceId);
		if (!device || !L || !map) return;

		const marker = L.marker([pos.latitude, pos.longitude], {
			icon: createMarkerIcon(device.status)
		})
			.addTo(map)
			.bindPopup(getPopupContent(device, pos));

		marker.on('click', () => {
			selectedDeviceId = device.id;
		});

		markers.set(pos.deviceId, marker);
	}

	function updateMarker(pos: Position) {
		const L = leafletMap.getLeaflet();
		const device = devices.find((d) => d.id === pos.deviceId);
		if (!device || !L) return;

		const existing = markers.get(pos.deviceId);
		if (existing) {
			existing.setLatLng([pos.latitude, pos.longitude]);
			existing.setIcon(createMarkerIcon(device.status));
			existing.getPopup()?.setContent(getPopupContent(device, pos));
		} else {
			addMarker(pos);
		}

		// Update trail if viewing this device
		if (showTrail && selectedDeviceId === pos.deviceId) {
			trailPositions = [...trailPositions, pos];
			drawTrail();
		}
	}

	function selectDevice(deviceId: number) {
		const map = leafletMap.getMap();
		selectedDeviceId = deviceId;
		showTrail = false;
		trailPositions = [];
		trailLayer?.clearLayers();

		const pos = positions.get(deviceId);
		if (pos && map) {
			map.setView([pos.latitude, pos.longitude], 16);
			markers.get(deviceId)?.openPopup();
		}
	}

	async function loadTrail() {
		const L = leafletMap.getLeaflet();
		const map = leafletMap.getMap();
		if (!selectedDeviceId) return;
		showTrail = true;

		const now = new Date();
		const from = new Date(now.getTime() - 24 * 60 * 60 * 1000);

		try {
			trailPositions = await api.getPositions({
				deviceId: selectedDeviceId,
				from: from.toISOString(),
				to: now.toISOString(),
				limit: 5000
			});
			drawTrail();

			// Fit map to trail bounds if we have positions
			if (trailPositions.length > 1 && trailLayer && L && map) {
				const coords = trailPositions.map((p) => L.latLng(p.latitude, p.longitude));
				const bounds = L.latLngBounds(coords);
				map.fitBounds(bounds.pad(0.1));
			}
		} catch {
			console.error('Failed to load trail');
		}
	}

	function hideTrail() {
		showTrail = false;
		trailPositions = [];
		trailLayer?.clearLayers();
	}

	function drawTrail() {
		const L = leafletMap.getLeaflet();
		if (!trailLayer || !L) return;
		trailLayer.clearLayers();

		if (trailPositions.length < 2) return;

		const coords = trailPositions.map((p) => [p.latitude, p.longitude]);
		L.polyline(coords as [number, number][], {
			color: '#00d4ff',
			weight: 3,
			opacity: 0.8
		}).addTo(trailLayer);

		// Start marker
		const first = trailPositions[0];
		L.circleMarker([first.latitude, first.longitude], {
			radius: 6,
			fillColor: '#00ff88',
			fillOpacity: 1,
			color: 'white',
			weight: 2
		})
			.addTo(trailLayer)
			.bindPopup(`Start: ${new Date(first.fixTime).toLocaleString()}`);
	}

	function updateLiveTrail(position: Position) {
		const L = leafletMap.getLeaflet();
		const map = leafletMap.getMap();
		if (!L || !map) return;

		const deviceId = position.deviceId;

		if (!liveTrails.has(deviceId)) {
			const trail = L.polyline([], {
				color: '#00d4ff',
				weight: 3,
				opacity: 0.8
			}).addTo(map);
			liveTrails.set(deviceId, trail);
		}

		const trail = liveTrails.get(deviceId)!;
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const coords: any[] = trail.getLatLngs();

		// Add new point
		const newPoint = L.latLng(position.latitude, position.longitude);
		coords.push(newPoint);

		// Keep only last 100 points to avoid memory bloat
		if (coords.length > 100) {
			coords.shift();
		}

		trail.setLatLngs(coords);
	}

	function toggleAutoFollow(deviceId: number) {
		const map = leafletMap.getMap();
		if (autoFollow && selectedDeviceForFollow === deviceId) {
			autoFollow = false;
			selectedDeviceForFollow = null;
		} else {
			autoFollow = true;
			selectedDeviceForFollow = deviceId;
			selectedDeviceId = deviceId;

			// Immediately center on the device
			const pos = positions.get(deviceId);
			if (pos && map) {
				map.setView([pos.latitude, pos.longitude], 16);
			}
		}
	}

	function clearLiveTrails() {
		const map = leafletMap.getMap();
		for (const trail of liveTrails.values()) {
			if (map) trail.removeFrom(map);
		}
		liveTrails.clear();
	}

	async function loadGeofences() {
		const L = leafletMap.getLeaflet();
		const map = leafletMap.getMap();
		if (!L || !map) return;

		try {
			const geofences = await api.getGeofences();
			if (geofenceLayer) {
				map.removeLayer(geofenceLayer);
			}
			geofenceLayer = L.layerGroup().addTo(map);
			for (const gf of geofences) {
				if (!gf.geometry) continue;
				try {
					const geometry = JSON.parse(gf.geometry);
					const layer = L.geoJSON(geometry, { style: () => GEOFENCE_STYLE });
					layer.bindPopup(`<strong>${gf.name}</strong>${gf.description ? '<br>' + gf.description : ''}`);
					layer.addTo(geofenceLayer);
				} catch {
					console.error('Failed to parse geofence geometry for', gf.name);
				}
			}
		} catch (err) {
			console.error('Failed to load geofences:', err);
		}
	}

	function applyOverlay(overlayId: string, opacity: number) {
		const L = leafletMap.getLeaflet();
		const map = leafletMap.getMap();
		if (!L || !map) return;

		// Remove existing overlay layer if present
		if (overlayTileLayer) {
			map.removeLayer(overlayTileLayer);
			overlayTileLayer = null;
		}

		// Apply new overlay if not "none"
		if (overlayId !== 'none') {
			const overlay = getOverlayById(overlayId);
			if (overlay && overlay.url) {
				overlayTileLayer = L.tileLayer(overlay.url, {
					attribution: overlay.attribution,
					maxZoom: overlay.maxZoom,
					opacity: opacity / 100,
				}).addTo(map);
			}
		}

		// Persist to settings
		currentOverlayId = overlayId;
		currentOverlayOpacity = opacity;
		settings.update((s) => ({
			...s,
			mapOverlay: overlayId,
			mapOverlayOpacity: opacity,
		}));
	}

	function handleLayerChange(event: CustomEvent<{ overlayId: string; opacity: number }>) {
		applyOverlay(event.detail.overlayId, event.detail.opacity);
	}

	function toggleSidebar() {
		sidebarOpen = !sidebarOpen;
	}

	async function reloadDevices() {
		const isAdmin = ($currentUser as Record<string, unknown> | null)?.administrator === true;
		try {
			const [newDevices, newPositions] = await Promise.all([
				fetchDevices(isAdmin),
				fetchPositions(isAdmin)
			]);
			devices = newDevices;

			// Remove markers for devices no longer in the list
			const deviceIds = new Set(newDevices.map((d) => d.id));
			for (const [devId, marker] of markers) {
				if (!deviceIds.has(devId)) {
					marker.remove();
					markers.delete(devId);
				}
			}

			// Update positions map and add/update markers
			positions.clear();
			for (const p of newPositions) {
				positions.set(p.deviceId, p);
				updateMarker(p);
			}
			positions = positions;
		} catch {
			console.error('Failed to reload devices');
		}
	}

	function getStatusType(status: string): 'online' | 'offline' | 'idle' | 'moving' {
		if (status === 'online') return 'online';
		if (status === 'idle') return 'idle';
		if (status === 'moving') return 'moving';
		return 'offline';
	}
</script>

<svelte:head>
	<title>Map - Motus</title>
</svelte:head>

<div class="map-page">
	<!-- Sidebar -->
	<aside class="sidebar" class:collapsed={!sidebarOpen}>
		<div class="sidebar-header">
			<h2 class="sidebar-title">Devices</h2>
			<AllDevicesToggle on:change={reloadDevices} />
			<button class="sidebar-toggle" on:click={toggleSidebar} aria-label="Toggle sidebar">
				{sidebarOpen ? '\u276E' : '\u276F'}
			</button>
		</div>

		{#if sidebarOpen}
			<div class="search-box">
				<input
					type="search"
					placeholder="Search..."
					class="search-input"
					bind:value={searchQuery}
				/>
			</div>

			<div class="device-list">
				{#each filtered as device (device.id)}
					{@const pos = positions.get(device.id)}
					<div
						class="device-item"
						class:selected={selectedDeviceId === device.id}
						class:other-user={device.ownerName}
					>
						<button
							class="device-item-btn"
							on:click={() => selectDevice(device.id)}
						>
							<div class="device-info">
								<div class="device-top">
									<span class="device-name">{device.name}</span>
									{#if device.ownerName}
										<span class="owner-badge" title="Owned by {device.ownerName}">{device.ownerName}</span>
									{/if}
									<StatusIndicator status={getStatusType(device.status)} />
								</div>
								{#if device.status === 'online' || device.status === 'moving'}
									{#if pos}
										<span class="device-meta">
											{pos.speed != null ? formatSpeed(pos.speed) : 'N/A'}
											{#if pos.course != null}
												&middot; {getCardinalDirection(pos.course)}
											{/if}
										</span>
									{/if}
								{:else}
									<span class="device-meta device-meta--muted">
										{#if device.lastUpdate}
											Last seen {formatRelative(new Date(device.lastUpdate))}
										{:else}
											Never seen
										{/if}
									</span>
								{/if}
							</div>
						</button>
					</div>
				{/each}
			</div>

			<!-- Device detail panel -->
			{#if selectedDevice && selectedPosition}
				<div class="detail-panel">
					<h3 class="detail-title">{selectedDevice.name}</h3>
					<div class="detail-grid">
						<div class="detail-item">
							<span class="detail-label">Latitude</span>
							<span class="detail-value">{selectedPosition.latitude.toFixed(6)}</span>
						</div>
						<div class="detail-item">
							<span class="detail-label">Longitude</span>
							<span class="detail-value">{selectedPosition.longitude.toFixed(6)}</span>
						</div>
						<div class="detail-item">
							<span class="detail-label">Speed</span>
							<span class="detail-value">{selectedPosition.speed != null ? formatSpeed(selectedPosition.speed) : 'N/A'}</span>
						</div>
						<div class="detail-item">
							<span class="detail-label">Course</span>
							<span class="detail-value">{selectedPosition.course != null ? selectedPosition.course.toFixed(0) : '0'}&deg;</span>
						</div>
					</div>
					<div class="detail-actions">
						{#if showTrail}
							<Button
								variant="secondary"
								size="sm"
								on:click={hideTrail}
							>
								Hide Trail
							</Button>
							<Button
								variant="secondary"
								size="sm"
								on:click={loadTrail}
							>
								Refresh
							</Button>
						{:else}
							<Button
								variant="secondary"
								size="sm"
								on:click={loadTrail}
							>
								Show Trail (24h)
							</Button>
						{/if}
					</div>
				</div>
			{/if}
		{/if}
	</aside>

	<!-- Map container -->
	<div class="map-container" bind:this={mapContainer}>
		{#if loading}
			<div class="map-loading">
				<div class="spinner"></div>
			</div>
		{/if}

		<!-- Map layer overlay control -->
		<MapLayerControl
			selectedOverlayId={currentOverlayId}
			opacity={currentOverlayOpacity}
			on:change={handleLayerChange}
		/>

		<!-- WebSocket connection indicator -->
		<div class="ws-indicator" class:connected={wsConnected} title={wsConnected ? 'Live tracking active' : 'WebSocket disconnected'}>
			<span class="ws-dot"></span>
			<span class="ws-label">{wsConnected ? 'Live' : 'Offline'}</span>
		</div>

		<!-- Locate Me button -->
		<button
			class="locate-me-btn"
			class:active={userLocation.active}
			on:click={toggleLocateMe}
			title={userLocation.active ? 'Stop locating me' : 'Show my location'}
			aria-label={userLocation.active ? 'Stop locating me' : 'Show my location'}
		>
			<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
				<circle cx="12" cy="12" r="3"/>
				<path d="M12 2v4m0 12v4M2 12h4m12 0h4"/>
				<circle cx="12" cy="12" r="8" stroke-opacity="0.3"/>
			</svg>
		</button>

		{#if userLocation.error}
			<div class="locate-error" role="alert">
				{userLocation.error}
			</div>
		{/if}
	</div>
</div>

<style>
	.map-page {
		display: flex;
		height: calc(100vh - 65px);
		overflow: hidden;
	}

	.sidebar {
		width: 320px;
		background-color: var(--bg-secondary);
		border-right: 1px solid var(--border-color);
		display: flex;
		flex-direction: column;
		overflow: hidden;
		transition: width var(--transition-slow);
	}

	.sidebar.collapsed {
		width: 48px;
	}

	.sidebar-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: var(--space-3) var(--space-4);
		border-bottom: 1px solid var(--border-color);
		min-height: 48px;
	}

	.sidebar-title {
		font-size: var(--text-base);
		font-weight: var(--font-semibold);
		color: var(--text-primary);
	}

	.collapsed .sidebar-title {
		display: none;
	}

	.sidebar-toggle {
		background: none;
		border: none;
		color: var(--text-secondary);
		cursor: pointer;
		font-size: var(--text-lg);
		padding: var(--space-1);
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
		border-bottom: 1px solid var(--border-color);
		transition: background-color var(--transition-fast);
	}

	.device-item:hover {
		background-color: var(--bg-hover);
	}

	.device-item.selected {
		background-color: var(--bg-active);
		border-left: 3px solid var(--accent-primary);
	}

	.device-item-btn {
		flex: 1;
		display: flex;
		padding: var(--space-3) var(--space-4);
		background: none;
		border: none;
		cursor: pointer;
		text-align: left;
	}

	.device-info {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.device-top {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.device-name {
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		color: var(--text-primary);
	}

	.device-meta {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
	}

	.device-meta--muted {
		color: var(--text-tertiary);
		font-style: italic;
	}

	.detail-panel {
		border-top: 1px solid var(--border-color);
		padding: var(--space-4);
		background-color: var(--bg-primary);
	}

	.detail-title {
		font-size: var(--text-base);
		font-weight: var(--font-semibold);
		color: var(--text-primary);
		margin-bottom: var(--space-3);
	}

	.detail-grid {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--space-3);
		margin-bottom: var(--space-3);
	}

	.detail-item {
		display: flex;
		flex-direction: column;
	}

	.detail-label {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
	}

	.detail-value {
		font-size: var(--text-sm);
		color: var(--text-primary);
		font-weight: var(--font-medium);
	}

	.detail-actions {
		display: flex;
		gap: var(--space-2);
	}

	.map-container {
		flex: 1;
		position: relative;
	}

	.map-container :global(.leaflet-container) {
		height: 100%;
		width: 100%;
		background-color: var(--bg-tertiary);
	}

	.map-loading {
		position: absolute;
		inset: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		background-color: var(--bg-primary);
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

	/* WebSocket connection indicator */
	.ws-indicator {
		position: absolute;
		top: var(--space-3);
		left: var(--space-3);
		z-index: 500;
		display: flex;
		align-items: center;
		gap: var(--space-2);
		padding: var(--space-1) var(--space-3);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-full);
		font-size: var(--text-xs);
		color: var(--text-secondary);
		box-shadow: var(--shadow-md);
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

	/* Locate Me button — sits below the ws-indicator pill (~28px) */
	.locate-me-btn {
		position: absolute;
		top: calc(var(--space-3) + 28px + var(--space-2));
		left: var(--space-3);
		z-index: 500;
		display: flex;
		align-items: center;
		justify-content: center;
		width: 36px;
		height: 36px;
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-full);
		color: var(--text-secondary);
		cursor: pointer;
		box-shadow: var(--shadow-md);
		transition: all var(--transition-fast);
	}

	.locate-me-btn:hover {
		color: var(--text-primary);
		border-color: var(--accent-primary);
	}

	.locate-me-btn.active {
		color: #4285F4;
		border-color: #4285F4;
		background-color: rgba(66, 133, 244, 0.1);
	}

	.locate-error {
		position: absolute;
		top: calc(var(--space-3) + 28px + var(--space-2) + 36px + var(--space-2));
		left: var(--space-3);
		z-index: 500;
		max-width: 200px;
		padding: var(--space-2) var(--space-3);
		background-color: rgba(255, 68, 68, 0.15);
		border: 1px solid rgba(255, 68, 68, 0.4);
		border-radius: var(--radius-md);
		font-size: var(--text-xs);
		color: var(--error);
	}

	/* User location dot (injected into Leaflet as divIcon) */
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

	@media (max-width: 768px) {
		.map-page {
			flex-direction: column;
		}

		.sidebar {
			width: 100%;
			max-height: 40vh;
			border-right: none;
			border-bottom: 1px solid var(--border-color);
		}

		.sidebar.collapsed {
			width: 100%;
			max-height: 48px;
		}
	}

	.device-item.other-user {
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

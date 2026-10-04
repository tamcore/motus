<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/stores';
	import { replaceState } from '$app/navigation';
	import { api, fetchDevices, fetchPositions } from '$lib/api/client';
	import { wsManager } from '$lib/stores/websocket';
	import { settings } from '$lib/stores/settings';
	import { refreshHandler } from '$lib/stores/refresh';
	import { useLeaflet } from '$lib/composables/useLeaflet';
	import { getOverlayById } from '$lib/utils/map-overlays';
	import { buildPopupElement, type PopupRow } from '$lib/utils/popup';
	import type { Device, DeviceStatus, Position, TrailBookmark, TrailBookmarkPayload } from '$lib/types/api';
	import {
		positionToRoutePosition,
		toRoutePositions,
		type RoutePosition
	} from '$lib/utils/route-points';
	import StatusIndicator from '$lib/components/StatusIndicator.svelte';
	import BatteryIndicator from '$lib/components/BatteryIndicator.svelte';
	import MapLayerControl from '$lib/components/MapLayerControl.svelte';
	import AllDevicesToggle from '$lib/components/AllDevicesToggle.svelte';
	import Button from '$lib/components/Button.svelte';
	import { formatSpeed, formatRelative, getCardinalDirection } from '$lib/utils/formatting';
	import { useUserLocation, userLocationLayers } from '$lib/composables/useUserLocation';
	import TrailRangeSelector from '$lib/components/TrailRangeSelector.svelte';
	import { trailRange } from '$lib/stores/trailRange';
	import {
		isLiveRange,
		normalizeTrailRange,
		resolveTrailRange,
		trailRangeFromSearchParams,
		trailRangeToSearchParams,
		type TrailRange
	} from '$lib/utils/trail-range';
	import TrailBookmarkList from '$lib/components/TrailBookmarkList.svelte';
	import TrailBookmarkModal from '$lib/components/TrailBookmarkModal.svelte';
	import { bookmarkToTrailRange } from '$lib/utils/trail-bookmarks';
	import { GEOFENCE_STYLE } from '$lib/utils/geofence-draw';

	const leafletMap = useLeaflet();
	const userLocation = useUserLocation();
	const userLayers = userLocationLayers(() => leafletMap.getLeaflet(), () => leafletMap.getMap());

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
	let trailPositions: RoutePosition[] = [];
	// Applied range: starts at the saved one; a URL range applies to this view
	// only, an explicit selection is also saved.
	let activeTrailRange: TrailRange = $trailRange;
	let trailLoading = false;
	let trailError = '';
	// Incremented per trail request so stale responses (range changed or trail
	// hidden mid-flight) are discarded.
	let trailRequestId = 0;
	// Max points per trail request. The server samples long ranges down to
	// this, so 30 days / all time stay responsive.
	const TRAIL_POINT_LIMIT = 5000;

	// Trail bookmarks of the selected device.
	let deviceBookmarks: TrailBookmark[] = [];
	let bookmarksLoading = false;
	let bookmarksError = '';
	let bookmarksDeviceId: number | null = null;
	let bookmarkRequestId = 0;
	let bookmarkModalOpen = false;
	let editingBookmark: TrailBookmark | null = null;

	let loading = true;
	let wsConnected = false;

	// Live tracking state
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let liveTrails: Map<number, any> = new Map();
	let autoFollow = false;
	let selectedDeviceForFollow: number | null = null;
	let wsUnsubscribe: (() => void) | null = null;
	let wsConnUnsubscribe: (() => void) | null = null;


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

	$: if (selectedDeviceId !== bookmarksDeviceId) void loadDeviceBookmarks(selectedDeviceId);

	$: userLayers.sync($userLocation);

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

		// Trail range from URL (?trail=7d or ?from=…&to=…) applies to this view only.
		const urlRange = trailRangeFromSearchParams($page.url.searchParams);
		if (urlRange) activeTrailRange = urlRange;

		// Check for device query param
		const deviceParam = $page.url.searchParams.get('device');
		if (deviceParam) {
			const parsed = parseInt(deviceParam, 10);
			if (!isNaN(parsed)) {
				selectedDeviceId = parsed;
			}
		}

		try {
			const [devs, pos] = await Promise.all([
				fetchDevices(),
				fetchPositions()
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

		// A device link with an explicit range shows that trail right away.
		if (urlRange && selectedDeviceId && positions.has(selectedDeviceId)) {
			void loadTrail();
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
	function createMarkerIcon(status: DeviceStatus): any {
		const L = leafletMap.getLeaflet();
		if (!L) return null;

		const color = { online: 'var(--map-marker-online)', offline: 'var(--map-marker-offline)', unknown: 'var(--text-tertiary)' }[status];

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

	function getPopupContent(device: Device, pos: Position): HTMLElement {
		const speed = pos.speed != null ? formatSpeed(pos.speed) : 'N/A';
		const time = pos.fixTime ? new Date(pos.fixTime).toLocaleString() : 'Unknown';
		return buildPopupElement([
			{ type: 'heading', text: device.name },
			{ type: 'text', text: `Speed: ${speed}` },
			{ type: 'text', text: `Time: ${time}` }
		]);
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
		if (showTrail && selectedDeviceId === pos.deviceId && isLiveRange(activeTrailRange)) {
			trailPositions = [...trailPositions, positionToRoutePosition(pos)];
			drawTrail();
		}
	}

	function selectDevice(deviceId: number) {
		const map = leafletMap.getMap();
		selectedDeviceId = deviceId;
		hideTrail();

		const pos = positions.get(deviceId);
		if (pos && map) {
			map.setView([pos.latitude, pos.longitude], 16);
			markers.get(deviceId)?.openPopup();
		}
	}

	async function loadTrail() {
		const L = leafletMap.getLeaflet();
		const map = leafletMap.getMap();
		const deviceId = selectedDeviceId;
		if (!deviceId) return;
		showTrail = true;
		trailError = '';
		trailLoading = true;
		const requestId = ++trailRequestId;
		// Marker clicks and auto-follow change the selection without hiding the
		// trail, so the device is checked too.
		const isStale = () =>
			requestId !== trailRequestId || !showTrail || selectedDeviceId !== deviceId;

		const { from, to } = resolveTrailRange(activeTrailRange);

		try {
			const result = toRoutePositions(
				await api.getPositionPoints({
					deviceId,
					from: from.toISOString(),
					to: to.toISOString(),
					limit: TRAIL_POINT_LIMIT
				})
			);
			if (isStale()) return;
			trailPositions = result;
			drawTrail();

			// Fit map to trail bounds if we have positions
			if (trailPositions.length > 1 && trailLayer && L && map) {
				const coords = trailPositions.map((p) => L.latLng(p.latitude, p.longitude));
				const bounds = L.latLngBounds(coords);
				map.fitBounds(bounds.pad(0.1));
			}
		} catch {
			if (isStale()) return;
			trailError = 'Failed to load trail';
			console.error('Failed to load trail');
		} finally {
			if (requestId === trailRequestId) trailLoading = false;
		}
	}

	function hideTrail() {
		trailRequestId++;
		showTrail = false;
		trailLoading = false;
		trailError = '';
		trailPositions = [];
		trailLayer?.clearLayers();
	}

	/** An explicit selection in the range selector: applied and saved. */
	function handleTrailRangeChange(range: TrailRange) {
		applyTrailRange(range, { save: true });
	}

	/**
	 * Applies a range to the current view: URL (with the selected device, so
	 * the link reopens this trail) and trail reload. `save` also makes it the
	 * user's saved default range; bookmarks (like URL ranges) apply to the
	 * view only.
	 */
	function applyTrailRange(range: TrailRange, { save }: { save: boolean }) {
		if (save) {
			trailRange.set(range);
			activeTrailRange = $trailRange;
		} else {
			activeTrailRange = normalizeTrailRange(range) ?? activeTrailRange;
		}
		// Reflect the range in the URL so the view can be shared/bookmarked.
		const url = new URL($page.url);
		const params = new URLSearchParams(url.searchParams);
		if (selectedDeviceId != null) params.set('device', String(selectedDeviceId));
		url.search = trailRangeToSearchParams(activeTrailRange, params).toString();
		replaceState(url, $page.state);
		// Drop the previous range's trail so a failed request cannot leave
		// unrelated coordinates on the map.
		trailPositions = [];
		trailLayer?.clearLayers();
		// Picking a range means "show me this trail" for the selected device.
		void loadTrail();
	}

	async function loadDeviceBookmarks(deviceId: number | null) {
		const deviceChanged = deviceId !== bookmarksDeviceId;
		bookmarksDeviceId = deviceId;
		const requestId = ++bookmarkRequestId;
		bookmarksError = '';
		if (deviceId == null) {
			deviceBookmarks = [];
			bookmarksLoading = false;
			return;
		}
		// Another device's bookmarks must not stay visible (and clickable)
		// while this device's list loads; a same-device refresh keeps them.
		if (deviceChanged) deviceBookmarks = [];
		bookmarksLoading = true;
		try {
			const list = await api.getTrailBookmarks(deviceId);
			if (requestId !== bookmarkRequestId) return;
			deviceBookmarks = list;
		} catch {
			if (requestId !== bookmarkRequestId) return;
			deviceBookmarks = [];
			bookmarksError = 'Failed to load bookmarks';
		} finally {
			if (requestId === bookmarkRequestId) bookmarksLoading = false;
		}
	}

	/** Shows the bookmarked range of the selected device (view only). */
	function openBookmark(bookmark: TrailBookmark) {
		applyTrailRange(bookmarkToTrailRange(bookmark), { save: false });
	}

	function openSaveBookmark() {
		editingBookmark = null;
		bookmarkModalOpen = true;
	}

	function openEditBookmark(bookmark: TrailBookmark) {
		editingBookmark = bookmark;
		bookmarkModalOpen = true;
	}

	function closeBookmarkModal() {
		bookmarkModalOpen = false;
		editingBookmark = null;
	}

	async function saveBookmark(payload: TrailBookmarkPayload) {
		if (editingBookmark) {
			await api.updateTrailBookmark(editingBookmark.id, payload);
		} else {
			await api.createTrailBookmark(payload);
		}
		closeBookmarkModal();
		await loadDeviceBookmarks(selectedDeviceId);
	}

	async function deleteBookmark(bookmark: TrailBookmark) {
		if (!confirm(`Delete bookmark "${bookmark.name}"?`)) return;
		try {
			await api.deleteTrailBookmark(bookmark.id);
			await loadDeviceBookmarks(selectedDeviceId);
		} catch (err: unknown) {
			bookmarksError = (err instanceof Error ? err.message : 'Failed to delete bookmark');
		}
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
					const popupRows: PopupRow[] = [{ type: 'heading', text: gf.name }];
					if (gf.description) popupRows.push({ type: 'text', text: gf.description });
					layer.bindPopup(buildPopupElement(popupRows));
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
		try {
			const [newDevices, newPositions] = await Promise.all([
				fetchDevices(),
				fetchPositions()
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
					class="search-input field-sm"
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
										<span class="owner-badge owner-badge-sm" title="Owned by {device.ownerName}">{device.ownerName}</span>
									{/if}
									<span class="device-indicators">
										<BatteryIndicator level={device.batteryLevel} />
										<StatusIndicator status={device.status} />
									</span>
								</div>
								{#if device.status === 'online'}
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
					<TrailRangeSelector range={activeTrailRange} onChange={handleTrailRangeChange} />
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
								Show Trail
							</Button>
						{/if}
						<Button
							variant="secondary"
							size="sm"
							on:click={openSaveBookmark}
						>
							Save as bookmark
						</Button>
					</div>
					{#if showTrail}
						<p class="trail-status" role="status">
							{#if trailLoading}
								Loading trail…
							{:else if trailError}
								{trailError}
							{:else if trailPositions.length === 0}
								No positions in this range
							{:else}
								{trailPositions.length.toLocaleString()} points
							{/if}
						</p>
					{/if}
					<TrailBookmarkList
						bookmarks={deviceBookmarks}
						activeRange={activeTrailRange}
						loading={bookmarksLoading}
						error={bookmarksError}
						onOpen={openBookmark}
						onEdit={openEditBookmark}
						onDelete={deleteBookmark}
					/>
				</div>
			{/if}
		{/if}
	</aside>

	<!-- Map container -->
	<div class="map-container" bind:this={mapContainer}>
		{#if loading}
			<div class="map-loading">
				<div class="spinner spinner-lg"></div>
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
			class:active={$userLocation.active}
			on:click={() => userLayers.toggle(userLocation)}
			title={$userLocation.active ? 'Stop locating me' : 'Show my location'}
			aria-label={$userLocation.active ? 'Stop locating me' : 'Show my location'}
		>
			<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
				<circle cx="12" cy="12" r="3"/>
				<path d="M12 2v4m0 12v4M2 12h4m12 0h4"/>
				<circle cx="12" cy="12" r="8" stroke-opacity="0.3"/>
			</svg>
		</button>

		{#if $userLocation.error}
			<div class="locate-error" role="alert">
				{$userLocation.error}
			</div>
		{/if}
	</div>
</div>

<TrailBookmarkModal
	bind:open={bookmarkModalOpen}
	bookmark={editingBookmark}
	deviceId={selectedDeviceId}
	range={activeTrailRange}
	onSave={saveBookmark}
	onClose={closeBookmarkModal}
/>

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

	.device-indicators {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
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
		/* Range selector + bookmarks can get tall: scroll instead of clipping. */
		max-height: 65%;
		overflow-y: auto;
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
		flex-wrap: wrap;
		gap: var(--space-2);
		margin-top: var(--space-2);
	}

	.trail-status {
		margin: var(--space-2) 0 0;
		font-size: var(--text-xs);
		color: var(--text-tertiary);
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

	/* Locate Me button — sits below the ws-indicator pill (~28px) */
	.locate-me-btn {
		position: absolute;
		top: calc(var(--space-3) + 28px + var(--space-2));
		left: var(--space-3);
	}

	.locate-error {
		top: calc(var(--space-3) + 28px + var(--space-2) + 36px + var(--space-2));
		left: var(--space-3);
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
</style>

<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api, fetchGeofences, fetchCalendars } from '$lib/api/client';
	import { currentUser } from '$lib/stores/auth';
	import { refreshHandler } from '$lib/stores/refresh';
	import { useUserLocation } from '$lib/composables/useUserLocation';
	import Button from '$lib/components/Button.svelte';
	import Input from '$lib/components/Input.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import AllDevicesToggle from '$lib/components/AllDevicesToggle.svelte';
	import type { Geofence as ApiGeofence, Calendar } from '$lib/types/api';

	interface Geofence extends ApiGeofence {
		layer?: any;
	}

	let mapContainer: HTMLDivElement;
	let map: any;
	let L: any;
	let drawnItems: any;
	let drawControl: any;

	let geofences: Geofence[] = [];
	let calendars: Calendar[] = [];
	let calendarActiveStatus: Record<number, boolean> = {};
	let selectedGeofence: Geofence | null = null;
	let showCreateModal = false;
	let showEditModal = false;
	let newGeofenceName = '';
	let newGeofenceCalendarId: number | null = null;
	let editGeofenceName = '';
	let editGeofenceCalendarId: number | null = null;
	let editingGeofence: Geofence | null = null;
	let pendingLayer: any = null;
	let loading = true;
	let saving = false;
	let error = '';

	// User location
	const userLocation = useUserLocation();
	let userAccuracyCircle: any = null;
	let userDotMarker: any = null;
	let userHeadingMarker: any = null;
	let firstUserFix = false;

	// Default center (Germany) - used as last resort
	const DEFAULT_CENTER: [number, number] = [51.1657, 10.4515];
	const DEFAULT_ZOOM = 6;

	const GEOFENCE_STYLE = {
		color: '#00d4ff',
		weight: 2,
		fillOpacity: 0.15
	};

	/**
	 * Look up a calendar name by its ID from the loaded calendar list.
	 */
	function getCalendarName(calendarId: number | null | undefined): string {
		if (calendarId == null) return '';
		const cal = calendars.find((c) => c.id === calendarId);
		return cal ? cal.name : `Calendar #${calendarId}`;
	}

	/**
	 * Check the active status of all calendars used by geofences.
	 * This provides a visual indicator of whether a calendar schedule is
	 * currently active (within its defined time windows).
	 */
	async function checkCalendarStatuses() {
		const calendarIds = new Set<number>();
		for (const gf of geofences) {
			if (gf.calendarId != null) {
				calendarIds.add(gf.calendarId);
			}
		}
		const newStatus: Record<number, boolean> = {};
		const checks = Array.from(calendarIds).map(async (id) => {
			try {
				const result = await api.checkCalendar(id);
				newStatus[id] = result.active;
			} catch {
				// Calendar may have been deleted; treat as inactive
				newStatus[id] = false;
			}
		});
		await Promise.all(checks);
		calendarActiveStatus = newStatus;
	}

	/**
	 * Convert a Leaflet drawn layer into a GeoJSON geometry object.
	 * Handles Circle (converted to a 32-point polygon approximation),
	 * Polygon, and Rectangle.
	 */
	function layerToGeoJSON(layer: any): any {
		if (layer instanceof L.Circle) {
			// Approximate circle as a polygon with 32 vertices
			const center = layer.getLatLng();
			const radius = layer.getRadius();
			const points: [number, number][] = [];
			for (let i = 0; i < 32; i++) {
				const angle = (i / 32) * 2 * Math.PI;
				// Approximate meter offset to degrees
				const dLat = (radius * Math.cos(angle)) / 111320;
				const dLng = (radius * Math.sin(angle)) / (111320 * Math.cos((center.lat * Math.PI) / 180));
				points.push([center.lng + dLng, center.lat + dLat]);
			}
			points.push(points[0]); // Close the ring
			return {
				type: 'Polygon',
				coordinates: [points]
			};
		} else if (layer instanceof L.Polygon || layer instanceof L.Rectangle) {
			const coords = layer.getLatLngs()[0].map((ll: any) => [ll.lng, ll.lat]);
			coords.push(coords[0]); // Close the polygon
			return {
				type: 'Polygon',
				coordinates: [coords]
			};
		}
		return null;
	}

	/**
	 * Parse the geometry JSON string from a backend geofence and add it to the
	 * map as a GeoJSON layer. Returns the Leaflet layer.
	 */
	function addGeofenceToMap(gf: Geofence): any {
		const geometry = JSON.parse(gf.geometry || '{}');
		const layer = L.geoJSON(geometry, { style: () => GEOFENCE_STYLE });
		const calName = getCalendarName(gf.calendarId);
		const calHtml = calName ? `<br><small style="color: var(--text-tertiary)">Schedule: ${calName}</small>` : '';
		layer.bindPopup(`<strong>${gf.name}</strong>${gf.description ? '<br>' + gf.description : ''}${calHtml}`);
		layer.addTo(map);
		return layer;
	}

	/**
	 * Load calendars from the backend for the calendar selector dropdown.
	 */
	async function loadCalendars() {
		try {
			const isAdmin = ($currentUser as Record<string, unknown> | null)?.administrator === true;
			calendars = await fetchCalendars(isAdmin);
		} catch {
			calendars = [];
		}
	}

	/**
	 * Load all geofences from the backend and render them on the map.
	 * If geofences exist, the map view is adjusted to fit all of them.
	 */
	async function loadGeofences() {
		try {
			const isAdmin = ($currentUser as Record<string, unknown> | null)?.administrator === true;
			const data: any[] = await fetchGeofences(isAdmin);
			geofences = data.map((gf) => {
				const layer = addGeofenceToMap(gf);
				return { ...gf, layer };
			});

			if (geofences.length > 0) {
				const group = L.featureGroup(geofences.map((g) => g.layer));
				map.fitBounds(group.getBounds().pad(0.2));
			}

			// Check calendar active statuses in the background
			checkCalendarStatuses();
		} catch (err: any) {
			console.error('Failed to load geofences:', err);
			error = 'Failed to load geofences';
		}
	}

	async function getInitialCenter(): Promise<{ center: [number, number]; zoom: number }> {
		// Priority 1: User's explicitly configured default location from settings
		try {
			const savedSettings = localStorage.getItem('motus_settings');
			if (savedSettings) {
				const s = JSON.parse(savedSettings);
				if (s.mapLocationSet && s.defaultMapLat != null && s.defaultMapLng != null) {
					return {
						center: [s.defaultMapLat, s.defaultMapLng],
						zoom: s.defaultMapZoom || DEFAULT_ZOOM
					};
				}
			}
		} catch {
			// Corrupted settings, continue to next method
		}

		// Priority 2: Center on device positions if available
		try {
			const positions = await api.getPositions();
			if (positions.length > 0) {
				const lats = positions.map((p: any) => p.latitude);
				const lngs = positions.map((p: any) => p.longitude);
				const avgLat = lats.reduce((a: number, b: number) => a + b, 0) / lats.length;
				const avgLng = lngs.reduce((a: number, b: number) => a + b, 0) / lngs.length;
				return { center: [avgLat, avgLng], zoom: DEFAULT_ZOOM };
			}
		} catch {
			// Ignore, try next method
		}

		// Priority 3: Browser geolocation
		try {
			const pos = await new Promise<GeolocationPosition>((resolve, reject) => {
				navigator.geolocation.getCurrentPosition(resolve, reject, { timeout: 5000 });
			});
			return {
				center: [pos.coords.latitude, pos.coords.longitude],
				zoom: DEFAULT_ZOOM
			};
		} catch {
			// Geolocation denied or unavailable
		}

		// Fallback: default location
		return { center: DEFAULT_CENTER, zoom: DEFAULT_ZOOM };
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
		if (!L || !map || !userLocation.position) return;
		const pos = userLocation.position;
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
			map.setView(latlng, Math.max(map.getZoom(), 15));
		}
	}

	function updateUserHeadingLayer() {
		if (!L || !map || !userLocation.position) return;
		const pos = userLocation.position;
		const heading = userLocation.heading;
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
		// Import Leaflet and leaflet-draw
		const leafletModule = await import('leaflet');
		L = leafletModule.default || leafletModule;
		await import('leaflet/dist/leaflet.css');

		// Import leaflet-draw - this extends the L object
		await import('leaflet-draw');
		await import('leaflet-draw/dist/leaflet.draw.css');

		const initialView = await getInitialCenter();
		const center = initialView.center;
		const zoom = initialView.zoom;

		map = L.map(mapContainer, {
			center,
			zoom,
			zoomControl: false
		});

		L.control.zoom({ position: 'topright' }).addTo(map);

		L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
			attribution: '&copy; OpenStreetMap contributors',
			maxZoom: 19
		}).addTo(map);

		drawnItems = new L.FeatureGroup().addTo(map);

		// Check if L.Control.Draw is available
		if (!L.Control.Draw) {
			console.error('Leaflet.draw not loaded properly');
			loading = false;
			return;
		}

		drawControl = new L.Control.Draw({
			position: 'topleft',
			draw: {
				polyline: false,
				marker: false,
				circlemarker: false,
				rectangle: {
					shapeOptions: GEOFENCE_STYLE
				},
				polygon: {
					allowIntersection: false,
					shapeOptions: GEOFENCE_STYLE
				},
				circle: {
					shapeOptions: GEOFENCE_STYLE
				}
			},
			edit: {
				featureGroup: drawnItems,
				remove: false // We handle deletion via sidebar
			}
		});

		map.addControl(drawControl);

		// When a shape is drawn, open the create modal
		map.on('draw:created', (e: any) => {
			pendingLayer = e.layer;
			newGeofenceName = '';
			newGeofenceCalendarId = null;
			showCreateModal = true;
		});

		// Load calendars first so calendar names are available for geofence popups
		await loadCalendars();

		// Load existing geofences from the backend
		await loadGeofences();

		loading = false;
		$refreshHandler = loadGeofences;
	});

	onDestroy(() => {
		$refreshHandler = null;
		userLocation.stop();
		if (map) map.remove();
	});

	/**
	 * Save a newly drawn geofence to the backend and add it to the list.
	 */
	async function saveGeofence() {
		if (!pendingLayer || !newGeofenceName.trim()) return;

		const geometry = layerToGeoJSON(pendingLayer);
		if (!geometry) {
			cancelCreate();
			return;
		}

		saving = true;
		try {
			const created = await api.createGeofence({
				name: newGeofenceName.trim(),
				description: '',
				geometry: JSON.stringify(geometry),
				calendarId: newGeofenceCalendarId
			});

			// Remove the temporary drawn layer and render from the persisted data
			// (This keeps our rendering consistent with what the backend stores.)
			const layer = addGeofenceToMap(created);
			geofences = [...geofences, { ...created, layer }];

			// Refresh calendar statuses if a calendar was assigned
			if (newGeofenceCalendarId != null) {
				checkCalendarStatuses();
			}
		} catch (err: any) {
			console.error('Failed to create geofence:', err);
			error = 'Failed to save geofence';
		}

		saving = false;
		pendingLayer = null;
		showCreateModal = false;
	}

	function cancelCreate() {
		pendingLayer = null;
		showCreateModal = false;
	}

	/**
	 * Open the edit modal for a geofence to change its name or calendar.
	 */
	function openEditModal(geofence: Geofence) {
		editingGeofence = geofence;
		editGeofenceName = geofence.name;
		editGeofenceCalendarId = geofence.calendarId ?? null;
		showEditModal = true;
	}

	/**
	 * Save edits to an existing geofence (name and calendar assignment).
	 */
	async function saveEdit() {
		if (!editingGeofence || !editGeofenceName.trim()) return;

		saving = true;
		try {
			const updated = await api.updateGeofence(editingGeofence.id, {
				name: editGeofenceName.trim(),
				calendarId: editGeofenceCalendarId
			});

			// Re-render the map layer with updated popup content
			if (editingGeofence.layer) {
				map.removeLayer(editingGeofence.layer);
			}
			const newLayer = addGeofenceToMap(updated);

			geofences = geofences.map((g) =>
				g.id === updated.id ? { ...updated, layer: newLayer } : g
			);

			// Refresh calendar statuses
			checkCalendarStatuses();

			showEditModal = false;
			editingGeofence = null;
		} catch (err: any) {
			console.error('Failed to update geofence:', err);
			error = 'Failed to update geofence';
		}
		saving = false;
	}

	function cancelEdit() {
		showEditModal = false;
		editingGeofence = null;
	}

	/**
	 * Focus the map on a specific geofence.
	 */
	function selectGeofence(geofence: Geofence) {
		selectedGeofence = geofence;
		if (geofence.layer) {
			const bounds = geofence.layer.getBounds();
			if (bounds && bounds.isValid()) {
				map.fitBounds(bounds.pad(0.3));
			}
			geofence.layer.eachLayer?.((l: any) => {
				if (l.openPopup) l.openPopup();
			});
		}
	}

	/**
	 * Delete a geofence from the backend and remove it from the map.
	 */
	async function removeGeofence(geofence: Geofence) {
		try {
			await api.deleteGeofence(geofence.id);

			if (geofence.layer) {
				map.removeLayer(geofence.layer);
			}

			geofences = geofences.filter((g) => g.id !== geofence.id);

			if (selectedGeofence?.id === geofence.id) {
				selectedGeofence = null;
			}
		} catch (err: any) {
			console.error('Failed to delete geofence:', err);
			error = 'Failed to delete geofence';
		}
	}

	/**
	 * Determine geofence shape type from its GeoJSON geometry for display in the sidebar.
	 */
	function getGeofenceType(geofence: Geofence): string {
		try {
			const geom = JSON.parse(geofence.geometry || '{}');
			return geom.type || 'polygon';
		} catch {
			return 'polygon';
		}
	}
</script>

<svelte:head>
	<title>Geofences - Motus</title>
</svelte:head>

<div class="geofences-page">
	<aside class="sidebar">
		<div class="sidebar-header">
			<h2 class="sidebar-title">Geofences</h2>
			<AllDevicesToggle on:change={loadGeofences} />
			<span class="fence-count">{geofences.length}</span>
		</div>

		<div class="sidebar-help">
			<p>Use the drawing tools on the map to create geofences. Draw polygons, rectangles, or circles.</p>
		</div>

		{#if error}
			<div class="error-banner">
				<span>{error}</span>
				<button class="error-dismiss" on:click={() => (error = '')}>&#x2715;</button>
			</div>
		{/if}

		<div class="fence-list">
			{#if loading}
				<div class="loading-state">
					<div class="spinner small"></div>
					<span>Loading geofences...</span>
				</div>
			{:else}
				{#each geofences as fence (fence.id)}
					<div
						class="fence-item"
						class:selected={selectedGeofence?.id === fence.id}
						class:other-user={fence.ownerName}
					>
						<button
							class="fence-info"
							on:click={() => selectGeofence(fence)}
						>
							<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="var(--accent-primary)" stroke-width="2">
								{#if getGeofenceType(fence) === 'Point'}
									<circle cx="12" cy="12" r="10"/>
								{:else}
									<polygon points="12,2 22,20 2,20"/>
								{/if}
							</svg>
							<div class="fence-details">
								<span class="fence-name">{fence.name}</span>
								<span class="fence-type">{getGeofenceType(fence)}</span>
								{#if fence.ownerName}
									<span class="owner-badge" title="Owned by {fence.ownerName}">{fence.ownerName}</span>
								{/if}
								{#if fence.calendarId != null}
									<span
										class="calendar-badge"
										class:active={calendarActiveStatus[fence.calendarId] === true}
										class:inactive={calendarActiveStatus[fence.calendarId] === false}
										title="This geofence only triggers events during the scheduled time windows defined in the calendar '{getCalendarName(fence.calendarId)}'."
									>
										<svg viewBox="0 0 24 24" width="11" height="11" fill="none" stroke="currentColor" stroke-width="2">
											<rect x="3" y="4" width="18" height="18" rx="2" ry="2"/>
											<line x1="16" y1="2" x2="16" y2="6"/>
											<line x1="8" y1="2" x2="8" y2="6"/>
											<line x1="3" y1="10" x2="21" y2="10"/>
										</svg>
										{getCalendarName(fence.calendarId)}
										{#if calendarActiveStatus[fence.calendarId] === true}
											<span class="status-dot active-dot" title="Calendar is currently active"></span>
										{:else if calendarActiveStatus[fence.calendarId] === false}
											<span class="status-dot inactive-dot" title="Calendar is currently inactive"></span>
										{/if}
									</span>
								{/if}
							</div>
						</button>
						<button
							class="fence-edit"
							on:click={() => openEditModal(fence)}
							aria-label="Edit {fence.name}"
							title="Edit geofence"
						>
							<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2">
								<path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>
								<path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/>
							</svg>
						</button>
						<button
							class="fence-delete"
							on:click={() => removeGeofence(fence)}
							aria-label="Delete {fence.name}"
						>
							&#x2715;
						</button>
					</div>
				{:else}
					<div class="no-fences">
						<p>No geofences defined</p>
						<p class="hint">Draw on the map to create one</p>
					</div>
				{/each}
			{/if}
		</div>
	</aside>

	<div class="map-container" bind:this={mapContainer}>
		{#if loading}
			<div class="map-loading">
				<div class="spinner"></div>
			</div>
		{/if}

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

<!-- Create Geofence Modal -->
<Modal bind:open={showCreateModal} title="New Geofence" on:close={cancelCreate}>
	<form on:submit|preventDefault={saveGeofence}>
		<div class="modal-form">
			<Input
				name="geofence-name"
				label="Geofence Name"
				placeholder="Office Zone"
				required
				bind:value={newGeofenceName}
			/>

			<div class="form-group">
				<label for="create-calendar" class="form-label">
					Calendar Schedule
					<span
						class="form-hint-icon"
						title="When a calendar is assigned, this geofence will only trigger events (enter/exit) during the calendar's scheduled time windows. Leave as 'None' for the geofence to be always active."
					>?</span>
				</label>
				<select
					id="create-calendar"
					class="select"
					bind:value={newGeofenceCalendarId}
				>
					<option value={null}>None (Always Active)</option>
					{#each calendars as cal (cal.id)}
						<option value={cal.id}>{cal.name}</option>
					{/each}
				</select>
				{#if calendars.length === 0}
					<span class="form-hint">No calendars defined. Create one in the Calendars page.</span>
				{/if}
			</div>
		</div>
	</form>

	<svelte:fragment slot="footer">
		<div class="modal-actions">
			<Button variant="secondary" on:click={cancelCreate}>Cancel</Button>
			<Button variant="primary" on:click={saveGeofence} disabled={!newGeofenceName.trim() || saving} loading={saving}>
				Save Geofence
			</Button>
		</div>
	</svelte:fragment>
</Modal>

<!-- Edit Geofence Modal -->
<Modal bind:open={showEditModal} title="Edit Geofence" on:close={cancelEdit}>
	<form on:submit|preventDefault={saveEdit}>
		<div class="modal-form">
			<Input
				name="edit-geofence-name"
				label="Geofence Name"
				placeholder="Office Zone"
				required
				bind:value={editGeofenceName}
			/>

			<div class="form-group">
				<label for="edit-calendar" class="form-label">
					Calendar Schedule
					<span
						class="form-hint-icon"
						title="When a calendar is assigned, this geofence will only trigger events (enter/exit) during the calendar's scheduled time windows. Choose 'None' to make the geofence always active."
					>?</span>
				</label>
				<select
					id="edit-calendar"
					class="select"
					bind:value={editGeofenceCalendarId}
				>
					<option value={null}>None (Always Active)</option>
					{#each calendars as cal (cal.id)}
						<option value={cal.id}>{cal.name}</option>
					{/each}
				</select>
				{#if calendars.length === 0}
					<span class="form-hint">No calendars defined. Create one in the Calendars page.</span>
				{/if}
			</div>
		</div>
	</form>

	<svelte:fragment slot="footer">
		<div class="modal-actions">
			<Button variant="secondary" on:click={cancelEdit}>Cancel</Button>
			<Button variant="primary" on:click={saveEdit} disabled={!editGeofenceName.trim() || saving} loading={saving}>
				Save Changes
			</Button>
		</div>
	</svelte:fragment>
</Modal>

<style>
	.geofences-page {
		display: flex;
		height: calc(100vh - 65px);
		overflow: hidden;
	}

	.sidebar {
		width: 300px;
		background-color: var(--bg-secondary);
		border-right: 1px solid var(--border-color);
		display: flex;
		flex-direction: column;
		overflow: hidden;
	}

	.sidebar-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: var(--space-4);
		border-bottom: 1px solid var(--border-color);
	}

	.sidebar-title {
		font-size: var(--text-lg);
		font-weight: var(--font-semibold);
		color: var(--text-primary);
	}

	.fence-count {
		font-size: var(--text-sm);
		color: var(--text-secondary);
		background-color: var(--bg-tertiary);
		padding: var(--space-1) var(--space-2);
		border-radius: var(--radius-full);
	}

	.sidebar-help {
		padding: var(--space-3) var(--space-4);
		border-bottom: 1px solid var(--border-color);
	}

	.sidebar-help p {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
		line-height: 1.5;
	}

	.error-banner {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: var(--space-2) var(--space-4);
		background-color: rgba(255, 68, 68, 0.15);
		color: var(--error);
		font-size: var(--text-sm);
		border-bottom: 1px solid var(--border-color);
	}

	.error-dismiss {
		background: none;
		border: none;
		color: var(--error);
		cursor: pointer;
		font-size: var(--text-base);
		padding: 0;
	}

	.fence-list {
		flex: 1;
		overflow-y: auto;
	}

	.loading-state {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		padding: var(--space-4);
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}

	.fence-item {
		display: flex;
		align-items: center;
		border-bottom: 1px solid var(--border-color);
		transition: background-color var(--transition-fast);
	}

	.fence-item:hover {
		background-color: var(--bg-hover);
	}

	.fence-item.selected {
		background-color: var(--bg-active);
	}

	.fence-info {
		flex: 1;
		display: flex;
		align-items: flex-start;
		gap: var(--space-3);
		padding: var(--space-3) var(--space-4);
		background: none;
		border: none;
		cursor: pointer;
		text-align: left;
	}

	.fence-info > svg {
		flex-shrink: 0;
		margin-top: 2px;
	}

	.fence-details {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.fence-name {
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		color: var(--text-primary);
	}

	.fence-type {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
		text-transform: capitalize;
	}

	.calendar-badge {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		margin-top: 3px;
		padding: 1px 6px;
		font-size: 10px;
		line-height: 1.4;
		border-radius: var(--radius-full);
		background-color: var(--bg-tertiary);
		color: var(--text-secondary);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		max-width: 100%;
		cursor: help;
	}

	.calendar-badge.active {
		background-color: rgba(52, 211, 153, 0.15);
		color: #34d399;
	}

	.calendar-badge.inactive {
		background-color: rgba(251, 191, 36, 0.15);
		color: #fbbf24;
	}

	.status-dot {
		display: inline-block;
		width: 6px;
		height: 6px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.active-dot {
		background-color: #34d399;
	}

	.inactive-dot {
		background-color: #fbbf24;
	}

	.fence-edit {
		padding: var(--space-2);
		background: none;
		border: none;
		color: var(--text-tertiary);
		cursor: pointer;
		transition: color var(--transition-fast);
		display: flex;
		align-items: center;
	}

	.fence-edit:hover {
		color: var(--accent-primary);
	}

	.fence-delete {
		padding: var(--space-2) var(--space-3);
		background: none;
		border: none;
		color: var(--text-tertiary);
		cursor: pointer;
		font-size: var(--text-base);
		transition: color var(--transition-fast);
	}

	.fence-delete:hover {
		color: var(--error);
	}

	.no-fences {
		padding: var(--space-8) var(--space-4);
		text-align: center;
	}

	.no-fences p {
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}

	.no-fences .hint {
		color: var(--text-tertiary);
		font-size: var(--text-xs);
		margin-top: var(--space-2);
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

	.spinner.small {
		width: 20px;
		height: 20px;
		border-width: 2px;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	/* Locate Me button — sits in top-right area, below Leaflet draw controls */
	.locate-me-btn {
		position: absolute;
		top: var(--space-3);
		right: calc(var(--space-3) + 36px + var(--space-2)); /* left of zoom control */
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
		top: calc(var(--space-3) + 36px + var(--space-2));
		right: calc(var(--space-3) + 36px + var(--space-2));
		z-index: 500;
		max-width: 200px;
		padding: var(--space-2) var(--space-3);
		background-color: rgba(255, 68, 68, 0.15);
		border: 1px solid rgba(255, 68, 68, 0.4);
		border-radius: var(--radius-md);
		font-size: var(--text-xs);
		color: var(--error);
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

	/* Override Leaflet Draw toolbar for dark theme */
	.map-container :global(.leaflet-draw-toolbar a) {
		background-color: var(--bg-secondary);
		border-color: var(--border-color);
	}

	.map-container :global(.leaflet-popup-content-wrapper) {
		background-color: var(--bg-secondary);
		color: var(--text-primary);
		border-radius: var(--radius-lg);
	}

	.map-container :global(.leaflet-popup-tip) {
		background-color: var(--bg-secondary);
	}

	/* Modal form styles */
	.modal-form {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
	}

	.form-group {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	.form-label {
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		color: var(--text-primary);
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}

	.form-hint-icon {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 16px;
		height: 16px;
		border-radius: 50%;
		background-color: var(--bg-tertiary);
		color: var(--text-tertiary);
		font-size: 10px;
		font-weight: var(--font-semibold);
		cursor: help;
		flex-shrink: 0;
	}

	.form-hint {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
		font-style: italic;
	}

	.select {
		padding: var(--space-3) var(--space-4);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-base);
	}

	.select:hover:not(:disabled) {
		border-color: var(--border-hover);
	}

	.select:focus {
		outline: none;
		border-color: var(--accent-primary);
		box-shadow: 0 0 0 3px rgba(0, 212, 255, 0.1);
	}

	.modal-actions {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-3);
	}

	@media (max-width: 768px) {
		.geofences-page {
			flex-direction: column;
		}

		.sidebar {
			width: 100%;
			max-height: 35vh;
			border-right: none;
			border-bottom: 1px solid var(--border-color);
		}
	}

	.fence-item.other-user {
		border-left: 3px solid var(--color-warning, #f59e0b);
		background: color-mix(in srgb, var(--color-warning, #f59e0b) 4%, transparent);
	}

	.owner-badge {
		display: inline-block;
		font-size: 0.65rem;
		padding: 0.1rem 0.35rem;
		border-radius: 0.25rem;
		background: color-mix(in srgb, var(--color-warning, #f59e0b) 15%, transparent);
		color: var(--text-secondary, #666);
		line-height: 1.2;
		white-space: nowrap;
	}
</style>

<script lang="ts">
	import { onMount } from 'svelte';
	import { currentUser } from '$lib/stores/auth';
	import { settings } from '$lib/stores/settings';
	import type { UserSettings } from '$lib/stores/settings';
	import { api } from '$lib/api/client';
	import Input from '$lib/components/Input.svelte';
	import Button from '$lib/components/Button.svelte';
	import { MAP_OVERLAYS } from '$lib/utils/map-overlays';
	import ApiKeyManager from '$lib/components/ApiKeyManager.svelte';
	import SessionManager from '$lib/components/SessionManager.svelte';

	let loading = true;
	let saving = false;
	let message = '';
	let messageType: 'success' | 'error' = 'success';

	// Profile fields
	let name = '';
	let email = '';

	// Password fields
	let currentPassword = '';
	let newPassword = '';
	let confirmPassword = '';
	let passwordError = '';

	// Settings form (bound to local state, saved on submit)
	let dateFormat: UserSettings['dateFormat'] = 'iso';
	let timezone = 'local';
	let units: UserSettings['units'] = 'metric';
	let defaultMapLat = 49.79;
	let defaultMapLng = 9.95;
	let defaultMapZoom = 13;
	let mapOverlay = 'none';
	let mapOverlayOpacity = 80;

	onMount(() => {
		// Load profile
		if ($currentUser) {
			name = ($currentUser.name as string) || '';
			email = ($currentUser.email as string) || '';
		}

		// Load current settings
		const s = $settings;
		dateFormat = s.dateFormat;
		timezone = s.timezone;
		units = s.units;
		defaultMapLat = s.defaultMapLat;
		defaultMapLng = s.defaultMapLng;
		defaultMapZoom = s.defaultMapZoom;
		mapOverlay = s.mapOverlay;
		mapOverlayOpacity = s.mapOverlayOpacity;

		loading = false;
	});

	function validatePassword(): boolean {
		passwordError = '';
		if (newPassword && !currentPassword) {
			passwordError = 'Current password is required to set a new password';
			return false;
		}
		if (newPassword && newPassword.length < 6) {
			passwordError = 'New password must be at least 6 characters';
			return false;
		}
		if (newPassword && newPassword !== confirmPassword) {
			passwordError = 'Passwords do not match';
			return false;
		}
		return true;
	}

	async function saveSettings() {
		if (!validatePassword()) return;

		saving = true;
		message = '';

		try {
			// Save display preferences to settings store (persisted to localStorage)
			settings.set({
				dateFormat,
				timezone,
				units,
				defaultMapLat,
				defaultMapLng,
				defaultMapZoom,
				mapLocationSet: true,
				mapOverlay,
				mapOverlayOpacity,
				showAllDevices: $settings.showAllDevices,
			});

			message = 'Settings saved successfully';
			messageType = 'success';

			// Clear password fields
			currentPassword = '';
			newPassword = '';
			confirmPassword = '';

			// Auto-clear the message after a few seconds
			setTimeout(() => {
				message = '';
			}, 3000);
		} catch (error: any) {
			message = `Failed to save: ${error.message || 'Unknown error'}`;
			messageType = 'error';
		} finally {
			saving = false;
		}
	}

	function getCurrentLocation() {
		if (!navigator.geolocation) {
			message = 'Geolocation is not supported by your browser';
			messageType = 'error';
			return;
		}

		navigator.geolocation.getCurrentPosition(
			(position) => {
				defaultMapLat = Math.round(position.coords.latitude * 10000) / 10000;
				defaultMapLng = Math.round(position.coords.longitude * 10000) / 10000;
			},
			() => {
				message = 'Could not get your location. Please check browser permissions.';
				messageType = 'error';
			},
			{ timeout: 10000 }
		);
	}

	function resetDefaults() {
		settings.reset();
		const s = $settings;
		dateFormat = s.dateFormat;
		timezone = s.timezone;
		units = s.units;
		defaultMapLat = s.defaultMapLat;
		defaultMapLng = s.defaultMapLng;
		defaultMapZoom = s.defaultMapZoom;
		mapOverlay = s.mapOverlay;
		mapOverlayOpacity = s.mapOverlayOpacity;
		message = 'Settings reset to defaults';
		messageType = 'success';
		setTimeout(() => {
			message = '';
		}, 3000);
	}
</script>

<svelte:head>
	<title>Settings - Motus</title>
</svelte:head>

<div class="settings-page">
	<div class="container">
		<h1 class="page-title">Settings</h1>

		{#if loading}
			<p class="loading-text">Loading settings...</p>
		{:else}
			<form on:submit|preventDefault={saveSettings} class="settings-form">
				<!-- Profile Section -->
				<section class="settings-section">
					<h2 class="section-title">Profile</h2>

					<div class="form-row">
						<Input
							label="Name"
							name="name"
							bind:value={name}
							placeholder="Your display name"
						/>
					</div>

					<div class="form-row">
						<Input
							label="Email"
							name="email"
							type="email"
							bind:value={email}
							disabled
						/>
					</div>
				</section>

				<!-- Password Section -->
				<section class="settings-section">
					<h2 class="section-title">Change Password</h2>

					<div class="form-row">
						<Input
							label="Current Password"
							name="currentPassword"
							type="password"
							bind:value={currentPassword}
							placeholder="Enter current password"
						/>
					</div>

					<div class="form-row">
						<Input
							label="New Password"
							name="newPassword"
							type="password"
							bind:value={newPassword}
							placeholder="Enter new password"
						/>
					</div>

					<div class="form-row">
						<Input
							label="Confirm New Password"
							name="confirmPassword"
							type="password"
							bind:value={confirmPassword}
							placeholder="Confirm new password"
							error={passwordError}
						/>
					</div>
				</section>

				<!-- Display Preferences -->
				<section class="settings-section">
					<h2 class="section-title">Display Preferences</h2>

					<div class="form-row">
						<div class="form-group">
							<label for="dateFormat" class="form-label">Date Format</label>
							<select id="dateFormat" bind:value={dateFormat} class="select">
								<option value="iso">ISO 8601 (2026-01-11 10:02:28)</option>
								<option value="locale">Locale (1/11/2026, 10:02:28 AM)</option>
								<option value="relative">Relative (2 hours ago)</option>
							</select>
						</div>
					</div>

					<div class="form-row">
						<div class="form-group">
							<label for="timezone" class="form-label">Timezone</label>
							<select id="timezone" bind:value={timezone} class="select">
								<option value="local">Local Time</option>
								<option value="UTC">UTC</option>
								<option value="Europe/Berlin">Europe/Berlin</option>
								<option value="Europe/London">Europe/London</option>
								<option value="America/New_York">America/New York</option>
								<option value="America/Los_Angeles">America/Los Angeles</option>
								<option value="Asia/Tokyo">Asia/Tokyo</option>
							</select>
						</div>
					</div>

					<div class="form-row">
						<div class="form-group">
							<label for="units" class="form-label">Units</label>
							<select id="units" bind:value={units} class="select">
								<option value="metric">Metric (km, km/h)</option>
								<option value="imperial">Imperial (miles, mph)</option>
							</select>
						</div>
					</div>
				</section>

				<!-- Map Defaults -->
				<section class="settings-section">
					<h2 class="section-title">Default Map Location</h2>
					<p class="section-description">This location is used when no device positions are available.</p>

					<div class="map-coords">
						<div class="form-group">
							<label for="mapLat" class="form-label">Latitude</label>
							<input
								id="mapLat"
								type="number"
								step="0.0001"
								class="input"
								bind:value={defaultMapLat}
							/>
						</div>

						<div class="form-group">
							<label for="mapLng" class="form-label">Longitude</label>
							<input
								id="mapLng"
								type="number"
								step="0.0001"
								class="input"
								bind:value={defaultMapLng}
							/>
						</div>

						<div class="form-group">
							<label for="mapZoom" class="form-label">Zoom</label>
							<input
								id="mapZoom"
								type="number"
								min="1"
								max="18"
								class="input"
								bind:value={defaultMapZoom}
							/>
						</div>
					</div>

					<div class="location-action">
						<Button type="button" variant="secondary" size="sm" on:click={getCurrentLocation}>
							Use My Location
						</Button>
					</div>
				</section>

				<!-- Map Overlay -->
				<section class="settings-section">
					<h2 class="section-title">Map Overlay</h2>
					<p class="section-description">Choose an overlay to display on the map. This can also be changed from the map page.</p>

					<div class="form-row">
						<div class="form-group">
							<label for="mapOverlay" class="form-label">Overlay Layer</label>
							<select id="mapOverlay" bind:value={mapOverlay} class="select">
								{#each MAP_OVERLAYS as overlay (overlay.id)}
									<option value={overlay.id}>{overlay.name}</option>
								{/each}
							</select>
						</div>
					</div>

					{#if mapOverlay !== 'none'}
						<div class="form-row">
							<div class="form-group">
								<label for="mapOverlayOpacity" class="form-label">
									Overlay Opacity: {mapOverlayOpacity}%
								</label>
								<input
									id="mapOverlayOpacity"
									type="range"
									min="10"
									max="100"
									step="5"
									class="opacity-range"
									bind:value={mapOverlayOpacity}
								/>
							</div>
						</div>
					{/if}
				</section>

				<!-- Message -->
				{#if message}
					<div class="message" class:error={messageType === 'error'}>
						{message}
					</div>
				{/if}

				<!-- Actions -->
				<div class="form-actions">
					<Button type="button" variant="secondary" on:click={resetDefaults}>
						Reset to Defaults
					</Button>
					<Button type="submit" loading={saving}>
						{saving ? 'Saving...' : 'Save Settings'}
					</Button>
				</div>
			</form>

			<!-- API Keys Management Section -->
			<ApiKeyManager />

			<!-- Active Sessions Section -->
			<SessionManager />
		{/if}
	</div>
</div>

<style>
	.settings-page {
		padding: var(--space-6) 0;
	}

	.container {
		max-width: 800px;
		margin: 0 auto;
		padding: 0 var(--space-4);
	}

	.page-title {
		font-size: var(--text-3xl);
		font-weight: var(--font-bold);
		color: var(--text-primary);
		margin-bottom: var(--space-6);
	}

	.loading-text {
		color: var(--text-secondary);
		font-size: var(--text-lg);
	}

	.settings-form {
		display: flex;
		flex-direction: column;
		gap: var(--space-6);
	}

	.settings-section {
		padding: var(--space-6);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
	}

	.section-title {
		font-size: var(--text-xl);
		font-weight: var(--font-semibold);
		color: var(--text-primary);
		margin-bottom: var(--space-4);
	}

	.section-description {
		font-size: var(--text-sm);
		color: var(--text-tertiary);
		margin-bottom: var(--space-4);
		margin-top: calc(-1 * var(--space-2));
	}

	.form-row {
		margin-bottom: var(--space-4);
	}

	.form-row:last-child {
		margin-bottom: 0;
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
	}

	.select {
		width: 100%;
		padding: var(--space-3) var(--space-4);
		background-color: var(--bg-primary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-base);
		transition: border-color var(--transition-fast);
	}

	.select:hover {
		border-color: var(--border-hover);
	}

	.select:focus {
		outline: none;
		border-color: var(--accent-primary);
		box-shadow: 0 0 0 3px rgba(0, 212, 255, 0.1);
	}

	.input {
		width: 100%;
		padding: var(--space-3) var(--space-4);
		background-color: var(--bg-primary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-base);
		transition: border-color var(--transition-fast);
		box-sizing: border-box;
	}

	.input:hover {
		border-color: var(--border-hover);
	}

	.input:focus {
		outline: none;
		border-color: var(--accent-primary);
		box-shadow: 0 0 0 3px rgba(0, 212, 255, 0.1);
	}

	.opacity-range {
		width: 100%;
		height: 6px;
		-webkit-appearance: none;
		appearance: none;
		background: var(--bg-tertiary);
		border-radius: 3px;
		outline: none;
		cursor: pointer;
		margin-top: var(--space-1);
	}

	.opacity-range::-webkit-slider-thumb {
		-webkit-appearance: none;
		appearance: none;
		width: 18px;
		height: 18px;
		border-radius: 50%;
		background: var(--accent-primary);
		cursor: pointer;
		border: 2px solid var(--bg-secondary);
		box-shadow: var(--shadow-sm);
	}

	.opacity-range::-moz-range-thumb {
		width: 18px;
		height: 18px;
		border-radius: 50%;
		background: var(--accent-primary);
		cursor: pointer;
		border: 2px solid var(--bg-secondary);
		box-shadow: var(--shadow-sm);
	}

	.map-coords {
		display: grid;
		grid-template-columns: 1fr 1fr 100px;
		gap: var(--space-3);
		margin-bottom: var(--space-3);
	}

	.location-action {
		display: flex;
	}

	.message {
		padding: var(--space-4);
		background-color: rgba(0, 255, 136, 0.1);
		color: var(--success);
		border: 1px solid var(--success);
		border-radius: var(--radius-md);
	}

	.message.error {
		background-color: rgba(255, 68, 68, 0.1);
		color: var(--error);
		border-color: var(--error);
	}

	.form-actions {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-3);
	}

	@media (max-width: 768px) {
		.map-coords {
			grid-template-columns: 1fr 1fr;
		}

		.map-coords .form-group:last-child {
			grid-column: span 2;
		}
	}

	@media (max-width: 480px) {
		.map-coords {
			grid-template-columns: 1fr;
		}

		.map-coords .form-group:last-child {
			grid-column: span 1;
		}

		.form-actions {
			flex-direction: column;
		}
	}
</style>

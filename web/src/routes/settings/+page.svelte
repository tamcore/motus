<script lang="ts">
	import { onMount } from 'svelte';
	import { currentUser } from '$lib/stores/auth';
	import { settings } from '$lib/stores/settings';
	import { api } from '$lib/api/client';
	import Input from '$lib/components/Input.svelte';
	import Button from '$lib/components/Button.svelte';
	import { MAP_OVERLAYS } from '$lib/utils/map-overlays';
	import ApiKeyManager from '$lib/components/ApiKeyManager.svelte';
	import PasskeyManager from '$lib/components/PasskeyManager.svelte';
	import SessionManager from '$lib/components/SessionManager.svelte';

	let saving = false;
	let message = '';
	let messageType: 'success' | 'error' = 'success';

	// Profile fields
	let name = '';
	let email = '';
	let originalName = '';
	let originalEmail = '';

	// Password fields
	let currentPassword = '';
	let newPassword = '';
	let confirmPassword = '';
	let passwordError = '';

	// Bound to local state, saved on submit
	let form = { ...$settings };

	onMount(() => {
		if ($currentUser) {
			name = ($currentUser.name as string) || '';
			email = ($currentUser.email as string) || '';
			originalName = name;
			originalEmail = email;
		}
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
			const profileChanged = name !== originalName || email !== originalEmail;
			const passwordChanging = !!newPassword;

			if (profileChanged || passwordChanging) {
				const updated = await api.updateProfile({
					name: name || undefined,
					email: email || undefined,
					currentPassword: currentPassword || undefined,
					password: newPassword || undefined,
				});
				currentUser.set(updated);
				originalName = updated.name;
				originalEmail = updated.email;
			}

			// Save display preferences to settings store (persisted to localStorage)
			settings.set({ ...form, mapLocationSet: true, showAllDevices: $settings.showAllDevices });

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
		} catch (error: unknown) {
			message = `Failed to save: ${error instanceof Error ? error.message : 'Unknown error'}`;
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
				form.defaultMapLat = Math.round(position.coords.latitude * 10000) / 10000;
				form.defaultMapLng = Math.round(position.coords.longitude * 10000) / 10000;
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
		form = { ...$settings };
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
		<h1 class="page-title mb-6">Settings</h1>

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
						<select id="dateFormat" bind:value={form.dateFormat} class="select">
							<option value="iso">ISO 8601 (2026-01-11 10:02:28)</option>
							<option value="locale">Locale (1/11/2026, 10:02:28 AM)</option>
							<option value="relative">Relative (2 hours ago)</option>
						</select>
					</div>
				</div>

				<div class="form-row">
					<div class="form-group">
						<label for="timezone" class="form-label">Timezone</label>
						<select id="timezone" bind:value={form.timezone} class="select">
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
						<select id="units" bind:value={form.units} class="select">
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
							bind:value={form.defaultMapLat}
						/>
					</div>

					<div class="form-group">
						<label for="mapLng" class="form-label">Longitude</label>
						<input
							id="mapLng"
							type="number"
							step="0.0001"
							class="input"
							bind:value={form.defaultMapLng}
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
							bind:value={form.defaultMapZoom}
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
						<select id="mapOverlay" bind:value={form.mapOverlay} class="select">
							{#each MAP_OVERLAYS as overlay (overlay.id)}
								<option value={overlay.id}>{overlay.name}</option>
							{/each}
						</select>
					</div>
				</div>

				{#if form.mapOverlay !== 'none'}
					<div class="form-row">
						<div class="form-group">
							<label for="mapOverlayOpacity" class="form-label">
								Overlay Opacity: {form.mapOverlayOpacity}%
							</label>
							<input
								id="mapOverlayOpacity"
								type="range"
								min="10"
								max="100"
								step="5"
								class="opacity-range"
								bind:value={form.mapOverlayOpacity}
							/>
						</div>
					</div>
				{/if}
			</section>

			<!-- Message -->
			{#if message}
				<div class="save-message" class:error={messageType === 'error'}>
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

		<!-- Passkeys Management Section -->
		<PasskeyManager />

		<!-- Active Sessions Section -->
		<SessionManager />
	</div>
</div>

<style>
	.settings-page {
		padding: var(--space-6) 0;
	}

	.container {
		max-width: 800px;
	}

	/* Shared with ApiKeyManager, PasskeyManager and SessionManager. */
	:global {
		.settings-page .settings-section {
			padding: var(--space-6);
			background-color: var(--bg-secondary);
			border: 1px solid var(--border-color);
			border-radius: var(--radius-lg);
		}

		.settings-page .api-keys-section,
		.settings-page .passkeys-section,
		.settings-page .sessions-section {
			margin-top: var(--space-6);
		}

		.settings-page .section-header {
			display: flex;
			justify-content: space-between;
			align-items: flex-start;
			gap: var(--space-4);
			margin-bottom: var(--space-4);
		}

		.settings-page .section-title {
			font-size: var(--text-xl);
			font-weight: var(--font-semibold);
			color: var(--text-primary);
			margin-bottom: var(--space-1);
		}

		.settings-page .section-description {
			font-size: var(--text-sm);
			color: var(--text-tertiary);
		}

		.settings-page .loading-text {
			color: var(--text-secondary);
			font-size: var(--text-sm);
		}

		.settings-page .empty-state {
			text-align: center;
			padding: var(--space-8) var(--space-4);
		}

		.settings-page .empty-title {
			font-size: var(--text-lg);
			font-weight: var(--font-semibold);
			color: var(--text-primary);
			margin-bottom: var(--space-2);
		}

		.settings-page .empty-description {
			font-size: var(--text-sm);
			color: var(--text-tertiary);
			max-width: 400px;
			margin: 0 auto;
			line-height: 1.5;
		}

		.settings-page .keys-list,
		.settings-page .sessions-list {
			display: flex;
			flex-direction: column;
			gap: var(--space-3);
		}

		.settings-page .create-form {
			display: flex;
			flex-direction: column;
			gap: var(--space-4);
		}

		.settings-page .key-card,
		.settings-page .session-card {
			display: flex;
			justify-content: space-between;
			align-items: center;
			gap: var(--space-4);
			padding: var(--space-4);
			background-color: var(--bg-primary);
			border: 1px solid var(--border-color);
			border-radius: var(--radius-md);
			transition: border-color var(--transition-fast);
		}

		.settings-page .key-card:hover,
		.settings-page .session-card:hover {
			border-color: var(--border-hover);
		}

		.settings-page .key-info,
		.settings-page .session-info {
			flex: 1;
			min-width: 0;
		}

		.settings-page .key-header,
		.settings-page .session-header {
			display: flex;
			align-items: center;
			gap: var(--space-3);
			margin-bottom: var(--space-2);
		}

		.settings-page .key-name {
			font-weight: var(--font-semibold);
			color: var(--text-primary);
			font-size: var(--text-base);
		}

		.settings-page .key-meta,
		.settings-page .session-meta {
			display: flex;
			align-items: center;
			gap: var(--space-2);
			font-size: var(--text-sm);
			color: var(--text-tertiary);
			flex-wrap: wrap;
		}

		.settings-page .meta-separator {
			color: var(--border-color);
		}

		.settings-page .key-actions,
		.settings-page .session-actions {
			flex-shrink: 0;
		}

		.settings-page .select {
			width: 100%;
			background-color: var(--bg-primary);
			transition: border-color var(--transition-fast);
		}

		.settings-page .select:hover:not(:focus) {
			border-color: var(--border-hover);
		}

		.settings-page .form-error {
			padding: var(--space-3) var(--space-4);
		}

		@media (max-width: 768px) {
			.settings-page .api-keys-section .section-header,
			.settings-page .passkeys-section .section-header,
			.settings-page .key-card,
			.settings-page .session-card {
				flex-direction: column;
				align-items: stretch;
			}

			.settings-page .key-actions,
			.settings-page .session-actions {
				display: flex;
				justify-content: flex-end;
			}

			.settings-page .key-meta,
			.settings-page .session-meta {
				flex-direction: column;
				align-items: flex-start;
				gap: var(--space-1);
			}

			.settings-page .meta-separator {
				display: none;
			}
		}
	}

	.settings-form {
		display: flex;
		flex-direction: column;
		gap: var(--space-6);
	}

	.section-title {
		margin-bottom: var(--space-4);
	}

	.section-description {
		margin-bottom: var(--space-4);
		margin-top: calc(-1 * var(--space-2));
	}

	.form-row {
		margin-bottom: var(--space-4);
	}

	.form-row:last-child {
		margin-bottom: 0;
	}

	.input {
		width: 100%;
		background-color: var(--bg-primary);
	}

	.opacity-range {
		width: 100%;
		margin-top: var(--space-1);
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

	.save-message {
		padding: var(--space-4);
		background-color: rgba(0, 255, 136, 0.1);
		color: var(--success);
		border: 1px solid var(--success);
		border-radius: var(--radius-md);
	}

	.save-message.error {
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

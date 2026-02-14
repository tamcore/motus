<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { slide } from 'svelte/transition';
	import { api, fetchDevices } from '$lib/api/client';
	import { currentUser } from '$lib/stores/auth';
	import { refreshHandler } from '$lib/stores/refresh';
	import { mileageToDisplay, mileageFromDisplay, formatMileage, formatRelative } from '$lib/utils/formatting';
	import { settings } from '$lib/stores/settings';
	import AllDevicesToggle from '$lib/components/AllDevicesToggle.svelte';
	import Button from '$lib/components/Button.svelte';
	import Input from '$lib/components/Input.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import ShareModal from '$lib/components/ShareModal.svelte';
	import StatusIndicator from '$lib/components/StatusIndicator.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';

	interface Device {
		id: number;
		name: string;
		uniqueId: string;
		status: string;
		phone?: string;
		model?: string;
		category?: string;
		disabled?: boolean;
		lastUpdate?: string;
		ownerName?: string;
		mileage?: number | null;
	}

	let loading = true;
	let devices: Device[] = [];
	let searchQuery = '';
	let showModal = false;
	let showDeleteConfirm = false;
	let showShareModal = false;
	let sharingDevice: Device | null = null;
	let editingDevice: Device | null = null;
	let saving = false;
	let error = '';
	let expandedIds: Set<number> = new Set();

	// GPX import state
	let gpxFileInput: HTMLInputElement;
	let gpxTargetDeviceId: number | null = null;
	let gpxImportingIds: Set<number> = new Set();
	let gpxToast: { deviceId: number; msg: string; ok: boolean } | null = null;
	let gpxToastTimer: ReturnType<typeof setTimeout> | null = null;

	// Command modal state
	let showCommandModal = false;
	let commandDevice: Device | null = null;
	let commandType = 'rebootDevice';
	let commandText = '';
	let commandFrequency = '';
	let commandSosNumber = '';
	let commandSending = false;
	let commandError = '';
	let commandHistory: import('$lib/types/api').Command[] = [];
	let commandResult: { msg: string; ok: boolean } | null = null;
	let commandResultTimer: ReturnType<typeof setTimeout> | null = null;
	let commandSpeed = '';

	const commandTypeLabels: Record<string, string> = {
		rebootDevice: 'Reboot Device',
		positionPeriodic: 'Set Reporting Interval',
		positionSingle: 'Request Position',
		sosNumber: 'Set SOS Number',
		custom: 'Custom (raw text)',
		setSpeedAlarm: 'Set Speed Alarm',
		factoryReset: 'Factory Reset'
	};

	async function openCommandModal(device: Device) {
		commandDevice = device;
		commandType = 'rebootDevice';
		commandText = '';
		commandFrequency = '';
		commandSosNumber = '';
		commandSpeed = '';
		commandError = '';
		commandResult = null;
		showCommandModal = true;
		commandHistory = [];
		try {
			commandHistory = await api.listCommands(device.id, 10);
		} catch {
			// history is non-critical
		}
	}

	async function handleSendCommand() {
		if (!commandDevice) return;
		commandError = '';
		commandSending = true;

		const attributes: Record<string, unknown> = {};
		if (commandType === 'positionPeriodic') {
			const freq = parseInt(commandFrequency, 10);
			if (!freq || freq <= 0) {
				commandError = 'Interval must be a positive number';
				commandSending = false;
				return;
			}
			attributes.frequency = freq;
		} else if (commandType === 'sosNumber') {
			if (!commandSosNumber.trim()) {
				commandError = 'SOS number is required';
				commandSending = false;
				return;
			}
			attributes.phoneNumber = commandSosNumber.trim();
		} else if (commandType === 'setSpeedAlarm') {
			const speed = parseInt(commandSpeed, 10);
			if (isNaN(speed) || speed < 0) {
				commandError = 'Speed must be 0 or a positive number';
				commandSending = false;
				return;
			}
			attributes.speed = speed;
		} else if (commandType === 'custom') {
			if (!commandText.trim()) {
				commandError = 'Command text is required';
				commandSending = false;
				return;
			}
			attributes.text = commandText.trim();
		}

		try {
			await api.sendCommand({ deviceId: commandDevice.id, type: commandType, attributes });

			// Poll for result up to 5s
			let resultFound = false;
			for (let i = 0; i < 5; i++) {
				await new Promise((r) => setTimeout(r, 1000));
				const updated = await api.listCommands(commandDevice.id, 10);
				commandHistory = updated;
				const newest = updated[0];
				if (newest && newest.result) {
					if (commandResultTimer) clearTimeout(commandResultTimer);
					commandResult = { msg: newest.result, ok: true };
					commandResultTimer = setTimeout(() => { commandResult = null; }, 8000);
					resultFound = true;
					break;
				}
			}
			if (!resultFound) {
				commandHistory = await api.listCommands(commandDevice.id, 10);
			}
		} catch (e: unknown) {
			const msg = e instanceof Error ? e.message : 'Failed to send command';
			commandError = msg;
		} finally {
			commandSending = false;
		}
	}

	function openShareModal(device: Device) {
		sharingDevice = device;
		showShareModal = true;
	}

	// Form fields
	let formName = '';
	let formUniqueId = '';
	let formPhone = '';
	let formModel = '';
	let formCategory = '';
	let formMileage = '';

	$: filtered = devices.filter(
		(d) =>
			d.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
			d.uniqueId.toLowerCase().includes(searchQuery.toLowerCase())
	);

	onMount(async () => {
		await loadDevices();
		$refreshHandler = loadDevices;
	});

	onDestroy(() => { $refreshHandler = null; });

	async function loadDevices() {
		loading = true;
		try {
			const isAdmin = ($currentUser as Record<string, unknown> | null)?.administrator === true;
			devices = (await fetchDevices(isAdmin)) as Device[];
		} catch {
			console.error('Failed to load devices');
		} finally {
			loading = false;
		}
	}

	function toggleDevice(deviceId: number) {
		const next = new Set(expandedIds);
		if (next.has(deviceId)) {
			next.delete(deviceId);
		} else {
			next.add(deviceId);
		}
		expandedIds = next;
	}

	function handleRowKeydown(event: KeyboardEvent, deviceId: number) {
		if (event.key === 'Enter' || event.key === ' ') {
			event.preventDefault();
			toggleDevice(deviceId);
		}
	}

	function openCreateModal() {
		editingDevice = null;
		formName = '';
		formUniqueId = '';
		formPhone = '';
		formModel = '';
		formCategory = '';
		formMileage = '';
		error = '';
		showModal = true;
	}

	function openEditModal(device: Device) {
		editingDevice = device;
		formName = device.name;
		formUniqueId = device.uniqueId;
		formPhone = device.phone || '';
		formModel = device.model || '';
		formCategory = device.category || '';
		formMileage = device.mileage != null ? Math.round(mileageToDisplay(device.mileage)).toString() : '';
		error = '';
		showModal = true;
	}

	function openDeleteConfirm(device: Device) {
		editingDevice = device;
		showDeleteConfirm = true;
	}

	function openGPXImport(device: Device) {
		gpxTargetDeviceId = device.id;
		gpxFileInput.click();
	}

	async function handleGPXFile(event: Event) {
		const input = event.target as HTMLInputElement;
		const file = input.files?.[0];
		if (!file || gpxTargetDeviceId === null) return;
		const deviceId = gpxTargetDeviceId;
		input.value = '';

		gpxImportingIds = new Set([...gpxImportingIds, deviceId]);
		if (gpxToastTimer) clearTimeout(gpxToastTimer);
		gpxToast = null;

		try {
			const result = await api.importGPX(deviceId, file);
			gpxToast = { deviceId, msg: `Imported ${result.imported} positions`, ok: true };
		} catch {
			gpxToast = { deviceId, msg: 'Import failed', ok: false };
		} finally {
			gpxImportingIds = new Set([...gpxImportingIds].filter((id) => id !== deviceId));
			gpxToastTimer = setTimeout(() => { gpxToast = null; }, 5000);
		}
	}

	async function handleSave() {
		if (!formName.trim() || !formUniqueId.trim()) {
			error = 'Name and Identifier are required';
			return;
		}

		saving = true;
		error = '';

		try {
			const mileageKm = formMileage.trim()
				? mileageFromDisplay(parseFloat(formMileage.trim()))
				: undefined;

			if (mileageKm !== undefined && (isNaN(mileageKm) || mileageKm < 0)) {
				error = 'Mileage must be a positive number';
				saving = false;
				return;
			}

			const payload = {
				name: formName.trim(),
				uniqueId: formUniqueId.trim(),
				phone: formPhone.trim() || undefined,
				model: formModel.trim() || undefined,
				category: formCategory.trim() || undefined,
				mileage: mileageKm !== undefined ? mileageKm : (editingDevice ? undefined : undefined)
			};

			if (editingDevice) {
				await api.updateDevice(editingDevice.id, payload);
			} else {
				await api.createDevice(payload);
			}

			showModal = false;
			await loadDevices();
		} catch {
			error = editingDevice ? 'Failed to update device' : 'Failed to create device';
		} finally {
			saving = false;
		}
	}

	async function handleDelete() {
		if (!editingDevice) return;

		saving = true;
		try {
			await api.deleteDevice(editingDevice.id);
			showDeleteConfirm = false;
			editingDevice = null;
			await loadDevices();
		} catch {
			console.error('Failed to delete device');
		} finally {
			saving = false;
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
	<title>Devices - Motus</title>
</svelte:head>

<!-- Hidden GPX file input shared across all devices -->
<input
	type="file"
	accept=".gpx"
	style="display:none"
	bind:this={gpxFileInput}
	on:change={handleGPXFile}
/>

<div class="devices-page">
	<div class="container">
		<div class="page-header">
			<h1 class="page-title">Devices</h1>
			<Button variant="primary" on:click={openCreateModal}>Add Device</Button>
		</div>

		<div class="toolbar">
			<input
				type="search"
				placeholder="Search devices..."
				class="search-input"
				bind:value={searchQuery}
			/>
			<AllDevicesToggle on:change={loadDevices} />
			<span class="result-count">{filtered.length} device{filtered.length !== 1 ? 's' : ''}</span>
		</div>

		{#if loading}
			<div class="list-skeleton">
				{#each Array(5) as _}
					<Skeleton width="100%" height="64px" variant="rect" />
				{/each}
			</div>
		{:else if filtered.length === 0}
			<div class="empty-state">
				<svg viewBox="0 0 24 24" width="64" height="64" fill="none" stroke="var(--text-tertiary)" stroke-width="1">
					<rect x="5" y="2" width="14" height="20" rx="2"/>
					<line x1="12" y1="18" x2="12" y2="18.01" stroke-width="2"/>
				</svg>
				<p>{searchQuery ? 'No devices match your search' : 'No devices yet'}</p>
				{#if !searchQuery}
					<Button variant="primary" on:click={openCreateModal}>Add your first device</Button>
				{/if}
			</div>
		{:else}
			<!-- Desktop: Table view (hidden on mobile) -->
			<div class="desktop-view">
				<div class="table-wrapper">
					<table class="device-table" aria-label="Devices">
						<thead>
							<tr>
								<th>Status</th>
								<th>Name</th>
								<th>Identifier</th>
								<th>Last Seen</th>
								<th class="th-actions">Actions</th>
							</tr>
						</thead>
						<tbody>
							{#each filtered as device (device.id)}
								<tr class="table-row" class:other-user={device.ownerName}>
									<td class="td-status">
										<StatusIndicator status={getStatusType(device.status)} showLabel />
									</td>
									<td class="td-name">
										<span class="device-name">{device.name}</span>
										{#if device.ownerName}
											<span class="owner-badge" title="Owned by {device.ownerName}">{device.ownerName}</span>
										{/if}
									</td>
									<td class="td-uid">
										<code class="uid-badge">{device.uniqueId}</code>
									</td>
									<td class="td-lastseen">
										{#if device.lastUpdate}
											{formatRelative(new Date(device.lastUpdate))}
										{:else}
											<span class="text-muted">Never</span>
										{/if}
									</td>
									<td class="td-actions">
										<div class="table-actions">
											<a href="/map?device={device.id}" class="action-link" title="View on map">
												<Button variant="primary" size="sm">Map</Button>
											</a>
											<a href="/reports?device={device.id}" class="action-link" title="View reports">
												<Button variant="secondary" size="sm">Reports</Button>
											</a>
											<a href="/reports/charts?device={device.id}" class="action-link" title="View charts">
												<Button variant="secondary" size="sm">Charts</Button>
											</a>
											<Button variant="secondary" size="sm" on:click={() => openShareModal(device)}>Share</Button>
											<Button variant="secondary" size="sm" on:click={() => openEditModal(device)}>Edit</Button>
											<Button variant="secondary" size="sm" on:click={() => openCommandModal(device)}>Commands</Button>
											<Button variant="secondary" size="sm" loading={gpxImportingIds.has(device.id)} on:click={() => openGPXImport(device)}>Import GPX</Button>
											<Button variant="danger" size="sm" on:click={() => openDeleteConfirm(device)}>Delete</Button>
										</div>
										{#if gpxToast && gpxToast.deviceId === device.id}
											<div class="gpx-toast" class:gpx-toast-ok={gpxToast.ok} class:gpx-toast-err={!gpxToast.ok}>
												{gpxToast.msg}
											</div>
										{/if}
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</div>

			<!-- Mobile: Card view (hidden on desktop) -->
			<div class="mobile-view">
				<div class="device-list" role="list">
					{#each filtered as device (device.id)}
						{@const isExpanded = expandedIds.has(device.id)}
						<div
							class="device-card"
							class:expanded={isExpanded}
							class:other-user={device.ownerName}
							role="listitem"
						>
							<!-- Collapsed summary row - always visible -->
							<div
								class="device-summary"
								role="button"
								tabindex="0"
								aria-expanded={isExpanded}
								aria-controls="device-detail-{device.id}"
								on:click={() => toggleDevice(device.id)}
								on:keydown={(e) => handleRowKeydown(e, device.id)}
							>
								<div class="summary-main">
									<div class="summary-name-status">
										<StatusIndicator status={getStatusType(device.status)} />
										<span class="device-name">{device.name}</span>
										{#if device.ownerName}
											<span class="owner-badge" title="Owned by {device.ownerName}">{device.ownerName}</span>
										{/if}
									</div>
									<span class="summary-uid">{device.uniqueId}</span>
								</div>

								<div class="summary-meta">
									<span class="summary-last-seen">
										{#if device.lastUpdate}
											{formatRelative(new Date(device.lastUpdate))}
										{:else}
											Never
										{/if}
									</span>
								</div>

								<svg
									class="chevron"
									class:chevron-open={isExpanded}
									viewBox="0 0 24 24"
									width="20"
									height="20"
									fill="none"
									stroke="currentColor"
									stroke-width="2"
									stroke-linecap="round"
									stroke-linejoin="round"
									aria-hidden="true"
								>
									<polyline points="6 9 12 15 18 9"></polyline>
								</svg>
							</div>

							<!-- Expanded detail section -->
							{#if isExpanded}
								<div
									id="device-detail-{device.id}"
									class="device-detail"
									transition:slide={{ duration: 200 }}
								>
									<div class="detail-grid">
										<div class="detail-item">
											<span class="detail-label">Identifier</span>
											<code class="detail-value uid">{device.uniqueId}</code>
										</div>
										<div class="detail-item">
											<span class="detail-label">Status</span>
											<span class="detail-value">
												<StatusIndicator status={getStatusType(device.status)} showLabel />
											</span>
										</div>
										<div class="detail-item">
											<span class="detail-label">Last Seen</span>
											<span class="detail-value">
												{#if device.lastUpdate}
													{formatRelative(new Date(device.lastUpdate))}
												{:else}
													Never
												{/if}
											</span>
										</div>
										<div class="detail-item">
											<span class="detail-label">Model</span>
											<span class="detail-value">{device.model || '-'}</span>
										</div>
										<div class="detail-item">
											<span class="detail-label">Phone</span>
											<span class="detail-value">{device.phone || '-'}</span>
										</div>
										<div class="detail-item">
											<span class="detail-label">Category</span>
											<span class="detail-value">{device.category || '-'}</span>
										</div>
										{#if device.mileage != null}
										<div class="detail-item">
											<span class="detail-label">Mileage</span>
											<span class="detail-value">{formatMileage(device.mileage)}</span>
										</div>
										{:else}
										<div class="detail-item">
											<span class="detail-label">Mileage</span>
											<span class="detail-value">—</span>
										</div>
										{/if}
									</div>

									<!-- svelte-ignore a11y-click-events-have-key-events a11y-no-noninteractive-element-interactions -->
									<div class="detail-actions" on:click|stopPropagation role="group" aria-label="Device actions">
										<a href="/map?device={device.id}" class="action-link" on:click|stopPropagation>
											<Button variant="primary" size="sm">
												<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
													<polygon points="1 6 1 22 8 18 16 22 23 18 23 2 16 6 8 2 1 6"></polygon>
													<line x1="8" y1="2" x2="8" y2="18"></line>
													<line x1="16" y1="6" x2="16" y2="22"></line>
												</svg>
												Map
											</Button>
										</a>
										<a href="/reports?device={device.id}" class="action-link" on:click|stopPropagation>
											<Button variant="secondary" size="sm">
												<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
													<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path>
													<polyline points="14 2 14 8 20 8"></polyline>
													<line x1="16" y1="13" x2="8" y2="13"></line>
													<line x1="16" y1="17" x2="8" y2="17"></line>
													<polyline points="10 9 9 9 8 9"></polyline>
												</svg>
												Reports
											</Button>
										</a>
										<a href="/reports/charts?device={device.id}" class="action-link" on:click|stopPropagation>
											<Button variant="secondary" size="sm">
												<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
													<polyline points="22 12 18 12 15 21 9 3 6 12 2 12"></polyline>
												</svg>
												Charts
											</Button>
										</a>
										<div on:click|stopPropagation on:keydown|stopPropagation role="presentation">
											<Button variant="secondary" size="sm" on:click={() => openShareModal(device)}>
												<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
													<circle cx="18" cy="5" r="3"></circle>
													<circle cx="6" cy="12" r="3"></circle>
													<circle cx="18" cy="19" r="3"></circle>
													<line x1="8.59" y1="13.51" x2="15.42" y2="17.49"></line>
													<line x1="15.41" y1="6.51" x2="8.59" y2="10.49"></line>
												</svg>
												Share
											</Button>
										</div>
										<div on:click|stopPropagation on:keydown|stopPropagation role="presentation">
											<Button variant="secondary" size="sm" on:click={() => openEditModal(device)}>
												<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
													<path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"></path>
													<path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"></path>
												</svg>
												Edit
											</Button>
										</div>
										<div on:click|stopPropagation on:keydown|stopPropagation role="presentation">
											<Button variant="secondary" size="sm" on:click={() => openCommandModal(device)}>
												<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
													<polyline points="4 17 10 11 4 5"></polyline>
													<line x1="12" y1="19" x2="20" y2="19"></line>
												</svg>
												Commands
											</Button>
										</div>
										<div on:click|stopPropagation on:keydown|stopPropagation role="presentation">
											<Button variant="secondary" size="sm" loading={gpxImportingIds.has(device.id)} on:click={() => openGPXImport(device)}>
												<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
													<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
													<polyline points="17 8 12 3 7 8"></polyline>
													<line x1="12" y1="3" x2="12" y2="15"></line>
												</svg>
												Import GPX
											</Button>
										</div>
										<div on:click|stopPropagation on:keydown|stopPropagation role="presentation">
											<Button variant="danger" size="sm" on:click={() => openDeleteConfirm(device)}>
												<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
													<polyline points="3 6 5 6 21 6"></polyline>
													<path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path>
												</svg>
												Delete
											</Button>
										</div>
									</div>
									{#if gpxToast && gpxToast.deviceId === device.id}
										<div class="gpx-toast" class:gpx-toast-ok={gpxToast.ok} class:gpx-toast-err={!gpxToast.ok}>
											{gpxToast.msg}
										</div>
									{/if}
								</div>
							{/if}
						</div>
					{/each}
				</div>
			</div>
		{/if}
	</div>
</div>

<!-- Create/Edit Modal -->
<Modal bind:open={showModal} title={editingDevice ? 'Edit Device' : 'Add Device'}>
	<form on:submit|preventDefault={handleSave} class="device-form">
		<Input
			name="name"
			label="Name"
			placeholder="My Vehicle"
			required
			bind:value={formName}
		/>
		<Input
			name="uniqueId"
			label="Identifier"
			placeholder="ABC123"
			required
			disabled={!!editingDevice}
			bind:value={formUniqueId}
		/>
		<Input name="phone" label="Phone" placeholder="+1234567890" bind:value={formPhone} />
		<Input name="model" label="Model" placeholder="TK103" bind:value={formModel} />
		<Input name="category" label="Category" placeholder="car" bind:value={formCategory} />
		<Input
			name="mileage"
			label="Mileage ({$settings.units === 'imperial' ? 'mi' : 'km'})"
			placeholder="117000"
			type="number"
			bind:value={formMileage}
		/>

		{#if error}
			<div class="form-error" role="alert">{error}</div>
		{/if}
	</form>

	<svelte:fragment slot="footer">
		<div class="modal-actions">
			<Button variant="secondary" on:click={() => (showModal = false)}>Cancel</Button>
			<Button variant="primary" loading={saving} on:click={handleSave}>
				{editingDevice ? 'Save Changes' : 'Create Device'}
			</Button>
		</div>
	</svelte:fragment>
</Modal>

<!-- Share Modal -->
{#if sharingDevice}
	<ShareModal
		bind:open={showShareModal}
		deviceId={sharingDevice.id}
		deviceName={sharingDevice.name}
		on:close={() => { showShareModal = false; sharingDevice = null; }}
	/>
{/if}

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title="Delete Device">
	<p class="delete-message">
		Are you sure you want to delete <strong>{editingDevice?.name}</strong>?
		This action cannot be undone.
	</p>

	<svelte:fragment slot="footer">
		<div class="modal-actions">
			<Button variant="secondary" on:click={() => (showDeleteConfirm = false)}>Cancel</Button>
			<Button variant="danger" loading={saving} on:click={handleDelete}>Delete</Button>
		</div>
	</svelte:fragment>
</Modal>

<!-- Command Modal -->
{#if commandDevice}
<Modal bind:open={showCommandModal} title="Send Command — {commandDevice.name}">
	<div class="cmd-form">
		<div class="form-group">
			<label class="form-label" for="cmd-type">Command Type</label>
			<select id="cmd-type" class="cmd-select" bind:value={commandType}>
				{#each Object.entries(commandTypeLabels) as [value, label]}
					<option {value}>{label}</option>
				{/each}
			</select>
		</div>

		{#if commandType === 'positionPeriodic'}
			<div class="form-group">
				<Input name="frequency" label="Interval (seconds)" placeholder="30" bind:value={commandFrequency} />
			</div>
		{:else if commandType === 'sosNumber'}
			<div class="form-group">
				<Input name="sosNumber" label="SOS Phone Number" placeholder="+1234567890" bind:value={commandSosNumber} />
			</div>
		{:else if commandType === 'setSpeedAlarm'}
			<div class="form-group">
				<Input name="speed" label="Speed Limit (km/h, 0 = disable)" placeholder="80" bind:value={commandSpeed} />
			</div>
		{:else if commandType === 'custom'}
			<div class="form-group">
				<Input name="text" label="Raw Command" placeholder="rconf" bind:value={commandText} />
			</div>
		{/if}

		{#if commandError}
			<div class="form-error" role="alert">{commandError}</div>
		{/if}

		{#if commandResult}
			<div class="cmd-result" class:cmd-result-ok={commandResult.ok}>
				<div class="cmd-result-header">
					<span>Device Response</span>
					<button class="cmd-result-close" on:click={() => { commandResult = null; }} aria-label="Dismiss">×</button>
				</div>
				<pre class="cmd-result-body">{commandResult.msg}</pre>
			</div>
		{/if}

		{#if commandHistory.length > 0}
			<div class="cmd-history">
				<h4 class="cmd-history-title">Recent Commands</h4>
				<div class="cmd-history-list">
					{#each commandHistory as cmd}
						<div class="cmd-history-item">
							<div class="cmd-history-meta">
								<span class="cmd-history-type">{commandTypeLabels[cmd.type] ?? cmd.type}</span>
								<span class="cmd-history-status cmd-status-{cmd.status}">{cmd.status}</span>
								<span class="cmd-history-time">{new Date(cmd.createdAt).toLocaleString()}</span>
							</div>
							{#if cmd.result}
								<pre class="cmd-history-result">{cmd.result}</pre>
							{/if}
						</div>
					{/each}
				</div>
			</div>
		{/if}
	</div>

	<svelte:fragment slot="footer">
		<div class="modal-actions">
			<Button variant="secondary" on:click={() => (showCommandModal = false)}>Cancel</Button>
			<Button variant="primary" loading={commandSending} on:click={handleSendCommand}>Send</Button>
		</div>
	</svelte:fragment>
</Modal>
{/if}

<style>
	.devices-page {
		padding: var(--space-6) 0;
	}

	.page-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: var(--space-6);
	}

	.page-title {
		font-size: var(--text-3xl);
		font-weight: var(--font-bold);
		color: var(--text-primary);
	}

	.toolbar {
		display: flex;
		align-items: center;
		gap: var(--space-4);
		margin-bottom: var(--space-4);
	}

	.search-input {
		flex: 1;
		max-width: 400px;
		padding: var(--space-3) var(--space-4);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-sm);
	}

	.search-input:focus {
		outline: none;
		border-color: var(--accent-primary);
	}

	.result-count {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}

	.list-skeleton {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	/* ---- Responsive view toggle ---- */
	.desktop-view {
		display: block;
	}

	.mobile-view {
		display: none;
	}

	@media (max-width: 768px) {
		.desktop-view {
			display: none;
		}

		.mobile-view {
			display: block;
		}
	}

	/* ==== DESKTOP TABLE VIEW ==== */
	.table-wrapper {
		overflow-x: auto;
		border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
		background-color: var(--bg-primary);
	}

	.device-table {
		width: 100%;
		border-collapse: collapse;
		font-size: var(--text-sm);
	}

	.device-table thead {
		background-color: var(--bg-secondary);
	}

	.device-table th {
		padding: var(--space-3) var(--space-4);
		text-align: left;
		font-weight: var(--font-semibold);
		font-size: var(--text-xs);
		color: var(--text-tertiary);
		text-transform: uppercase;
		letter-spacing: 0.05em;
		border-bottom: 1px solid var(--border-color);
		white-space: nowrap;
	}

	.th-actions {
		text-align: right;
	}

	.device-table td {
		padding: var(--space-3) var(--space-4);
		border-bottom: 1px solid var(--border-color);
		color: var(--text-primary);
		vertical-align: middle;
	}

	.device-table tbody tr:last-child td {
		border-bottom: none;
	}

	.table-row {
		transition: background-color var(--transition-fast);
	}

	.table-row:hover {
		background-color: var(--bg-hover);
	}

	.td-status {
		white-space: nowrap;
	}

	.td-name {
		font-weight: var(--font-semibold);
		max-width: 200px;
	}

	.td-name .device-name {
		display: block;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.uid-badge {
		font-size: var(--text-xs);
		padding: var(--space-1) var(--space-2);
		background-color: var(--bg-tertiary);
		border-radius: var(--radius-sm);
		color: var(--text-secondary);
		font-family: monospace;
	}

	.td-lastseen {
		color: var(--text-tertiary);
		white-space: nowrap;
	}

	.text-muted {
		color: var(--text-tertiary);
	}

	.td-actions {
		text-align: right;
		white-space: nowrap;
	}

	.table-actions {
		display: inline-flex;
		gap: var(--space-2);
		align-items: center;
	}

	.table-actions .action-link {
		text-decoration: none;
	}

	/* ==== MOBILE CARD VIEW ==== */
	.device-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	.device-card {
		border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
		background-color: var(--bg-primary);
		overflow: hidden;
		transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
	}

	.device-card:hover {
		border-color: var(--border-hover);
	}

	.device-card.expanded {
		border-color: var(--accent-primary);
		box-shadow: 0 0 0 1px var(--accent-primary);
	}

	.device-summary {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		padding: var(--space-3);
		cursor: pointer;
		user-select: none;
		-webkit-user-select: none;
	}

	.device-summary:hover {
		background-color: var(--bg-hover);
	}

	.device-summary:focus-visible {
		outline: 2px solid var(--accent-primary);
		outline-offset: -2px;
		border-radius: var(--radius-lg);
	}

	.summary-main {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}

	.summary-name-status {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}

	.device-name {
		font-weight: var(--font-semibold);
		color: var(--text-primary);
		font-size: var(--text-base);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.summary-uid {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
		font-family: monospace;
	}

	.summary-meta {
		flex-shrink: 0;
		text-align: right;
	}

	.summary-last-seen {
		font-size: var(--text-sm);
		color: var(--text-tertiary);
		white-space: nowrap;
	}

	.chevron {
		flex-shrink: 0;
		color: var(--text-tertiary);
		transition: transform var(--transition-fast);
	}

	.chevron-open {
		transform: rotate(180deg);
	}

	.device-detail {
		border-top: 1px solid var(--border-color);
		background-color: var(--bg-secondary);
		padding: var(--space-3);
	}

	.detail-grid {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: var(--space-3);
		margin-bottom: var(--space-4);
	}

	.detail-item {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}

	.detail-label {
		font-size: var(--text-xs);
		font-weight: var(--font-semibold);
		color: var(--text-tertiary);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.detail-value {
		font-size: var(--text-sm);
		color: var(--text-primary);
	}

	.uid {
		font-size: var(--text-xs);
		padding: var(--space-1) var(--space-2);
		background-color: var(--bg-tertiary);
		border-radius: var(--radius-sm);
		color: var(--text-secondary);
		display: inline-block;
		width: fit-content;
	}

	.detail-actions {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		padding-top: var(--space-3);
		border-top: 1px solid var(--border-color);
	}

	.action-link {
		text-decoration: none;
		width: 100%;
	}

	.detail-actions :global(.btn) {
		width: 100%;
		justify-content: center;
	}

	.action-link :global(.btn) {
		width: 100%;
		justify-content: center;
	}

	.detail-actions > div {
		width: 100%;
	}

	.detail-actions > div :global(.btn) {
		width: 100%;
		justify-content: center;
	}

	/* ---- Empty state ---- */
	.empty-state {
		text-align: center;
		padding: var(--space-16) var(--space-4);
	}

	.empty-state p {
		color: var(--text-secondary);
		margin: var(--space-4) 0;
	}

	/* ---- Form / modal styles ---- */
	.device-form {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
	}

	.form-error {
		padding: var(--space-3);
		background-color: rgba(255, 68, 68, 0.1);
		border: 1px solid var(--error);
		border-radius: var(--radius-md);
		color: var(--error);
		font-size: var(--text-sm);
	}

	.modal-actions {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-3);
	}

	.delete-message {
		color: var(--text-secondary);
		line-height: 1.6;
	}

	.delete-message strong {
		color: var(--text-primary);
	}

	/* ---- GPX import toast ---- */
	.gpx-toast {
		margin-top: var(--space-2);
		padding: var(--space-2) var(--space-3);
		border-radius: var(--radius-md);
		font-size: var(--text-xs);
		font-weight: var(--font-semibold);
	}

	.gpx-toast-ok {
		background-color: rgba(34, 197, 94, 0.12);
		color: #16a34a;
		border: 1px solid rgba(34, 197, 94, 0.3);
	}

	.gpx-toast-err {
		background-color: rgba(255, 68, 68, 0.1);
		color: var(--error);
		border: 1px solid var(--error);
	}

	/* ---- Command modal ---- */
	.cmd-form {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
	}

	.form-group {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}

	.form-label {
		font-size: var(--text-sm);
		font-weight: var(--font-semibold);
		color: var(--text-secondary);
	}

	.cmd-select {
		padding: var(--space-2) var(--space-3);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-sm);
		width: 100%;
	}

	.cmd-select:focus {
		outline: none;
		border-color: var(--accent-primary);
	}

	.cmd-result {
		border-radius: var(--radius-md);
		border: 1px solid rgba(34, 197, 94, 0.3);
		background-color: rgba(34, 197, 94, 0.08);
		overflow: hidden;
	}

	.cmd-result-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: var(--space-2) var(--space-3);
		font-size: var(--text-xs);
		font-weight: var(--font-semibold);
		color: #16a34a;
		background-color: rgba(34, 197, 94, 0.12);
	}

	.cmd-result-close {
		background: none;
		border: none;
		cursor: pointer;
		color: inherit;
		font-size: var(--text-base);
		line-height: 1;
		padding: 0;
	}

	.cmd-result-body {
		margin: 0;
		padding: var(--space-3);
		font-family: monospace;
		font-size: var(--text-xs);
		color: var(--text-primary);
		white-space: pre-wrap;
		word-break: break-all;
		max-height: 200px;
		overflow-y: auto;
	}

	.cmd-history {
		border-top: 1px solid var(--border-color);
		padding-top: var(--space-3);
	}

	.cmd-history-title {
		font-size: var(--text-sm);
		font-weight: var(--font-semibold);
		color: var(--text-secondary);
		margin: 0 0 var(--space-2);
	}

	.cmd-history-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		max-height: 240px;
		overflow-y: auto;
	}

	.cmd-history-item {
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		padding: var(--space-2) var(--space-3);
	}

	.cmd-history-meta {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		flex-wrap: wrap;
		font-size: var(--text-xs);
	}

	.cmd-history-type {
		font-weight: var(--font-semibold);
		color: var(--text-primary);
	}

	.cmd-history-status {
		padding: 1px var(--space-2);
		border-radius: var(--radius-full, 9999px);
		font-size: 10px;
		font-weight: var(--font-semibold);
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}

	.cmd-status-pending {
		background-color: rgba(234, 179, 8, 0.15);
		color: #a16207;
	}

	.cmd-status-sent {
		background-color: rgba(59, 130, 246, 0.15);
		color: #1d4ed8;
	}

	.cmd-status-executed {
		background-color: rgba(34, 197, 94, 0.15);
		color: #15803d;
	}

	.cmd-history-time {
		color: var(--text-tertiary);
		margin-left: auto;
	}

	.cmd-history-result {
		margin: var(--space-2) 0 0;
		padding: var(--space-2);
		font-family: monospace;
		font-size: var(--text-xs);
		color: var(--text-secondary);
		background-color: var(--bg-tertiary);
		border-radius: var(--radius-sm);
		white-space: pre-wrap;
		word-break: break-all;
		max-height: 120px;
		overflow-y: auto;
	}

	/* ---- Responsive: small mobile ---- */
	@media (max-width: 480px) {
		.devices-page {
			padding: var(--space-4) 0;
		}

		.page-header {
			margin-bottom: var(--space-4);
		}

		.page-title {
			font-size: var(--text-2xl);
		}

		.toolbar {
			flex-direction: column;
			align-items: stretch;
			gap: var(--space-2);
		}

		.search-input {
			max-width: none;
		}

		.result-count {
			text-align: right;
		}
	}

	.table-row.other-user,
	.device-card.other-user {
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
		vertical-align: middle;
	}
</style>

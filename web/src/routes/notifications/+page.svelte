<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api, fetchNotifications, stripOwnOwnerName } from '$lib/api/client';
	import { isAdmin } from '$lib/stores/auth';
	import { formatDate } from '$lib/utils/formatting';
	import { refreshHandler } from '$lib/stores/refresh';
	import Button from '$lib/components/Button.svelte';
	import Input from '$lib/components/Input.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import AllDevicesToggle from '$lib/components/AllDevicesToggle.svelte';
	import CommandParamFields from '$lib/components/CommandParamFields.svelte';
	import RuleFilterCheckboxes from '$lib/components/RuleFilterCheckboxes.svelte';
	import ReportingIntervalPicker from '$lib/components/ReportingIntervalPicker.svelte';
	import type {
		Device,
		Geofence,
		NotificationRule,
		NotificationChannel,
		NotificationConfig,
		NotificationConfigWebhook
	} from '$lib/types/api';
	import { COMMAND_TYPE_LABELS, DEFAULT_REPORTING_INTERVAL_SECONDS } from '$lib/utils/commands';
	import {
		EVENT_TYPES,
		CHANNELS,
		TEMPLATE_VARIABLES,
		DEFAULT_TEMPLATE,
		DEFAULT_AWAY_INTERVAL,
		DEFAULT_HOME_INTERVAL,
		NOTIFICATION_COMMAND_TYPES,
		buildCommandConfig,
		buildIntervalAutomationRules,
		createAllOrNone,
		commandEventConflict,
		commandFormValues,
		describeCommandAction,
		describeDeviceFilter,
		describeGeofenceFilter,
		deviceFilterOptions,
		geofenceFilterOptions,
		getEventLabel,
		hasGeofenceEvent
	} from '$lib/utils/notificationRules';

	let notificationRules: NotificationRule[] = [];
	let loading = true;
	let error = '';
	let showModal = false;
	let editingRule: NotificationRule | null = null;
	let formError = '';

	// Geofence lookup for the geofence filter and the rule cards. Admins load
	// every geofence, so the filters of other users' rules (admin "All users"
	// toggle) resolve as well.
	let geofences: Geofence[] = [];
	// Device lookup for the device filter, loaded with the same scope.
	let devices: Device[] = [];

	// Form state
	let formName = '';
	let formEventTypes: string[] = [];
	let formChannel: NotificationChannel = 'webhook';
	let formWebhookUrl = '';
	let formHeaders: Array<{ key: string; value: string }> = [];
	let formTemplate = DEFAULT_TEMPLATE;
	let formGeofenceIds: number[] = [];
	let formDeviceIds: number[] = [];
	let formCommandType = NOTIFICATION_COMMAND_TYPES[0];
	let formFrequency = String(DEFAULT_REPORTING_INTERVAL_SECONDS);
	let formSosNumber = '';
	let formSpeed = '';
	let formText = '';

	$: showGeofenceFilter = hasGeofenceEvent(formEventTypes);
	$: geofenceOptions = geofenceFilterOptions(formGeofenceIds, geofences, editingRule?.geofenceIds ?? []);
	$: deviceOptions = deviceFilterOptions(formDeviceIds, devices, editingRule?.deviceIds ?? []);
	$: commandConflict =
		formChannel === 'command' ? commandEventConflict(formCommandType, formEventTypes) : null;

	// Interval automation (e.g. pet tracking). The rules are created for the
	// current user, so the dialog offers only the user's own geofences and
	// devices, loaded when it opens (not inferred from ownerName).
	let showAutomationModal = false;
	let automationGeofences: Geofence[] = [];
	let automationDevices: Device[] = [];
	let automationGeofenceId: number | null = null;
	let automationDeviceIds: number[] = [];
	let automationAway = String(DEFAULT_AWAY_INTERVAL);
	let automationHome = String(DEFAULT_HOME_INTERVAL);
	let automationError = '';
	let automationLoading = false;
	let automationSaving = false;

	// Test notification state
	let testingId: number | null = null;

	onMount(async () => {
		await Promise.all([loadRules(), loadGeofences(), loadDevices()]);
		$refreshHandler = refresh;
	});

	async function refresh() {
		await Promise.all([loadRules(), loadGeofences(), loadDevices()]);
	}

	async function loadGeofences() {
		try {
			if ($isAdmin) {
				// Full lookup regardless of the "All users" toggle; the owner is
				// shown only for other users' geofences.
				geofences = stripOwnOwnerName(await api.getAllGeofences());
			} else {
				geofences = await api.getGeofences();
			}
		} catch (err) {
			// The geofence filter is optional; rules still work without it.
			console.error('Failed to load geofences:', err);
			geofences = [];
		}
	}

	async function loadDevices() {
		try {
			devices = $isAdmin ? stripOwnOwnerName(await api.getAllDevices()) : await api.getDevices();
		} catch (err) {
			// The device filter is optional; rules still work without it.
			console.error('Failed to load devices:', err);
			devices = [];
		}
	}

	onDestroy(() => { $refreshHandler = null; });

	async function loadRules() {
		loading = true;
		error = '';
		try {
			const rules = await fetchNotifications();
			notificationRules = rules;
		} catch (err: any) {
			error = 'Failed to load notification rules';
			console.error(err);
		} finally {
			loading = false;
		}
	}

	function resetForm() {
		formName = '';
		formEventTypes = [];
		formChannel = 'webhook';
		formWebhookUrl = '';
		formHeaders = [];
		formTemplate = DEFAULT_TEMPLATE;
		formGeofenceIds = [];
		formDeviceIds = [];
		formCommandType = NOTIFICATION_COMMAND_TYPES[0];
		formFrequency = String(DEFAULT_REPORTING_INTERVAL_SECONDS);
		formSosNumber = '';
		formSpeed = '';
		formText = '';
		formError = '';
	}

	async function openAutomation() {
		automationAway = String(DEFAULT_AWAY_INTERVAL);
		automationHome = String(DEFAULT_HOME_INTERVAL);
		automationError = '';
		automationLoading = true;
		showAutomationModal = true;
		try {
			[automationGeofences, automationDevices] = await Promise.all([api.getGeofences(), api.getDevices()]);
		} catch (err: any) {
			automationGeofences = [];
			automationDevices = [];
			automationError = 'Failed to load geofences and devices';
			console.error(err);
		} finally {
			automationLoading = false;
		}
		automationGeofenceId = automationGeofences[0]?.id ?? null;
		// Preselect the only device; with several, the user must choose.
		automationDeviceIds = automationDevices.length === 1 ? [automationDevices[0].id] : [];
	}

	async function handleCreateAutomation() {
		automationError = '';
		const geofence = automationGeofences.find((g) => g.id === automationGeofenceId);
		if (!geofence) {
			automationError = 'Select a geofence';
			return;
		}
		let rules;
		try {
			rules = buildIntervalAutomationRules({
				geofenceId: geofence.id,
				geofenceName: geofence.name,
				deviceIds: automationDeviceIds,
				awayInterval: automationAway,
				homeInterval: automationHome
			});
		} catch (err: any) {
			automationError = err.message;
			return;
		}
		automationSaving = true;
		try {
			await createAllOrNone(rules, api.createNotification, api.deleteNotification);
			showAutomationModal = false;
		} catch (err: any) {
			automationError = 'Failed to create rules' + (err?.message ? `: ${err.message}` : '');
			console.error(err);
		} finally {
			automationSaving = false;
			await loadRules();
		}
	}

	function openCreate() {
		editingRule = null;
		resetForm();
		showModal = true;
	}

	function openEdit(rule: NotificationRule) {
		resetForm();
		editingRule = rule;
		formName = rule.name;
		formEventTypes = [...rule.eventTypes];
		formChannel = rule.channel;
		formGeofenceIds = [...(rule.geofenceIds ?? [])];
		formDeviceIds = [...(rule.deviceIds ?? [])];

		if (rule.config?.channel === 'command') {
			formCommandType = rule.config.commandType;
			const values = commandFormValues(rule.config);
			formFrequency = values.frequency;
			formSosNumber = values.phoneNumber;
			formSpeed = values.speed;
			formText = values.text;
		} else if (rule.config?.channel === 'webhook') {
			formTemplate = rule.template;
			formWebhookUrl = rule.config.webhookUrl || '';
			formHeaders = rule.config.headers
				? Object.entries(rule.config.headers).map(([key, value]) => ({
						key,
						value: String(value)
					}))
				: [];
		}

		showModal = true;
	}

	function buildWebhookConfig(): NotificationConfigWebhook {
		const cfg: NotificationConfigWebhook = { channel: 'webhook', webhookUrl: formWebhookUrl };
		const filteredHeaders = formHeaders.filter((h) => h.key.trim());
		if (filteredHeaders.length > 0) {
			cfg.headers = Object.fromEntries(filteredHeaders.map((h) => [h.key, h.value]));
		}
		return cfg;
	}

	async function handleSubmit() {
		error = '';
		formError = '';

		let config: NotificationConfig;
		if (formChannel === 'command') {
			if (commandConflict) {
				formError = commandConflict;
				return;
			}
			const built = buildCommandConfig(formCommandType, {
				frequency: formFrequency,
				phoneNumber: formSosNumber,
				speed: formSpeed,
				text: formText
			});
			if (!built.config) {
				formError = built.error ?? 'Invalid command';
				return;
			}
			config = built.config;
		} else {
			config = buildWebhookConfig();
		}

		const ruleData = {
			name: formName,
			eventTypes: formEventTypes,
			channel: formChannel,
			config,
			template: formChannel === 'webhook' ? formTemplate : '',
			enabled: editingRule ? editingRule.enabled : true,
			// An empty filter means "all geofences".
			geofenceIds: showGeofenceFilter ? formGeofenceIds : [],
			// An empty filter means "all devices".
			deviceIds: formDeviceIds
		};

		try {
			if (editingRule) {
				await api.updateNotification(editingRule.id, ruleData);
			} else {
				await api.createNotification(ruleData);
			}
			showModal = false;
			resetForm();
			await loadRules();
		} catch (err: any) {
			error = editingRule
				? 'Failed to update notification rule'
				: 'Failed to create notification rule';
			formError = err?.message ? `${error}: ${err.message}` : error;
			console.error(err);
		}
	}

	async function toggleRule(rule: NotificationRule) {
		error = '';
		try {
			await api.updateNotification(rule.id, {
				name: rule.name,
				eventTypes: rule.eventTypes,
				channel: rule.channel,
				config: rule.config,
				template: rule.template,
				enabled: !rule.enabled,
				geofenceIds: rule.geofenceIds ?? [],
				deviceIds: rule.deviceIds ?? []
			});
			await loadRules();
		} catch (err: any) {
			error = 'Failed to toggle notification rule';
			console.error(err);
		}
	}

	async function deleteRule(id: number) {
		if (!confirm('Are you sure you want to delete this notification rule?')) return;

		error = '';
		try {
			await api.deleteNotification(id);
			await loadRules();
		} catch (err: any) {
			error = 'Failed to delete notification rule';
			console.error(err);
		}
	}

	async function testRule(rule: NotificationRule) {
		testingId = rule.id;
		error = '';
		try {
			await api.testNotification(rule.id);
			alert('Test notification sent successfully! Check your destination.');
		} catch (err: any) {
			error = 'Failed to send test notification';
			alert('Failed to send test notification: ' + (err.message || 'Unknown error'));
		} finally {
			testingId = null;
		}
	}

	function addHeader() {
		formHeaders = [...formHeaders, { key: '', value: '' }];
	}
	function removeHeader(i: number) {
		formHeaders = formHeaders.filter((_, idx) => idx !== i);
	}
	function insertVariable(v: string) {
		formTemplate += ` ${v}`;
	}

	function getChannelLabel(channel: string): string {
		return CHANNELS.find((c) => c.value === channel)?.label || channel;
	}

	function getDestination(rule: NotificationRule): string {
		if (rule.config?.channel === 'command') return describeCommandAction(rule.config);
		if (rule.config?.channel === 'webhook') return rule.config.webhookUrl || 'No URL set';
		return 'No URL set';
	}

</script>

<svelte:head><title>Notifications - Motus</title></svelte:head>

<div class="notifications-page">
	<div class="container">
		<div class="page-header page-header-stack">
			<h1 class="page-title">Notification Rules</h1>
			<div class="header-actions">
				<AllDevicesToggle on:change={refresh} />
				<a href="/notifications/history" class="history-link">
					<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
						<circle cx="12" cy="12" r="10"/>
						<polyline points="12 6 12 12 16 14"/>
					</svg>
					History
				</a>
				<Button variant="secondary" on:click={openAutomation}>Interval Automation</Button>
				<Button on:click={openCreate}>+ Create Rule</Button>
			</div>
		</div>

		{#if error}
			<div class="error-banner" role="alert">
				<span>{error}</span>
				<button class="dismiss-btn" on:click={() => (error = '')} aria-label="Dismiss error">X</button>
			</div>
		{/if}

		{#if loading}
			<div class="loading-state">
				<div class="spinner" aria-hidden="true"></div>
				<p>Loading notification rules...</p>
			</div>
		{:else if notificationRules.length === 0}
			<div class="empty-state">
				<svg viewBox="0 0 24 24" width="64" height="64" fill="none" stroke="var(--text-tertiary)" stroke-width="1">
					<path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" />
					<path d="M13.73 21a2 2 0 0 1-3.46 0" />
				</svg>
				<p>No notification rules yet</p>
				<p class="empty-subtitle">Create rules to get alerts when devices enter geofences, go online/offline, and more.</p>
				<Button on:click={openCreate}>Create your first rule</Button>
			</div>
		{:else}
			<div class="rules-list">
				{#each notificationRules as rule (rule.id)}
					<div class="rule-card" class:disabled={!rule.enabled} class:other-user={rule.ownerName}>
						<div class="rule-header">
							<div class="rule-info">
								<h3 class="rule-name">{rule.name}</h3>
								<div class="rule-badges">
									{#each rule.eventTypes as et}
										<span class="event-badge">{getEventLabel(et)}</span>
									{/each}
									<span class="channel-badge channel-{rule.channel}">{getChannelLabel(rule.channel)}</span>
									{#if rule.ownerName}
										<span class="owner-badge" title="Owned by {rule.ownerName}">{rule.ownerName}</span>
									{/if}
								</div>
							</div>
							<label class="toggle-switch">
								<input
									type="checkbox"
									checked={rule.enabled}
									on:change={() => toggleRule(rule)}
								/>
								<span class="toggle-slider"></span>
							</label>
						</div>
						<div class="rule-body">
							<div class="rule-detail">
								<span class="detail-label">{rule.channel === 'command' ? 'Action:' : 'Destination:'}</span>
								<span class="detail-value truncate rule-destination">{getDestination(rule)}</span>
							</div>
							{#if hasGeofenceEvent(rule.eventTypes)}
								<div class="rule-detail">
									<span class="detail-label">Geofences:</span>
									<span class="detail-value truncate rule-geofences">{describeGeofenceFilter(rule.geofenceIds, geofences)}</span>
								</div>
							{/if}
							{#if rule.deviceIds?.length}
								<div class="rule-detail">
									<span class="detail-label">Devices:</span>
									<span class="detail-value truncate rule-devices">{describeDeviceFilter(rule.deviceIds, devices)}</span>
								</div>
							{/if}
							{#if rule.updatedAt}
								<div class="rule-detail">
									<span class="detail-label">Last updated:</span>
									<span class="detail-value">{formatDate(rule.updatedAt)}</span>
								</div>
							{/if}
						</div>
						<div class="rule-footer">
							{#if rule.channel !== 'command'}
								<Button
									size="sm"
									variant="secondary"
									loading={testingId === rule.id}
									on:click={() => testRule(rule)}
								>Test</Button>
							{/if}
							<a class="logs-link" href="/notifications/history?rule={rule.id}">Logs</a>
							<Button size="sm" variant="secondary" on:click={() => openEdit(rule)}>Edit</Button>
							<Button size="sm" variant="danger" on:click={() => deleteRule(rule.id)}>Delete</Button>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>

<!-- Create/Edit Modal -->
<Modal
	open={showModal}
	title={editingRule ? 'Edit Notification Rule' : 'Create Notification Rule'}
	on:close={() => (showModal = false)}
>
	<form on:submit|preventDefault={handleSubmit} class="notification-form">
		<Input
			label="Rule Name"
			name="name"
			placeholder="e.g., Geofence Alert"
			required
			bind:value={formName}
		/>

		<div class="form-group">
			<div class="form-label-row">
				<span class="form-label">Event Types</span>
				<button type="button" class="select-all-btn" on:click={() => {
					if (formEventTypes.length === EVENT_TYPES.length) {
						formEventTypes = [];
					} else {
						formEventTypes = EVENT_TYPES.map(e => e.value);
					}
				}}>
					{formEventTypes.length === EVENT_TYPES.length ? 'Deselect all' : 'Select all'}
				</button>
			</div>
			<div class="event-type-grid">
				{#each EVENT_TYPES as type}
					<label class="event-type-checkbox">
						<input
							type="checkbox"
							value={type.value}
							checked={formEventTypes.includes(type.value)}
							on:change={(e) => {
								const target = e.target;
								if (target instanceof HTMLInputElement) {
									if (target.checked) {
										formEventTypes = [...formEventTypes, type.value];
									} else {
										formEventTypes = formEventTypes.filter(v => v !== type.value);
									}
								}
							}}
						/>
						<span>{type.label}</span>
					</label>
				{/each}
			</div>
			{#if formEventTypes.length === 0}
				<span class="form-hint form-hint--error">Select at least one event type</span>
			{/if}
		</div>

		{#if showGeofenceFilter}
			<RuleFilterCheckboxes
				name="geofence"
				label="Geofences"
				options={geofenceOptions}
				bind:selected={formGeofenceIds}
				noOptionsHint="No geofences yet. The rule applies to all geofences."
				allHint="No geofence selected: geofence events of all geofences trigger this rule."
				someHint="Only enter/exit events of the selected geofences trigger this rule."
			/>
		{/if}

		<RuleFilterCheckboxes
			name="device"
			label="Devices"
			options={deviceOptions}
			bind:selected={formDeviceIds}
			noOptionsHint="No devices yet. The rule applies to all devices."
			allHint="No device selected: events of all devices trigger this rule."
			someHint="Only events of the selected devices trigger this rule."
		/>

		<div class="form-group">
			<label for="channel" class="form-label">Channel</label>
			<select id="channel" bind:value={formChannel} class="select">
				{#each CHANNELS as ch}
					<option value={ch.value}>{ch.label}</option>
				{/each}
			</select>
		</div>

		{#if formChannel === 'command'}
			<div class="form-group">
				<label for="command-type" class="form-label">Command</label>
				<select id="command-type" bind:value={formCommandType} class="select">
					{#each NOTIFICATION_COMMAND_TYPES as type}
						<option value={type}>{COMMAND_TYPE_LABELS[type] ?? type}</option>
					{/each}
				</select>
				<span class="form-hint">
					Sent to the device that triggered the event. Offline devices receive it when they reconnect.
				</span>
				{#if commandConflict}
					<span class="form-hint form-hint--error command-conflict">{commandConflict}</span>
				{/if}
			</div>

			<CommandParamFields
				type={formCommandType}
				bind:frequency={formFrequency}
				bind:sosNumber={formSosNumber}
				bind:speed={formSpeed}
				bind:text={formText}
			/>
		{:else}
			<Input
				label="Webhook URL"
				name="webhookUrl"
				placeholder="https://example.com/webhook"
				required
				bind:value={formWebhookUrl}
			/>

			<div class="form-group">
				<span class="form-label">Headers (optional)</span>
				{#each formHeaders as header, i}
					<div class="header-row">
						<input type="text" placeholder="Key" bind:value={header.key} class="header-input field-sm" />
						<input type="text" placeholder="Value" bind:value={header.value} class="header-input field-sm" />
						<button
							type="button"
							on:click={() => removeHeader(i)}
							class="remove-btn"
							aria-label="Remove header"
						>X</button>
					</div>
				{/each}
				<Button type="button" size="sm" variant="secondary" on:click={addHeader}>
					+ Add Header
				</Button>
			</div>

			<div class="form-group">
				<label for="template" class="form-label">Message Template</label>
				<textarea id="template" bind:value={formTemplate} rows="5" class="template-textarea"></textarea>
				<div class="template-variables">
					<span class="variables-label">Available variables:</span>
					{#each TEMPLATE_VARIABLES as variable}
						<button type="button" class="variable-btn" on:click={() => insertVariable(variable)}>
							{variable}
						</button>
					{/each}
				</div>
			</div>
		{/if}

		{#if formError}
			<div class="form-error" role="alert">{formError}</div>
		{/if}
	</form>

	<svelte:fragment slot="footer">
		<Button variant="secondary" on:click={() => (showModal = false)}>Cancel</Button>
		<Button on:click={handleSubmit}>{editingRule ? 'Update' : 'Create'}</Button>
	</svelte:fragment>
</Modal>

<!-- Reporting Interval Automation Modal -->
<Modal
	open={showAutomationModal}
	title="Reporting Interval Automation"
	on:close={() => (showAutomationModal = false)}
>
	<form on:submit|preventDefault={handleCreateAutomation} class="notification-form automation-form">
		<p class="automation-intro">
			Report frequently while a device is away from a geofence and save battery once it is back,
			e.g. to track a pet that leaves home. This creates two Device Command rules for the selected
			geofence and devices: one on exit and one on enter.
		</p>
		{#if automationLoading}
			<p class="form-hint">Loading geofences and devices...</p>
		{:else if automationGeofences.length === 0}
			<p class="form-hint">
				Create a geofence (e.g. "Home") on the <a href="/geofences">Geofences page</a> first.
			</p>
		{:else if automationDevices.length === 0}
			<p class="form-hint">Add a device on the <a href="/devices">Devices page</a> first.</p>
		{:else}
			<div class="form-group">
				<label for="automation-geofence" class="form-label">Geofence</label>
				<select id="automation-geofence" bind:value={automationGeofenceId} class="select">
					{#each automationGeofences as g (g.id)}
						<option value={g.id}>{g.name}</option>
					{/each}
				</select>
			</div>
			<div class="form-group">
				<span class="form-label">Devices</span>
				<div class="event-type-grid">
					{#each automationDevices as d (d.id)}
						<label class="event-type-checkbox automation-device-checkbox">
							<input type="checkbox" value={d.id} bind:group={automationDeviceIds} />
							<span>{d.name}</span>
						</label>
					{/each}
				</div>
				<span class="form-hint">Only the selected devices change their reporting interval.</span>
			</div>
			<div class="automation-away">
				<ReportingIntervalPicker name="awayInterval" label="Interval after leaving" bind:value={automationAway} />
			</div>
			<div class="automation-home">
				<ReportingIntervalPicker name="homeInterval" label="Interval after entering" bind:value={automationHome} />
			</div>
		{/if}
		{#if automationError}
			<div class="form-error" role="alert">{automationError}</div>
		{/if}
	</form>

	<svelte:fragment slot="footer">
		<Button variant="secondary" on:click={() => (showAutomationModal = false)}>Cancel</Button>
		<Button
			loading={automationSaving}
			disabled={automationLoading || automationGeofences.length === 0 || automationDevices.length === 0}
			on:click={handleCreateAutomation}
		>Create Rules</Button>
	</svelte:fragment>
</Modal>

<style>
	.notifications-page {
		padding: var(--space-6) 0;
	}
	/* The header gains a third action; wrap instead of overflowing on phones. */
	.header-actions {
		flex-wrap: wrap;
		justify-content: flex-end;
		max-width: 100%;
	}
	.automation-intro {
		margin: 0;
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}
	.automation-form a {
		color: var(--accent-primary);
	}
	.history-link,
	.logs-link {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		padding: var(--space-2) var(--space-3);
		color: var(--text-secondary);
		text-decoration: none;
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		background-color: var(--bg-secondary);
		transition: all var(--transition-fast);
	}
	.logs-link {
		padding: var(--space-1) var(--space-3);
	}
	.logs-link:hover,
	.history-link:hover {
		color: var(--accent-primary);
		border-color: var(--accent-primary);
		background-color: var(--bg-hover);
	}
	/* Empty */

	/* Rules list */
	.rules-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
	}
	.rule-card {
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
		padding: var(--space-4);
		transition: opacity var(--transition-fast);
	}
	.rule-card.disabled {
		opacity: 0.6;
	}
	.rule-header {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		margin-bottom: var(--space-3);
	}
	.rule-info {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}
	.rule-name {
		font-size: var(--text-lg);
		font-weight: var(--font-semibold);
		color: var(--text-primary);
		margin: 0;
	}
	.rule-badges {
		display: flex;
		gap: var(--space-2);
		flex-wrap: wrap;
	}

	/* Toggle */
	.toggle-switch {
		position: relative;
		display: inline-block;
		width: 50px;
		height: 24px;
		flex-shrink: 0;
	}
	.toggle-switch input {
		opacity: 0;
		width: 0;
		height: 0;
	}
	.toggle-slider {
		position: absolute;
		cursor: pointer;
		top: 0;
		left: 0;
		right: 0;
		bottom: 0;
		background-color: var(--bg-tertiary);
		border-radius: var(--radius-full);
		transition: var(--transition-fast);
	}
	.toggle-slider::before {
		position: absolute;
		content: '';
		height: 18px;
		width: 18px;
		left: 3px;
		bottom: 3px;
		background-color: white;
		border-radius: 50%;
		transition: var(--transition-fast);
	}
	input:checked + .toggle-slider {
		background-color: var(--accent-primary);
	}
	input:checked + .toggle-slider::before {
		transform: translateX(26px);
	}

	/* Rule body */
	.rule-body {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		margin-bottom: var(--space-4);
	}
	.rule-detail {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}
	.detail-label {
		color: var(--text-secondary);
		font-size: var(--text-sm);
		flex-shrink: 0;
	}
	.detail-value {
		color: var(--text-primary);
		font-size: var(--text-sm);
	}
	.truncate {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		max-width: 400px;
	}

	/* Rule footer */
	.rule-footer {
		display: flex;
		gap: var(--space-2);
		flex-wrap: wrap;
	}

	/* Form */
	.notification-form {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
	}

	.form-label-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}

	.select-all-btn {
		background: none;
		border: none;
		color: var(--accent-primary);
		font-size: var(--text-xs);
		cursor: pointer;
		padding: 0;
	}
	.select-all-btn:hover {
		text-decoration: underline;
	}

	.notification-form :global(.event-type-grid) {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: var(--space-1) var(--space-3);
	}

	.notification-form :global(.event-type-checkbox) {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-sm);
		color: var(--text-primary);
		cursor: pointer;
		user-select: none;
	}

	.notification-form :global(.event-type-checkbox input[type="checkbox"]) {
		width: 0.9rem;
		height: 0.9rem;
		accent-color: var(--accent-primary);
		cursor: pointer;
		margin: 0;
	}

	.notification-form :global(.form-hint) {
		font-size: var(--text-xs);
		color: var(--text-secondary);
	}
	.form-hint--error {
		font-size: var(--text-xs);
		color: var(--error);
	}
	.form-error {
		border: none;
		padding: var(--space-2) var(--space-3);
		border-radius: var(--radius-md);
		background-color: color-mix(in srgb, var(--error) 12%, transparent);
		color: var(--error);
		font-size: var(--text-sm);
	}
	.notification-form :global(.form-hint--warning) {
		font-size: var(--text-xs);
		color: var(--warning);
	}

	/* Headers */
	.header-row {
		display: flex;
		gap: var(--space-2);
	}
	.header-input {
		flex: 1;
		background-color: var(--bg-secondary);
	}
	.remove-btn {
		padding: var(--space-2) var(--space-3);
		background-color: var(--error);
		border: none;
		border-radius: var(--radius-md);
		color: white;
		cursor: pointer;
		font-weight: var(--font-bold);
	}

	/* Template */
	.template-textarea {
		padding: var(--space-3);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-family: 'Courier New', monospace;
		font-size: var(--text-sm);
		resize: vertical;
	}
	.template-variables {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
		padding: var(--space-3);
		background-color: var(--bg-tertiary);
		border-radius: var(--radius-md);
	}
	.variables-label {
		width: 100%;
		font-size: var(--text-xs);
		color: var(--text-secondary);
		margin-bottom: var(--space-1);
	}
	.variable-btn {
		padding: var(--space-1) var(--space-2);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-sm);
		color: var(--accent-primary);
		font-family: 'Courier New', monospace;
		font-size: var(--text-xs);
		cursor: pointer;
		transition: all var(--transition-fast);
	}
	.variable-btn:hover {
		background-color: var(--accent-primary);
		color: var(--text-inverse);
	}
</style>

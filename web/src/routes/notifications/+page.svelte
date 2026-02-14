<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api, fetchNotifications } from '$lib/api/client';
	import { currentUser } from '$lib/stores/auth';
	import { refreshHandler } from '$lib/stores/refresh';
	import Button from '$lib/components/Button.svelte';
	import Input from '$lib/components/Input.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import StatusIndicator from '$lib/components/StatusIndicator.svelte';
	import AllDevicesToggle from '$lib/components/AllDevicesToggle.svelte';
	import {
		notificationRules,
		EVENT_TYPES,
		CHANNELS,
		TEMPLATE_VARIABLES,
		DEFAULT_TEMPLATE
	} from '$lib/stores/notifications';
	import type { NotificationRule, NotificationLog } from '$lib/stores/notifications';

	let loading = true;
	let error = '';
	let showModal = false;
	let editingRule: NotificationRule | null = null;

	// Form state
	let formName = '';
	let formEventTypes: string[] = [];
	let formChannel: 'webhook' = 'webhook';
	let formWebhookUrl = '';
	let formHeaders: Array<{ key: string; value: string }> = [];
	let formTemplate = DEFAULT_TEMPLATE;

	// Test notification state
	let testingId: number | null = null;

	// Delivery logs state
	let showLogsModal = false;
	let logsRule: NotificationRule | null = null;
	let logs: NotificationLog[] = [];
	let logsLoading = false;

	onMount(async () => {
		await loadRules();
		$refreshHandler = loadRules;
	});

	onDestroy(() => { $refreshHandler = null; });

	async function loadRules() {
		loading = true;
		error = '';
		try {
			const isAdmin = ($currentUser as Record<string, unknown> | null)?.administrator === true;
			const rules = await fetchNotifications(isAdmin);
			notificationRules.set(rules);
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
	}

	function openCreate() {
		editingRule = null;
		resetForm();
		showModal = true;
	}

	function openEdit(rule: NotificationRule) {
		editingRule = rule;
		formName = rule.name;
		formEventTypes = [...rule.eventTypes];
		formChannel = 'webhook';
		formTemplate = rule.template;

		formWebhookUrl = rule.config?.webhookUrl || '';
		formHeaders = rule.config?.headers
			? Object.entries(rule.config.headers).map(([key, value]) => ({
					key,
					value: String(value)
				}))
			: [];

		showModal = true;
	}

	function buildConfig(): Record<string, any> {
		const config: Record<string, any> = {};

		config.webhookUrl = formWebhookUrl;
		const filteredHeaders = formHeaders.filter((h) => h.key.trim());
		if (filteredHeaders.length > 0) {
			config.headers = Object.fromEntries(filteredHeaders.map((h) => [h.key, h.value]));
		}

		return config;
	}

	async function handleSubmit() {
		error = '';

		const ruleData = {
			name: formName,
			eventTypes: formEventTypes,
			channel: formChannel,
			config: buildConfig(),
			template: formTemplate,
			enabled: editingRule ? editingRule.enabled : true
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
				enabled: !rule.enabled
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
			const result = await api.testNotification(rule.id);
			if (result.status === 'sent') {
				alert('Test notification sent successfully! Check your destination.');
			} else {
				alert('Test notification failed: ' + (result.error || 'Unknown error'));
			}
		} catch (err: any) {
			error = 'Failed to send test notification';
			alert('Failed to send test notification: ' + (err.message || 'Unknown error'));
		} finally {
			testingId = null;
		}
	}

	async function viewLogs(rule: NotificationRule) {
		logsRule = rule;
		logsLoading = true;
		logs = [];
		showLogsModal = true;

		try {
			logs = await api.getNotificationLogs(rule.id);
		} catch (err: any) {
			console.error('Failed to load logs:', err);
		} finally {
			logsLoading = false;
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

	function getEventLabel(eventType: string): string {
		return EVENT_TYPES.find((e) => e.value === eventType)?.label || eventType;
	}

	function getDestination(rule: NotificationRule): string {
		return rule.config?.webhookUrl || 'No URL set';
	}

	function formatLogTime(dateStr: string): string {
		try {
			return new Date(dateStr).toLocaleString();
		} catch {
			return dateStr;
		}
	}
</script>

<svelte:head><title>Notifications - Motus</title></svelte:head>

<div class="notifications-page">
	<div class="container">
		<div class="page-header">
			<h1 class="page-title">Notification Rules</h1>
			<div class="header-actions">
				<AllDevicesToggle on:change={loadRules} />
				<a href="/notifications/history" class="history-link">
					<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
						<circle cx="12" cy="12" r="10"/>
						<polyline points="12 6 12 12 16 14"/>
					</svg>
					History
				</a>
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
		{:else if $notificationRules.length === 0}
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
				{#each $notificationRules as rule (rule.id)}
					<div class="rule-card" class:disabled={!rule.enabled} class:other-user={rule.ownerName}>
						<div class="rule-header">
							<div class="rule-info">
								<h3 class="rule-name">{rule.name}</h3>
								<div class="rule-badges">
									{#each rule.eventTypes as et}
										<span class="rule-event">{getEventLabel(et)}</span>
									{/each}
									<span class="rule-channel channel-{rule.channel}">{getChannelLabel(rule.channel)}</span>
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
								<span class="detail-label">Destination:</span>
								<span class="detail-value truncate">{getDestination(rule)}</span>
							</div>
							{#if rule.updatedAt}
								<div class="rule-detail">
									<span class="detail-label">Last updated:</span>
									<span class="detail-value">{formatLogTime(rule.updatedAt)}</span>
								</div>
							{/if}
						</div>
						<div class="rule-footer">
							<Button
								size="sm"
								variant="secondary"
								loading={testingId === rule.id}
								on:click={() => testRule(rule)}
							>Test</Button>
							<Button size="sm" variant="secondary" on:click={() => viewLogs(rule)}>Logs</Button>
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

		<div class="form-group">
			<label for="channel" class="form-label">Channel</label>
			<select id="channel" bind:value={formChannel} class="select">
				{#each CHANNELS as ch}
					<option value={ch.value}>{ch.label}</option>
				{/each}
			</select>
		</div>

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
					<input type="text" placeholder="Key" bind:value={header.key} class="header-input" />
					<input type="text" placeholder="Value" bind:value={header.value} class="header-input" />
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
	</form>

	<svelte:fragment slot="footer">
		<Button variant="secondary" on:click={() => (showModal = false)}>Cancel</Button>
		<Button on:click={handleSubmit}>{editingRule ? 'Update' : 'Create'}</Button>
	</svelte:fragment>
</Modal>

<!-- Delivery Logs Modal -->
<Modal
	open={showLogsModal}
	title={logsRule ? `Delivery Logs: ${logsRule.name}` : 'Delivery Logs'}
	on:close={() => (showLogsModal = false)}
>
	{#if logsLoading}
		<div class="logs-loading">
			<div class="spinner" aria-hidden="true"></div>
			<p>Loading delivery logs...</p>
		</div>
	{:else if logs.length === 0}
		<div class="logs-empty">
			<p>No delivery logs yet for this rule.</p>
		</div>
	{:else}
		<div class="logs-list">
			{#each logs as log (log.id)}
				<div class="log-entry" class:log-success={log.status === 'sent'} class:log-failure={log.status === 'failed'}>
					<div class="log-header">
						<StatusIndicator status={log.status === 'sent' ? 'online' : 'offline'} />
						<span class="log-status">{log.status}</span>
						<span class="log-time">{formatLogTime(log.createdAt)}</span>
					</div>
					{#if log.responseCode}
						<div class="log-detail">
							<span class="detail-label">Response code:</span>
							<span class="detail-value">{log.responseCode}</span>
						</div>
					{/if}
					{#if log.error}
						<div class="log-detail log-error-text">
							<span class="detail-label">Error:</span>
							<span class="detail-value">{log.error}</span>
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}

	<svelte:fragment slot="footer">
		<Button variant="secondary" on:click={() => (showLogsModal = false)}>Close</Button>
	</svelte:fragment>
</Modal>

<style>
	.notifications-page {
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
		margin: 0;
	}
	.header-actions {
		display: flex;
		align-items: center;
		gap: var(--space-3);
	}
	.history-link {
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
	.history-link:hover {
		color: var(--accent-primary);
		border-color: var(--accent-primary);
		background-color: var(--bg-hover);
	}

	/* Error banner */
	.error-banner {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: var(--space-3) var(--space-4);
		background-color: rgba(255, 59, 48, 0.15);
		border: 1px solid var(--error);
		border-radius: var(--radius-md);
		color: var(--error);
		margin-bottom: var(--space-4);
		font-size: var(--text-sm);
	}
	.dismiss-btn {
		background: none;
		border: none;
		color: var(--error);
		cursor: pointer;
		font-weight: var(--font-bold);
		padding: var(--space-1) var(--space-2);
	}

	/* Loading */
	.loading-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		padding: var(--space-16) var(--space-4);
		gap: var(--space-4);
	}
	.loading-state p,
	.logs-loading p {
		color: var(--text-secondary);
	}
	.spinner {
		width: 32px;
		height: 32px;
		border: 3px solid var(--border-color);
		border-top-color: var(--accent-primary);
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	/* Empty */
	.empty-state {
		text-align: center;
		padding: var(--space-16) var(--space-4);
	}
	.empty-state p {
		color: var(--text-secondary);
		margin: var(--space-4) 0;
	}
	.empty-subtitle {
		font-size: var(--text-sm);
		max-width: 400px;
		margin-left: auto;
		margin-right: auto;
	}

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
	.rule-event {
		display: inline-block;
		padding: var(--space-1) var(--space-2);
		background-color: var(--bg-tertiary);
		border-radius: var(--radius-sm);
		font-size: var(--text-xs);
		color: var(--text-secondary);
		text-transform: uppercase;
	}
	.rule-channel {
		display: inline-block;
		padding: var(--space-1) var(--space-2);
		border-radius: var(--radius-sm);
		font-size: var(--text-xs);
		font-weight: var(--font-medium);
	}
	.channel-webhook {
		background-color: rgba(0, 212, 255, 0.15);
		color: var(--accent-primary);
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
		padding: var(--space-3) var(--space-4);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-base);
	}
	.select:focus {
		outline: none;
		border-color: var(--accent-primary);
		box-shadow: 0 0 0 3px rgba(0, 212, 255, 0.1);
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
		font-size: var(--text-xs, 0.75rem);
		cursor: pointer;
		padding: 0;
	}
	.select-all-btn:hover {
		text-decoration: underline;
	}

	.event-type-grid {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: var(--space-1, 0.25rem) var(--space-3, 0.75rem);
	}

	.event-type-checkbox {
		display: flex;
		align-items: center;
		gap: var(--space-2, 0.5rem);
		font-size: var(--text-sm, 0.875rem);
		color: var(--text-primary);
		cursor: pointer;
		user-select: none;
	}

	.event-type-checkbox input[type="checkbox"] {
		width: 0.9rem;
		height: 0.9rem;
		accent-color: var(--accent-primary, #3b82f6);
		cursor: pointer;
		margin: 0;
	}

	.form-hint--error {
		font-size: var(--text-xs, 0.75rem);
		color: var(--error, #ef4444);
	}

	/* Headers */
	.header-row {
		display: flex;
		gap: var(--space-2);
	}
	.header-input {
		flex: 1;
		padding: var(--space-2) var(--space-3);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-sm);
	}
	.header-input:focus {
		outline: none;
		border-color: var(--accent-primary);
		box-shadow: 0 0 0 3px rgba(0, 212, 255, 0.1);
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
	.template-textarea:focus {
		outline: none;
		border-color: var(--accent-primary);
		box-shadow: 0 0 0 3px rgba(0, 212, 255, 0.1);
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

	/* Logs modal */
	.logs-loading {
		display: flex;
		flex-direction: column;
		align-items: center;
		padding: var(--space-8);
		gap: var(--space-3);
	}
	.logs-empty {
		text-align: center;
		padding: var(--space-8);
		color: var(--text-secondary);
	}
	.logs-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		max-height: 400px;
		overflow-y: auto;
	}
	.log-entry {
		padding: var(--space-3);
		background-color: var(--bg-secondary);
		border-radius: var(--radius-md);
		border-left: 3px solid var(--border-color);
	}
	.log-entry.log-success {
		border-left-color: var(--status-online);
	}
	.log-entry.log-failure {
		border-left-color: var(--error);
	}
	.log-header {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		margin-bottom: var(--space-2);
	}
	.log-status {
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		color: var(--text-primary);
		text-transform: capitalize;
	}
	.log-time {
		font-size: var(--text-xs);
		color: var(--text-secondary);
		margin-left: auto;
	}
	.log-error-text .detail-value {
		color: var(--error);
	}

	.rule-card.other-user {
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

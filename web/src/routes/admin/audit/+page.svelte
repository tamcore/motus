<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { currentUser } from '$lib/stores/auth';
	import { refreshHandler } from '$lib/stores/refresh';
	import { api } from '$lib/api/client';
	import { formatDate } from '$lib/utils/formatting';
	import Button from '$lib/components/Button.svelte';

	interface AuditEntry {
		id: number;
		timestamp: string;
		userId: number | null;
		action: string;
		resourceType: string | null;
		resourceId: number | null;
		details: Record<string, unknown> | null;
		ipAddress: string | null;
		userAgent: string | null;
	}

	interface AuditResponse {
		entries: AuditEntry[];
		total: number;
		limit: number;
		offset: number;
	}

	const ACTIONS = [
		{ value: '', label: 'All Actions' },
		// Session
		{ value: 'session.login', label: 'Login' },
		{ value: 'session.login_failed', label: 'Login Failed' },
		{ value: 'session.logout', label: 'Logout' },
		{ value: 'session.sudo', label: 'Sudo Start' },
		{ value: 'session.sudo_end', label: 'Sudo End' },
		// Users
		{ value: 'user.create', label: 'User Created' },
		{ value: 'user.update', label: 'User Updated' },
		{ value: 'user.delete', label: 'User Deleted' },
		// Devices
		{ value: 'device.create', label: 'Device Created' },
		{ value: 'device.update', label: 'Device Updated' },
		{ value: 'device.delete', label: 'Device Deleted' },
		{ value: 'device.online', label: 'Device Online' },
		{ value: 'device.offline', label: 'Device Offline' },
		{ value: 'device.assign', label: 'Device Assigned' },
		{ value: 'device.unassign', label: 'Device Unassigned' },
		// Geofences
		{ value: 'geofence.create', label: 'Geofence Created' },
		{ value: 'geofence.update', label: 'Geofence Updated' },
		{ value: 'geofence.delete', label: 'Geofence Deleted' },
		// Calendars
		{ value: 'calendar.create', label: 'Calendar Created' },
		{ value: 'calendar.update', label: 'Calendar Updated' },
		{ value: 'calendar.delete', label: 'Calendar Deleted' },
		// Notifications
		{ value: 'notification.create', label: 'Notification Rule Created' },
		{ value: 'notification.update', label: 'Notification Rule Updated' },
		{ value: 'notification.delete', label: 'Notification Rule Deleted' },
		{ value: 'notification.sent', label: 'Notification Sent' },
		{ value: 'notification.failed', label: 'Notification Failed' },
		// API Keys
		{ value: 'apikey.create', label: 'API Key Created' },
		{ value: 'apikey.delete', label: 'API Key Deleted' },
		// Shares
		{ value: 'share.create', label: 'Share Created' },
		{ value: 'share.delete', label: 'Share Deleted' },
	];

	const RESOURCE_TYPES = [
		{ value: '', label: 'All Resources' },
		{ value: 'session', label: 'Session' },
		{ value: 'user', label: 'User' },
		{ value: 'device', label: 'Device' },
		{ value: 'geofence', label: 'Geofence' },
		{ value: 'calendar', label: 'Calendar' },
		{ value: 'notification', label: 'Notification' },
		{ value: 'apikey', label: 'API Key' },
		{ value: 'share', label: 'Share' },
	];

	const PAGE_SIZE = 50;

	let loading = true;
	let error = '';
	let entries: AuditEntry[] = [];
	let total = 0;
	let currentPage = 0;

	// Filters
	let filterAction = '';
	let filterResourceType = '';

	// Detail expansion
	let expandedId: number | null = null;

	$: isAdmin = ($currentUser as Record<string, unknown> | null)?.administrator === true;
	$: totalPages = Math.ceil(total / PAGE_SIZE);
	$: offset = currentPage * PAGE_SIZE;

	onMount(() => {
		if (!isAdmin) {
			goto('/');
			return;
		}
		loadAudit();
		$refreshHandler = loadAudit;
	});

	onDestroy(() => { $refreshHandler = null; });

	async function loadAudit() {
		loading = true;
		error = '';
		try {
			const response = await api.getAuditLog({
				action: filterAction,
				resourceType: filterResourceType,
				limit: PAGE_SIZE,
				offset
			}) as AuditResponse;
			entries = response.entries || [];
			total = response.total || 0;
		} catch (err: any) {
			error = err.status === 403
				? 'Access denied. Admin privileges required.'
				: 'Failed to load audit log';
			console.error(err);
		} finally {
			loading = false;
		}
	}

	function handleFilterChange() {
		currentPage = 0;
		loadAudit();
	}

	function goToPage(page: number) {
		if (page < 0 || page >= totalPages) return;
		currentPage = page;
		loadAudit();
	}

	function toggleDetails(id: number) {
		expandedId = expandedId === id ? null : id;
	}

	function getActionLabel(action: string): string {
		const found = ACTIONS.find(a => a.value === action);
		return found ? found.label : action;
	}

	function getActionClass(action: string): string {
		if (action.startsWith('session.')) return 'action-auth';
		if (action.includes('delete') || action.includes('unassign')) return 'action-danger';
		if (action.includes('create') || action.includes('assign')) return 'action-create';
		if (action.includes('update')) return 'action-update';
		if (action === 'device.online' || action === 'device.offline') return 'action-device';
		if (action === 'notification.sent') return 'action-notif';
		if (action === 'notification.failed') return 'action-danger';
		return 'action-default';
	}

	function formatDetails(details: Record<string, unknown> | null): string {
		if (!details) return '(none)';
		return JSON.stringify(details, null, 2);
	}
</script>

<svelte:head><title>Audit Log - Motus</title></svelte:head>

<div class="audit-page">
	<div class="container">
		<div class="page-header">
			<div class="page-header-left">
				<h1 class="page-title">Audit Log</h1>
				{#if !loading}
					<span class="entry-count">{total} entr{total !== 1 ? 'ies' : 'y'}</span>
				{/if}
			</div>
			<Button variant="secondary" on:click={loadAudit}>Refresh</Button>
		</div>

		<!-- Filters -->
		<div class="filters-bar">
			<div class="filter-group">
				<label for="filterAction" class="filter-label">Action</label>
				<select id="filterAction" bind:value={filterAction} on:change={handleFilterChange} class="select">
					{#each ACTIONS as action}
						<option value={action.value}>{action.label}</option>
					{/each}
				</select>
			</div>
			<div class="filter-group">
				<label for="filterResource" class="filter-label">Resource Type</label>
				<select id="filterResource" bind:value={filterResourceType} on:change={handleFilterChange} class="select">
					{#each RESOURCE_TYPES as rt}
						<option value={rt.value}>{rt.label}</option>
					{/each}
				</select>
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
				<p>Loading audit log...</p>
			</div>
		{:else if entries.length === 0}
			<div class="empty-state">
				<svg viewBox="0 0 24 24" width="64" height="64" fill="none" stroke="var(--text-tertiary)" stroke-width="1">
					<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
					<polyline points="14 2 14 8 20 8"/>
					<line x1="16" y1="13" x2="8" y2="13"/>
					<line x1="16" y1="17" x2="8" y2="17"/>
					<polyline points="10 9 9 9 8 9"/>
				</svg>
				<p>No audit entries found</p>
				<p class="empty-subtitle">
					{#if filterAction || filterResourceType}
						Try adjusting your filters.
					{:else}
						Audit events will appear here as actions are performed.
					{/if}
				</p>
			</div>
		{:else}
			<div class="table-wrapper">
				<table class="audit-table">
					<thead>
						<tr>
							<th>Time</th>
							<th>Action</th>
							<th>Resource</th>
							<th>IP Address</th>
							<th class="details-col">Details</th>
						</tr>
					</thead>
					<tbody>
						{#each entries as entry (entry.id)}
							<tr>
								<td class="time-cell">
									{formatDate(entry.timestamp)}
								</td>
								<td>
									<span class="action-badge {getActionClass(entry.action)}">
										{getActionLabel(entry.action)}
									</span>
								</td>
								<td class="resource-cell">
									{#if entry.resourceType}
										<span class="resource-type">{entry.resourceType}</span>
										{#if entry.resourceId}
											<span class="resource-id">#{entry.resourceId}</span>
										{/if}
									{:else}
										<span class="text-muted">-</span>
									{/if}
								</td>
								<td class="ip-cell">
									{#if entry.ipAddress}
										<code class="ip-address">{entry.ipAddress}</code>
									{:else}
										<span class="text-muted">-</span>
									{/if}
								</td>
								<td class="details-cell">
									{#if entry.details && Object.keys(entry.details).length > 0}
										<button
											class="details-toggle"
											on:click={() => toggleDetails(entry.id)}
											aria-expanded={expandedId === entry.id}
										>
											{expandedId === entry.id ? 'Hide' : 'Show'}
										</button>
									{:else}
										<span class="text-muted">-</span>
									{/if}
								</td>
							</tr>
							{#if expandedId === entry.id && entry.details}
								<tr class="details-row">
									<td colspan="5">
										<pre class="details-content">{formatDetails(entry.details)}</pre>
									</td>
								</tr>
							{/if}
						{/each}
					</tbody>
				</table>
			</div>

			<!-- Pagination -->
			{#if totalPages > 1}
				<div class="pagination">
					<Button
						size="sm"
						variant="secondary"
						disabled={currentPage === 0}
						on:click={() => goToPage(currentPage - 1)}
					>
						Previous
					</Button>
					<span class="page-info">
						Page {currentPage + 1} of {totalPages}
					</span>
					<Button
						size="sm"
						variant="secondary"
						disabled={currentPage >= totalPages - 1}
						on:click={() => goToPage(currentPage + 1)}
					>
						Next
					</Button>
				</div>
			{/if}
		{/if}
	</div>
</div>

<style>
	.audit-page {
		padding: var(--space-6) 0;
	}

	.container {
		max-width: 1200px;
		margin: 0 auto;
		padding: 0 var(--space-4);
	}

	/* Header */
	.page-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: var(--space-4);
	}

	.page-header-left {
		display: flex;
		align-items: baseline;
		gap: var(--space-3);
	}

	.page-title {
		font-size: var(--text-3xl);
		font-weight: var(--font-bold);
		color: var(--text-primary);
		margin: 0;
	}

	.entry-count {
		font-size: var(--text-sm);
		color: var(--text-secondary);
		background-color: var(--bg-secondary);
		padding: var(--space-1) var(--space-3);
		border-radius: var(--radius-full);
		border: 1px solid var(--border-color);
	}

	/* Filters */
	.filters-bar {
		display: flex;
		gap: var(--space-4);
		margin-bottom: var(--space-6);
		padding: var(--space-4);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
	}

	.filter-group {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}

	.filter-label {
		font-size: var(--text-xs);
		font-weight: var(--font-semibold);
		color: var(--text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}

	.select {
		padding: var(--space-2) var(--space-3);
		background-color: var(--bg-primary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-sm);
		min-width: 160px;
	}

	.select:focus {
		outline: none;
		border-color: var(--accent-primary);
		box-shadow: 0 0 0 3px rgba(0, 212, 255, 0.1);
	}

	/* Error */
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

	.loading-state p {
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

	/* Empty state */
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

	/* Table */
	.table-wrapper {
		overflow-x: auto;
		border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
		background-color: var(--bg-secondary);
	}

	.audit-table {
		width: 100%;
		border-collapse: collapse;
	}

	.audit-table th {
		text-align: left;
		padding: var(--space-3) var(--space-4);
		font-size: var(--text-xs);
		font-weight: var(--font-semibold);
		color: var(--text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.5px;
		border-bottom: 1px solid var(--border-color);
		background-color: var(--bg-tertiary);
	}

	.audit-table td {
		padding: var(--space-3) var(--space-4);
		border-bottom: 1px solid var(--border-color);
		font-size: var(--text-sm);
		color: var(--text-primary);
		vertical-align: middle;
	}

	.audit-table tbody tr:last-child td {
		border-bottom: none;
	}

	.audit-table tbody tr:hover {
		background-color: var(--bg-hover);
	}

	.time-cell {
		white-space: nowrap;
		color: var(--text-secondary);
		font-size: var(--text-xs);
		font-family: monospace;
	}

	/* Action badges */
	.action-badge {
		display: inline-block;
		padding: var(--space-1) var(--space-2);
		border-radius: var(--radius-md);
		font-size: var(--text-xs);
		font-weight: var(--font-semibold);
		white-space: nowrap;
	}

	.action-auth {
		background-color: rgba(0, 122, 255, 0.12);
		color: #007aff;
	}

	.action-admin {
		background-color: rgba(255, 170, 0, 0.12);
		color: var(--warning);
	}

	.action-danger {
		background-color: rgba(255, 59, 48, 0.12);
		color: var(--error);
	}

	.action-create {
		background-color: rgba(0, 255, 136, 0.12);
		color: var(--success);
	}

	.action-update {
		background-color: rgba(0, 212, 255, 0.12);
		color: var(--accent-primary);
	}

	.action-device {
		background-color: rgba(142, 142, 147, 0.12);
		color: #8e8e93;
	}

	.action-notif {
		background-color: rgba(255, 59, 48, 0.12);
		color: var(--error);
	}

	.action-default {
		background-color: var(--bg-tertiary);
		color: var(--text-secondary);
	}

	/* Resource cell */
	.resource-cell {
		white-space: nowrap;
	}

	.resource-type {
		text-transform: capitalize;
		font-weight: var(--font-medium);
	}

	.resource-id {
		color: var(--text-secondary);
		font-size: var(--text-xs);
		margin-left: var(--space-1);
	}

	/* IP cell */
	.ip-cell {
		white-space: nowrap;
	}

	.ip-address {
		font-size: var(--text-xs);
		padding: var(--space-1) var(--space-2);
		background-color: var(--bg-tertiary);
		border-radius: var(--radius-sm);
		color: var(--text-secondary);
	}

	.text-muted {
		color: var(--text-tertiary);
	}

	/* Details */
	.details-col {
		text-align: center;
		width: 80px;
	}

	.details-cell {
		text-align: center;
	}

	.details-toggle {
		background: none;
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		padding: var(--space-1) var(--space-2);
		font-size: var(--text-xs);
		color: var(--accent-primary);
		cursor: pointer;
		transition: all var(--transition-fast);
	}

	.details-toggle:hover {
		background-color: var(--bg-hover);
		border-color: var(--accent-primary);
	}

	.details-row td {
		padding: 0 !important;
		background-color: var(--bg-tertiary);
	}

	.details-content {
		margin: 0;
		padding: var(--space-3) var(--space-4);
		font-size: var(--text-xs);
		font-family: monospace;
		color: var(--text-secondary);
		white-space: pre-wrap;
		word-break: break-all;
		max-height: 200px;
		overflow-y: auto;
	}

	/* Pagination */
	.pagination {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: var(--space-4);
		margin-top: var(--space-6);
	}

	.page-info {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}

	/* Responsive */
	@media (max-width: 768px) {
		.page-header {
			flex-direction: column;
			align-items: flex-start;
			gap: var(--space-3);
		}

		.page-header-left {
			flex-direction: column;
			align-items: flex-start;
			gap: var(--space-2);
		}

		.filters-bar {
			flex-direction: column;
		}

		.audit-table th:nth-child(4),
		.audit-table td:nth-child(4) {
			display: none;
		}

		.audit-table th:nth-child(5),
		.audit-table td:nth-child(5) {
			display: none;
		}
	}
</style>

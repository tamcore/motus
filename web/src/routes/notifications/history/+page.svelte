<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api/client';
	import { refreshHandler } from '$lib/stores/refresh';
	import { formatDate } from '$lib/utils/formatting';
	import { EVENT_TYPES } from '$lib/stores/notifications';
	import type { NotificationRule, NotificationLog } from '$lib/stores/notifications';
	import Button from '$lib/components/Button.svelte';
	import StatusIndicator from '$lib/components/StatusIndicator.svelte';

	interface EnrichedLog extends NotificationLog {
		ruleName: string;
		eventTypes: string[];
		channel: string;
	}

	let logs: EnrichedLog[] = [];
	let filteredLogs: EnrichedLog[] = [];
	let loading = true;
	let error = '';
	let statusFilter = 'all';

	$: {
		if (statusFilter === 'all') {
			filteredLogs = logs;
		} else {
			filteredLogs = logs.filter((l) => l.status === statusFilter);
		}
	}

	function getEventLabel(eventType: string): string {
		return EVENT_TYPES.find((e) => e.value === eventType)?.label || eventType;
	}

	async function loadHistory() {
		loading = true;
		error = '';
		try {
			// Fetch all notification rules first
			const rules: NotificationRule[] = await api.getNotifications();

			if (rules.length === 0) {
				logs = [];
				return;
			}

			// Fetch logs for each rule in parallel
			const logPromises = rules.map(async (rule) => {
				try {
					const ruleLogs: NotificationLog[] = await api.getNotificationLogs(rule.id);
					return ruleLogs.map((log) => ({
						...log,
						ruleName: rule.name,
						eventTypes: rule.eventTypes,
						channel: rule.channel
					}));
				} catch {
					// If fetching logs for a specific rule fails, skip it
					return [];
				}
			});

			const allLogs = await Promise.all(logPromises);
			const merged = allLogs.flat();

			// Sort by createdAt descending (most recent first)
			merged.sort((a, b) => {
				const timeA = new Date(a.createdAt).getTime();
				const timeB = new Date(b.createdAt).getTime();
				return timeB - timeA;
			});

			logs = merged;
		} catch (err: any) {
			console.error('Failed to load notification history:', err);
			error = 'Failed to load notification history';
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		loadHistory();
		$refreshHandler = loadHistory;
	});

	onDestroy(() => { $refreshHandler = null; });
</script>

<svelte:head>
	<title>Notification History - Motus</title>
</svelte:head>

<div class="history-page">
	<div class="container">
		<div class="page-header">
			<div class="header-left">
				<a href="/notifications" class="back-link">
					<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2">
						<path d="M19 12H5M12 19l-7-7 7-7"/>
					</svg>
					Back to Rules
				</a>
				<h1 class="page-title">Notification History</h1>
			</div>

			<div class="header-actions">
				<div class="filter-group">
					<label for="status-filter" class="filter-label">Status:</label>
					<select id="status-filter" bind:value={statusFilter} class="select">
						<option value="all">All</option>
						<option value="sent">Sent</option>
						<option value="failed">Failed</option>
					</select>
				</div>
				<Button variant="secondary" on:click={loadHistory}>Refresh</Button>
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
				<p>Loading notification history...</p>
			</div>
		{:else if filteredLogs.length === 0}
			<div class="empty-state">
				<svg viewBox="0 0 24 24" width="64" height="64" fill="none" stroke="var(--text-tertiary)" stroke-width="1">
					<path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" />
					<path d="M13.73 21a2 2 0 0 1-3.46 0" />
				</svg>
				{#if statusFilter !== 'all'}
					<p>No {statusFilter} notifications found</p>
					<p class="empty-subtitle">Try changing the filter or wait for notifications to be triggered.</p>
				{:else}
					<p>No notification history yet</p>
					<p class="empty-subtitle">Notification deliveries will appear here once your rules are triggered.</p>
				{/if}
			</div>
		{:else}
			<div class="results-count">
				Showing {filteredLogs.length} entr{filteredLogs.length === 1 ? 'y' : 'ies'}
			</div>

			<div class="history-table-wrapper">
				<table class="history-table">
					<thead>
						<tr>
							<th>Time</th>
							<th>Rule</th>
							<th>Event</th>
							<th>Channel</th>
							<th>Status</th>
							<th>Details</th>
						</tr>
					</thead>
					<tbody>
						{#each filteredLogs as log (log.id)}
							<tr class:row-success={log.status === 'sent'} class:row-failure={log.status === 'failed'}>
								<td class="cell-time">{formatDate(log.createdAt)}</td>
								<td class="cell-rule">{log.ruleName}</td>
								<td>
									{#each log.eventTypes as et}
										<span class="event-badge">{getEventLabel(et)}</span>
									{/each}
								</td>
								<td>
									<span class="channel-badge channel-{log.channel}">{log.channel}</span>
								</td>
								<td>
									<div class="status-cell">
										<StatusIndicator status={log.status === 'sent' ? 'online' : 'offline'} />
										<span class="status-text status-{log.status}">{log.status}</span>
									</div>
								</td>
								<td class="cell-details">
									{#if log.status === 'sent' && log.responseCode}
										<span class="response-code">HTTP {log.responseCode}</span>
									{:else if log.error}
										<span class="error-text">{log.error}</span>
									{:else}
										<span class="no-details">--</span>
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>
</div>

<style>
	.history-page {
		padding: var(--space-6) 0;
	}

	.container {
		max-width: 1200px;
		margin: 0 auto;
		padding: 0 var(--space-4);
	}

	.page-header {
		display: flex;
		justify-content: space-between;
		align-items: flex-end;
		margin-bottom: var(--space-6);
		gap: var(--space-4);
		flex-wrap: wrap;
	}

	.header-left {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	.back-link {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		color: var(--text-secondary);
		text-decoration: none;
		font-size: var(--text-sm);
		transition: color var(--transition-fast);
	}

	.back-link:hover {
		color: var(--accent-primary);
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

	.filter-group {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}

	.filter-label {
		font-size: var(--text-sm);
		color: var(--text-secondary);
		white-space: nowrap;
	}

	.select {
		padding: var(--space-2) var(--space-3);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-sm);
	}

	.select:focus {
		outline: none;
		border-color: var(--accent-primary);
		box-shadow: 0 0 0 3px rgba(0, 212, 255, 0.1);
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

	/* Results */
	.results-count {
		font-size: var(--text-sm);
		color: var(--text-tertiary);
		margin-bottom: var(--space-3);
	}

	/* Table */
	.history-table-wrapper {
		overflow-x: auto;
		border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
	}

	.history-table {
		width: 100%;
		border-collapse: collapse;
		font-size: var(--text-sm);
	}

	.history-table thead {
		background-color: var(--bg-secondary);
	}

	.history-table th {
		text-align: left;
		padding: var(--space-3) var(--space-4);
		font-weight: var(--font-semibold);
		color: var(--text-secondary);
		border-bottom: 1px solid var(--border-color);
		white-space: nowrap;
	}

	.history-table td {
		padding: var(--space-3) var(--space-4);
		border-bottom: 1px solid var(--border-color);
		color: var(--text-primary);
	}

	.history-table tbody tr {
		transition: background-color var(--transition-fast);
	}

	.history-table tbody tr:hover {
		background-color: var(--bg-hover);
	}

	.history-table tbody tr:last-child td {
		border-bottom: none;
	}

	.cell-time {
		white-space: nowrap;
		color: var(--text-secondary);
		font-family: 'Courier New', monospace;
		font-size: var(--text-xs);
	}

	.cell-rule {
		font-weight: var(--font-medium);
	}

	.event-badge {
		display: inline-block;
		padding: var(--space-1) var(--space-2);
		background-color: var(--bg-tertiary);
		border-radius: var(--radius-sm);
		font-size: var(--text-xs);
		color: var(--text-secondary);
		text-transform: uppercase;
		white-space: nowrap;
	}

	.channel-badge {
		display: inline-block;
		padding: var(--space-1) var(--space-2);
		border-radius: var(--radius-sm);
		font-size: var(--text-xs);
		font-weight: var(--font-medium);
		text-transform: capitalize;
	}

	.channel-webhook {
		background-color: rgba(0, 212, 255, 0.15);
		color: var(--accent-primary);
	}

	.status-cell {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}

	.status-text {
		text-transform: capitalize;
		font-weight: var(--font-medium);
	}

	.status-sent {
		color: var(--status-online);
	}

	.status-failed {
		color: var(--error);
	}

	.cell-details {
		max-width: 300px;
	}

	.response-code {
		font-family: 'Courier New', monospace;
		color: var(--text-secondary);
		font-size: var(--text-xs);
	}

	.error-text {
		color: var(--error);
		font-size: var(--text-xs);
		word-break: break-word;
	}

	.no-details {
		color: var(--text-tertiary);
	}

	.row-success {
		border-left: 3px solid var(--status-online);
	}

	.row-failure {
		border-left: 3px solid var(--error);
	}

	@media (max-width: 768px) {
		.page-header {
			flex-direction: column;
			align-items: flex-start;
		}

		.header-actions {
			width: 100%;
			justify-content: space-between;
		}

		.history-table {
			font-size: var(--text-xs);
		}

		.history-table th,
		.history-table td {
			padding: var(--space-2) var(--space-3);
		}
	}
</style>

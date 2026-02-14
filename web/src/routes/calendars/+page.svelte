<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api, fetchCalendars } from '$lib/api/client';
	import { currentUser } from '$lib/stores/auth';
	import { refreshHandler } from '$lib/stores/refresh';
	import Button from '$lib/components/Button.svelte';
	import CalendarEditorModal from '$lib/components/CalendarEditorModal.svelte';
	import AllDevicesToggle from '$lib/components/AllDevicesToggle.svelte';
	import type { Calendar } from '$lib/types/api';
	import { getScheduleSummary, getActiveStatus, getDateRange } from '$lib/utils/ical';
	import { formatDate } from '$lib/utils/formatting';

	let calendars: Calendar[] = [];
	let loading = true;
	let error = '';
	let showModal = false;
	let editingCalendar: Calendar | null = null;
	let saving = false;
	let deletingId: number | null = null;

	// Track active status for each calendar (checked via API)
	let activeStatuses: Record<number, { active: boolean; label: string }> = {};

	onMount(async () => {
		await loadCalendars();
		$refreshHandler = loadCalendars;
	});

	onDestroy(() => { $refreshHandler = null; });

	async function loadCalendars() {
		loading = true;
		error = '';
		try {
			const isAdmin = ($currentUser as Record<string, unknown> | null)?.administrator === true;
			calendars = await fetchCalendars(isAdmin);
			// Check active status for each calendar via client-side approximation
			const statuses: Record<number, { active: boolean; label: string }> = {};
			for (const cal of calendars) {
				statuses[cal.id] = getActiveStatus(cal.data);
			}
			activeStatuses = statuses;
		} catch (err: unknown) {
			const message = err instanceof Error ? err.message : 'Unknown error';
			error = `Failed to load calendars: ${message}`;
			console.error('Failed to load calendars:', err);
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		editingCalendar = null;
		showModal = true;
	}

	function openEdit(calendar: Calendar) {
		editingCalendar = calendar;
		showModal = true;
	}

	async function handleSave(event: CustomEvent<{ name: string; data: string }>) {
		saving = true;
		error = '';
		const { name, data } = event.detail;

		try {
			if (editingCalendar) {
				await api.updateCalendar(editingCalendar.id, { name, data });
			} else {
				await api.createCalendar({ name, data });
			}
			showModal = false;
			editingCalendar = null;
			await loadCalendars();
		} catch (err: unknown) {
			const message = err instanceof Error ? err.message : 'Unknown error';
			error = editingCalendar
				? `Failed to update calendar: ${message}`
				: `Failed to create calendar: ${message}`;
			console.error('Calendar save error:', err);
		} finally {
			saving = false;
		}
	}

	async function deleteCalendar(calendar: Calendar) {
		if (!confirm(`Are you sure you want to delete "${calendar.name}"? Geofences using this calendar will no longer have a schedule.`)) {
			return;
		}

		deletingId = calendar.id;
		error = '';

		try {
			await api.deleteCalendar(calendar.id);
			await loadCalendars();
		} catch (err: unknown) {
			const message = err instanceof Error ? err.message : 'Unknown error';
			error = `Failed to delete calendar: ${message}`;
			console.error('Calendar delete error:', err);
		} finally {
			deletingId = null;
		}
	}

	async function checkCalendarStatus(calendar: Calendar) {
		try {
			const result = await api.checkCalendar(calendar.id);
			activeStatuses = {
				...activeStatuses,
				[calendar.id]: {
					active: result.active,
					label: result.active ? 'Active now' : 'Inactive'
				}
			};
		} catch {
			// Fall back to client-side check
		}
	}

	function getCalendarStatus(calendarId: number): { active: boolean; label: string } {
		return activeStatuses[calendarId] || { active: false, label: 'Unknown' };
	}
</script>

<svelte:head><title>Calendars - Motus</title></svelte:head>

<div class="calendars-page">
	<div class="container">
		<div class="page-header">
			<div class="page-title-row">
				<svg class="page-icon" viewBox="0 0 24 24" width="28" height="28" fill="none" stroke="currentColor" stroke-width="2">
					<rect x="3" y="4" width="18" height="18" rx="2" ry="2"/>
					<line x1="16" y1="2" x2="16" y2="6"/>
					<line x1="8" y1="2" x2="8" y2="6"/>
					<line x1="3" y1="10" x2="21" y2="10"/>
				</svg>
				<h1 class="page-title">Calendars</h1>
			</div>
			<div class="page-header-actions">
				<AllDevicesToggle on:change={loadCalendars} />
				<Button on:click={openCreate}>+ Create Calendar</Button>
			</div>
		</div>

		{#if error}
			<div class="error-banner" role="alert">
				<span>{error}</span>
				<button class="dismiss-btn" on:click={() => (error = '')} aria-label="Dismiss error">
					X
				</button>
			</div>
		{/if}

		{#if loading}
			<div class="loading-state">
				<div class="spinner" aria-hidden="true"></div>
				<p>Loading calendars...</p>
			</div>
		{:else if calendars.length === 0}
			<div class="empty-state">
				<svg viewBox="0 0 24 24" width="64" height="64" fill="none" stroke="var(--text-tertiary)" stroke-width="1">
					<rect x="3" y="4" width="18" height="18" rx="2" ry="2"/>
					<line x1="16" y1="2" x2="16" y2="6"/>
					<line x1="8" y1="2" x2="8" y2="6"/>
					<line x1="3" y1="10" x2="21" y2="10"/>
				</svg>
				<p>No calendars yet</p>
				<p class="empty-subtitle">
					Create calendars to define time-based schedules for your geofences.
					For example, only trigger alerts during business hours.
				</p>
				<Button on:click={openCreate}>Create your first calendar</Button>
			</div>
		{:else}
			<div class="calendars-grid">
				{#each calendars as calendar (calendar.id)}
					{@const status = getCalendarStatus(calendar.id)}
					<div class="calendar-card" class:other-user={calendar.ownerName}>
						<div class="card-header">
							<div class="card-title-row">
								<svg class="card-icon" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2">
									<rect x="3" y="4" width="18" height="18" rx="2" ry="2"/>
									<line x1="16" y1="2" x2="16" y2="6"/>
									<line x1="8" y1="2" x2="8" y2="6"/>
									<line x1="3" y1="10" x2="21" y2="10"/>
								</svg>
								<h3 class="card-name">{calendar.name}</h3>
								{#if calendar.ownerName}
									<span class="owner-badge" title="Owned by {calendar.ownerName}">{calendar.ownerName}</span>
								{/if}
							</div>
							<button
								class="status-badge"
								class:active={status.active}
								on:click={() => checkCalendarStatus(calendar)}
								title="Click to refresh status"
								aria-label="Calendar status: {status.label}. Click to refresh."
							>
								<span class="status-dot"></span>
								<span>{status.label}</span>
							</button>
						</div>

						<div class="card-body">
							<div class="card-detail">
								<span class="detail-label">Schedule</span>
								<span class="detail-value">{getScheduleSummary(calendar.data)}</span>
							{#if getDateRange(calendar.data).start}
								<div class="date-range">
									<span class="date-label">Period:</span>
									<span>{getDateRange(calendar.data).start} to {getDateRange(calendar.data).end || "ongoing"}</span>
								</div>
							{/if}
							</div>
							<div class="card-detail">
								<span class="detail-label">Created</span>
								<span class="detail-value">{formatDate(calendar.createdAt)}</span>
							</div>
							{#if calendar.updatedAt && calendar.updatedAt !== calendar.createdAt}
								<div class="card-detail">
									<span class="detail-label">Updated</span>
									<span class="detail-value">{formatDate(calendar.updatedAt)}</span>
								</div>
							{/if}
						</div>

						<div class="card-footer">
							<Button size="sm" variant="secondary" on:click={() => openEdit(calendar)}>
								Edit
							</Button>
							<Button
								size="sm"
								variant="danger"
								loading={deletingId === calendar.id}
								on:click={() => deleteCalendar(calendar)}
							>
								Delete
							</Button>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>

<!-- Calendar Editor Modal -->
<CalendarEditorModal
	bind:open={showModal}
	calendar={editingCalendar}
	{saving}
	on:save={handleSave}
	on:close={() => {
		showModal = false;
		editingCalendar = null;
	}}
/>

<style>
	.calendars-page {
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
		margin-bottom: var(--space-6);
	}

	.page-title-row {
		display: flex;
		align-items: center;
		gap: var(--space-3);
	}

	.page-icon {
		color: var(--accent-primary);
	}

	.page-title {
		font-size: var(--text-3xl);
		font-weight: var(--font-bold);
		color: var(--text-primary);
		margin: 0;
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
		max-width: 460px;
		margin-left: auto;
		margin-right: auto;
	}

	/* Calendar grid */
	.calendars-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
		gap: var(--space-4);
	}

	/* Calendar card */
	.calendar-card {
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
		padding: var(--space-5);
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		transition: border-color var(--transition-fast);
	}

	.calendar-card:hover {
		border-color: var(--border-hover);
	}

	.card-header {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: var(--space-3);
	}

	.card-title-row {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-width: 0;
	}

	.card-icon {
		color: var(--accent-primary);
		flex-shrink: 0;
	}

	.card-name {
		font-size: var(--text-lg);
		font-weight: var(--font-semibold);
		color: var(--text-primary);
		margin: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	/* Status badge */
	.status-badge {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		padding: var(--space-1) var(--space-2);
		background-color: var(--bg-tertiary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-full);
		font-size: var(--text-xs);
		color: var(--text-secondary);
		cursor: pointer;
		transition: all var(--transition-fast);
		white-space: nowrap;
		flex-shrink: 0;
	}

	.status-badge:hover {
		background-color: var(--bg-hover);
	}

	.status-badge .status-dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background-color: var(--status-offline);
	}

	.status-badge.active {
		background-color: rgba(0, 255, 136, 0.1);
		border-color: rgba(0, 255, 136, 0.3);
		color: var(--status-online);
	}

	.status-badge.active .status-dot {
		background-color: var(--status-online);
	}

	/* Card body */
	.card-body {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		flex: 1;
	}

	.card-detail {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}

	.detail-label {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.detail-value {
		font-size: var(--text-sm);
		color: var(--text-primary);
		word-break: break-word;
	}

	.date-range {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-sm);
		color: var(--text-secondary);
		margin-top: var(--space-1);
	}

	.date-label {
		font-weight: var(--font-medium);
		color: var(--text-tertiary);
	}

	/* Card footer */
	.card-footer {
		display: flex;
		gap: var(--space-2);
		padding-top: var(--space-3);
		border-top: 1px solid var(--border-color);
	}

	/* Responsive */
	@media (max-width: 480px) {
		.page-header {
			flex-direction: column;
			align-items: flex-start;
			gap: var(--space-3);
		}

		.calendars-grid {
			grid-template-columns: 1fr;
		}
	}

	.page-header-actions {
		display: flex;
		align-items: center;
		gap: var(--space-3);
	}

	.calendar-card.other-user {
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

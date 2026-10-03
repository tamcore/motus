<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import type { Session } from '$lib/types/api';
	import { formatDate } from '$lib/utils/formatting';
	import Button from '$lib/components/Button.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';

	// ---------------------------------------------------------------------------
	// List state
	// ---------------------------------------------------------------------------
	let loading = true;
	let sessions: Session[] = [];
	let listError = '';

	// ---------------------------------------------------------------------------
	// Delete confirmation state
	// ---------------------------------------------------------------------------
	let confirmingDeleteId: string | null = null;

	// ---------------------------------------------------------------------------
	// Revoke all state
	// ---------------------------------------------------------------------------
	let confirmingRevokeAll = false;

	$: otherSessionCount = sessions.filter((s) => !s.isCurrent).length;

	// ---------------------------------------------------------------------------
	// Lifecycle
	// ---------------------------------------------------------------------------
	onMount(() => {
		loadSessions();
	});

	async function loadSessions() {
		loading = true;
		listError = '';
		try {
			sessions = await api.getSessions();
		} catch (e: unknown) {
			listError = e instanceof Error ? `Failed to load sessions: ${e.message}` : 'Failed to load sessions. Please try again.';
		} finally {
			loading = false;
		}
	}

	// ---------------------------------------------------------------------------
	// Delete
	// ---------------------------------------------------------------------------
	async function confirmDelete(id: string) {
		await api.revokeSession(id);
		sessions = sessions.filter((s) => s.id !== id);
		confirmingDeleteId = null;
	}

	// ---------------------------------------------------------------------------
	// Revoke all
	// ---------------------------------------------------------------------------
	async function confirmRevokeAll() {
		await api.revokeAllOtherSessions();
		await loadSessions();
		confirmingRevokeAll = false;
	}

	// ---------------------------------------------------------------------------
	// Helpers
	// ---------------------------------------------------------------------------
	function truncateId(id: string): string {
		if (id.length > 12) return id.substring(0, 12) + '\u2026';
		return id;
	}

	function getSessionTypeLabel(session: Session): string {
		if (session.rememberMe) return 'Persistent';
		return '24h';
	}
</script>

<section class="settings-section sessions-section">
	<div class="section-header">
		<div>
			<h2 class="section-title">Active Sessions</h2>
			<p class="section-description">
				View and manage your active login sessions. Revoking a session will immediately log it out.
			</p>
		</div>
		{#if otherSessionCount > 0}
			<Button variant="danger" size="sm" on:click={() => (confirmingRevokeAll = true)}>
				Revoke all other sessions
			</Button>
		{/if}
	</div>

	{#if loading}
		<p class="loading-text">Loading sessions...</p>
	{:else if listError}
		<div class="message error">{listError}</div>
	{:else if sessions.length === 0}
		<div class="empty-state">
			<p class="empty-title">No active sessions</p>
			<p class="empty-description">
				You have no other active sessions besides the current one.
			</p>
		</div>
	{:else}
		<div class="sessions-list">
			{#each sessions as session (session.id)}
				<div class="session-card">
					<div class="session-info">
						<div class="session-header">
							<span class="session-id" title={session.id}>{truncateId(session.id)}</span>
							{#if session.isCurrent}
								<span class="session-badge badge-current">This session</span>
							{/if}
							{#if session.apiKeyName}
								<span class="session-badge badge-apikey">via {session.apiKeyName}</span>
							{/if}
							<span
								class="session-badge"
								class:badge-persistent={session.rememberMe}
								class:badge-temporary={!session.rememberMe}
							>
								{getSessionTypeLabel(session)}
							</span>
						</div>
						<div class="session-meta">
							<span class="session-date">Created {formatDate(session.createdAt)}</span>
							<span class="meta-separator">|</span>
							<span class="session-date">Expires {formatDate(session.expiresAt)}</span>
						</div>
						{#if session.lastSeenAt || session.lastSeenIp || session.lastSeenUserAgent}
							<div class="session-last-seen">
								{#if session.lastSeenAt}
									<span class="last-seen-item">Last seen {formatDate(session.lastSeenAt)}</span>
								{/if}
								{#if session.lastSeenIp}
									{#if session.lastSeenAt}<span class="meta-separator">·</span>{/if}
									<span class="last-seen-item last-seen-ip">{session.lastSeenIp}</span>
								{/if}
								{#if session.lastSeenUserAgent}
									{#if session.lastSeenAt || session.lastSeenIp}<span class="meta-separator">·</span>{/if}
									<span class="last-seen-item last-seen-ua" title={session.lastSeenUserAgent}>
										{session.lastSeenUserAgent}
									</span>
								{/if}
							</div>
						{/if}
					</div>
					<div class="session-actions">
						{#if session.isCurrent}
							<span class="current-hint">Use logout to end</span>
						{:else}
							<Button variant="danger" size="sm" on:click={() => (confirmingDeleteId = session.id)}>
								Revoke
							</Button>
						{/if}
					</div>
				</div>
			{/each}
		</div>
	{/if}

	<!-- Revoke all confirmation -->
	{#if confirmingRevokeAll}
		<ConfirmDialog
			confirmLabel="Revoke All Other Sessions"
			busyLabel="Revoking..."
			fallbackError="Failed to revoke sessions. Please try again."
			onConfirm={confirmRevokeAll}
			onCancel={() => (confirmingRevokeAll = false)}
		>
			Are you sure you want to revoke all other sessions? Every session except this one will be
			immediately logged out.
		</ConfirmDialog>
	{/if}

	<!-- Delete confirmation -->
	{#if confirmingDeleteId !== null}
		{@const id = confirmingDeleteId}
		<ConfirmDialog
			confirmLabel="Revoke Session"
			busyLabel="Revoking..."
			fallbackError="Failed to revoke session. Please try again."
			onConfirm={() => confirmDelete(id)}
			onCancel={() => (confirmingDeleteId = null)}
		>
			Are you sure you want to revoke session <strong>{truncateId(confirmingDeleteId)}</strong>?
			That session will be immediately logged out.
		</ConfirmDialog>
	{/if}
</section>

<style>
	.sessions-section {
		margin-top: var(--space-6);
	}

	.section-header {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: var(--space-4);
		margin-bottom: var(--space-4);
	}

	.section-title {
		font-size: var(--text-xl);
		font-weight: var(--font-semibold);
		color: var(--text-primary);
		margin-bottom: var(--space-1);
	}

	.section-description {
		font-size: var(--text-sm);
		color: var(--text-tertiary);
	}

	.settings-section {
		padding: var(--space-6);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
	}

	.loading-text {
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}

	/* Empty state */
	.empty-state {
		text-align: center;
		padding: var(--space-8) var(--space-4);
	}

	.empty-title {
		font-size: var(--text-lg);
		font-weight: var(--font-semibold);
		color: var(--text-primary);
		margin-bottom: var(--space-2);
	}

	.empty-description {
		font-size: var(--text-sm);
		color: var(--text-tertiary);
		max-width: 400px;
		margin: 0 auto;
		line-height: 1.5;
	}

	/* Sessions list */
	.sessions-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}

	.session-card {
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

	.session-card:hover {
		border-color: var(--border-hover);
	}

	.session-info {
		flex: 1;
		min-width: 0;
	}

	.session-header {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		margin-bottom: var(--space-2);
		flex-wrap: wrap;
	}

	.session-id {
		font-family: 'SF Mono', 'Fira Code', 'Fira Mono', 'Roboto Mono', 'Courier New', monospace;
		font-weight: var(--font-semibold);
		color: var(--text-primary);
		font-size: var(--text-sm);
	}

	.session-badge {
		display: inline-flex;
		align-items: center;
		padding: var(--space-1) var(--space-2);
		border-radius: var(--radius-sm);
		font-size: var(--text-xs);
		font-weight: var(--font-medium);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.badge-current {
		background-color: rgba(0, 212, 255, 0.15);
		color: var(--accent-primary);
		border: 1px solid rgba(0, 212, 255, 0.3);
	}

	.badge-apikey {
		background-color: rgba(168, 85, 247, 0.15);
		color: #a855f7;
		border: 1px solid rgba(168, 85, 247, 0.3);
		text-transform: none;
	}

	.badge-persistent {
		background-color: rgba(0, 255, 136, 0.15);
		color: var(--success);
		border: 1px solid rgba(0, 255, 136, 0.3);
	}

	.badge-temporary {
		background-color: rgba(255, 170, 0, 0.15);
		color: var(--warning);
		border: 1px solid rgba(255, 170, 0, 0.3);
	}

	.session-meta {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-sm);
		color: var(--text-tertiary);
		flex-wrap: wrap;
	}

	.meta-separator {
		color: var(--border-color);
	}

	.session-last-seen {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		margin-top: var(--space-1);
		font-size: var(--text-xs);
		color: var(--text-tertiary);
		flex-wrap: wrap;
	}

	.last-seen-item {
		white-space: nowrap;
	}

	.last-seen-ua {
		overflow: hidden;
		text-overflow: ellipsis;
		max-width: 280px;
		white-space: nowrap;
		cursor: default;
	}

	.session-actions {
		flex-shrink: 0;
	}

	.current-hint {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
		font-style: italic;
	}

	/* Messages */
	.message {
		padding: var(--space-3) var(--space-4);
		border-radius: var(--radius-md);
		font-size: var(--text-sm);
	}

	.message.error {
		background-color: rgba(255, 68, 68, 0.1);
		color: var(--error);
		border: 1px solid var(--error);
	}

	/* Responsive */
	@media (max-width: 768px) {
		.session-card {
			flex-direction: column;
			align-items: stretch;
		}

		.session-actions {
			display: flex;
			justify-content: flex-end;
		}

		.session-meta {
			flex-direction: column;
			align-items: flex-start;
			gap: var(--space-1);
		}

		.meta-separator {
			display: none;
		}
	}
</style>

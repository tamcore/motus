<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import type { Session } from '$lib/types/api';
	import { formatDate } from '$lib/utils/formatting';
	import Button from '$lib/components/Button.svelte';

	let loading = true;
	let sessions: Session[] = [];
	let listError = '';

	let actionError = '';

	$: otherSessionCount = sessions.filter((s) => !s.isCurrent).length;

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

	async function revokeSession(id: string) {
		if (!confirm(`Revoke session ${truncateId(id)}? That session will be immediately logged out.`)) return;
		actionError = '';
		try {
			await api.revokeSession(id);
			sessions = sessions.filter((s) => s.id !== id);
		} catch (e: unknown) {
			actionError = e instanceof Error ? e.message : 'Failed to revoke session. Please try again.';
		}
	}

	async function revokeAll() {
		if (!confirm('Revoke all other sessions? Every session except this one will be immediately logged out.')) return;
		actionError = '';
		try {
			await api.revokeAllOtherSessions();
			await loadSessions();
		} catch (e: unknown) {
			actionError = e instanceof Error ? e.message : 'Failed to revoke sessions. Please try again.';
		}
	}

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
			<Button variant="danger" size="sm" on:click={revokeAll}>
				Revoke all other sessions
			</Button>
		{/if}
	</div>

	{#if loading}
		<p class="loading-text">Loading sessions...</p>
	{:else if listError}
		<div class="form-error">{listError}</div>
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
							<Button variant="danger" size="sm" on:click={() => revokeSession(session.id)}>
								Revoke
							</Button>
						{/if}
					</div>
				</div>
			{/each}
		</div>
	{/if}

	{#if actionError}
		<div class="form-error" role="alert">{actionError}</div>
	{/if}
</section>

<style>
	.session-header {
		flex-wrap: wrap;
	}

	.session-id {
		font-family: 'SF Mono', 'Fira Code', 'Fira Mono', 'Roboto Mono', 'Courier New', monospace;
		font-weight: var(--font-semibold);
		color: var(--text-primary);
		font-size: var(--text-sm);
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

	.current-hint {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
		font-style: italic;
	}
</style>

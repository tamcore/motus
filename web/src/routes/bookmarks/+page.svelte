<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api/client';
	import { refreshHandler } from '$lib/stores/refresh';
	import Button from '$lib/components/Button.svelte';
	import TrailBookmarkModal from '$lib/components/TrailBookmarkModal.svelte';
	import type { TrailBookmark, TrailBookmarkPayload } from '$lib/types/api';
	import {
		bookmarkDuration,
		bookmarkMapHref,
		bookmarkRangeLabel,
		filterBookmarks
	} from '$lib/utils/trail-bookmarks';

	let bookmarks: TrailBookmark[] = [];
	let loading = true;
	let error = '';
	let searchQuery = '';
	let modalOpen = false;
	let editing: TrailBookmark | null = null;
	let deletingId: number | null = null;

	$: visible = filterBookmarks(bookmarks, searchQuery);

	onMount(async () => {
		await loadBookmarks();
		$refreshHandler = loadBookmarks;
	});

	onDestroy(() => {
		$refreshHandler = null;
	});

	async function loadBookmarks() {
		loading = true;
		error = '';
		try {
			bookmarks = await api.getTrailBookmarks();
		} catch (err: unknown) {
			error = `Failed to load bookmarks: ${(err instanceof Error ? err.message : 'Unknown error')}`;
		} finally {
			loading = false;
		}
	}

	function openEdit(bookmark: TrailBookmark) {
		editing = bookmark;
		modalOpen = true;
	}

	function closeModal() {
		modalOpen = false;
		editing = null;
	}

	async function save(payload: TrailBookmarkPayload) {
		if (!editing) return;
		await api.updateTrailBookmark(editing.id, payload);
		closeModal();
		await loadBookmarks();
	}

	async function remove(bookmark: TrailBookmark) {
		if (!confirm(`Delete bookmark "${bookmark.name}"?`)) return;
		deletingId = bookmark.id;
		error = '';
		try {
			await api.deleteTrailBookmark(bookmark.id);
			bookmarks = bookmarks.filter((b) => b.id !== bookmark.id);
		} catch (err: unknown) {
			error = `Failed to delete bookmark: ${(err instanceof Error ? err.message : 'Unknown error')}`;
		} finally {
			deletingId = null;
		}
	}
</script>

<svelte:head><title>Bookmarks - Motus</title></svelte:head>

<div class="bookmarks-page">
	<div class="container">
		<div class="page-header">
			<div class="page-title-row">
				<svg class="page-icon" viewBox="0 0 24 24" width="28" height="28" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
					<path d="M19 21l-7-5-7 5V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2z" />
				</svg>
				<h1 class="page-title">Trail Bookmarks</h1>
			</div>
			{#if bookmarks.length > 0}
				<input
					type="search"
					class="bookmark-search field-sm"
					placeholder="Search bookmarks..."
					aria-label="Search bookmarks"
					bind:value={searchQuery}
				/>
			{/if}
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
				<p>Loading bookmarks...</p>
			</div>
		{:else if bookmarks.length === 0}
			<div class="empty-state">
				<p>No bookmarks yet</p>
				<p class="empty-subtitle">
					Select a device on the map, pick a trail range and choose
					<strong>Save as bookmark</strong> to keep it, e.g. for your hikes.
				</p>
				<a href="/map" class="empty-link">Go to map</a>
			</div>
		{:else if visible.length === 0}
			<p class="no-results">No bookmarks match "{searchQuery}".</p>
		{:else}
			<div class="card-grid">
				{#each visible as bookmark (bookmark.id)}
					<article class="card bookmark-card">
						<div class="card-header">
							<h3 class="card-name" title={bookmark.name}>{bookmark.name}</h3>
							{#if bookmark.deviceName}
								<span class="device-badge">{bookmark.deviceName}</span>
							{/if}
						</div>
						<div class="card-body">
							<span class="bookmark-range">{bookmarkRangeLabel(bookmark)}</span>
							<span class="bookmark-duration">{bookmarkDuration(bookmark)}</span>
							{#if bookmark.description}
								<p class="bookmark-description">{bookmark.description}</p>
							{/if}
						</div>
						<div class="card-footer">
							<a class="open-link" href={bookmarkMapHref(bookmark)}>Open on map</a>
							<Button size="sm" variant="secondary" on:click={() => openEdit(bookmark)}>Edit</Button>
							<Button
								size="sm"
								variant="danger"
								loading={deletingId === bookmark.id}
								on:click={() => remove(bookmark)}
							>
								Delete
							</Button>
						</div>
					</article>
				{/each}
			</div>
		{/if}
	</div>
</div>

<TrailBookmarkModal bind:open={modalOpen} bookmark={editing} onSave={save} onClose={closeModal} />

<style>
	.bookmarks-page {
		padding: var(--space-6) 0;
	}

	.page-header {
		gap: var(--space-3);
	}

	.page-title-row {
		display: flex;
		align-items: center;
		gap: var(--space-3);
	}

	.page-icon {
		color: var(--accent-primary);
	}

	.bookmark-search {
		background-color: var(--bg-secondary);
		min-width: 220px;
	}

	.no-results {
		color: var(--text-secondary);
	}

	.open-link {
		color: var(--accent-primary);
		text-decoration: none;
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
	}

	.open-link:hover {
		text-decoration: underline;
	}

	.card-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: var(--space-2);
	}

	.device-badge {
		font-size: var(--text-xs);
		padding: 0.1rem 0.5rem;
		border-radius: var(--radius-full);
		background-color: var(--bg-tertiary);
		border: 1px solid var(--border-color);
		color: var(--text-secondary);
		white-space: nowrap;
		flex-shrink: 0;
	}

	.card-body {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		flex: 1;
	}

	.bookmark-range {
		font-size: var(--text-sm);
		color: var(--text-primary);
	}

	.bookmark-duration {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
	}

	.bookmark-description {
		margin: var(--space-2) 0 0;
		font-size: var(--text-sm);
		color: var(--text-secondary);
		white-space: pre-wrap;
		word-break: break-word;
	}

	.card-footer {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		padding-top: var(--space-3);
		border-top: 1px solid var(--border-color);
	}

	.card-footer .open-link {
		margin-right: auto;
	}

	@media (max-width: 480px) {
		.page-header {
			flex-direction: column;
			align-items: flex-start;
		}

		.bookmark-search {
			width: 100%;
			min-width: 0;
		}
	}
</style>

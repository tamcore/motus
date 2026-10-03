<script lang="ts">
	import type { TrailBookmark } from '$lib/types/api';
	import type { TrailRange } from '$lib/utils/trail-range';
	import { bookmarkMatchesRange, bookmarkRangeLabel } from '$lib/utils/trail-bookmarks';

	/** Bookmarks of the selected device (newest first). */
	export let bookmarks: TrailBookmark[];
	/** Currently applied trail range; the matching bookmark is highlighted. */
	export let activeRange: TrailRange;
	export let loading = false;
	export let error = '';
	export let onOpen: (bookmark: TrailBookmark) => void;
	export let onEdit: (bookmark: TrailBookmark) => void;
	export let onDelete: (bookmark: TrailBookmark) => void;
</script>

<section class="trail-bookmarks" aria-label="Trail bookmarks">
	<div class="trail-bookmarks-header">
		<span class="trail-bookmarks-title">Bookmarks</span>
		<a href="/bookmarks" class="trail-bookmarks-all">All bookmarks</a>
	</div>

	{#if error}
		<p class="trail-bookmarks-error" role="alert">{error}</p>
	{:else if loading && bookmarks.length === 0}
		<p class="trail-bookmarks-empty">Loading bookmarks…</p>
	{:else if bookmarks.length === 0}
		<p class="trail-bookmarks-empty">No bookmarks for this device yet.</p>
	{:else}
		<ul class="trail-bookmark-list">
			{#each bookmarks as bookmark (bookmark.id)}
				<li
					class="trail-bookmark-item"
					class:active={bookmarkMatchesRange(bookmark, activeRange)}
				>
					<button
						type="button"
						class="trail-bookmark-open"
						aria-label="Show bookmark {bookmark.name}"
						title={bookmark.description || bookmark.name}
						on:click={() => onOpen(bookmark)}
					>
						<span class="trail-bookmark-name">{bookmark.name}</span>
						<span class="trail-bookmark-range">{bookmarkRangeLabel(bookmark)}</span>
					</button>
					<button
						type="button"
						class="trail-bookmark-action"
						aria-label="Edit bookmark {bookmark.name}"
						title="Edit"
						on:click={() => onEdit(bookmark)}
					>
						&#9998;
					</button>
					<button
						type="button"
						class="trail-bookmark-action trail-bookmark-action--danger"
						aria-label="Delete bookmark {bookmark.name}"
						title="Delete"
						on:click={() => onDelete(bookmark)}
					>
						&#x2715;
					</button>
				</li>
			{/each}
		</ul>
	{/if}
</section>

<style>
	.trail-bookmarks {
		margin-top: var(--space-3);
		border-top: 1px solid var(--border-color);
		padding-top: var(--space-2);
	}

	.trail-bookmarks-header {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		margin-bottom: var(--space-1);
	}

	.trail-bookmarks-title {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
	}

	.trail-bookmarks-all {
		font-size: var(--text-xs);
		color: var(--accent-primary);
		text-decoration: none;
	}

	.trail-bookmarks-all:hover {
		text-decoration: underline;
	}

	.trail-bookmarks-empty,
	.trail-bookmarks-error {
		margin: 0;
		font-size: var(--text-xs);
		color: var(--text-tertiary);
	}

	.trail-bookmarks-error {
		color: var(--error, #ff4444);
	}

	.trail-bookmark-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 2px;
		max-height: 12rem;
		overflow-y: auto;
	}

	.trail-bookmark-item {
		display: flex;
		align-items: center;
		gap: var(--space-1);
		border-radius: var(--radius-md);
		border-left: 3px solid transparent;
	}

	.trail-bookmark-item:hover {
		background-color: var(--bg-hover);
	}

	.trail-bookmark-item.active {
		background-color: var(--bg-active);
		border-left-color: var(--accent-primary);
	}

	.trail-bookmark-open {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		padding: var(--space-1) var(--space-2);
		background: none;
		border: none;
		cursor: pointer;
		text-align: left;
	}

	.trail-bookmark-name {
		font-size: var(--text-sm);
		color: var(--text-primary);
		font-weight: var(--font-medium);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		max-width: 100%;
	}

	.trail-bookmark-range {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
	}

	.trail-bookmark-action {
		background: none;
		border: none;
		color: var(--text-tertiary);
		cursor: pointer;
		padding: var(--space-1);
		font-size: var(--text-sm);
		line-height: 1;
	}

	.trail-bookmark-action:hover {
		color: var(--text-primary);
	}

	.trail-bookmark-action--danger:hover {
		color: var(--error, #ff4444);
	}
</style>

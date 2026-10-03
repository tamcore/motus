<script lang="ts">
	import Modal from './Modal.svelte';
	import Button from './Button.svelte';
	import type { TrailBookmark, TrailBookmarkPayload } from '$lib/types/api';
	import { rangeToInputs, type TrailRange } from '$lib/utils/trail-range';
	import {
		BOOKMARK_DESCRIPTION_MAX,
		BOOKMARK_NAME_MAX,
		bookmarkOriginalFromRange,
		buildBookmarkPayload,
		charCount,
		type BookmarkRangeOriginal
	} from '$lib/utils/trail-bookmarks';

	export let open = false;
	/** Bookmark being edited; null saves a new bookmark for `deviceId`/`range`. */
	export let bookmark: TrailBookmark | null = null;
	/** Device of a new bookmark (ignored when editing). */
	export let deviceId: number | null = null;
	/** Range to prefill for a new bookmark (relative presets are frozen to now). */
	export let range: TrailRange | null = null;
	/** Persists the bookmark; a rejection is shown in the dialog. */
	export let onSave: (payload: TrailBookmarkPayload) => Promise<void>;
	export let onClose: () => void;

	let name = '';
	let description = '';
	let fromDate = '';
	let fromTime = '';
	let toDate = '';
	let toTime = '';
	// Exact boundaries the form was prefilled with; kept when left unchanged.
	let original: BookmarkRangeOriginal | null = null;
	let error = '';
	let saving = false;

	$: title = bookmark ? 'Edit trail bookmark' : 'Save trail bookmark';
	$: nameLength = charCount(name.trim());
	$: descriptionLength = charCount(description.trim());

	function reset() {
		name = bookmark?.name ?? '';
		description = bookmark?.description ?? '';
		// A new bookmark freezes relative presets to concrete boundaries ending now.
		original = bookmark
			? { from: bookmark.from, to: bookmark.to }
			: range
				? bookmarkOriginalFromRange(range)
				: null;
		const fields = original
			? rangeToInputs({ preset: 'custom', ...original })
			: { fromDate: '', fromTime: '', toDate: '', toTime: '' };
		({ fromDate, fromTime, toDate, toTime } = fields);
		error = '';
		saving = false;
	}

	// Re-initialise the form whenever the dialog opens.
	let wasOpen = false;
	$: if (open !== wasOpen) {
		wasOpen = open;
		if (open) reset();
	}

	async function submit() {
		const targetDevice = bookmark?.deviceId ?? deviceId;
		if (targetDevice == null) {
			error = 'No device selected';
			return;
		}
		const result = buildBookmarkPayload({
			deviceId: targetDevice,
			name,
			description,
			fromDate,
			fromTime,
			toDate,
			toTime
		}, original);
		if (!result.ok) {
			error = result.error;
			return;
		}
		error = '';
		saving = true;
		try {
			await onSave(result.payload);
		} catch (err: unknown) {
			error = (err instanceof Error ? err.message : 'Failed to save bookmark');
		} finally {
			saving = false;
		}
	}
</script>

<Modal bind:open {title} on:close={onClose}>
	<form class="bookmark-form" on:submit|preventDefault={submit} novalidate>
		{#if bookmark?.deviceName}
			<p class="bookmark-device">Device: <strong>{bookmark.deviceName}</strong></p>
		{/if}

		<label class="bookmark-label" for="bookmark-name">Name <span aria-hidden="true">*</span></label>
		<!-- No maxlength: it counts UTF-16 units, the limit is in characters. -->
		<input
			id="bookmark-name"
			class="bookmark-input"
			type="text"
			placeholder="e.g. Zugspitze via Höllental"
			autocomplete="off"
			required
			aria-describedby="bookmark-name-count"
			bind:value={name}
		/>
		<span
			id="bookmark-name-count"
			class="bookmark-count"
			class:bookmark-count--over={nameLength > BOOKMARK_NAME_MAX}
		>
			{nameLength}/{BOOKMARK_NAME_MAX} characters
		</span>

		<label class="bookmark-label" for="bookmark-description">Description</label>
		<textarea
			id="bookmark-description"
			class="bookmark-input bookmark-textarea"
			rows="3"
			placeholder="Optional notes"
			aria-describedby="bookmark-description-count"
			bind:value={description}
		></textarea>
		<span
			id="bookmark-description-count"
			class="bookmark-count"
			class:bookmark-count--over={descriptionLength > BOOKMARK_DESCRIPTION_MAX}
		>
			{descriptionLength}/{BOOKMARK_DESCRIPTION_MAX} characters
		</span>

		<span class="bookmark-label">From</span>
		<div class="bookmark-datetime">
			<input
				id="bookmark-from-date"
				class="bookmark-input"
				type="date"
				aria-label="Bookmark from date"
				bind:value={fromDate}
			/>
			<input
				id="bookmark-from-time"
				class="bookmark-input bookmark-input--time"
				type="time"
				aria-label="Bookmark from time (optional)"
				bind:value={fromTime}
			/>
		</div>

		<span class="bookmark-label">To</span>
		<div class="bookmark-datetime">
			<input
				id="bookmark-to-date"
				class="bookmark-input"
				type="date"
				aria-label="Bookmark to date"
				bind:value={toDate}
			/>
			<input
				id="bookmark-to-time"
				class="bookmark-input bookmark-input--time"
				type="time"
				aria-label="Bookmark to time (optional)"
				bind:value={toTime}
			/>
		</div>
		<span class="bookmark-hint">Time is optional; empty means the whole day.</span>

		{#if error}
			<div class="bookmark-error" role="alert">{error}</div>
		{/if}

		<div class="bookmark-actions">
			<Button variant="secondary" on:click={onClose}>Cancel</Button>
			<Button type="submit" loading={saving}>{bookmark ? 'Save changes' : 'Save bookmark'}</Button>
		</div>
	</form>
</Modal>

<style>
	.bookmark-form {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	.bookmark-device {
		font-size: var(--text-sm);
		color: var(--text-secondary);
		margin: 0;
	}

	.bookmark-label {
		font-size: var(--text-sm);
		color: var(--text-secondary);
		margin-top: var(--space-1);
	}

	.bookmark-input {
		padding: var(--space-2) var(--space-3);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-sm);
		width: 100%;
		min-width: 0;
		font-family: inherit;
	}

	.bookmark-input:focus {
		outline: none;
		border-color: var(--accent-primary);
	}

	.bookmark-textarea {
		resize: vertical;
	}

	.bookmark-datetime {
		display: flex;
		gap: var(--space-2);
	}

	.bookmark-input--time {
		flex: 0 0 7rem;
		width: 7rem;
	}

	.bookmark-count {
		align-self: flex-end;
		font-size: var(--text-xs);
		color: var(--text-tertiary);
	}

	.bookmark-count--over {
		color: var(--error, #ff4444);
	}

	.bookmark-hint {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
	}

	.bookmark-error {
		font-size: var(--text-sm);
		color: var(--error, #ff4444);
	}

	.bookmark-actions {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-2);
		margin-top: var(--space-3);
	}
</style>

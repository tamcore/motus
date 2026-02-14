<script lang="ts">
	import { onMount, createEventDispatcher } from 'svelte';
	import { api } from '$lib/api/client';
	import { formatDate } from '$lib/utils/formatting';
	import Button from './Button.svelte';
	import Modal from './Modal.svelte';

	export let deviceId: number;
	export let deviceName: string;
	export let open = false;

	interface Share {
		id: number;
		deviceId: number;
		token: string;
		createdBy: number;
		expiresAt: string | null;
		createdAt: string;
	}

	const dispatch = createEventDispatcher();

	let expiry = '24h';
	let shareLink = '';
	let activeShares: Share[] = [];
	let creating = false;
	let loadingShares = false;
	let copied = false;
	let error = '';

	const expiryOptions: { value: string; label: string; hours: number | null }[] = [
		{ value: '1h', label: '1 Hour', hours: 1 },
		{ value: '24h', label: '24 Hours', hours: 24 },
		{ value: '7d', label: '7 Days', hours: 168 },
		{ value: 'never', label: 'Never', hours: null },
	];

	$: if (open) {
		loadShares();
		shareLink = '';
		error = '';
		copied = false;
	}

	async function loadShares() {
		loadingShares = true;
		try {
			activeShares = (await api.listDeviceShares(deviceId)) || [];
		} catch (err) {
			console.error('Failed to load shares:', err);
			activeShares = [];
		} finally {
			loadingShares = false;
		}
	}

	async function createShare() {
		creating = true;
		error = '';
		copied = false;
		try {
			const option = expiryOptions.find((o) => o.value === expiry);
			const hours = option?.hours ?? null;
			const share = await api.createDeviceShare(deviceId, hours);
			shareLink = `${window.location.origin}${share.shareUrl || '/share/' + share.token}`;
			await loadShares();
		} catch (err) {
			error = 'Failed to create share link';
			console.error(err);
		} finally {
			creating = false;
		}
	}

	async function copyLink() {
		try {
			await navigator.clipboard.writeText(shareLink);
			copied = true;
			setTimeout(() => { copied = false; }, 2000);
		} catch {
			// Fallback: select the input text
			const input = document.querySelector('.share-link-input') as HTMLInputElement;
			if (input) {
				input.select();
				document.execCommand('copy');
				copied = true;
				setTimeout(() => { copied = false; }, 2000);
			}
		}
	}

	async function revokeShare(shareId: number) {
		if (!confirm('Are you sure you want to revoke this share link?')) return;
		try {
			await api.deleteShare(shareId);
			await loadShares();
		} catch (err) {
			error = 'Failed to revoke share';
			console.error(err);
		}
	}

	function handleClose() {
		open = false;
		dispatch('close');
	}

	function formatExpiry(expiresAt: string | null): string {
		if (!expiresAt) return 'Never';
		const date = new Date(expiresAt);
		if (date < new Date()) return 'Expired';
		return formatDate(expiresAt);
	}

	function isExpired(expiresAt: string | null): boolean {
		if (!expiresAt) return false;
		return new Date(expiresAt) < new Date();
	}
</script>

<Modal {open} title="Share {deviceName}" on:close={handleClose}>
	<div class="share-form">
		<div class="form-row">
			<div class="form-group">
				<label for="share-expiry" class="form-label">Expiry</label>
				<select id="share-expiry" bind:value={expiry} class="select">
					{#each expiryOptions as option}
						<option value={option.value}>{option.label}</option>
					{/each}
				</select>
			</div>
			<div class="form-action">
				<Button on:click={createShare} loading={creating}>Generate Link</Button>
			</div>
		</div>

		{#if error}
			<div class="error-msg" role="alert">{error}</div>
		{/if}

		{#if shareLink}
			<div class="share-link">
				<input class="share-link-input" readonly value={shareLink} on:click|preventDefault={(e) => e.currentTarget.select()} />
				<Button variant="secondary" on:click={copyLink}>
					{copied ? 'Copied' : 'Copy'}
				</Button>
			</div>
		{/if}

		<div class="shares-section">
			<h4 class="shares-heading">
				Active Shares
				{#if !loadingShares}
					<span class="share-count">({activeShares.length})</span>
				{/if}
			</h4>

			{#if loadingShares}
				<p class="shares-loading">Loading...</p>
			{:else if activeShares.length === 0}
				<p class="shares-empty">No active share links</p>
			{:else}
				<div class="shares-list">
					{#each activeShares as share (share.id)}
						<div class="share-item" class:expired={isExpired(share.expiresAt)}>
							<div class="share-info">
								<span class="share-token" title={share.token}>
									...{share.token.slice(-8)}
								</span>
								<span class="share-expiry" class:expired={isExpired(share.expiresAt)}>
									{formatExpiry(share.expiresAt)}
								</span>
							</div>
							<button class="revoke-btn" on:click={() => revokeShare(share.id)} title="Revoke share">
								Revoke
							</button>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	</div>

	<svelte:fragment slot="footer">
		<Button variant="secondary" on:click={handleClose}>Close</Button>
	</svelte:fragment>
</Modal>

<style>
	.share-form {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
	}

	.form-row {
		display: flex;
		align-items: flex-end;
		gap: var(--space-3);
	}

	.form-group {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		flex: 1;
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

	.form-action {
		flex-shrink: 0;
	}

	.error-msg {
		padding: var(--space-3);
		background-color: rgba(255, 59, 48, 0.15);
		border: 1px solid var(--error);
		border-radius: var(--radius-md);
		color: var(--error);
		font-size: var(--text-sm);
	}

	.share-link {
		display: flex;
		gap: var(--space-2);
		padding: var(--space-3);
		background-color: var(--bg-tertiary);
		border-radius: var(--radius-md);
	}

	.share-link-input {
		flex: 1;
		padding: var(--space-2) var(--space-3);
		background-color: var(--bg-primary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-sm);
		color: var(--text-primary);
		font-size: var(--text-sm);
		font-family: monospace;
	}

	.shares-section {
		border-top: 1px solid var(--border-color);
		padding-top: var(--space-4);
	}

	.shares-heading {
		font-size: var(--text-sm);
		font-weight: var(--font-semibold);
		color: var(--text-primary);
		margin: 0 0 var(--space-3) 0;
	}

	.share-count {
		color: var(--text-secondary);
		font-weight: var(--font-normal);
	}

	.shares-loading,
	.shares-empty {
		font-size: var(--text-sm);
		color: var(--text-secondary);
		margin: 0;
	}

	.shares-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		max-height: 200px;
		overflow-y: auto;
	}

	.share-item {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: var(--space-2) var(--space-3);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
	}

	.share-item.expired {
		opacity: 0.5;
	}

	.share-info {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.share-token {
		font-size: var(--text-xs);
		font-family: monospace;
		color: var(--text-secondary);
	}

	.share-expiry {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
	}

	.share-expiry.expired {
		color: var(--error);
	}

	.revoke-btn {
		padding: var(--space-1) var(--space-3);
		background: none;
		border: 1px solid var(--error);
		border-radius: var(--radius-sm);
		color: var(--error);
		font-size: var(--text-xs);
		cursor: pointer;
		transition: all var(--transition-fast);
	}

	.revoke-btn:hover {
		background-color: rgba(255, 59, 48, 0.1);
	}
</style>

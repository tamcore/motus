<script lang="ts">
	import { onMount } from 'svelte';
	import { api, APIError } from '$lib/api/client';
	import type { ApiKey } from '$lib/types/api';
	import { formatDate } from '$lib/utils/formatting';
	import Button from '$lib/components/Button.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import Input from '$lib/components/Input.svelte';
	import QrCodeDialog from '$lib/components/QrCodeDialog.svelte';

	/** Show Home Assistant usage instructions below the key list */
	export let showUsageInstructions: boolean = true;

	// ---------------------------------------------------------------------------
	// List state
	// ---------------------------------------------------------------------------
	let loading = true;
	let apiKeys: ApiKey[] = [];
	let listError = '';

	// ---------------------------------------------------------------------------
	// Create modal state
	// ---------------------------------------------------------------------------
	let showCreateModal = false;
	let newKeyName = '';
	let newKeyPermissions = 'full';
	let newKeyExpiration = '168'; // Default: 7 days (168 hours)
	let newKeyCustomDate = '';
	let creating = false;
	let createdToken = '';
	let createError = '';
	let tokenCopied = false;
	let tokenInputEl: HTMLInputElement;

	// ---------------------------------------------------------------------------
	// Delete confirmation state
	// ---------------------------------------------------------------------------
	let confirmingDeleteId: number | null = null;
	let deleting = false;
	let deleteError = '';

	// ---------------------------------------------------------------------------
	// QR Code dialog state
	// ---------------------------------------------------------------------------
	let showQrDialog = false;
	let qrToken = '';

	// ---------------------------------------------------------------------------
	// Lifecycle
	// ---------------------------------------------------------------------------
	onMount(() => {
		loadKeys();
	});

	async function loadKeys() {
		loading = true;
		listError = '';
		try {
			apiKeys = await api.getApiKeys();
		} catch (e: unknown) {
			if (e instanceof APIError) {
				listError = `Failed to load API keys: ${e.message}`;
			} else if (e instanceof Error) {
				listError = `Failed to load API keys: ${e.message}`;
			} else {
				listError = 'Failed to load API keys. Please try again.';
			}
		} finally {
			loading = false;
		}
	}

	// ---------------------------------------------------------------------------
	// Create
	// ---------------------------------------------------------------------------
	function openCreateModal() {
		showCreateModal = true;
		newKeyName = '';
		newKeyPermissions = 'full';
		newKeyExpiration = '168';
		newKeyCustomDate = '';
		createdToken = '';
		createError = '';
		tokenCopied = false;
	}

	function closeCreateModal() {
		showCreateModal = false;
		newKeyName = '';
		newKeyPermissions = 'full';
		newKeyExpiration = '168';
		newKeyCustomDate = '';
		createdToken = '';
		createError = '';
		tokenCopied = false;
	}

	async function handleCreate() {
		const trimmedName = newKeyName.trim();
		if (!trimmedName) {
			createError = 'Name is required.';
			return;
		}

		// Validate custom date if selected.
		if (newKeyExpiration === 'custom' && !newKeyCustomDate) {
			createError = 'Please select an expiration date.';
			return;
		}
		if (newKeyExpiration === 'custom' && newKeyCustomDate) {
			const selected = new Date(newKeyCustomDate);
			if (selected <= new Date()) {
				createError = 'Expiration date must be in the future.';
				return;
			}
		}

		creating = true;
		createError = '';

		// Build expiration payload.
		let expiresInHours: number | null = null;
		let expiresAt: string | null = null;
		if (newKeyExpiration === 'custom') {
			// Convert local date input to RFC 3339.
			expiresAt = new Date(newKeyCustomDate).toISOString();
		} else if (newKeyExpiration !== 'never') {
			expiresInHours = parseInt(newKeyExpiration, 10);
		}

		try {
			const result = await api.createApiKey({
				name: trimmedName,
				permissions: newKeyPermissions,
				...(expiresInHours !== null ? { expiresInHours } : {}),
				...(expiresAt !== null ? { expiresAt } : {}),
			});

			// Show the full token (only available now)
			createdToken = result.token;

			// Add the key to the local list with a redacted token
			const redactedKey: ApiKey = {
				...result,
				token: result.token.length > 8
					? result.token.substring(0, 8) + '...'
					: result.token,
			};
			apiKeys = [...apiKeys, redactedKey];

			// Auto-select the token for easy copying
			setTimeout(() => {
				if (tokenInputEl) {
					tokenInputEl.select();
				}
			}, 50);
		} catch (e: unknown) {
			if (e instanceof APIError) {
				createError = e.message;
			} else if (e instanceof Error) {
				createError = e.message;
			} else {
				createError = 'Failed to create API key. Please try again.';
			}
		} finally {
			creating = false;
		}
	}

	// ---------------------------------------------------------------------------
	// Copy token
	// ---------------------------------------------------------------------------
	async function copyToken() {
		if (!createdToken) return;

		try {
			await navigator.clipboard.writeText(createdToken);
			tokenCopied = true;
			setTimeout(() => {
				tokenCopied = false;
			}, 2000);
		} catch {
			// Fallback: select text for manual copy
			if (tokenInputEl) {
				tokenInputEl.select();
				document.execCommand('copy');
				tokenCopied = true;
				setTimeout(() => {
					tokenCopied = false;
				}, 2000);
			}
		}
	}

	// ---------------------------------------------------------------------------
	// Delete
	// ---------------------------------------------------------------------------
	function requestDelete(id: number) {
		confirmingDeleteId = id;
		deleteError = '';
	}

	function cancelDelete() {
		confirmingDeleteId = null;
		deleteError = '';
	}

	async function confirmDelete() {
		if (confirmingDeleteId === null) return;

		deleting = true;
		deleteError = '';

		try {
			await api.deleteApiKey(confirmingDeleteId);
			apiKeys = apiKeys.filter((k) => k.id !== confirmingDeleteId);
			confirmingDeleteId = null;
		} catch (e: unknown) {
			if (e instanceof APIError) {
				deleteError = e.message;
			} else if (e instanceof Error) {
				deleteError = e.message;
			} else {
				deleteError = 'Failed to revoke API key. Please try again.';
			}
		} finally {
			deleting = false;
		}
	}

	// ---------------------------------------------------------------------------
	// QR Code
	// ---------------------------------------------------------------------------
	function openQrDialog(token: string) {
		qrToken = token;
		showQrDialog = true;
	}

	function closeQrDialog() {
		showQrDialog = false;
		qrToken = '';
	}

	// ---------------------------------------------------------------------------
	// Helpers
	// ---------------------------------------------------------------------------
	function formatLastUsed(lastUsedAt: string | null | undefined): string {
		if (!lastUsedAt) return 'Never';
		return formatDate(lastUsedAt);
	}

	function getPermissionLabel(perm: string): string {
		return perm === 'full' ? 'Full Access' : 'Read-Only';
	}

	function isExpired(expiresAt: string | null | undefined): boolean {
		if (!expiresAt) return false;
		return new Date(expiresAt) < new Date();
	}

	function formatExpiration(expiresAt: string | null | undefined): string {
		if (!expiresAt) return 'Never';
		return formatDate(expiresAt);
	}

	/** Returns a minimum date string for the custom date picker (tomorrow). */
	function getMinDate(): string {
		const tomorrow = new Date();
		tomorrow.setDate(tomorrow.getDate() + 1);
		return tomorrow.toISOString().split('T')[0];
	}

	$: confirmingKeyName = confirmingDeleteId !== null
		? apiKeys.find((k) => k.id === confirmingDeleteId)?.name ?? 'this key'
		: '';
</script>

<section class="settings-section api-keys-section">
	<div class="section-header">
		<div>
			<h2 class="section-title">API Keys</h2>
			<p class="section-description">
				Manage API keys for external integrations. Each key can have different permission levels.
			</p>
		</div>
		<Button variant="primary" size="sm" on:click={openCreateModal}>
			Create API Key
		</Button>
	</div>

	{#if loading}
		<p class="loading-text">Loading API keys...</p>
	{:else if listError}
		<div class="message error">{listError}</div>
	{:else if apiKeys.length === 0}
		<div class="empty-state">
			<p class="empty-title">No API keys</p>
			<p class="empty-description">
				Create an API key to authenticate external integrations such as Home Assistant, Grafana, or custom scripts.
			</p>
		</div>
	{:else}
		<div class="keys-list">
			{#each apiKeys as key (key.id)}
				<div class="key-card" class:key-expired={isExpired(key.expiresAt)}>
					<div class="key-info">
						<div class="key-header">
							<span class="key-name">{key.name}</span>
							<span
								class="permission-badge"
								class:badge-full={key.permissions === 'full'}
								class:badge-readonly={key.permissions === 'readonly'}
							>
								{getPermissionLabel(key.permissions)}
							</span>
							{#if isExpired(key.expiresAt)}
								<span class="permission-badge badge-expired">Expired</span>
							{/if}
						</div>
						<div class="key-meta">
							<span class="key-token" title="Redacted token">{key.token}</span>
							<span class="meta-separator">|</span>
							<span class="key-date">Created {formatDate(key.createdAt)}</span>
							<span class="meta-separator">|</span>
							<span class="key-last-used">Last used: {formatLastUsed(key.lastUsedAt)}</span>
							<span class="meta-separator">|</span>
							<span class="key-expires" class:text-expired={isExpired(key.expiresAt)}>
								Expires: {formatExpiration(key.expiresAt)}
							</span>
						</div>
					</div>
					<div class="key-actions">
						<Button variant="danger" size="sm" on:click={() => requestDelete(key.id)}>
							Revoke
						</Button>
					</div>
				</div>
			{/each}
		</div>
	{/if}

	<!-- Home Assistant usage instructions -->
	{#if showUsageInstructions}
		<details class="usage-details">
			<summary class="usage-summary">Usage instructions for Home Assistant</summary>
			<div class="usage-content">
				<p>To connect Motus with Home Assistant using the Traccar integration:</p>
				<ol class="usage-steps">
					<li>Create a <strong>Full Access</strong> API key above (or use a Read-Only key if you only need tracking data).</li>
					<li>In Home Assistant, go to <strong>Settings &rarr; Devices & Services</strong>.</li>
					<li>Click <strong>Add Integration</strong> and search for <strong>Traccar Server</strong>.</li>
					<li>Enter the Motus server URL (e.g., <code>https://your-motus-server.com</code>).</li>
					<li>Paste the API key token into the token field.</li>
					<li>Your tracked devices will appear as entities in Home Assistant.</li>
				</ol>
				<p class="usage-note">
					The API key authenticates via the <code>GET /api/session?token=...</code> endpoint,
					which is compatible with the Traccar protocol used by Home Assistant.
				</p>
			</div>
		</details>
	{/if}

	<!-- Delete confirmation -->
	{#if confirmingDeleteId !== null}
		<div class="confirm-overlay">
			<div class="confirm-box">
				<p class="confirm-text">
					Are you sure you want to revoke <strong>{confirmingKeyName}</strong>?
					Any integrations using this key will immediately stop working.
				</p>
				{#if deleteError}
					<div class="message error">{deleteError}</div>
				{/if}
				<div class="confirm-actions">
					<Button variant="secondary" size="sm" on:click={cancelDelete}>
						Cancel
					</Button>
					<Button variant="danger" size="sm" loading={deleting} on:click={confirmDelete}>
						{deleting ? 'Revoking...' : 'Revoke Key'}
					</Button>
				</div>
			</div>
		</div>
	{/if}
</section>

<!-- Create API Key Modal -->
<Modal bind:open={showCreateModal} title={createdToken ? 'API Key Created' : 'Create API Key'} on:close={closeCreateModal}>
	{#if createdToken}
		<!-- Success: show the token -->
		<div class="token-created">
			<p class="token-created-notice">
				Your API key has been created. Copy the token below -- it will only be shown once.
			</p>
			<div class="token-field">
				<input
					type="text"
					class="token-input"
					value={createdToken}
					readonly
					bind:this={tokenInputEl}
					on:click={() => tokenInputEl?.select()}
					aria-label="Generated API token"
				/>
				<button
					type="button"
					class="copy-btn"
					class:copied={tokenCopied}
					on:click={copyToken}
					aria-label="Copy token to clipboard"
				>
					{#if tokenCopied}
						Copied!
					{:else}
						Copy
					{/if}
				</button>
			</div>
			<div class="token-actions">
				<button
					type="button"
					class="qr-btn"
					on:click={() => openQrDialog(createdToken)}
					aria-label="Generate QR code for this token"
				>
					<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
						<rect x="3" y="3" width="7" height="7"/>
						<rect x="14" y="3" width="7" height="7"/>
						<rect x="3" y="14" width="7" height="7"/>
						<rect x="14" y="14" width="3" height="3"/>
						<line x1="21" y1="14" x2="21" y2="14.01"/>
						<line x1="21" y1="21" x2="21" y2="21.01"/>
						<line x1="17" y1="21" x2="17" y2="21.01"/>
					</svg>
					QR Code for Traccar Manager
				</button>
			</div>
			<p class="token-warning-text">
				Store this token securely. You will not be able to see it again.
			</p>
		</div>
	{:else}
		<!-- Form: create a new key -->
		<form on:submit|preventDefault={handleCreate} class="create-form">
			<div class="form-row">
				<Input
					label="Key Name"
					name="keyName"
					bind:value={newKeyName}
					placeholder="e.g. Home Assistant, Grafana"
					required
				/>
			</div>

			<div class="form-row">
				<div class="form-group">
					<label for="keyPermissions" class="form-label">Permissions</label>
					<select id="keyPermissions" bind:value={newKeyPermissions} class="select">
						<option value="full">Full Access -- read and write all data</option>
						<option value="readonly">Read-Only -- read data only, no modifications</option>
					</select>
				</div>
			</div>

			<div class="form-row">
				<div class="form-group">
					<label for="keyExpiration" class="form-label">Expiration</label>
					<select id="keyExpiration" bind:value={newKeyExpiration} class="select">
						<option value="24">1 day</option>
						<option value="168">7 days (recommended)</option>
						<option value="720">1 month (30 days)</option>
						<option value="custom">Custom date</option>
						<option value="never">Never</option>
					</select>
				</div>
			</div>

			{#if newKeyExpiration === 'custom'}
				<div class="form-row">
					<div class="form-group">
						<label for="keyCustomDate" class="form-label">Expiration Date</label>
						<input
							id="keyCustomDate"
							type="date"
							class="date-input"
							bind:value={newKeyCustomDate}
							min={getMinDate()}
						/>
					</div>
				</div>
			{/if}

			{#if newKeyExpiration === 'never'}
				<div class="security-notice">
					<strong>Warning:</strong> Keys without an expiration date remain valid indefinitely.
					Consider setting an expiration for better security.
				</div>
			{/if}

			{#if newKeyPermissions === 'full'}
				<div class="security-notice">
					<strong>Security notice:</strong> This key provides full API access to your account,
					equivalent to your password. Do not share it publicly or commit it to version control.
				</div>
			{/if}

			{#if createError}
				<div class="message error">{createError}</div>
			{/if}
		</form>
	{/if}

	<svelte:fragment slot="footer">
		{#if createdToken}
			<div class="modal-actions">
				<Button variant="primary" on:click={closeCreateModal}>
					Done
				</Button>
			</div>
		{:else}
			<div class="modal-actions">
				<Button variant="secondary" on:click={closeCreateModal}>
					Cancel
				</Button>
				<Button variant="primary" loading={creating} on:click={handleCreate}>
					{creating ? 'Creating...' : 'Create Key'}
				</Button>
			</div>
		{/if}
	</svelte:fragment>
</Modal>

<!-- QR Code Dialog for API Key tokens -->
<QrCodeDialog
	open={showQrDialog}
	onClose={closeQrDialog}
	apiToken={qrToken || undefined}
/>

<style>
	.api-keys-section {
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

	/* Keys list */
	.keys-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}

	.key-card {
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

	.key-card:hover {
		border-color: var(--border-hover);
	}

	.key-info {
		flex: 1;
		min-width: 0;
	}

	.key-header {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		margin-bottom: var(--space-2);
	}

	.key-name {
		font-weight: var(--font-semibold);
		color: var(--text-primary);
		font-size: var(--text-base);
	}

	.permission-badge {
		display: inline-flex;
		align-items: center;
		padding: var(--space-1) var(--space-2);
		border-radius: var(--radius-sm);
		font-size: var(--text-xs);
		font-weight: var(--font-medium);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.badge-full {
		background-color: rgba(0, 212, 255, 0.15);
		color: var(--accent-primary);
		border: 1px solid rgba(0, 212, 255, 0.3);
	}

	.badge-readonly {
		background-color: rgba(255, 170, 0, 0.15);
		color: var(--warning);
		border: 1px solid rgba(255, 170, 0, 0.3);
	}

	.badge-expired {
		background-color: rgba(255, 68, 68, 0.15);
		color: var(--error);
		border: 1px solid rgba(255, 68, 68, 0.3);
	}

	.key-expired {
		opacity: 0.6;
		border-color: var(--error);
	}

	.text-expired {
		color: var(--error);
		font-weight: var(--font-medium);
	}

	.key-meta {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-sm);
		color: var(--text-tertiary);
		flex-wrap: wrap;
	}

	.key-token {
		font-family: 'SF Mono', 'Fira Code', 'Fira Mono', 'Roboto Mono', 'Courier New', monospace;
		color: var(--text-secondary);
		font-size: var(--text-xs);
	}

	.meta-separator {
		color: var(--border-color);
	}

	.key-actions {
		flex-shrink: 0;
	}

	/* Delete confirmation */
	.confirm-overlay {
		margin-top: var(--space-4);
		padding: var(--space-4);
		background-color: rgba(255, 68, 68, 0.05);
		border: 1px solid var(--error);
		border-radius: var(--radius-md);
	}

	.confirm-text {
		font-size: var(--text-sm);
		color: var(--text-primary);
		margin-bottom: var(--space-3);
		line-height: 1.5;
	}

	.confirm-actions {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-3);
	}

	/* Create modal form */
	.create-form {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
	}

	.form-row {
		display: flex;
		flex-direction: column;
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
		width: 100%;
		padding: var(--space-3) var(--space-4);
		background-color: var(--bg-primary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-base);
		transition: border-color var(--transition-fast);
	}

	.select:hover {
		border-color: var(--border-hover);
	}

	.select:focus {
		outline: none;
		border-color: var(--accent-primary);
		box-shadow: 0 0 0 3px rgba(0, 212, 255, 0.1);
	}

	.date-input {
		width: 100%;
		padding: var(--space-3) var(--space-4);
		background-color: var(--bg-primary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-base);
		transition: border-color var(--transition-fast);
	}

	.date-input:hover {
		border-color: var(--border-hover);
	}

	.date-input:focus {
		outline: none;
		border-color: var(--accent-primary);
		box-shadow: 0 0 0 3px rgba(0, 212, 255, 0.1);
	}

	/* Token display after creation */
	.token-created {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}

	.token-created-notice {
		font-size: var(--text-sm);
		color: var(--text-primary);
		line-height: 1.5;
	}

	.token-field {
		display: flex;
		gap: 0;
	}

	.token-input {
		flex: 1;
		padding: var(--space-3) var(--space-4);
		background-color: var(--bg-primary);
		border: 1px solid var(--border-color);
		border-right: none;
		border-radius: var(--radius-md) 0 0 var(--radius-md);
		color: var(--text-primary);
		font-family: 'SF Mono', 'Fira Code', 'Fira Mono', 'Roboto Mono', 'Courier New', monospace;
		font-size: var(--text-sm);
		cursor: text;
		min-width: 0;
	}

	.token-input:focus {
		outline: none;
		border-color: var(--accent-primary);
		box-shadow: 0 0 0 3px rgba(0, 212, 255, 0.1);
	}

	.copy-btn {
		padding: var(--space-3) var(--space-4);
		background-color: var(--bg-tertiary);
		border: 1px solid var(--border-color);
		border-radius: 0 var(--radius-md) var(--radius-md) 0;
		color: var(--text-primary);
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		cursor: pointer;
		white-space: nowrap;
		transition: all var(--transition-fast);
		min-width: 72px;
		text-align: center;
	}

	.copy-btn:hover {
		background-color: var(--bg-hover);
	}

	.copy-btn.copied {
		background-color: rgba(0, 255, 136, 0.15);
		color: var(--success);
		border-color: var(--success);
	}

	.token-actions {
		display: flex;
		gap: var(--space-3);
	}

	.qr-btn {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		padding: var(--space-2) var(--space-3);
		background-color: var(--bg-tertiary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		cursor: pointer;
		transition: all var(--transition-fast);
		white-space: nowrap;
	}

	.qr-btn:hover {
		background-color: var(--bg-hover);
		border-color: var(--accent-primary);
		color: var(--accent-primary);
	}

	.qr-btn svg {
		flex-shrink: 0;
	}

	.token-warning-text {
		font-size: var(--text-sm);
		color: var(--warning);
		font-weight: var(--font-medium);
	}

	/* Modal footer actions */
	.modal-actions {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-3);
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

	/* Security notice */
	.security-notice {
		padding: var(--space-3) var(--space-4);
		background-color: rgba(255, 68, 68, 0.05);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		font-size: var(--text-sm);
		color: var(--text-secondary);
		line-height: 1.5;
	}

	.security-notice strong {
		color: var(--text-primary);
	}

	/* Usage instructions */
	.usage-details {
		margin-top: var(--space-4);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		overflow: hidden;
	}

	.usage-summary {
		padding: var(--space-3) var(--space-4);
		background-color: var(--bg-tertiary);
		color: var(--text-primary);
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		cursor: pointer;
		transition: background-color var(--transition-fast);
		list-style: none;
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}

	.usage-summary::-webkit-details-marker {
		display: none;
	}

	.usage-summary::before {
		content: '\25B6';
		font-size: var(--text-xs);
		transition: transform var(--transition-fast);
	}

	.usage-details[open] .usage-summary::before {
		transform: rotate(90deg);
	}

	.usage-summary:hover {
		background-color: var(--bg-hover);
	}

	.usage-content {
		padding: var(--space-4);
		font-size: var(--text-sm);
		color: var(--text-secondary);
		line-height: 1.6;
	}

	.usage-content p {
		margin-bottom: var(--space-3);
	}

	.usage-steps {
		padding-left: var(--space-6);
		margin-bottom: var(--space-3);
	}

	.usage-steps li {
		margin-bottom: var(--space-2);
	}

	.usage-steps strong {
		color: var(--text-primary);
	}

	.usage-content code {
		padding: var(--space-1) var(--space-2);
		background-color: var(--bg-tertiary);
		border-radius: var(--radius-sm);
		font-family: 'SF Mono', 'Fira Code', 'Fira Mono', 'Roboto Mono', 'Courier New', monospace;
		font-size: 0.85em;
		color: var(--accent-primary);
	}

	.usage-note {
		color: var(--text-tertiary);
		font-size: var(--text-xs);
	}

	/* Responsive */
	@media (max-width: 768px) {
		.section-header {
			flex-direction: column;
			align-items: stretch;
		}

		.key-card {
			flex-direction: column;
			align-items: stretch;
		}

		.key-actions {
			display: flex;
			justify-content: flex-end;
		}

		.key-meta {
			flex-direction: column;
			align-items: flex-start;
			gap: var(--space-1);
		}

		.meta-separator {
			display: none;
		}
	}

	@media (max-width: 480px) {
		.token-field {
			flex-direction: column;
		}

		.token-input {
			border-right: 1px solid var(--border-color);
			border-radius: var(--radius-md) var(--radius-md) 0 0;
		}

		.copy-btn {
			border-radius: 0 0 var(--radius-md) var(--radius-md);
		}
	}
</style>

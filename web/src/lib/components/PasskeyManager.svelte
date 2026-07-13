<script lang="ts">
	import { onMount } from 'svelte';
	import { api, APIError } from '$lib/api/client';
	import type { PasskeyCredentialInfo } from '$lib/types/api';
	import { formatDate } from '$lib/utils/formatting';
	import {
		isPasskeySupported,
		registerPasskey,
		isPasskeyCancellation,
	} from '$lib/utils/webauthn';
	import Button from '$lib/components/Button.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import Input from '$lib/components/Input.svelte';

	// Evaluated in the browser only; false during SSR.
	const supported = typeof window !== 'undefined' && isPasskeySupported();

	// ---------------------------------------------------------------------------
	// List state
	// ---------------------------------------------------------------------------
	let loading = true;
	let passkeys: PasskeyCredentialInfo[] = [];
	let listError = '';

	// ---------------------------------------------------------------------------
	// Create modal state
	// ---------------------------------------------------------------------------
	let showCreateModal = false;
	let newPasskeyName = '';
	let creating = false;
	let createError = '';

	// ---------------------------------------------------------------------------
	// Delete confirmation state
	// ---------------------------------------------------------------------------
	let confirmingDeleteId: number | null = null;
	let deleting = false;
	let deleteError = '';

	// ---------------------------------------------------------------------------
	// Lifecycle
	// ---------------------------------------------------------------------------
	onMount(() => {
		if (supported) {
			loadPasskeys();
		} else {
			loading = false;
		}
	});

	async function loadPasskeys() {
		loading = true;
		listError = '';
		try {
			passkeys = await api.listPasskeys();
		} catch (e: unknown) {
			listError = `Failed to load passkeys: ${errorMessage(e)}`;
		} finally {
			loading = false;
		}
	}

	function errorMessage(e: unknown): string {
		if (e instanceof APIError || e instanceof Error) return e.message;
		return 'Please try again.';
	}

	// ---------------------------------------------------------------------------
	// Create
	// ---------------------------------------------------------------------------
	function openCreateModal() {
		showCreateModal = true;
		newPasskeyName = '';
		createError = '';
	}

	function closeCreateModal() {
		showCreateModal = false;
		newPasskeyName = '';
		createError = '';
	}

	async function handleCreate() {
		const trimmedName = newPasskeyName.trim();
		if (!trimmedName) {
			createError = 'Name is required.';
			return;
		}

		creating = true;
		createError = '';

		try {
			const created = await registerPasskey(trimmedName);
			passkeys = [...passkeys, created];
			closeCreateModal();
		} catch (e: unknown) {
			// User dismissed the browser prompt: keep the modal open, no error.
			if (isPasskeyCancellation(e)) {
				return;
			}
			createError = errorMessage(e);
		} finally {
			creating = false;
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
			await api.deletePasskey(confirmingDeleteId);
			passkeys = passkeys.filter((p) => p.id !== confirmingDeleteId);
			confirmingDeleteId = null;
		} catch (e: unknown) {
			deleteError = errorMessage(e);
		} finally {
			deleting = false;
		}
	}

	// ---------------------------------------------------------------------------
	// Helpers
	// ---------------------------------------------------------------------------
	function formatLastUsed(lastUsedAt: string | null | undefined): string {
		if (!lastUsedAt) return 'Never';
		return formatDate(lastUsedAt);
	}

	$: confirmingPasskeyName = confirmingDeleteId !== null
		? passkeys.find((p) => p.id === confirmingDeleteId)?.name ?? 'this passkey'
		: '';
</script>

{#if supported}
	<section class="settings-section passkeys-section">
		<div class="section-header">
			<div>
				<h2 class="section-title">Passkeys</h2>
				<p class="section-description">
					Sign in without a password using a passkey stored on your device, security key, or password manager.
				</p>
			</div>
			<Button variant="primary" size="sm" on:click={openCreateModal}>
				Add Passkey
			</Button>
		</div>

		{#if loading}
			<p class="loading-text">Loading passkeys...</p>
		{:else if listError}
			<div class="message error">{listError}</div>
		{:else if passkeys.length === 0}
			<div class="empty-state">
				<p class="empty-title">No passkeys</p>
				<p class="empty-description">
					Add a passkey to sign in quickly and securely without entering your password.
				</p>
			</div>
		{:else}
			<div class="keys-list">
				{#each passkeys as passkey (passkey.id)}
					<div class="key-card">
						<div class="key-info">
							<div class="key-header">
								<span class="key-name">{passkey.name}</span>
							</div>
							<div class="key-meta">
								<span class="key-date">Created {formatDate(passkey.createdAt)}</span>
								<span class="meta-separator">|</span>
								<span class="key-last-used">Last used: {formatLastUsed(passkey.lastUsedAt)}</span>
							</div>
						</div>
						<div class="key-actions">
							<Button variant="danger" size="sm" on:click={() => requestDelete(passkey.id)}>
								Remove
							</Button>
						</div>
					</div>
				{/each}
			</div>
		{/if}

		<!-- Delete confirmation -->
		{#if confirmingDeleteId !== null}
			<div class="confirm-overlay">
				<div class="confirm-box">
					<p class="confirm-text">
						Are you sure you want to remove <strong>{confirmingPasskeyName}</strong>?
						You will no longer be able to sign in with this passkey.
					</p>
					{#if deleteError}
						<div class="message error">{deleteError}</div>
					{/if}
					<div class="confirm-actions">
						<Button variant="secondary" size="sm" on:click={cancelDelete}>
							Cancel
						</Button>
						<Button variant="danger" size="sm" loading={deleting} on:click={confirmDelete}>
							{deleting ? 'Removing...' : 'Remove Passkey'}
						</Button>
					</div>
				</div>
			</div>
		{/if}
	</section>

	<!-- Add Passkey Modal -->
	<Modal bind:open={showCreateModal} title="Add Passkey" on:close={closeCreateModal}>
		<form on:submit|preventDefault={handleCreate} class="create-form">
			<div class="form-row">
				<Input
					label="Passkey Name"
					name="passkeyName"
					bind:value={newPasskeyName}
					placeholder="e.g. My Laptop, iPhone, YubiKey"
					required
				/>
			</div>

			<p class="form-hint">
				When you continue, your browser will prompt you to create a passkey using your device,
				security key, or password manager.
			</p>

			{#if createError}
				<div class="message error">{createError}</div>
			{/if}
		</form>

		<svelte:fragment slot="footer">
			<div class="modal-actions">
				<Button variant="secondary" on:click={closeCreateModal}>
					Cancel
				</Button>
				<Button variant="primary" loading={creating} on:click={handleCreate}>
					{creating ? 'Waiting for passkey...' : 'Create Passkey'}
				</Button>
			</div>
		</svelte:fragment>
	</Modal>
{/if}

<style>
	.passkeys-section {
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

	.key-meta {
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

	.form-hint {
		font-size: var(--text-sm);
		color: var(--text-secondary);
		line-height: 1.5;
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
</style>

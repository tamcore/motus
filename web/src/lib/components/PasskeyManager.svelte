<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import type { PasskeyCredentialInfo } from '$lib/types/api';
	import { formatDate, formatLastUsed } from '$lib/utils/formatting';
	import {
		isPasskeySupported,
		registerPasskey,
		isPasskeyCancellation,
	} from '$lib/utils/webauthn';
	import Button from '$lib/components/Button.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import Input from '$lib/components/Input.svelte';

	const supported = isPasskeySupported();

	let loading = true;
	let passkeys: PasskeyCredentialInfo[] = [];
	let listError = '';

	let showCreateModal = false;
	let newPasskeyName = '';
	let creating = false;
	let createError = '';

	let actionError = '';

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
			listError = `Failed to load passkeys: ${e instanceof Error ? e.message : 'Please try again.'}`;
		} finally {
			loading = false;
		}
	}

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
			createError = e instanceof Error ? e.message : 'Please try again.';
		} finally {
			creating = false;
		}
	}

	async function removePasskey(passkey: PasskeyCredentialInfo) {
		if (!confirm(`Remove "${passkey.name}"? You will no longer be able to sign in with this passkey.`)) return;
		actionError = '';
		try {
			await api.deletePasskey(passkey.id);
			passkeys = passkeys.filter((p) => p.id !== passkey.id);
		} catch (e: unknown) {
			actionError = e instanceof Error ? e.message : 'Please try again.';
		}
	}

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
			<div class="form-error">{listError}</div>
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
							<Button variant="danger" size="sm" on:click={() => removePasskey(passkey)}>
								Remove
							</Button>
						</div>
					</div>
				{/each}
			</div>
		{/if}

		{#if actionError}
			<div class="form-error" role="alert">{actionError}</div>
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
				<div class="form-error">{createError}</div>
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
	.form-hint {
		font-size: var(--text-sm);
		color: var(--text-secondary);
		line-height: 1.5;
	}
</style>

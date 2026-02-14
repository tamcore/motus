<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { slide } from 'svelte/transition';
	import { goto } from '$app/navigation';
	import { currentUser } from '$lib/stores/auth';
	import { refreshHandler } from '$lib/stores/refresh';
	import { api } from '$lib/api/client';
	import { formatDate } from '$lib/utils/formatting';
	import Button from '$lib/components/Button.svelte';
	import Input from '$lib/components/Input.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import Skeleton from '$lib/components/Skeleton.svelte';
	import type { User } from '$lib/types/api';

	// Helper to get role string from user object
	function getRoleFromUser(user: User): string {
		if (user.administrator) return 'admin';
		if (user.readonly) return 'readonly';
		return 'user';
	}

	interface Device {
		id: number;
		name: string;
		uniqueId: string;
	}

	const ROLES = [
		{ value: 'admin', label: 'Admin', color: 'role-admin' },
		{ value: 'user', label: 'User', color: 'role-user' },
		{ value: 'readonly', label: 'Read Only', color: 'role-readonly' }
	];

	let loading = true;
	let error = '';
	let users: User[] = [];
	let expandedIds: Set<number> = new Set();

	// User form modal
	let showUserModal = false;
	let editingUser: User | null = null;
	let formEmail = '';
	let formName = '';
	let formPassword = '';
	let formRole = 'user';
	let savingUser = false;

	// Device assignment modal
	let showDevicesModal = false;
	let deviceUser: User | null = null;
	let allDevices: Device[] = [];
	let assignedDeviceIds: Set<number> = new Set();
	let loadingDevices = false;
	let savingDevices = false;

	$: isAdmin = ($currentUser as Record<string, unknown> | null)?.administrator === true;
	$: currentUserId = ($currentUser as Record<string, unknown> | null)?.id as number | undefined;

	$: roleCounts = users.reduce(
		(acc, u) => {
			acc[u.role] = (acc[u.role] || 0) + 1;
			return acc;
		},
		{} as Record<string, number>
	);

	onMount(() => {
		if (!isAdmin) {
			goto('/');
			return;
		}
		loadUsers();
		$refreshHandler = loadUsers;
	});

	onDestroy(() => { $refreshHandler = null; });

	async function loadUsers() {
		loading = true;
		error = '';
		try {
			users = await api.getUsers();
		} catch (err: any) {
			error = err.status === 403
				? 'Access denied. Admin privileges required.'
				: 'Failed to load users';
			console.error(err);
		} finally {
			loading = false;
		}
	}

	function toggleUser(userId: number) {
		const next = new Set(expandedIds);
		if (next.has(userId)) {
			next.delete(userId);
		} else {
			next.add(userId);
		}
		expandedIds = next;
	}

	function handleRowKeydown(event: KeyboardEvent, userId: number) {
		if (event.key === 'Enter' || event.key === ' ') {
			event.preventDefault();
			toggleUser(userId);
		}
	}

	function resetUserForm() {
		formEmail = '';
		formName = '';
		formPassword = '';
		formRole = 'user';
	}

	function openCreateUser() {
		editingUser = null;
		resetUserForm();
		showUserModal = true;
	}

	function openEditUser(user: User) {
		editingUser = user;
		formEmail = user.email;
		formName = user.name;
		formPassword = '';
		formRole = getRoleFromUser(user);
		showUserModal = true;
	}

	async function handleUserSubmit() {
		error = '';
		savingUser = true;

		try {
			if (editingUser) {
				const updateData: Record<string, unknown> = {
					email: formEmail,
					name: formName,
					role: formRole
				};
				if (formPassword) {
					updateData.password = formPassword;
				}
				await api.updateUser(editingUser.id, updateData);
			} else {
				if (!formPassword) {
					error = 'Password is required for new users';
					savingUser = false;
					return;
				}
				await api.createUser({
					email: formEmail,
					password: formPassword,
					name: formName,
					role: formRole
				});
			}
			showUserModal = false;
			resetUserForm();
			await loadUsers();
		} catch (err: any) {
			error = editingUser
				? 'Failed to update user'
				: 'Failed to create user';
			console.error(err);
		} finally {
			savingUser = false;
		}
	}

	async function deleteUser(user: User) {
		if (user.id === currentUserId) {
			error = 'You cannot delete your own account';
			return;
		}

		if (!confirm(`Are you sure you want to delete "${user.name || user.email}"? This action cannot be undone.`)) {
			return;
		}

		error = '';
		try {
			await api.deleteUser(user.id);
			await loadUsers();
		} catch (err: any) {
			error = 'Failed to delete user';
			console.error(err);
		}
	}

	async function openDevices(user: User) {
		deviceUser = user;
		loadingDevices = true;
		assignedDeviceIds = new Set();
		allDevices = [];
		showDevicesModal = true;

		try {
			const [devices, userDevices] = await Promise.all([
				api.getAllDevices(),
				api.getUserDevices(user.id)
			]);
			allDevices = devices;
			assignedDeviceIds = new Set(userDevices.map((d: Device) => d.id));
		} catch (err: any) {
			error = 'Failed to load devices';
			console.error(err);
		} finally {
			loadingDevices = false;
		}
	}

	async function toggleDevice(deviceId: number) {
		if (!deviceUser) return;
		savingDevices = true;
		error = '';

		const wasAssigned = assignedDeviceIds.has(deviceId);

		try {
			if (wasAssigned) {
				await api.unassignDevice(deviceUser.id, deviceId);
				assignedDeviceIds.delete(deviceId);
			} else {
				await api.assignDevice(deviceUser.id, deviceId);
				assignedDeviceIds.add(deviceId);
			}
			assignedDeviceIds = assignedDeviceIds;
		} catch (err: any) {
			error = wasAssigned
				? 'Failed to unassign device'
				: 'Failed to assign device';
			console.error(err);
		} finally {
			savingDevices = false;
		}
	}

	let sudoLoading: number | null = null;

	async function startSudo(user: User) {
		if (user.id === currentUserId) return;

		if (!confirm(`You are about to impersonate "${user.name || user.email}". You will see the application as this user. Continue?`)) {
			return;
		}

		sudoLoading = user.id;
		error = '';
		try {
			await api.startSudo(user.id);
			window.location.href = '/';
		} catch (err: any) {
			error = err.message || 'Failed to start sudo session';
			console.error(err);
		} finally {
			sudoLoading = null;
		}
	}

	function getRoleClass(role: string): string {
		const found = ROLES.find((r) => r.value === role);
		return found ? found.color : 'role-user';
	}

	function getRoleLabel(role: string): string {
		const found = ROLES.find((r) => r.value === role);
		return found ? found.label : role;
	}
</script>

<svelte:head><title>User Management - Motus</title></svelte:head>

<div class="users-page">
	<div class="container">
		<div class="page-header">
			<div class="page-header-left">
				<h1 class="page-title">User Management</h1>
				<span class="user-count">{users.length} user{users.length !== 1 ? 's' : ''}</span>
			</div>
			<Button on:click={openCreateUser}>+ Add User</Button>
		</div>

		{#if Object.keys(roleCounts).length > 0}
			<div class="role-distribution">
				{#each ROLES as role}
					{#if roleCounts[role.value]}
						<div class="role-stat">
							<span class="role-badge {role.color}">{role.label}</span>
							<span class="role-count">{roleCounts[role.value]}</span>
						</div>
					{/if}
				{/each}
			</div>
		{/if}

		{#if error}
			<div class="error-banner" role="alert">
				<span>{error}</span>
				<button class="dismiss-btn" on:click={() => (error = '')} aria-label="Dismiss error">X</button>
			</div>
		{/if}

		{#if loading}
			<div class="loading-state desktop-view">
				<div class="spinner" aria-hidden="true"></div>
				<p>Loading users...</p>
			</div>
			<div class="list-skeleton mobile-view">
				{#each Array(4) as _}
					<Skeleton width="100%" height="64px" variant="rect" />
				{/each}
			</div>
		{:else if users.length === 0}
			<div class="empty-state">
				<svg viewBox="0 0 24 24" width="64" height="64" fill="none" stroke="var(--text-tertiary)" stroke-width="1">
					<path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
					<circle cx="9" cy="7" r="4" />
					<path d="M23 21v-2a4 4 0 0 0-3-3.87" />
					<path d="M16 3.13a4 4 0 0 1 0 7.75" />
				</svg>
				<p>No users found</p>
				<p class="empty-subtitle">Create users to manage access to your Motus instance.</p>
				<Button on:click={openCreateUser}>Add your first user</Button>
			</div>
		{:else}
			<!-- Desktop: Table view (hidden on mobile) -->
			<div class="desktop-view">
				<div class="table-wrapper">
					<table class="users-table" aria-label="Users">
						<thead>
							<tr>
								<th>User</th>
								<th>Role</th>
								<th>Created</th>
								<th class="actions-col">Actions</th>
							</tr>
						</thead>
						<tbody>
							{#each users as user (user.id)}
								<tr class="table-row" class:is-self={user.id === currentUserId}>
									<td class="user-cell">
										<div class="user-avatar" aria-hidden="true">
											{(user.name || user.email).charAt(0).toUpperCase()}
										</div>
										<div class="user-details">
											<span class="user-name">
												{user.name || '(unnamed)'}
												{#if user.id === currentUserId}
													<span class="you-badge">you</span>
												{/if}
											</span>
											<span class="user-email">{user.email}</span>
										</div>
									</td>
									<td>
										<span class="role-badge {getRoleClass(getRoleFromUser(user))}">{getRoleLabel(getRoleFromUser(user))}</span>
									</td>
									<td class="date-cell">
										{formatDate(user.createdAt)}
									</td>
									<td class="actions-cell">
										<Button
											size="sm"
											variant="secondary"
											disabled={user.id === currentUserId || sudoLoading === user.id}
											loading={sudoLoading === user.id}
											on:click={() => startSudo(user)}
										>
											Sudo
										</Button>
										<Button size="sm" variant="secondary" on:click={() => openDevices(user)}>
											Devices
										</Button>
										<Button size="sm" variant="secondary" on:click={() => openEditUser(user)}>
											Edit
										</Button>
										<Button
											size="sm"
											variant="danger"
											disabled={user.id === currentUserId}
											on:click={() => deleteUser(user)}
										>
											Delete
										</Button>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</div>

			<!-- Mobile: Expandable card view (hidden on desktop) -->
			<div class="mobile-view">
				<div class="user-list" role="list">
					{#each users as user (user.id)}
						{@const isExpanded = expandedIds.has(user.id)}
						{@const userRole = getRoleFromUser(user)}
						<div
							class="user-card"
							class:expanded={isExpanded}
							class:is-self={user.id === currentUserId}
							role="listitem"
						>
							<!-- Collapsed summary row - always visible -->
							<div
								class="user-summary"
								role="button"
								tabindex="0"
								aria-expanded={isExpanded}
								aria-controls="user-detail-{user.id}"
								on:click={() => toggleUser(user.id)}
								on:keydown={(e) => handleRowKeydown(e, user.id)}
							>
								<div class="user-avatar" aria-hidden="true">
									{(user.name || user.email).charAt(0).toUpperCase()}
								</div>

								<div class="summary-main">
									<div class="summary-name-row">
										<span class="user-name-mobile">
											{user.name || '(unnamed)'}
											{#if user.id === currentUserId}
												<span class="you-badge">you</span>
											{/if}
										</span>
										<span class="role-badge {getRoleClass(userRole)}">{getRoleLabel(userRole)}</span>
									</div>
									<span class="summary-email">{user.email}</span>
								</div>

								<div class="summary-meta">
									<span class="summary-date">{formatDate(user.createdAt)}</span>
								</div>

								<svg
									class="chevron"
									class:chevron-open={isExpanded}
									viewBox="0 0 24 24"
									width="20"
									height="20"
									fill="none"
									stroke="currentColor"
									stroke-width="2"
									stroke-linecap="round"
									stroke-linejoin="round"
									aria-hidden="true"
								>
									<polyline points="6 9 12 15 18 9"></polyline>
								</svg>
							</div>

							<!-- Expanded detail section -->
							{#if isExpanded}
								<div
									id="user-detail-{user.id}"
									class="user-detail"
									transition:slide={{ duration: 200 }}
								>
									<div class="detail-grid">
										<div class="detail-item">
											<span class="detail-label">Name</span>
											<span class="detail-value">{user.name || '(unnamed)'}</span>
										</div>
										<div class="detail-item">
											<span class="detail-label">Email</span>
											<span class="detail-value detail-email">{user.email}</span>
										</div>
										<div class="detail-item">
											<span class="detail-label">Role</span>
											<span class="detail-value">
												<span class="role-badge {getRoleClass(userRole)}">{getRoleLabel(userRole)}</span>
											</span>
										</div>
										<div class="detail-item">
											<span class="detail-label">Created</span>
											<span class="detail-value">{formatDate(user.createdAt)}</span>
										</div>
									</div>

									<!-- svelte-ignore a11y-click-events-have-key-events a11y-no-noninteractive-element-interactions -->
									<div class="detail-actions" on:click|stopPropagation role="group" aria-label="User actions">
										<div on:click|stopPropagation on:keydown|stopPropagation role="presentation">
											<Button
												size="sm"
												variant="secondary"
												disabled={user.id === currentUserId || sudoLoading === user.id}
												loading={sudoLoading === user.id}
												on:click={() => startSudo(user)}
											>
												<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
													<path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2" />
													<circle cx="12" cy="7" r="4" />
												</svg>
												Sudo
											</Button>
										</div>
										<div on:click|stopPropagation on:keydown|stopPropagation role="presentation">
											<Button size="sm" variant="secondary" on:click={() => openDevices(user)}>
												<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
													<rect x="5" y="2" width="14" height="20" rx="2" />
													<line x1="12" y1="18" x2="12" y2="18.01" stroke-width="2" />
												</svg>
												Devices
											</Button>
										</div>
										<div on:click|stopPropagation on:keydown|stopPropagation role="presentation">
											<Button size="sm" variant="secondary" on:click={() => openEditUser(user)}>
												<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
													<path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"></path>
													<path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"></path>
												</svg>
												Edit
											</Button>
										</div>
										<div on:click|stopPropagation on:keydown|stopPropagation role="presentation">
											<Button
												size="sm"
												variant="danger"
												disabled={user.id === currentUserId}
												on:click={() => deleteUser(user)}
											>
												<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
													<polyline points="3 6 5 6 21 6"></polyline>
													<path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path>
												</svg>
												Delete
											</Button>
										</div>
									</div>
								</div>
							{/if}
						</div>
					{/each}
				</div>
			</div>
		{/if}
	</div>
</div>

<!-- Create/Edit User Modal -->
<Modal
	open={showUserModal}
	title={editingUser ? 'Edit User' : 'Add User'}
	on:close={() => (showUserModal = false)}
>
	<form on:submit|preventDefault={handleUserSubmit} class="user-form">
		<Input
			label="Name"
			name="userName"
			placeholder="Full name"
			required
			bind:value={formName}
		/>

		<Input
			type="email"
			label="Email"
			name="userEmail"
			placeholder="user@example.com"
			required
			bind:value={formEmail}
		/>

		{#if editingUser}
			<Input
				type="password"
				label="New Password (leave blank to keep current)"
				name="userPassword"
				placeholder="Enter new password"
				bind:value={formPassword}
			/>
		{:else}
			<Input
				type="password"
				label="Password"
				name="userPassword"
				placeholder="Enter password"
				required
				bind:value={formPassword}
			/>
		{/if}

		<div class="form-group">
			<label for="userRole" class="form-label">Role</label>
			<select id="userRole" bind:value={formRole} class="select">
				{#each ROLES as role}
					<option value={role.value}>{role.label}</option>
				{/each}
			</select>
			<div class="role-descriptions">
				{#if formRole === 'admin'}
					<p class="role-hint">Full access to all features including user management.</p>
				{:else if formRole === 'user'}
					<p class="role-hint">Can manage own devices, view map, create reports.</p>
				{:else if formRole === 'readonly'}
					<p class="role-hint">View-only access. Cannot modify any data.</p>
				{/if}
			</div>
		</div>
	</form>

	<svelte:fragment slot="footer">
		<Button variant="secondary" on:click={() => (showUserModal = false)}>Cancel</Button>
		<Button loading={savingUser} on:click={handleUserSubmit}>
			{editingUser ? 'Update User' : 'Create User'}
		</Button>
	</svelte:fragment>
</Modal>

<!-- Device Assignment Modal -->
<Modal
	open={showDevicesModal}
	title={deviceUser ? `Devices for ${deviceUser.name || deviceUser.email}` : 'Device Assignment'}
	on:close={() => (showDevicesModal = false)}
>
	{#if loadingDevices}
		<div class="devices-loading">
			<div class="spinner" aria-hidden="true"></div>
			<p>Loading devices...</p>
		</div>
	{:else if allDevices.length === 0}
		<div class="devices-empty">
			<p>No devices available. Create devices first before assigning them.</p>
		</div>
	{:else}
		<div class="devices-info">
			<p>{assignedDeviceIds.size} of {allDevices.length} device{allDevices.length !== 1 ? 's' : ''} assigned</p>
		</div>
		<div class="devices-list">
			{#each allDevices as device (device.id)}
				<label class="device-row" class:assigned={assignedDeviceIds.has(device.id)}>
					<input
						type="checkbox"
						checked={assignedDeviceIds.has(device.id)}
						disabled={savingDevices}
						on:change={() => toggleDevice(device.id)}
					/>
					<div class="device-info">
						<span class="device-name">{device.name}</span>
						<span class="device-id">{device.uniqueId}</span>
					</div>
				</label>
			{/each}
		</div>
	{/if}

	<svelte:fragment slot="footer">
		<Button variant="secondary" on:click={() => (showDevicesModal = false)}>Close</Button>
	</svelte:fragment>
</Modal>

<style>
	.users-page {
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

	.page-header-left {
		display: flex;
		align-items: baseline;
		gap: var(--space-3);
	}

	.page-title {
		font-size: var(--text-3xl);
		font-weight: var(--font-bold);
		color: var(--text-primary);
		margin: 0;
	}

	.user-count {
		font-size: var(--text-sm);
		color: var(--text-secondary);
		background-color: var(--bg-secondary);
		padding: var(--space-1) var(--space-3);
		border-radius: var(--radius-full);
		border: 1px solid var(--border-color);
	}

	/* Role distribution */
	.role-distribution {
		display: flex;
		gap: var(--space-4);
		margin-bottom: var(--space-6);
		padding: var(--space-4);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
	}

	.role-stat {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}

	.role-count {
		font-size: var(--text-lg);
		font-weight: var(--font-semibold);
		color: var(--text-primary);
	}

	/* Role badges */
	.role-badge {
		display: inline-block;
		padding: var(--space-1) var(--space-3);
		border-radius: var(--radius-full);
		font-size: var(--text-xs);
		font-weight: var(--font-semibold);
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}

	.role-admin {
		background-color: rgba(255, 59, 48, 0.15);
		color: #ff3b30;
	}

	.role-user {
		background-color: rgba(0, 122, 255, 0.15);
		color: #007aff;
	}

	.role-readonly {
		background-color: rgba(142, 142, 147, 0.15);
		color: #8e8e93;
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

	.loading-state p,
	.devices-loading p {
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

	.list-skeleton {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	/* Empty state */
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
		max-width: 400px;
		margin-left: auto;
		margin-right: auto;
	}

	/* ---- Responsive view toggle ---- */
	.desktop-view {
		display: block;
	}

	.mobile-view {
		display: none;
	}

	@media (max-width: 768px) {
		.desktop-view {
			display: none;
		}

		.mobile-view {
			display: block;
		}
	}

	/* ==== DESKTOP TABLE VIEW ==== */
	.table-wrapper {
		overflow-x: auto;
		border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
		background-color: var(--bg-secondary);
	}

	.users-table {
		width: 100%;
		border-collapse: collapse;
	}

	.users-table th {
		text-align: left;
		padding: var(--space-3) var(--space-4);
		font-size: var(--text-xs);
		font-weight: var(--font-semibold);
		color: var(--text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.5px;
		border-bottom: 1px solid var(--border-color);
		background-color: var(--bg-tertiary);
		white-space: nowrap;
	}

	.users-table td {
		padding: var(--space-3) var(--space-4);
		border-bottom: 1px solid var(--border-color);
		font-size: var(--text-sm);
		color: var(--text-primary);
		vertical-align: middle;
	}

	.users-table tbody tr:last-child td {
		border-bottom: none;
	}

	.table-row {
		transition: background-color var(--transition-fast);
	}

	.table-row:hover {
		background-color: var(--bg-hover);
	}

	.users-table tr.is-self,
	.table-row.is-self {
		background-color: rgba(0, 212, 255, 0.04);
	}

	.actions-col {
		text-align: right;
	}

	/* User cell */
	.user-cell {
		display: flex;
		align-items: center;
		gap: var(--space-3);
	}

	.user-avatar {
		width: 36px;
		height: 36px;
		border-radius: 50%;
		background-color: var(--accent-primary);
		color: var(--text-inverse);
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: var(--text-sm);
		font-weight: var(--font-bold);
		flex-shrink: 0;
	}

	.user-details {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.user-name {
		font-weight: var(--font-medium);
		color: var(--text-primary);
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}

	.user-email {
		font-size: var(--text-xs);
		color: var(--text-secondary);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.you-badge {
		font-size: 10px;
		padding: 1px 6px;
		background-color: rgba(0, 212, 255, 0.15);
		color: var(--accent-primary);
		border-radius: var(--radius-full);
		font-weight: var(--font-medium);
		text-transform: lowercase;
	}

	.date-cell {
		white-space: nowrap;
		color: var(--text-secondary);
	}

	.actions-cell {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-2);
	}

	/* ==== MOBILE CARD VIEW ==== */
	.user-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	.user-card {
		border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
		background-color: var(--bg-primary);
		overflow: hidden;
		transition: border-color var(--transition-fast), box-shadow var(--transition-fast);
	}

	.user-card:hover {
		border-color: var(--border-hover);
	}

	.user-card.expanded {
		border-color: var(--accent-primary);
		box-shadow: 0 0 0 1px var(--accent-primary);
	}

	.user-card.is-self {
		background-color: rgba(0, 212, 255, 0.04);
	}

	.user-summary {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		padding: var(--space-3);
		cursor: pointer;
		user-select: none;
		-webkit-user-select: none;
	}

	.user-summary:hover {
		background-color: var(--bg-hover);
	}

	.user-summary:focus-visible {
		outline: 2px solid var(--accent-primary);
		outline-offset: -2px;
		border-radius: var(--radius-lg);
	}

	.summary-main {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}

	.summary-name-row {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		flex-wrap: wrap;
	}

	.user-name-mobile {
		font-weight: var(--font-semibold);
		color: var(--text-primary);
		font-size: var(--text-base);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}

	.summary-email {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.summary-meta {
		flex-shrink: 0;
		text-align: right;
	}

	.summary-date {
		font-size: var(--text-sm);
		color: var(--text-tertiary);
		white-space: nowrap;
	}

	.chevron {
		flex-shrink: 0;
		color: var(--text-tertiary);
		transition: transform var(--transition-fast);
	}

	.chevron-open {
		transform: rotate(180deg);
	}

	.user-detail {
		border-top: 1px solid var(--border-color);
		background-color: var(--bg-secondary);
		padding: var(--space-3);
	}

	.detail-grid {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: var(--space-3);
		margin-bottom: var(--space-4);
	}

	.detail-item {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}

	.detail-label {
		font-size: var(--text-xs);
		font-weight: var(--font-semibold);
		color: var(--text-tertiary);
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.detail-value {
		font-size: var(--text-sm);
		color: var(--text-primary);
	}

	.detail-email {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.detail-actions {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		padding-top: var(--space-3);
		border-top: 1px solid var(--border-color);
	}

	.detail-actions :global(.btn) {
		width: 100%;
		justify-content: center;
	}

	.detail-actions > div {
		width: 100%;
	}

	.detail-actions > div :global(.btn) {
		width: 100%;
		justify-content: center;
	}

	/* Form */
	.user-form {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
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

	.role-descriptions {
		padding: 0;
	}

	.role-hint {
		font-size: var(--text-xs);
		color: var(--text-secondary);
		margin: 0;
	}

	/* Device assignment modal */
	.devices-loading {
		display: flex;
		flex-direction: column;
		align-items: center;
		padding: var(--space-8);
		gap: var(--space-3);
	}

	.devices-empty {
		text-align: center;
		padding: var(--space-8);
		color: var(--text-secondary);
	}

	.devices-info {
		padding: var(--space-3) 0;
		font-size: var(--text-sm);
		color: var(--text-secondary);
		border-bottom: 1px solid var(--border-color);
		margin-bottom: var(--space-3);
	}

	.devices-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		max-height: 400px;
		overflow-y: auto;
	}

	.device-row {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		padding: var(--space-3);
		border-radius: var(--radius-md);
		cursor: pointer;
		transition: background-color var(--transition-fast);
		border: 1px solid transparent;
	}

	.device-row:hover {
		background-color: var(--bg-hover);
	}

	.device-row.assigned {
		background-color: rgba(0, 212, 255, 0.06);
		border-color: rgba(0, 212, 255, 0.2);
	}

	.device-row input[type='checkbox'] {
		width: 18px;
		height: 18px;
		accent-color: var(--accent-primary);
		flex-shrink: 0;
		cursor: pointer;
	}

	.device-info {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.device-name {
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		color: var(--text-primary);
	}

	.device-id {
		font-size: var(--text-xs);
		color: var(--text-secondary);
		font-family: monospace;
	}

	/* Responsive */
	@media (max-width: 768px) {
		.page-header {
			flex-direction: column;
			align-items: flex-start;
			gap: var(--space-3);
		}

		.page-header-left {
			flex-direction: column;
			align-items: flex-start;
			gap: var(--space-2);
		}

		.role-distribution {
			flex-wrap: wrap;
		}
	}

	@media (max-width: 480px) {
		.users-page {
			padding: var(--space-4) 0;
		}

		.page-header {
			margin-bottom: var(--space-4);
		}

		.page-title {
			font-size: var(--text-2xl);
		}

		.summary-meta {
			display: none;
		}
	}
</style>

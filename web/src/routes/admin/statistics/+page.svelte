<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { currentUser } from '$lib/stores/auth';
	import { refreshHandler } from '$lib/stores/refresh';
	import { api } from '$lib/api/client';
	import { formatDate } from '$lib/utils/formatting';
	import Button from '$lib/components/Button.svelte';

	interface PlatformStats {
		totalUsers: number;
		totalDevices: number;
		totalPositions: number;
		totalEvents: number;
		notificationsSent: number;
		devicesByStatus: Record<string, number>;
		positionsToday: number;
		activeUsers: number;
	}

	interface UserStats {
		userId: number;
		devicesOwned: number;
		totalPositions: number;
		lastLogin: string | null;
		eventsTriggered: number;
		geofencesOwned: number;
	}

	interface User {
		id: number;
		email: string;
		name: string;
		administrator: boolean;
		readonly: boolean;
	}

	let loading = true;
	let error = '';
	let stats: PlatformStats | null = null;
	let users: User[] = [];
	let selectedUserId: number | null = null;
	let userStats: UserStats | null = null;
	let loadingUserStats = false;

	$: isAdmin = ($currentUser as Record<string, unknown> | null)?.administrator === true;

	onMount(() => {
		if (!isAdmin) {
			goto('/');
			return;
		}
		loadStats();
		$refreshHandler = loadStats;
	});

	onDestroy(() => { $refreshHandler = null; });

	async function loadStats() {
		loading = true;
		error = '';
		try {
			const [platformStats, userList] = await Promise.all([
				api.getPlatformStatistics(),
				api.getUsers()
			]);
			stats = platformStats as PlatformStats;
			users = userList as User[];
		} catch (err: any) {
			error = err.status === 403
				? 'Access denied. Admin privileges required.'
				: 'Failed to load statistics';
			console.error(err);
		} finally {
			loading = false;
		}
	}

	async function loadUserStats(userId: number) {
		selectedUserId = userId;
		loadingUserStats = true;
		userStats = null;
		try {
			userStats = await api.getUserStatistics(userId) as UserStats;
		} catch (err: any) {
			console.error('Failed to load user statistics:', err);
		} finally {
			loadingUserStats = false;
		}
	}

	function getStatusColor(status: string): string {
		switch (status) {
			case 'online': return 'var(--status-online)';
			case 'offline': return 'var(--status-offline)';
			case 'idle': return 'var(--status-idle)';
			case 'moving': return 'var(--status-moving)';
			default: return 'var(--text-secondary)';
		}
	}

	function getUserName(userId: number): string {
		const user = users.find(u => u.id === userId);
		return user?.name || user?.email || `User #${userId}`;
	}

	$: deviceStatusEntries = stats?.devicesByStatus
		? Object.entries(stats.devicesByStatus).sort((a, b) => b[1] - a[1])
		: [];

	$: totalDevicesByStatus = deviceStatusEntries.reduce((sum, [, count]) => sum + count, 0);
</script>

<svelte:head><title>Platform Statistics - Motus</title></svelte:head>

<div class="statistics-page">
	<div class="container">
		<div class="page-header">
			<div class="page-header-left">
				<h1 class="page-title">Platform Statistics</h1>
			</div>
			<Button variant="secondary" on:click={loadStats}>Refresh</Button>
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
				<p>Loading statistics...</p>
			</div>
		{:else if stats}
			<!-- Platform Overview Cards -->
			<div class="stats-grid">
				<div class="stat-card">
					<div class="stat-icon users-icon">
						<svg viewBox="0 0 24 24" width="28" height="28" fill="none" stroke="currentColor" stroke-width="1.5">
							<path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/>
							<circle cx="9" cy="7" r="4"/>
							<path d="M23 21v-2a4 4 0 0 0-3-3.87"/>
							<path d="M16 3.13a4 4 0 0 1 0 7.75"/>
						</svg>
					</div>
					<div class="stat-content">
						<div class="stat-value">{stats.totalUsers}</div>
						<div class="stat-label">Total Users</div>
					</div>
					<div class="stat-secondary">{stats.activeUsers} active (24h)</div>
				</div>

				<div class="stat-card">
					<div class="stat-icon devices-icon">
						<svg viewBox="0 0 24 24" width="28" height="28" fill="none" stroke="currentColor" stroke-width="1.5">
							<rect x="5" y="2" width="14" height="20" rx="2"/>
							<line x1="12" y1="18" x2="12" y2="18.01" stroke-width="2"/>
						</svg>
					</div>
					<div class="stat-content">
						<div class="stat-value">{stats.totalDevices}</div>
						<div class="stat-label">Total Devices</div>
					</div>
				</div>

				<div class="stat-card">
					<div class="stat-icon positions-icon">
						<svg viewBox="0 0 24 24" width="28" height="28" fill="none" stroke="currentColor" stroke-width="1.5">
							<path d="M12 2C8.13 2 5 5.13 5 9c0 5.25 7 13 7 13s7-7.75 7-13c0-3.87-3.13-7-7-7z"/>
							<circle cx="12" cy="9" r="2.5"/>
						</svg>
					</div>
					<div class="stat-content">
						<div class="stat-value">{stats.totalPositions.toLocaleString()}</div>
						<div class="stat-label">Total Positions</div>
					</div>
					<div class="stat-secondary">{stats.positionsToday.toLocaleString()} today</div>
				</div>

				<div class="stat-card">
					<div class="stat-icon events-icon">
						<svg viewBox="0 0 24 24" width="28" height="28" fill="none" stroke="currentColor" stroke-width="1.5">
							<path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z"/>
						</svg>
					</div>
					<div class="stat-content">
						<div class="stat-value">{stats.totalEvents.toLocaleString()}</div>
						<div class="stat-label">Total Events</div>
					</div>
				</div>

				<div class="stat-card">
					<div class="stat-icon notifs-icon">
						<svg viewBox="0 0 24 24" width="28" height="28" fill="none" stroke="currentColor" stroke-width="1.5">
							<path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/>
							<path d="M13.73 21a2 2 0 0 1-3.46 0"/>
						</svg>
					</div>
					<div class="stat-content">
						<div class="stat-value">{stats.notificationsSent.toLocaleString()}</div>
						<div class="stat-label">Notifications Sent</div>
					</div>
				</div>
			</div>

			<!-- Device Status Distribution -->
			{#if deviceStatusEntries.length > 0}
				<div class="section">
					<h2 class="section-title">Device Status Distribution</h2>
					<div class="status-distribution">
						<div class="status-bar">
							{#each deviceStatusEntries as [status, count]}
								<div
									class="status-segment"
									style="width: {totalDevicesByStatus > 0 ? (count / totalDevicesByStatus * 100) : 0}%; background-color: {getStatusColor(status)}"
									title="{status}: {count}"
								></div>
							{/each}
						</div>
						<div class="status-legend">
							{#each deviceStatusEntries as [status, count]}
								<div class="legend-item">
									<span class="legend-dot" style="background-color: {getStatusColor(status)}"></span>
									<span class="legend-label">{status}</span>
									<span class="legend-count">{count}</span>
								</div>
							{/each}
						</div>
					</div>
				</div>
			{/if}

			<!-- Per-User Statistics -->
			{#if users.length > 0}
				<div class="section">
					<h2 class="section-title">User Statistics</h2>
					<div class="user-stats-table-wrapper">
						<table class="user-stats-table">
							<thead>
								<tr>
									<th>User</th>
									<th>Role</th>
									<th class="text-right">Details</th>
								</tr>
							</thead>
							<tbody>
								{#each users as user (user.id)}
									<tr
										class:selected={selectedUserId === user.id}
									>
										<td class="user-cell">
											<div class="user-avatar" aria-hidden="true">
												{(user.name || user.email).charAt(0).toUpperCase()}
											</div>
											<div class="user-details">
												<span class="user-name">{user.name || '(unnamed)'}</span>
												<span class="user-email">{user.email}</span>
											</div>
										</td>
										<td>
											<span class="role-badge" class:role-admin={user.administrator} class:role-user={!user.administrator && !user.readonly} class:role-readonly={user.readonly}>
												{user.administrator ? 'Admin' : user.readonly ? 'Read Only' : 'User'}
											</span>
										</td>
										<td class="text-right">
											<Button
												size="sm"
												variant="secondary"
												loading={loadingUserStats && selectedUserId === user.id}
												on:click={() => loadUserStats(user.id)}
											>
												View Stats
											</Button>
										</td>
									</tr>
									{#if selectedUserId === user.id && userStats}
										<tr class="stats-detail-row">
											<td colspan="3">
												<div class="user-stats-detail">
													<div class="detail-item">
														<span class="detail-value">{userStats.devicesOwned}</span>
														<span class="detail-label">Devices</span>
													</div>
													<div class="detail-item">
														<span class="detail-value">{userStats.totalPositions.toLocaleString()}</span>
														<span class="detail-label">Positions</span>
													</div>
													<div class="detail-item">
														<span class="detail-value">{userStats.eventsTriggered.toLocaleString()}</span>
														<span class="detail-label">Events</span>
													</div>
													<div class="detail-item">
														<span class="detail-value">{userStats.geofencesOwned}</span>
														<span class="detail-label">Geofences</span>
													</div>
													<div class="detail-item">
														<span class="detail-value">{userStats.lastLogin ? formatDate(userStats.lastLogin) : 'Never'}</span>
														<span class="detail-label">Last Login</span>
													</div>
												</div>
											</td>
										</tr>
									{/if}
								{/each}
							</tbody>
						</table>
					</div>
				</div>
			{/if}
		{/if}
	</div>
</div>

<style>
	.statistics-page {
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

	/* Error */
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

	.loading-state p {
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

	/* Stats Grid */
	.stats-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
		gap: var(--space-4);
		margin-bottom: var(--space-8);
	}

	.stat-card {
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
		padding: var(--space-5);
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		transition: border-color var(--transition-fast);
	}

	.stat-card:hover {
		border-color: var(--border-hover);
	}

	.stat-icon {
		width: 44px;
		height: 44px;
		border-radius: var(--radius-md);
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.users-icon {
		background-color: rgba(0, 122, 255, 0.12);
		color: #007aff;
	}

	.devices-icon {
		background-color: rgba(0, 212, 255, 0.12);
		color: var(--accent-primary);
	}

	.positions-icon {
		background-color: rgba(0, 255, 136, 0.12);
		color: var(--success);
	}

	.events-icon {
		background-color: rgba(255, 170, 0, 0.12);
		color: var(--warning);
	}

	.notifs-icon {
		background-color: rgba(255, 59, 48, 0.12);
		color: var(--error);
	}

	.stat-content {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}

	.stat-value {
		font-size: var(--text-3xl);
		font-weight: var(--font-bold);
		color: var(--text-primary);
		line-height: 1;
	}

	.stat-label {
		font-size: var(--text-sm);
		color: var(--text-secondary);
	}

	.stat-secondary {
		font-size: var(--text-xs);
		color: var(--text-tertiary);
		padding-top: var(--space-1);
		border-top: 1px solid var(--border-color);
	}

	/* Sections */
	.section {
		margin-bottom: var(--space-8);
	}

	.section-title {
		font-size: var(--text-xl);
		font-weight: var(--font-semibold);
		color: var(--text-primary);
		margin: 0 0 var(--space-4) 0;
	}

	/* Device Status Distribution */
	.status-distribution {
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
		padding: var(--space-5);
	}

	.status-bar {
		display: flex;
		height: 24px;
		border-radius: var(--radius-md);
		overflow: hidden;
		gap: 2px;
		margin-bottom: var(--space-4);
	}

	.status-segment {
		min-width: 4px;
		transition: width var(--transition-fast);
		border-radius: 2px;
	}

	.status-legend {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-4);
	}

	.legend-item {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-sm);
	}

	.legend-dot {
		width: 10px;
		height: 10px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.legend-label {
		color: var(--text-secondary);
		text-transform: capitalize;
	}

	.legend-count {
		font-weight: var(--font-semibold);
		color: var(--text-primary);
	}

	/* User Stats Table */
	.user-stats-table-wrapper {
		overflow-x: auto;
		border: 1px solid var(--border-color);
		border-radius: var(--radius-lg);
		background-color: var(--bg-secondary);
	}

	.user-stats-table {
		width: 100%;
		border-collapse: collapse;
	}

	.user-stats-table th {
		text-align: left;
		padding: var(--space-3) var(--space-4);
		font-size: var(--text-xs);
		font-weight: var(--font-semibold);
		color: var(--text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.5px;
		border-bottom: 1px solid var(--border-color);
		background-color: var(--bg-tertiary);
	}

	.user-stats-table td {
		padding: var(--space-3) var(--space-4);
		border-bottom: 1px solid var(--border-color);
		font-size: var(--text-sm);
		color: var(--text-primary);
		vertical-align: middle;
	}

	.user-stats-table tbody tr:last-child td {
		border-bottom: none;
	}

	.user-stats-table tbody tr:hover {
		background-color: var(--bg-hover);
	}

	.user-stats-table tr.selected {
		background-color: rgba(0, 212, 255, 0.04);
	}

	.text-right {
		text-align: right;
	}

	/* User cell */
	.user-cell {
		display: flex;
		align-items: center;
		gap: var(--space-3);
	}

	.user-avatar {
		width: 32px;
		height: 32px;
		border-radius: 50%;
		background-color: var(--accent-primary);
		color: var(--text-inverse);
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: var(--text-xs);
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
	}

	.user-email {
		font-size: var(--text-xs);
		color: var(--text-secondary);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
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

	/* Stats detail row */
	.stats-detail-row td {
		padding: 0 !important;
		border-bottom: 1px solid var(--border-color);
	}

	.user-stats-detail {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-6);
		padding: var(--space-4) var(--space-6);
		background-color: var(--bg-tertiary);
	}

	.detail-item {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}

	.detail-value {
		font-size: var(--text-lg);
		font-weight: var(--font-bold);
		color: var(--text-primary);
	}

	.detail-label {
		font-size: var(--text-xs);
		color: var(--text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.5px;
	}

	/* Responsive */
	@media (max-width: 768px) {
		.page-header {
			flex-direction: column;
			align-items: flex-start;
			gap: var(--space-3);
		}

		.stats-grid {
			grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
		}

		.user-stats-detail {
			gap: var(--space-4);
		}
	}
</style>

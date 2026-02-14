<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { theme } from '$lib/stores/theme';
	import { currentUser, isAuthenticated } from '$lib/stores/auth';
	import { wsManager } from '$lib/stores/websocket';
	import { pwa } from '$lib/stores/pwa';
	import { api, APIError } from '$lib/api/client';
	import PullToRefresh from '$lib/components/PullToRefresh.svelte';
	import {
		initNativeTokenHandler,
		generateLoginToken,
		notifyNativeLogout,
		isNativeEnvironment,
		changeServerUrl,
	} from '$lib/utils/native-interface';
	import ThemeSwitcher from '$lib/components/ThemeSwitcher.svelte';
	import SudoBar from '$lib/components/SudoBar.svelte';
	import PwaInstallBanner from '$lib/components/PwaInstallBanner.svelte';
	import PwaUpdateNotification from '$lib/components/PwaUpdateNotification.svelte';
	import '$lib/styles/app.css';

	let loading = true;
	let menuOpen = false;
	let dropdownOpen = false;
	let sudoBarActive = false;
	let sudoBarHeight = 0;
	let isNative = false;

	$: isCurrentUserAdmin = $currentUser?.administrator === true;

	$: isPublicRoute = $page.url.pathname.startsWith('/share/') || $page.url.pathname === '/login';

	onMount(async () => {
		theme.initialize();
		pwa.initialize();

		// Check if running in Traccar Manager native app
		isNative = isNativeEnvironment();

		// Initialize the native token handler for Traccar Manager app.
		// This registers a global callback so the native app can send
		// stored login tokens for auto-authentication.
		initNativeTokenHandler();

		// Check for ?token= parameter (QR code / API key login)
		const urlToken = $page.url.searchParams.get('token');
		if (urlToken && !$isAuthenticated) {
			try {
				const response = await fetch(`/api/session?token=${encodeURIComponent(urlToken)}`, {
					credentials: 'include',
				});
				if (response.ok) {
					const userData = await response.json();
					currentUser.set(userData);
					isAuthenticated.set(true);
					wsManager.connect();
					// Remove token from URL for security
					window.history.replaceState({}, '', $page.url.pathname);
					loading = false;
					return;
				}
			} catch {
				// Token login failed; continue to normal flow
			}
		}

		// Skip auth for public routes (share pages, login)
		if (isPublicRoute) {
			loading = false;
			return;
		}

		if (!$isAuthenticated) {
			goto('/login');
			loading = false;
			return;
		}

		try {
			const user = await api.getCurrentUser();
			currentUser.set(user);
			isAuthenticated.set(true);
			wsManager.connect();

			// Generate and send login token to native app for persistent storage.
			// This enables auto-login when the Traccar Manager app is reopened.
			generateLoginToken();
		} catch (err) {
			// Only clear auth state on actual authentication failures (401),
			// not on aborted requests (e.g., user refreshing rapidly) or
			// transient network errors which would incorrectly log the user out.
			const isAuthError = err instanceof APIError && err.status === 401;
			if (isAuthError) {
				currentUser.set(null);
				isAuthenticated.set(false);
				if (!isPublicRoute) {
					goto('/login');
				}
			}
		} finally {
			loading = false;
		}
	});

	async function handleLogout() {
		try {
			await api.logout();
		} catch {
			// Logout may fail if session expired
		}

		// Notify the native Traccar Manager app to delete the stored
		// login token. Without this, the app would auto-login on next launch.
		notifyNativeLogout();

		wsManager.disconnect();
		currentUser.set(null);
		isAuthenticated.set(false);
		goto('/login');
	}

	function handleChangeServer() {
		// Silently switch to Traccar demo server - appears like fresh install
		changeServerUrl('https://demo.traccar.org');
	}

	function toggleMenu() {
		menuOpen = !menuOpen;
	}

	function toggleDropdown() {
		dropdownOpen = !dropdownOpen;
	}

	function handleClickOutside(e: MouseEvent) {
		const target = e.target as HTMLElement;
		if (dropdownOpen && !target.closest('.user-menu')) {
			dropdownOpen = false;
		}
	}

	function getUserDisplay(user: Record<string, unknown> | null): string {
		if (!user) return '';
		return (user.name as string) || (user.email as string) || '';
	}
</script>

<svelte:window on:click={handleClickOutside} />

{#if loading}
	<div class="loading-screen">
		<div class="spinner"></div>
	</div>
{:else if $page.url.pathname.startsWith('/share/')}
	<slot />
{:else if $isAuthenticated && $page.url.pathname !== '/login'}
	<SudoBar bind:sudoActive={sudoBarActive} bind:barHeight={sudoBarHeight} />
	<div class="app-layout" style:--sudo-bar-height="{sudoBarActive ? sudoBarHeight : 0}px">
		<nav class="navbar">
			<div class="nav-container">
				<div class="nav-left">
					<a href="/" class="logo">
						<svg class="logo-icon" viewBox="0 0 24 24" width="28" height="28" fill="none" stroke="currentColor" stroke-width="2">
							<path d="M12 2C8.13 2 5 5.13 5 9c0 5.25 7 13 7 13s7-7.75 7-13c0-3.87-3.13-7-7-7z"/>
							<circle cx="12" cy="9" r="2.5"/>
						</svg>
						<span class="logo-text">Motus</span>
					</a>
				</div>

				<div class="nav-center" class:open={menuOpen}>
					<a href="/" class="nav-link" class:active={$page.url.pathname === '/'}>
						Dashboard
					</a>
					<a href="/devices" class="nav-link" class:active={$page.url.pathname === '/devices'}>
						Devices
					</a>
					<a href="/map" class="nav-link" class:active={$page.url.pathname === '/map'}>
						Map
					</a>
					<a href="/heatmap" class="nav-link" class:active={$page.url.pathname === '/heatmap'}>
						Heatmap
					</a>
					<a href="/reports" class="nav-link" class:active={$page.url.pathname.startsWith('/reports')}>
						Reports
					</a>
					<a href="/geofences" class="nav-link" class:active={$page.url.pathname === '/geofences'}>
						Geofences
					</a>
					<a href="/calendars" class="nav-link" class:active={$page.url.pathname === '/calendars'}>
						Calendars
					</a>
					<a href="/notifications" class="nav-link" class:active={$page.url.pathname.startsWith('/notifications')}>
						Notifications
					</a>
					{#if isCurrentUserAdmin}
						<a href="/admin/users" class="nav-link" class:active={$page.url.pathname.startsWith('/admin')}>
							Admin
						</a>
					{/if}
				</div>

				<div class="nav-right">
					<ThemeSwitcher />
					<div class="user-menu">
						<button class="user-button" on:click={toggleDropdown}>
							{getUserDisplay($currentUser)}
						</button>
						{#if dropdownOpen}
							<div class="user-dropdown">
								<a href="/settings" class="dropdown-item" on:click={() => dropdownOpen = false}>
									Settings
								</a>
								<div class="dropdown-divider"></div>
								<button on:click={handleLogout} class="dropdown-item dropdown-danger">
									Logout
								</button>
								{#if isNative}
									<button on:click={handleChangeServer} class="dropdown-item">
										Change Server
									</button>
								{/if}
							</div>
						{/if}
					</div>

					<button class="menu-toggle" on:click={toggleMenu} aria-label="Toggle menu">
						{menuOpen ? '\u2715' : '\u2630'}
					</button>
				</div>
			</div>
		</nav>

		<main class="main-content">
			<PullToRefresh>
				<slot />
			</PullToRefresh>
		</main>
	</div>
{:else}
	<slot />
{/if}

<PwaInstallBanner />
<PwaUpdateNotification />

<style>
	.loading-screen {
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 100vh;
		background-color: var(--bg-primary);
	}

	.spinner {
		width: 48px;
		height: 48px;
		border: 4px solid var(--border-color);
		border-top-color: var(--accent-primary);
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}

	@keyframes spin {
		to { transform: rotate(360deg); }
	}

	.app-layout {
		min-height: 100vh;
		display: flex;
		flex-direction: column;
		padding-top: var(--sudo-bar-height, 0px);
	}

	.navbar {
		background-color: var(--bg-primary);
		border-bottom: 1px solid var(--border-color);
		position: sticky;
		top: var(--sudo-bar-height, 0px);
		z-index: 1000;
	}

	.nav-container {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: var(--space-4);
		max-width: 1400px;
		margin: 0 auto;
	}

	.nav-left {
		display: flex;
		align-items: center;
	}

	.logo {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		text-decoration: none;
		color: var(--text-primary);
		font-size: var(--text-xl);
		font-weight: var(--font-bold);
	}

	.logo-icon {
		color: var(--accent-primary);
	}

	.nav-center {
		display: flex;
		gap: var(--space-2);
	}

	.nav-link {
		padding: var(--space-3) var(--space-4);
		text-decoration: none;
		color: var(--text-secondary);
		font-weight: var(--font-medium);
		border-radius: var(--radius-md);
		transition: all var(--transition-fast);
		position: relative;
	}

	.nav-link:hover {
		color: var(--text-primary);
		background-color: var(--bg-hover);
	}

	.nav-link.active {
		color: var(--accent-primary);
	}

	.nav-link.active::after {
		content: '';
		position: absolute;
		bottom: 0;
		left: var(--space-4);
		right: var(--space-4);
		height: 2px;
		background-color: var(--accent-primary);
	}

	.nav-right {
		display: flex;
		align-items: center;
		gap: var(--space-4);
	}

	.user-menu {
		position: relative;
	}

	.user-button {
		padding: var(--space-2) var(--space-4);
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		cursor: pointer;
		font-size: var(--text-sm);
		transition: all var(--transition-fast);
	}

	.user-button:hover {
		background-color: var(--bg-hover);
	}

	.user-dropdown {
		position: absolute;
		top: calc(100% + var(--space-2));
		right: 0;
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-md);
		box-shadow: var(--shadow-lg);
		min-width: 150px;
		z-index: 10000; /* Above all Leaflet elements and modals */
	}

	.dropdown-item {
		display: block;
		width: 100%;
		padding: var(--space-3) var(--space-4);
		background: none;
		border: none;
		text-align: left;
		color: var(--text-primary);
		cursor: pointer;
		font-size: var(--text-sm);
		transition: all var(--transition-fast);
	}

	.dropdown-item:hover {
		background-color: var(--bg-hover);
	}

	.dropdown-item.dropdown-danger:hover {
		color: var(--error);
	}

	.dropdown-divider {
		height: 1px;
		background-color: var(--border-color);
		margin: var(--space-1) 0;
	}

	a.dropdown-item {
		text-decoration: none;
	}

	.menu-toggle {
		display: none;
		background: none;
		border: none;
		font-size: var(--text-2xl);
		color: var(--text-primary);
		cursor: pointer;
		padding: var(--space-2);
	}

	.main-content {
		flex: 1;
		background-color: var(--bg-primary);
	}

	@media (max-width: 768px) {
		.nav-center {
			display: none;
			position: absolute;
			top: 100%;
			left: 0;
			right: 0;
			background-color: var(--bg-primary);
			border-bottom: 1px solid var(--border-color);
			flex-direction: column;
			gap: 0;
		}

		.nav-center.open {
			display: flex;
		}

		.nav-link {
			border-radius: 0;
		}

		.nav-link.active::after {
			display: none;
		}

		.menu-toggle {
			display: block;
		}
	}
</style>

<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { currentUser, isAuthenticated } from '$lib/stores/auth';
	import { wsManager } from '$lib/stores/websocket';
	import {
		isNativeEnvironment,
		nativePostMessage,
		handleLoginTokenListeners,
		generateLoginToken,
		changeServerUrl,
	} from '$lib/utils/native-interface';
	import Input from '$lib/components/Input.svelte';
	import Button from '$lib/components/Button.svelte';
	import QrScannerDialog from '$lib/components/QrScannerDialog.svelte';

	let email = '';
	let password = '';
	let rememberMe = false;
	let loading = false;
	let error = '';
	let showScannerDialog = false;
	let oidcEnabled = false;

	// Check for errors from OIDC callback redirects.
	function readOIDCError(): string {
		if (typeof window === 'undefined') return '';
		const params = new URLSearchParams(window.location.search);
		const e = params.get('error');
		if (e === 'signup_disabled') return 'SSO login is not available for new accounts. Please contact your administrator.';
		if (e === 'provider_error') return 'SSO login failed. Please try again or use your email and password.';
		return '';
	}

	$: isNative = typeof window !== 'undefined' && isNativeEnvironment();
	// Touch/mobile browsers support getUserMedia camera; WKWebView (isNative) does not.
	$: isMobileBrowser =
		typeof window !== 'undefined' &&
		!isNativeEnvironment() &&
		window.matchMedia('(pointer: coarse)').matches;

	/**
	 * Handle token-based auto-login from the Traccar Manager native app.
	 * When the login page mounts, it sends an "authentication" message to
	 * the native app. If the app has a stored token, it calls back with the
	 * token via window.handleLoginToken(), which triggers this listener.
	 */
	async function handleTokenLogin(token: string) {
		if (!token) return;

		loading = true;
		error = '';
		try {
			// Use the token query param to create a session. The server
			// validates the token, creates a session cookie, and returns
			// the user object -- matching the pytraccar/Traccar flow.
			const response = await fetch(`/api/session?token=${encodeURIComponent(token)}`, {
				credentials: 'include',
			});
			if (!response.ok) {
				// Token is invalid or expired; let the user log in manually.
				loading = false;
				return;
			}
			const userData = await response.json();
			currentUser.set(userData);
			isAuthenticated.set(true);
			wsManager.connect();
			goto('/');
		} catch {
			// Token login failed; let the user log in manually.
			loading = false;
		}
	}

	function handleScanResult(url: string) {
		showScannerDialog = false;
		try {
			const parsed = new URL(url);
			const token = parsed.searchParams.get('token');
			if (token) {
				handleTokenLogin(token);
			} else {
				error = 'No login token found in QR code';
			}
		} catch {
			error = 'Invalid QR code — could not read a URL';
		}
	}

	onMount(async () => {
		// Check for error messages passed back from OIDC callback redirects.
		const oidcError = readOIDCError();
		if (oidcError) {
			error = oidcError;
			// Clean the error query param from the URL without reloading.
			const url = new URL(window.location.href);
			url.searchParams.delete('error');
			history.replaceState({}, '', url.toString());
		}

		// Fetch OIDC availability for this server.
		try {
			const res = await fetch('/api/auth/oidc/config');
			if (res.ok) {
				const data = await res.json();
				oidcEnabled = data.enabled === true;
			}
		} catch {
			// OIDC config unavailable; keep oidcEnabled = false.
		}

		// Request the stored login token from the native app.
		// If the app has one, handleTokenLogin will be called.
		if (isNativeEnvironment()) {
			handleLoginTokenListeners.add(handleTokenLogin);
			nativePostMessage('authentication');
		}
	});

	onDestroy(() => {
		handleLoginTokenListeners.delete(handleTokenLogin);
	});

	async function handleLogin() {
		error = '';
		loading = true;

		try {
			const user = await api.login(email, password, rememberMe);
			currentUser.set(user);
			isAuthenticated.set(true);
			wsManager.connect();

			// Generate and send a login token to the native app so it can
			// auto-login on subsequent launches without re-entering credentials.
			generateLoginToken();

			goto('/');
		} catch {
			error = 'Invalid email or password';
		} finally {
			loading = false;
		}
	}

	function handleChangeServer() {
		changeServerUrl('https://demo.traccar.org');
	}
</script>

<svelte:head>
	<title>Login - Motus</title>
</svelte:head>

<div class="login-page">
	<div class="login-card">
		{#if isMobileBrowser}
			<button class="qr-button" on:click={() => (showScannerDialog = true)} aria-label="Scan QR code">
				<!-- QR grid icon -->
				<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2">
					<rect x="3" y="3" width="7" height="7"/>
					<rect x="14" y="3" width="7" height="7"/>
					<rect x="3" y="14" width="7" height="7"/>
					<rect x="14" y="14" width="3" height="3"/>
					<rect x="18" y="18" width="3" height="3"/>
				</svg>
			</button>
		{/if}

		<div class="logo">
			<svg viewBox="0 0 24 24" width="48" height="48" fill="none" stroke="var(--accent-primary)" stroke-width="2">
				<path d="M12 2C8.13 2 5 5.13 5 9c0 5.25 7 13 7 13s7-7.75 7-13c0-3.87-3.13-7-7-7z"/>
				<circle cx="12" cy="9" r="2.5"/>
			</svg>
			<h1>Motus</h1>
		</div>

		<form on:submit|preventDefault={handleLogin} class="login-form">
			<Input
				type="email"
				name="email"
				label="Email"
				placeholder="Email address"
				required
				bind:value={email}
			/>

			<Input
				type="password"
				name="password"
				label="Password"
				placeholder="Enter your password"
				required
				bind:value={password}
			/>

			<label class="remember-me">
				<input type="checkbox" bind:checked={rememberMe} />
				<span>Remember me</span>
			</label>

			{#if error}
				<div class="error-message" role="alert">{error}</div>
			{/if}

			<Button type="submit" variant="primary" size="lg" {loading}>
				{loading ? 'Logging in...' : 'Login'}
			</Button>
		</form>

		{#if oidcEnabled}
			<div class="sso-divider">
				<span>or</span>
			</div>
			<a href="/api/auth/oidc/login" class="sso-button">
				<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
					<circle cx="12" cy="12" r="10"/>
					<path d="M12 8v4l3 3"/>
				</svg>
				Continue with SSO
			</a>
		{/if}

		{#if isNative}
			<div class="change-server">
				<button class="change-server-link" on:click={handleChangeServer}>
					Different Server?
				</button>
			</div>
		{/if}
	</div>
</div>

{#if isMobileBrowser}
	<QrScannerDialog
		open={showScannerDialog}
		onClose={() => (showScannerDialog = false)}
		onScan={handleScanResult}
	/>
{/if}

<style>
	.login-page {
		min-height: 100vh;
		display: flex;
		align-items: center;
		justify-content: center;
		background-color: var(--bg-primary);
		padding: var(--space-4);
	}

	.login-card {
		width: 100%;
		max-width: 400px;
		background-color: var(--bg-secondary);
		border-radius: var(--radius-xl);
		padding: var(--space-8);
		box-shadow: var(--shadow-xl);
		position: relative;
	}

	.qr-button {
		position: absolute;
		top: var(--space-4);
		right: var(--space-4);
		background: none;
		border: none;
		padding: var(--space-2);
		cursor: pointer;
		color: var(--text-secondary);
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: var(--radius-md);
		transition: all 0.2s;
	}

	.qr-button:hover {
		background-color: var(--bg-hover);
		color: var(--accent-primary);
	}

	.qr-button:focus-visible {
		outline: 2px solid var(--accent-primary);
		outline-offset: 2px;
	}

	.logo {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: var(--space-3);
		margin-bottom: var(--space-2);
	}

	.logo h1 {
		font-size: var(--text-3xl);
		color: var(--text-primary);
		margin: 0;
	}

	.login-form {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
	}

	.remember-me {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		cursor: pointer;
		font-size: var(--text-sm);
		color: var(--text-secondary);
		user-select: none;
	}

	.remember-me input[type="checkbox"] {
		width: 16px;
		height: 16px;
		accent-color: var(--accent-primary);
		cursor: pointer;
	}

	.remember-me span {
		line-height: 1;
	}

	.error-message {
		padding: var(--space-3);
		background-color: rgba(255, 68, 68, 0.1);
		border: 1px solid var(--error);
		border-radius: var(--radius-md);
		color: var(--error);
		font-size: var(--text-sm);
	}

	.change-server {
		margin-top: var(--space-4);
		text-align: center;
	}

	.change-server-link {
		background: none;
		border: none;
		color: var(--text-secondary);
		font-size: var(--text-sm);
		cursor: pointer;
		padding: var(--space-1) var(--space-2);
		border-radius: var(--radius-md);
		transition: all var(--transition-fast);
		text-decoration: underline;
		text-underline-offset: 2px;
	}

	.change-server-link:hover {
		color: var(--accent-primary);
	}

	.change-server-link:focus-visible {
		outline: 2px solid var(--accent-primary);
		outline-offset: 2px;
	}

	.sso-divider {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		margin-top: var(--space-4);
		color: var(--text-muted, var(--text-secondary));
		font-size: var(--text-sm);
	}

	.sso-divider::before,
	.sso-divider::after {
		content: '';
		flex: 1;
		height: 1px;
		background-color: var(--border-color, var(--bg-hover));
	}

	.sso-button {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: var(--space-2);
		width: 100%;
		margin-top: var(--space-3);
		padding: var(--space-3) var(--space-4);
		background-color: var(--bg-tertiary, var(--bg-hover));
		border: 1px solid var(--border-color, transparent);
		border-radius: var(--radius-md);
		color: var(--text-primary);
		font-size: var(--text-sm);
		font-weight: 500;
		text-decoration: none;
		cursor: pointer;
		transition: all var(--transition-fast);
	}

	.sso-button:hover {
		background-color: var(--bg-hover);
		color: var(--accent-primary);
		border-color: var(--accent-primary);
	}

	.sso-button:focus-visible {
		outline: 2px solid var(--accent-primary);
		outline-offset: 2px;
	}
</style>

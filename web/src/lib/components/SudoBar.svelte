<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { slide } from 'svelte/transition';
	import { api } from '$lib/api/client';
	import { currentUser } from '$lib/stores/auth';

	export let sudoActive = false;
	/** Pixel height of the sudo bar, exposed for layout offset calculations */
	export let barHeight = 0;
	let sudoOriginalUser: string | null = null;
	let sudoExpiresAt: string | null = null;
	let endingSudo = false;
	let error = '';
	let timeRemaining = '';
	let countdownInterval: ReturnType<typeof setInterval> | null = null;

	function getUserDisplay(user: Record<string, unknown> | string | null): string {
		if (!user) return '';
		if (typeof user === 'string') return user;
		return (user.name as string) || (user.email as string) || '';
	}

	function updateCountdown() {
		if (!sudoExpiresAt) {
			timeRemaining = '';
			return;
		}
		const now = Date.now();
		const expires = new Date(sudoExpiresAt).getTime();
		const diff = expires - now;

		if (diff <= 0) {
			timeRemaining = 'expired';
			// Session expired, reload to let the server handle it
			window.location.reload();
			return;
		}

		const minutes = Math.floor(diff / 60000);
		const seconds = Math.floor((diff % 60000) / 1000);

		if (minutes > 0) {
			timeRemaining = `${minutes}m ${seconds}s remaining`;
		} else {
			timeRemaining = `${seconds}s remaining`;
		}
	}

	onMount(async () => {
		try {
			const status = await api.getSudoStatus();
			if (status?.isSudo) {
				sudoActive = true;
				sudoOriginalUser = status.originalUser || null;
				sudoExpiresAt = status.expiresAt || null;

				if (sudoExpiresAt) {
					updateCountdown();
					countdownInterval = setInterval(updateCountdown, 1000);
				}
			}
		} catch {
			// Sudo status check is non-critical; silently ignore
		}
	});

	onDestroy(() => {
		if (countdownInterval) {
			clearInterval(countdownInterval);
		}
	});

	async function endSudo() {
		endingSudo = true;
		error = '';
		try {
			await api.endSudo();
			sudoActive = false;
			sudoOriginalUser = null;
			sudoExpiresAt = null;
			if (countdownInterval) {
				clearInterval(countdownInterval);
				countdownInterval = null;
			}
			window.location.href = '/admin/users';
		} catch (err) {
			error = 'Failed to end sudo session. Reloading...';
			setTimeout(() => {
				window.location.reload();
			}, 1500);
		} finally {
			endingSudo = false;
		}
	}

</script>

{#if sudoActive}
	<div class="sudo-bar" role="alert" aria-live="assertive" transition:slide={{ duration: 300 }} bind:clientHeight={barHeight}>
		<div class="sudo-bar-inner">
			<div class="sudo-bar-left">
				<svg class="sudo-icon" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
					<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>
				</svg>
				<div class="sudo-text">
					<span class="sudo-label">SUDO MODE</span>
					<span class="sudo-details">
						Viewing as <strong>{getUserDisplay($currentUser)}</strong>
						{#if sudoOriginalUser}
							<span class="sudo-separator">|</span>
							Admin: {getUserDisplay(sudoOriginalUser)}
						{/if}
						{#if timeRemaining}
							<span class="sudo-separator">|</span>
							<span class="sudo-timer">{timeRemaining}</span>
						{/if}
					</span>
				</div>
			</div>
			<div class="sudo-bar-right">
				{#if error}
					<span class="sudo-error">{error}</span>
				{/if}
				<button
					class="sudo-exit-btn"
					on:click={endSudo}
					disabled={endingSudo}
					aria-label="Exit sudo mode and return to admin account"
				>
					{#if endingSudo}
						<svg class="spin-icon" viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
							<path d="M21 12a9 9 0 1 1-6.219-8.56"/>
						</svg>
						Restoring...
					{:else}
						<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
							<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/>
							<polyline points="16 17 21 12 16 7"/>
							<line x1="21" y1="12" x2="9" y2="12"/>
						</svg>
						Exit Sudo
					{/if}
				</button>
			</div>
		</div>
	</div>
{/if}

<style>
	.sudo-bar {
		position: fixed;
		top: 0;
		left: 0;
		right: 0;
		z-index: 10001;
		background: linear-gradient(135deg, #dc2626, #b91c1c);
		color: #ffffff;
		box-shadow: 0 2px 8px rgba(220, 38, 38, 0.4);
		font-size: var(--text-sm);
		/* Override the global transition to prevent theme flicker */
		transition: none !important;
	}

	.sudo-bar-inner {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: var(--space-2) var(--space-4);
		max-width: 1400px;
		margin: 0 auto;
		gap: var(--space-4);
	}

	.sudo-bar-left {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		min-width: 0;
	}

	.sudo-icon {
		flex-shrink: 0;
		opacity: 0.9;
		color: #ffffff;
	}

	.sudo-text {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		flex-wrap: wrap;
		min-width: 0;
	}

	.sudo-label {
		font-weight: var(--font-bold);
		font-size: var(--text-xs);
		letter-spacing: 1px;
		background-color: rgba(255, 255, 255, 0.2);
		padding: 2px 8px;
		border-radius: var(--radius-sm);
		flex-shrink: 0;
	}

	.sudo-details {
		color: rgba(255, 255, 255, 0.9);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.sudo-details strong {
		color: #ffffff;
		font-weight: var(--font-semibold);
	}

	.sudo-separator {
		margin: 0 var(--space-1);
		opacity: 0.5;
	}

	.sudo-timer {
		font-variant-numeric: tabular-nums;
		opacity: 0.85;
	}

	.sudo-bar-right {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		flex-shrink: 0;
	}

	.sudo-error {
		font-size: var(--text-xs);
		color: rgba(255, 255, 255, 0.85);
		background-color: rgba(0, 0, 0, 0.2);
		padding: 2px 8px;
		border-radius: var(--radius-sm);
	}

	.sudo-exit-btn {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		padding: var(--space-1) var(--space-3);
		background-color: rgba(255, 255, 255, 0.2);
		color: #ffffff;
		border: 1px solid rgba(255, 255, 255, 0.3);
		border-radius: var(--radius-md);
		font-size: var(--text-sm);
		font-weight: var(--font-semibold);
		cursor: pointer;
		/* Override global transition */
		transition: background-color 150ms ease, border-color 150ms ease !important;
		white-space: nowrap;
	}

	.sudo-exit-btn:hover:not(:disabled) {
		background-color: rgba(255, 255, 255, 0.35);
		border-color: rgba(255, 255, 255, 0.5);
	}

	.sudo-exit-btn:focus-visible {
		outline: 2px solid #ffffff;
		outline-offset: 2px;
	}

	.sudo-exit-btn:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.spin-icon {
		animation: sudo-spin 1s linear infinite;
	}

	@keyframes sudo-spin {
		to {
			transform: rotate(360deg);
		}
	}

	/* Mobile: stack layout vertically */
	@media (max-width: 640px) {
		.sudo-bar-inner {
			flex-direction: column;
			align-items: stretch;
			gap: var(--space-2);
			padding: var(--space-2) var(--space-3);
		}

		.sudo-bar-left {
			gap: var(--space-2);
		}

		.sudo-text {
			flex-direction: column;
			align-items: flex-start;
			gap: var(--space-1);
		}

		.sudo-separator {
			display: none;
		}

		.sudo-details {
			white-space: normal;
		}

		.sudo-bar-right {
			justify-content: flex-end;
		}
	}
</style>

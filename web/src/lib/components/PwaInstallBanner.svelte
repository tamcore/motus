<script lang="ts">
	import { fade, fly } from 'svelte/transition';
	import { pwa, showInstallBanner } from '$lib/stores/pwa';

	async function handleInstall() {
		await pwa.promptInstall();
	}

	function handleDismiss() {
		pwa.dismissInstall();
	}
</script>

{#if $showInstallBanner}
	<div
		class="install-banner"
		role="alert"
		aria-label="Install Motus as an app"
		transition:fly={{ y: 100, duration: 300 }}
	>
		<div class="install-content">
			<img src="/icon-192.png" alt="" class="install-icon" width="40" height="40" />
			<div class="install-text">
				<strong>Install Motus</strong>
				<span class="install-description">Add to your home screen for quick access</span>
			</div>
		</div>
		<div class="install-actions">
			<button class="install-btn" on:click={handleInstall}>
				Install
			</button>
			<button
				class="dismiss-btn"
				on:click={handleDismiss}
				aria-label="Dismiss install prompt"
			>
				<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
					<line x1="18" y1="6" x2="6" y2="18"/>
					<line x1="6" y1="6" x2="18" y2="18"/>
				</svg>
			</button>
		</div>
	</div>
{/if}

<style>
	.install-banner {
		position: fixed;
		bottom: var(--space-4);
		left: var(--space-4);
		right: var(--space-4);
		max-width: 480px;
		margin: 0 auto;
		background-color: var(--bg-secondary);
		border: 1px solid var(--border-color);
		border-radius: var(--radius-xl);
		box-shadow: var(--shadow-xl);
		padding: var(--space-4);
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-4);
		z-index: 10001;
	}

	.install-content {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		min-width: 0;
	}

	.install-icon {
		flex-shrink: 0;
		border-radius: var(--radius-md);
	}

	.install-text {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.install-text strong {
		color: var(--text-primary);
		font-size: var(--text-base);
	}

	.install-description {
		color: var(--text-secondary);
		font-size: var(--text-sm);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.install-actions {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		flex-shrink: 0;
	}

	.install-btn {
		padding: var(--space-2) var(--space-4);
		background-color: var(--accent-primary);
		color: var(--text-inverse);
		border: none;
		border-radius: var(--radius-md);
		font-size: var(--text-sm);
		font-weight: var(--font-medium);
		cursor: pointer;
		transition: background-color var(--transition-fast);
		white-space: nowrap;
	}

	.install-btn:hover {
		background-color: var(--accent-hover);
	}

	.install-btn:active {
		background-color: var(--accent-active);
	}

	.dismiss-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 32px;
		height: 32px;
		background: none;
		border: none;
		color: var(--text-tertiary);
		cursor: pointer;
		border-radius: var(--radius-md);
		transition: all var(--transition-fast);
	}

	.dismiss-btn:hover {
		color: var(--text-primary);
		background-color: var(--bg-hover);
	}

	@media (max-width: 480px) {
		.install-banner {
			bottom: var(--space-2);
			left: var(--space-2);
			right: var(--space-2);
		}

		.install-description {
			display: none;
		}
	}
</style>
